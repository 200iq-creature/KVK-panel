package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ================= Telegram Bot API =================

var tgHTTP = &http.Client{Timeout: 40 * time.Second}

func tgCall(token, method string, params any, out any) error {
	b, _ := json.Marshal(params)
	resp, err := tgHTTP.Post("https://api.telegram.org/bot"+token+"/"+method, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	var res struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(rb, &res); err != nil {
		return fmt.Errorf("telegram: %s", resp.Status)
	}
	if !res.OK {
		return fmt.Errorf("%s", res.Description)
	}
	if out != nil {
		return json.Unmarshal(res.Result, out)
	}
	return nil
}

type tgUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

type tgMessage struct {
	MessageID int64 `json:"message_id"`
	From      *tgUser
	Chat      struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
	Text              string `json:"text"`
	Caption           string `json:"caption"`
	Photo             []any  `json:"photo"`
	Document          any    `json:"document"`
	SuccessfulPayment *struct {
		Currency       string `json:"currency"`
		TotalAmount    int    `json:"total_amount"`
		InvoicePayload string `json:"invoice_payload"`
		ChargeID       string `json:"telegram_payment_charge_id"`
	} `json:"successful_payment"`
}

type tgUpdate struct {
	UpdateID int64      `json:"update_id"`
	Message  *tgMessage `json:"message"`
	Callback *struct {
		ID      string     `json:"id"`
		From    tgUser     `json:"from"`
		Message *tgMessage `json:"message"`
		Data    string     `json:"data"`
	} `json:"callback_query"`
	PreCheckout *struct {
		ID             string `json:"id"`
		From           tgUser `json:"from"`
		InvoicePayload string `json:"invoice_payload"`
	} `json:"pre_checkout_query"`
}

type btn = map[string]string

func kb(rows ...[]btn) map[string]any {
	return map[string]any{"inline_keyboard": rows}
}

// ================= bot runtime =================

type shopBot struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	running  bool
	lastErr  string
	username string
}

var shop = &shopBot{}

func (a *App) shopToken() string {
	var t string
	a.Store.Read(func(d *Data) {
		if d.Settings.Shop.Enabled {
			t = d.Settings.Shop.BotToken
		}
	})
	return t
}

// StartShop (re)starts the shop bot according to settings.
func (a *App) StartShop() {
	shop.mu.Lock()
	if shop.cancel != nil {
		shop.cancel()
		shop.cancel = nil
	}
	shop.running = false
	token := a.shopToken()
	if token == "" {
		shop.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	shop.cancel = cancel
	shop.mu.Unlock()
	go a.shopLoop(ctx, token)
}

func (a *App) shopLoop(ctx context.Context, token string) {
	var me tgUser
	if err := tgCall(token, "getMe", map[string]any{}, &me); err != nil {
		shop.mu.Lock()
		shop.lastErr = "Токен бота не принят: " + err.Error()
		shop.mu.Unlock()
		return
	}
	_ = tgCall(token, "deleteWebhook", map[string]any{}, nil)
	_ = tgCall(token, "setMyCommands", map[string]any{"commands": []map[string]string{
		{"command": "start", "description": "Главное меню"},
		{"command": "buy", "description": "Купить или продлить"},
		{"command": "my", "description": "Моя подписка"},
		{"command": "promo", "description": "Ввести промокод"},
	}}, nil)
	shop.mu.Lock()
	shop.running, shop.lastErr, shop.username = true, "", me.Username
	shop.mu.Unlock()
	a.Store.Update(func(d *Data) error { d.Settings.Shop.BotUsername = me.Username; return nil })
	var offset int64
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			go a.checkCryptoOrders()
		default:
		}
		var ups []tgUpdate
		err := tgCall(token, "getUpdates", map[string]any{"offset": offset, "timeout": 25,
			"allowed_updates": []string{"message", "callback_query", "pre_checkout_query"}}, &ups)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			shop.mu.Lock()
			shop.lastErr = err.Error()
			shop.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}
		shop.mu.Lock()
		shop.lastErr = ""
		shop.mu.Unlock()
		for _, u := range ups {
			offset = u.UpdateID + 1
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("shop bot panic: %v", r)
					}
				}()
				a.shopHandle(token, u)
			}()
		}
	}
}

// ================= handlers =================

func (a *App) shopUser(d *Data, u tgUser) *ShopUser {
	for _, s := range d.ShopUsers {
		if s.TgID == u.ID {
			if u.Username != "" {
				s.Username = u.Username
			}
			if u.FirstName != "" {
				s.FirstName = u.FirstName
			}
			s.LastSeen = nowMs()
			s.Blocked = false
			return s
		}
	}
	s := &ShopUser{TgID: u.ID, Username: u.Username, FirstName: u.FirstName, JoinedAt: nowMs(), LastSeen: nowMs()}
	d.ShopUsers = append(d.ShopUsers, s)
	return s
}

func (a *App) shopSend(token string, chat int64, edit int64, text string, markup any) {
	p := map[string]any{"chat_id": chat, "text": text, "parse_mode": "HTML", "disable_web_page_preview": true}
	if markup != nil {
		p["reply_markup"] = markup
	}
	if edit > 0 {
		p["message_id"] = edit
		if err := tgCall(token, "editMessageText", p, nil); err == nil || strings.Contains(err.Error(), "not modified") {
			return
		}
		delete(p, "message_id")
	}
	if err := tgCall(token, "sendMessage", p, nil); err != nil && strings.Contains(err.Error(), "blocked") {
		a.Store.Update(func(d *Data) error {
			for _, s := range d.ShopUsers {
				if s.TgID == chat {
					s.Blocked = true
				}
			}
			return nil
		})
	}
}

func esch(s string) string { return html.EscapeString(s) }

func priceWithPromo(price, percent int) int {
	if percent <= 0 {
		return price
	}
	p := price * (100 - percent) / 100
	if p < 1 {
		p = 1
	}
	return p
}

func (a *App) promoValid(d *Data, code string) *Promo {
	for _, p := range d.Promos {
		if strings.EqualFold(p.Code, code) && p.Enabled && (p.MaxUses == 0 || p.Used < p.MaxUses) && (p.ExpiresAt == 0 || p.ExpiresAt > nowMs()) {
			return p
		}
	}
	return nil
}

func (a *App) shopHandle(token string, u tgUpdate) {
	if u.PreCheckout != nil {
		pc := u.PreCheckout
		okk := false
		a.Store.Read(func(d *Data) {
			for _, o := range d.Orders {
				if o.ID == pc.InvoicePayload && o.Status == "pending" {
					okk = true
				}
			}
		})
		p := map[string]any{"pre_checkout_query_id": pc.ID, "ok": okk}
		if !okk {
			p["error_message"] = "Заказ устарел — оформите новый через /buy"
		}
		_ = tgCall(token, "answerPreCheckoutQuery", p, nil)
		return
	}
	if u.Callback != nil {
		cb := u.Callback
		_ = tgCall(token, "answerCallbackQuery", map[string]any{"callback_query_id": cb.ID}, nil)
		var chat, mid int64
		if cb.Message != nil {
			chat, mid = cb.Message.Chat.ID, cb.Message.MessageID
		} else {
			chat = cb.From.ID
		}
		a.shopAction(token, cb.From, chat, mid, cb.Data)
		return
	}
	m := u.Message
	if m == nil || m.From == nil || m.Chat.Type != "private" {
		return
	}
	if sp := m.SuccessfulPayment; sp != nil {
		method := "card"
		if sp.Currency == "XTR" {
			method = "stars"
		}
		a.Store.Update(func(d *Data) error {
			for _, o := range d.Orders {
				if o.ID == sp.InvoicePayload {
					o.Method = method
					o.ExtID = sp.ChargeID
				}
			}
			return nil
		})
		a.fulfillOrder(sp.InvoicePayload)
		return
	}
	var state string
	a.Store.Update(func(d *Data) error {
		su := a.shopUser(d, *m.From)
		state = su.State
		return nil
	})
	text := strings.TrimSpace(m.Text)
	switch {
	case strings.HasPrefix(text, "/start"), text == "/menu":
		a.setShopState(m.From.ID, "")
		a.shopMenu(token, *m.From, m.Chat.ID, 0)
		return
	case text == "/buy":
		a.shopAction(token, *m.From, m.Chat.ID, 0, "buy")
		return
	case text == "/my":
		a.shopAction(token, *m.From, m.Chat.ID, 0, "my")
		return
	case strings.HasPrefix(text, "/promo"):
		code := strings.TrimSpace(strings.TrimPrefix(text, "/promo"))
		if code == "" {
			a.shopAction(token, *m.From, m.Chat.ID, 0, "promo")
		} else {
			a.applyPromoCode(token, *m.From, m.Chat.ID, code)
		}
		return
	}
	if state == "promo" && text != "" {
		a.setShopState(m.From.ID, "")
		a.applyPromoCode(token, *m.From, m.Chat.ID, text)
		return
	}
	if strings.HasPrefix(state, "receipt:") && (len(m.Photo) > 0 || m.Document != nil || text != "") {
		oid := strings.TrimPrefix(state, "receipt:")
		a.setShopState(m.From.ID, "")
		a.manualReceipt(token, *m.From, m, oid)
		return
	}
	a.shopMenu(token, *m.From, m.Chat.ID, 0)
}

func (a *App) setShopState(id int64, st string) {
	a.Store.Update(func(d *Data) error {
		for _, s := range d.ShopUsers {
			if s.TgID == id {
				s.State = st
			}
		}
		return nil
	})
}

func (a *App) shopMenu(token string, u tgUser, chat, edit int64) {
	var ss ShopSettings
	var su ShopUser
	hasSub := false
	a.Store.Update(func(d *Data) error {
		ss = d.Settings.Shop
		s := a.shopUser(d, u)
		su = *s
		hasSub = (s.ClientID != "" && d.ClientByID(s.ClientID) != nil) || s.ProxyID != ""
		return nil
	})
	rows := [][]btn{{{"text": "🛒 Купить / продлить", "callback_data": "buy"}}}
	if hasSub {
		rows = append(rows, []btn{{"text": "🔑 Моя подписка", "callback_data": "my"}})
	}
	if ss.TrialDays > 0 && !su.TrialUsed && !hasSub {
		rows = append(rows, []btn{{"text": fmt.Sprintf("🎁 Попробовать %d дн. бесплатно", ss.TrialDays), "callback_data": "trial"}})
	}
	rows = append(rows, []btn{{"text": "🏷 Промокод", "callback_data": "promo"}})
	if ss.Support != "" {
		rows = append(rows, []btn{{"text": "💬 Поддержка", "url": supportURL(ss.Support)}})
	}
	text := esch(ss.Welcome)
	if su.Promo != "" {
		text += "\n\n🏷 Активен промокод <b>" + esch(su.Promo) + "</b> — скидка применится при оплате."
	}
	a.shopSend(token, chat, edit, text, kb(rows...))
}

func supportURL(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "http") {
		return s
	}
	return "https://t.me/" + strings.TrimPrefix(s, "@")
}

func fmtTariff(t Tariff) string {
	parts := []string{fmt.Sprintf("%d дн.", t.Days)}
	if t.TrafficGB > 0 {
		parts = append(parts, fmt.Sprintf("%d ГБ", t.TrafficGB))
	} else {
		parts = append(parts, "безлимит")
	}
	if t.Devices > 0 && t.Kind != "proxy" {
		parts = append(parts, fmt.Sprintf("до %d устр.", t.Devices))
	}
	return strings.Join(parts, " · ")
}

func (a *App) shopAction(token string, u tgUser, chat, mid int64, data string) {
	var ss ShopSettings
	var su ShopUser
	var promoPct int
	a.Store.Update(func(d *Data) error {
		ss = d.Settings.Shop
		s := a.shopUser(d, u)
		su = *s
		if s.Promo != "" {
			if p := a.promoValid(d, s.Promo); p != nil {
				promoPct = p.Percent
			} else {
				s.Promo = ""
			}
		}
		return nil
	})
	parts := strings.Split(data, ":")
	back := []btn{{"text": "← Меню", "callback_data": "menu"}}
	switch parts[0] {
	case "menu":
		a.setShopState(u.ID, "")
		a.shopMenu(token, u, chat, mid)
	case "buy":
		rows := [][]btn{}
		for _, t := range ss.Tariffs {
			if !t.Enabled {
				continue
			}
			label := t.Name + " — " + strconv.Itoa(priceWithPromo(t.Price, promoPct)) + " ₽"
			if t.Kind == "proxy" {
				label = "✈️ " + label
			}
			rows = append(rows, []btn{{"text": label, "callback_data": "t:" + t.ID}})
		}
		rows = append(rows, back)
		text := "<b>Выберите тариф</b>"
		if promoPct > 0 {
			text += fmt.Sprintf("\nСкидка по промокоду: −%d%%", promoPct)
		}
		if len(rows) == 1 {
			text = "Тарифы пока не настроены. Напишите в поддержку."
		}
		a.shopSend(token, chat, mid, text, kb(rows...))
	case "t":
		if len(parts) < 2 {
			return
		}
		t := findTariff(ss, parts[1])
		if t == nil {
			a.shopSend(token, chat, mid, "Тариф больше недоступен.", kb(back))
			return
		}
		price := priceWithPromo(t.Price, promoPct)
		stars := priceWithPromo(t.Stars, promoPct)
		text := fmt.Sprintf("<b>%s</b>\n%s\n\nК оплате: <b>%d ₽</b>", esch(t.Name), fmtTariff(*t), price)
		if t.Kind == "proxy" {
			text += "\n\nДоступ к прокси для Telegram (SOCKS5/MTProto) — подключение в одно касание."
		}
		rows := [][]btn{}
		if ss.PayStars && t.Stars > 0 {
			rows = append(rows, []btn{{"text": fmt.Sprintf("⭐ Telegram Stars — %d", stars), "callback_data": "pay:" + t.ID + ":stars"}})
		}
		if ss.ProviderToken != "" && t.Price > 0 {
			rows = append(rows, []btn{{"text": "💳 Картой", "callback_data": "pay:" + t.ID + ":card"}})
		}
		if ss.CryptoToken != "" && t.Price > 0 {
			rows = append(rows, []btn{{"text": "🪙 Криптовалютой", "callback_data": "pay:" + t.ID + ":crypto"}})
		}
		if ss.PayManual {
			rows = append(rows, []btn{{"text": "🏦 Переводом", "callback_data": "pay:" + t.ID + ":manual"}})
		}
		if len(rows) == 0 {
			text += "\n\nСпособы оплаты пока не настроены — напишите в поддержку."
		}
		rows = append(rows, []btn{{"text": "← Тарифы", "callback_data": "buy"}})
		a.shopSend(token, chat, mid, text, kb(rows...))
	case "pay":
		if len(parts) < 3 {
			return
		}
		a.startPayment(token, u, chat, mid, ss, su, parts[1], parts[2], promoPct)
	case "chk":
		if len(parts) < 2 {
			return
		}
		paid := a.checkCryptoOrder(parts[1])
		if !paid {
			a.shopSend(token, chat, 0, "Оплата пока не поступила. Если вы уже оплатили — подождите минуту и нажмите ещё раз.", nil)
		}
	case "cancel":
		if len(parts) < 2 {
			return
		}
		a.Store.Update(func(d *Data) error {
			for _, o := range d.Orders {
				if o.ID == parts[1] && o.TgID == u.ID && (o.Status == "pending" || o.Status == "review") {
					o.Status = "canceled"
				}
			}
			return nil
		})
		a.setShopState(u.ID, "")
		a.shopMenu(token, u, chat, mid)
	case "my":
		a.shopMySub(token, u, chat, mid)
	case "trial":
		a.shopTrial(token, u, chat, mid)
	case "promo":
		a.setShopState(u.ID, "promo")
		a.shopSend(token, chat, mid, "Отправьте промокод одним сообщением.", kb(back))
	case "adm_ok", "adm_no":
		var admin string
		a.Store.Read(func(d *Data) { admin = d.Settings.TgAdminID })
		if strconv.FormatInt(u.ID, 10) != admin || len(parts) < 2 {
			return
		}
		if parts[0] == "adm_ok" {
			if err := a.fulfillOrder(parts[1]); err != nil {
				a.shopSend(token, chat, 0, "Не удалось: "+esch(err.Error()), nil)
				return
			}
			a.shopSend(token, chat, mid, "✅ Заказ "+parts[1]+" подтверждён, доступ выдан.", nil)
		} else {
			a.rejectOrder(parts[1])
			a.shopSend(token, chat, mid, "❌ Заказ "+parts[1]+" отклонён.", nil)
		}
	}
}

func findTariff(ss ShopSettings, id string) *Tariff {
	for i := range ss.Tariffs {
		if ss.Tariffs[i].ID == id && ss.Tariffs[i].Enabled {
			return &ss.Tariffs[i]
		}
	}
	return nil
}

func (a *App) startPayment(token string, u tgUser, chat, mid int64, ss ShopSettings, su ShopUser, tid, method string, promoPct int) {
	t := findTariff(ss, tid)
	if t == nil {
		return
	}
	o := &Order{ID: RandomDigits(8), TgID: u.ID, User: orDefault(u.Username, u.FirstName), TariffID: t.ID, Tariff: t.Name,
		Method: method, Status: "pending", CreatedAt: nowMs(), Promo: su.Promo}
	switch method {
	case "stars":
		o.Amount, o.Currency = priceWithPromo(t.Stars, promoPct), "XTR"
	default:
		o.Amount, o.Currency = priceWithPromo(t.Price, promoPct), "RUB"
	}
	a.Store.Update(func(d *Data) error {
		d.Orders = append(d.Orders, o)
		if len(d.Orders) > 5000 {
			d.Orders = d.Orders[len(d.Orders)-5000:]
		}
		return nil
	})
	title := t.Name
	desc := fmtTariff(*t)
	switch method {
	case "stars", "card":
		p := map[string]any{"chat_id": chat, "title": title, "description": desc, "payload": o.ID, "currency": o.Currency}
		if method == "stars" {
			p["prices"] = []map[string]any{{"label": title, "amount": o.Amount}}
			p["provider_token"] = ""
		} else {
			p["prices"] = []map[string]any{{"label": title, "amount": o.Amount * 100}}
			p["provider_token"] = ss.ProviderToken
		}
		if err := tgCall(token, "sendInvoice", p, nil); err != nil {
			a.shopSend(token, chat, 0, "Не удалось выставить счёт: "+esch(err.Error()), nil)
		}
	case "crypto":
		var inv struct {
			InvoiceID     int64  `json:"invoice_id"`
			BotInvoiceURL string `json:"bot_invoice_url"`
			PayURL        string `json:"pay_url"`
		}
		err := cryptoCall(ss.CryptoToken, "createInvoice", map[string]any{"currency_type": "fiat", "fiat": "RUB",
			"amount": strconv.Itoa(o.Amount), "description": title + " · " + desc, "payload": o.ID, "expires_in": 3600}, &inv)
		if err != nil {
			a.shopSend(token, chat, 0, "Криптооплата недоступна: "+esch(err.Error()), nil)
			return
		}
		link := orDefault(inv.BotInvoiceURL, inv.PayURL)
		a.Store.Update(func(d *Data) error {
			for _, x := range d.Orders {
				if x.ID == o.ID {
					x.ExtID = strconv.FormatInt(inv.InvoiceID, 10)
					x.PayURL = link
				}
			}
			return nil
		})
		a.shopSend(token, chat, mid, fmt.Sprintf("Счёт на <b>%d ₽</b> в CryptoBot. После оплаты доступ придёт автоматически.", o.Amount),
			kb([]btn{{"text": "🪙 Оплатить", "url": link}}, []btn{{"text": "Я оплатил — проверить", "callback_data": "chk:" + o.ID}}, []btn{{"text": "Отмена", "callback_data": "cancel:" + o.ID}}))
	case "manual":
		a.setShopState(u.ID, "receipt:"+o.ID)
		text := fmt.Sprintf("Заказ <b>№%s</b>\nСумма: <b>%d ₽</b>\n\n%s\n\nПосле перевода пришлите сюда чек или скриншот — администратор проверит и бот выдаст доступ.",
			o.ID, o.Amount, esch(orDefault(ss.ManualText, "Реквизиты уточните в поддержке.")))
		a.shopSend(token, chat, mid, text, kb([]btn{{"text": "Отмена", "callback_data": "cancel:" + o.ID}}))
	}
}

func (a *App) manualReceipt(token string, u tgUser, m *tgMessage, oid string) {
	var o Order
	found := false
	var admin string
	a.Store.Update(func(d *Data) error {
		admin = d.Settings.TgAdminID
		for _, x := range d.Orders {
			if x.ID == oid && x.TgID == u.ID && x.Status == "pending" {
				x.Status = "review"
				o = *x
				found = true
			}
		}
		return nil
	})
	if !found {
		a.shopSend(token, m.Chat.ID, 0, "Заказ не найден — оформите новый через /buy", nil)
		return
	}
	a.shopSend(token, m.Chat.ID, 0, "Спасибо! Чек отправлен администратору. Доступ придёт сюда после проверки.", nil)
	a.Store.Update(func(d *Data) error {
		d.AddEvent("info", fmt.Sprintf("Бот-магазин: чек по заказу №%s (%d ₽) ждёт проверки", o.ID, o.Amount), "")
		return nil
	})
	text := fmt.Sprintf("🧾 Оплата переводом\nЗаказ №%s · %s\nПокупатель: %s (id %d)\nСумма: %d ₽", o.ID, o.Tariff, orDefault("@"+u.Username, u.FirstName), u.ID, o.Amount)
	if admin == "" {
		return
	}
	aid, _ := strconv.ParseInt(admin, 10, 64)
	_ = tgCall(token, "forwardMessage", map[string]any{"chat_id": aid, "from_chat_id": m.Chat.ID, "message_id": m.MessageID}, nil)
	err := tgCall(token, "sendMessage", map[string]any{"chat_id": aid, "text": text,
		"reply_markup": kb([]btn{{"text": "✅ Подтвердить", "callback_data": "adm_ok:" + o.ID}, {"text": "❌ Отклонить", "callback_data": "adm_no:" + o.ID}})}, nil)
	if err != nil {
		a.notify(text + "\n\nПодтвердите заказ в панели: Бот-магазин → Заказы (запустите бота-магазин со своего аккаунта, чтобы получать кнопки).")
	}
}

func cryptoCall(token, method string, params map[string]any, out any) error {
	b, _ := json.Marshal(params)
	req, _ := http.NewRequest("POST", "https://pay.crypt.bot/api/"+method, bytes.NewReader(b))
	req.Header.Set("Crypto-Pay-API-Token", token)
	req.Header.Set("Content-Type", "application/json")
	c := &http.Client{Timeout: 20 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var res struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  any             `json:"error"`
	}
	if err := json.Unmarshal(rb, &res); err != nil {
		return fmt.Errorf("CryptoBot: %s", resp.Status)
	}
	if !res.OK {
		return fmt.Errorf("CryptoBot: %v", res.Error)
	}
	if out != nil {
		return json.Unmarshal(res.Result, out)
	}
	return nil
}

func (a *App) checkCryptoOrder(oid string) bool {
	var ext, token string
	a.Store.Read(func(d *Data) {
		token = d.Settings.Shop.CryptoToken
		for _, o := range d.Orders {
			if o.ID == oid && o.Status == "pending" && o.Method == "crypto" {
				ext = o.ExtID
			}
		}
	})
	if ext == "" || token == "" {
		return false
	}
	var res struct {
		Items []struct {
			InvoiceID int64  `json:"invoice_id"`
			Status    string `json:"status"`
		} `json:"items"`
	}
	if err := cryptoCall(token, "getInvoices", map[string]any{"invoice_ids": ext}, &res); err != nil {
		return false
	}
	for _, it := range res.Items {
		if strconv.FormatInt(it.InvoiceID, 10) == ext {
			switch it.Status {
			case "paid":
				return a.fulfillOrder(oid) == nil
			case "expired":
				a.Store.Update(func(d *Data) error {
					for _, o := range d.Orders {
						if o.ID == oid {
							o.Status = "canceled"
						}
					}
					return nil
				})
			}
		}
	}
	return false
}

func (a *App) checkCryptoOrders() {
	var ids []string
	cut := time.Now().Add(-2 * time.Hour).UnixMilli()
	a.Store.Read(func(d *Data) {
		for _, o := range d.Orders {
			if o.Method == "crypto" && o.Status == "pending" && o.CreatedAt > cut {
				ids = append(ids, o.ID)
			}
		}
	})
	for _, id := range ids {
		a.checkCryptoOrder(id)
	}
}

// fulfillOrder marks an order paid and grants/extends access.
func (a *App) fulfillOrder(oid string) error {
	var o Order
	var t *Tariff
	var su ShopUser
	var subURL string
	var links map[string]string
	var expires int64
	var proxy bool
	err := a.Store.Update(func(d *Data) error {
		var op *Order
		for _, x := range d.Orders {
			if x.ID == oid {
				op = x
			}
		}
		if op == nil {
			return errf(404, "Заказ не найден")
		}
		if op.Status == "paid" {
			return errf(409, "Заказ уже оплачен")
		}
		t = findTariff(d.Settings.Shop, op.TariffID)
		if t == nil {
			for i := range d.Settings.Shop.Tariffs {
				if d.Settings.Shop.Tariffs[i].ID == op.TariffID {
					t = &d.Settings.Shop.Tariffs[i]
				}
			}
		}
		if t == nil {
			return errf(400, "Тариф заказа удалён")
		}
		var s *ShopUser
		for _, x := range d.ShopUsers {
			if x.TgID == op.TgID {
				s = x
			}
		}
		if s == nil {
			s = &ShopUser{TgID: op.TgID, Username: op.User, JoinedAt: nowMs()}
			d.ShopUsers = append(d.ShopUsers, s)
		}
		op.Status = "paid"
		op.PaidAt = nowMs()
		if op.Promo != "" {
			for _, p := range d.Promos {
				if strings.EqualFold(p.Code, op.Promo) {
					p.Used++
				}
			}
			if strings.EqualFold(s.Promo, op.Promo) {
				s.Promo = ""
			}
		}
		if t.Kind == "proxy" {
			proxy = true
			var p *ProxyUser
			for _, x := range d.ProxyUsers {
				if x.ID == s.ProxyID {
					p = x
				}
			}
			if p == nil {
				p = a.createProxyUser(d, "tg_"+orDefault(s.Username, strconv.FormatInt(s.TgID, 10)), 0, s.TgID)
				s.ProxyID = p.ID
				p.ExpiresAt = nowMs()
			}
			base := nowMs()
			if p.ExpiresAt > base {
				base = p.ExpiresAt
			}
			p.ExpiresAt = base + int64(t.Days)*86400*1000
			p.Enabled = true
			expires = p.ExpiresAt
			links = a.proxyLinks(d, p)
		} else {
			c := a.shopEnsureClient(d, s)
			base := nowMs()
			if c.ExpiresAt > base {
				base = c.ExpiresAt
			}
			c.ExpiresAt = base + int64(t.Days)*86400*1000
			c.TrafficLimit = int64(t.TrafficGB) << 30
			c.Up, c.Down = 0, 0
			c.DeviceLimit = t.Devices
			c.Banned, c.Frozen = false, false
			expires = c.ExpiresAt
			subURL = a.subBaseURLLocked(d) + c.SubID
			op.ClientID = c.ID
		}
		su = *s
		o = *op
		cur := "₽"
		if o.Currency == "XTR" {
			cur = "⭐"
		}
		d.AddEvent("ok", fmt.Sprintf("Бот-магазин: оплачен заказ №%s — %s, %d %s (%s)", o.ID, t.Name, o.Amount, cur, orDefault(su.Username, strconv.FormatInt(su.TgID, 10))), "")
		return nil
	})
	if err != nil {
		return err
	}
	a.ScheduleApply()
	go a.SyncNodes()
	cur := "₽"
	if o.Currency == "XTR" {
		cur = "⭐"
	}
	a.notify(fmt.Sprintf("💰 Оплата в боте: %s — %d %s (%s)", t.Name, o.Amount, cur, orDefault("@"+su.Username, strconv.FormatInt(su.TgID, 10))))
	token := a.shopToken()
	if token == "" {
		return nil
	}
	until := time.UnixMilli(expires).In(moscow()).Format("02.01.2006")
	if proxy {
		rows := [][]btn{}
		if l := links["socks"]; l != "" {
			rows = append(rows, []btn{{"text": "✈️ Подключить SOCKS5", "url": l}})
		}
		if l := links["mtproto"]; l != "" {
			rows = append(rows, []btn{{"text": "🛡 Подключить MTProto", "url": l}})
		}
		rows = append(rows, []btn{{"text": "← Меню", "callback_data": "menu"}})
		a.shopSend(token, o.TgID, 0, fmt.Sprintf("✅ Оплата получена!\nПрокси для Telegram активен до <b>%s</b>.\nНажмите кнопку ниже — Telegram сам предложит подключить прокси.", until), kb(rows...))
	} else {
		a.shopSend(token, o.TgID, 0, fmt.Sprintf("✅ Оплата получена!\nПодписка активна до <b>%s</b>.\n\nВаша ссылка для приложения (Happ, v2RayTun, Hiddify, Streisand):\n<code>%s</code>\n\nСкопируйте её и добавьте в приложение через «+» → «Из буфера».", until, esch(subURL)),
			kb([]btn{{"text": "📲 Открыть страницу подписки", "url": subURL}}, []btn{{"text": "← Меню", "callback_data": "menu"}}))
	}
	return nil
}

func (a *App) shopEnsureClient(d *Data, s *ShopUser) *Client {
	if c := d.ClientByID(s.ClientID); c != nil {
		return c
	}
	base := "tg_" + orDefault(s.Username, strconv.FormatInt(s.TgID, 10))
	name := base
	for i := 2; d.ClientByName(name) != nil; i++ {
		name = fmt.Sprintf("%s_%d", base, i)
	}
	c := newClient(d)
	c.Name = name
	c.Telegram = strconv.FormatInt(s.TgID, 10)
	c.Tags = []string{"бот"}
	c.Origin = "shop"
	c.ExpiresAt = nowMs()
	grant := []string{}
	for _, id := range d.Settings.Shop.Inbounds {
		if d.ValidInboundID(id) {
			grant = append(grant, id)
		}
	}
	if len(grant) == 0 {
		grant = d.DefaultGrant()
	}
	c.Inbounds = grant
	d.Clients = append(d.Clients, c)
	s.ClientID = c.ID
	return c
}

func (a *App) rejectOrder(oid string) {
	var tg int64
	a.Store.Update(func(d *Data) error {
		for _, o := range d.Orders {
			if o.ID == oid && o.Status != "paid" {
				o.Status = "rejected"
				tg = o.TgID
			}
		}
		return nil
	})
	if token := a.shopToken(); token != "" && tg != 0 {
		a.shopSend(token, tg, 0, "❌ Оплата по заказу №"+oid+" не подтверждена. Если это ошибка — напишите в поддержку.", nil)
	}
}

func (a *App) shopMySub(token string, u tgUser, chat, mid int64) {
	var text string
	rows := [][]btn{}
	a.Store.Read(func(d *Data) {
		now := nowMs()
		var s *ShopUser
		for _, x := range d.ShopUsers {
			if x.TgID == u.ID {
				s = x
			}
		}
		if s == nil {
			text = "Подписки пока нет."
			return
		}
		if c := d.ClientByID(s.ClientID); c != nil {
			st := map[string]string{StActive: "✅ активна", StExpired: "⛔ срок истёк", StBanned: "⛔ отключена", StFrozen: "❄️ заморожена", StLimited: "⛔ трафик исчерпан"}[c.Status(now)]
			until := "без срока"
			if c.ExpiresAt > 0 {
				until = time.UnixMilli(c.ExpiresAt).In(moscow()).Format("02.01.2006 15:04")
			}
			traffic := humanBytes(c.Used())
			if c.TrafficLimit > 0 {
				traffic += " из " + humanBytes(c.TrafficLimit)
			}
			sub := a.subBaseURLLocked(d) + c.SubID
			text = fmt.Sprintf("<b>Ваша подписка</b>\nСтатус: %s\nДо: <b>%s</b>\nТрафик: %s\n\nСсылка для приложения:\n<code>%s</code>", st, until, traffic, esch(sub))
			rows = append(rows, []btn{{"text": "📲 Страница подписки", "url": sub}})
		}
		for _, p := range d.ProxyUsers {
			if p.ID == s.ProxyID {
				until := time.UnixMilli(p.ExpiresAt).In(moscow()).Format("02.01.2006")
				st := "✅ активен"
				if !p.Active(now) {
					st = "⛔ истёк"
				}
				text += fmt.Sprintf("\n\n<b>Прокси для Telegram</b>: %s до %s", st, until)
				l := a.proxyLinks(d, p)
				if l["socks"] != "" && p.Active(now) {
					rows = append(rows, []btn{{"text": "✈️ SOCKS5", "url": l["socks"]}})
				}
				if l["mtproto"] != "" && p.Active(now) {
					rows = append(rows, []btn{{"text": "🛡 MTProto", "url": l["mtproto"]}})
				}
			}
		}
		if text == "" {
			text = "Подписки пока нет."
		}
	})
	rows = append(rows, []btn{{"text": "🛒 Продлить", "callback_data": "buy"}}, []btn{{"text": "← Меню", "callback_data": "menu"}})
	a.shopSend(token, chat, mid, text, kb(rows...))
}

func (a *App) shopTrial(token string, u tgUser, chat, mid int64) {
	var subURL string
	var days int
	err := a.Store.Update(func(d *Data) error {
		ss := d.Settings.Shop
		if ss.TrialDays <= 0 {
			return errf(400, "Пробный период недоступен")
		}
		s := a.shopUser(d, u)
		if s.TrialUsed || d.ClientByID(s.ClientID) != nil {
			return errf(400, "Пробный период уже использован")
		}
		c := a.shopEnsureClient(d, s)
		c.ExpiresAt = nowMs() + int64(ss.TrialDays)*86400*1000
		c.TrafficLimit = int64(ss.TrialGB) << 30
		c.DeviceLimit = 1
		c.Tags = append(c.Tags, "пробный")
		s.TrialUsed = true
		days = ss.TrialDays
		subURL = a.subBaseURLLocked(d) + c.SubID
		d.AddEvent("info", "Бот-магазин: выдан пробный период "+c.Name, c.ID)
		return nil
	})
	if err != nil {
		a.shopSend(token, chat, mid, esch(err.Error()), kb([]btn{{"text": "← Меню", "callback_data": "menu"}}))
		return
	}
	a.ScheduleApply()
	go a.SyncNodes()
	a.shopSend(token, chat, mid, fmt.Sprintf("🎁 Пробный период на %d дн. активирован!\n\nСсылка для приложения:\n<code>%s</code>", days, esch(subURL)),
		kb([]btn{{"text": "📲 Страница подписки", "url": subURL}}, []btn{{"text": "← Меню", "callback_data": "menu"}}))
}

func (a *App) applyPromoCode(token string, u tgUser, chat int64, code string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	var msg string
	var bonusDays int
	var subURL string
	a.Store.Update(func(d *Data) error {
		p := a.promoValid(d, code)
		if p == nil {
			msg = "Промокод не найден или больше не действует."
			return nil
		}
		s := a.shopUser(d, u)
		if p.Days > 0 {
			c := a.shopEnsureClient(d, s)
			base := nowMs()
			if c.ExpiresAt > base {
				base = c.ExpiresAt
			}
			c.ExpiresAt = base + int64(p.Days)*86400*1000
			c.Banned, c.Frozen = false, false
			p.Used++
			bonusDays = p.Days
			subURL = a.subBaseURLLocked(d) + c.SubID
			d.AddEvent("info", "Бот-магазин: промокод "+p.Code+" (+"+strconv.Itoa(p.Days)+" дн.) — "+c.Name, c.ID)
		}
		if p.Percent > 0 {
			s.Promo = p.Code
			msg = fmt.Sprintf("Промокод принят: скидка −%d%% на следующую покупку.", p.Percent)
		}
		return nil
	})
	if bonusDays > 0 {
		a.ScheduleApply()
		go a.SyncNodes()
		msg = fmt.Sprintf("🎉 Промокод принят: +%d дн. к подписке.\n\nСсылка для приложения:\n<code>%s</code>", bonusDays, esch(subURL)) + "\n" + msg
	}
	a.shopSend(token, chat, 0, msg, kb([]btn{{"text": "🛒 К тарифам", "callback_data": "buy"}}, []btn{{"text": "← Меню", "callback_data": "menu"}}))
}

// shopRemind notifies buyers whose subscription ends within 3 days (once per day).
func (a *App) shopRemind() {
	token := a.shopToken()
	var remind bool
	a.Store.Read(func(d *Data) { remind = d.Settings.Shop.Remind })
	if token == "" || !remind {
		return
	}
	type rem struct {
		tg   int64
		days int
	}
	var list []rem
	now := nowMs()
	a.Store.Update(func(d *Data) error {
		for _, s := range d.ShopUsers {
			c := d.ClientByID(s.ClientID)
			if c == nil || s.Blocked || c.ExpiresAt == 0 || c.Status(now) != StActive {
				continue
			}
			left := c.ExpiresAt - now
			if left > 0 && left < 3*86400*1000 && now-s.RemindedAt > 20*3600*1000 {
				s.RemindedAt = now
				list = append(list, rem{s.TgID, int(left/86400000) + 1})
			}
		}
		return nil
	})
	for _, r := range list {
		a.shopSend(token, r.tg, 0, fmt.Sprintf("⏳ Подписка закончится через %d дн. Продлите, чтобы VPN не отключился.", r.days),
			kb([]btn{{"text": "🛒 Продлить", "callback_data": "buy"}}))
		time.Sleep(50 * time.Millisecond)
	}
}

// ================= panel API =================

func (a *App) apiShop(w http.ResponseWriter, r *http.Request) error {
	res := map[string]any{}
	a.Store.Read(func(d *Data) {
		ss := d.Settings.Shop
		ss.BotToken = maskToken(ss.BotToken)
		ss.ProviderToken = maskToken(ss.ProviderToken)
		ss.CryptoToken = maskToken(ss.CryptoToken)
		if ss.Inbounds == nil {
			ss.Inbounds = []string{}
		}
		res["settings"] = ss
		month := time.Now().AddDate(0, 0, -30).UnixMilli()
		revenue, stars, paid, pending := 0, 0, 0, 0
		for _, o := range d.Orders {
			if o.Status == "paid" {
				paid++
				if o.PaidAt > month {
					if o.Currency == "XTR" {
						stars += o.Amount
					} else {
						revenue += o.Amount
					}
				}
			}
			if o.Status == "review" {
				pending++
			}
		}
		active := 0
		now := nowMs()
		for _, s := range d.ShopUsers {
			if c := d.ClientByID(s.ClientID); c != nil && c.Status(now) == StActive {
				active++
			}
		}
		res["stats"] = map[string]any{"users": len(d.ShopUsers), "active": active, "paid": paid, "revenue30": revenue, "stars30": stars, "review": pending}
		res["tgAdmin"] = d.Settings.TgAdminID != ""
	})
	shop.mu.Lock()
	res["bot"] = map[string]any{"running": shop.running, "error": shop.lastErr, "username": shop.username}
	shop.mu.Unlock()
	ok(w, res)
	return nil
}

func (a *App) apiShopSave(w http.ResponseWriter, r *http.Request) error {
	var req ShopSettings
	if err := readJSON(r, &req); err != nil {
		return err
	}
	for i := range req.Tariffs {
		t := &req.Tariffs[i]
		t.Name = strings.TrimSpace(t.Name)
		if t.Name == "" || t.Days <= 0 {
			return errf(400, "У каждого тарифа должны быть название и срок")
		}
		if t.Price < 0 || t.Stars < 0 || t.TrafficGB < 0 || t.Devices < 0 {
			return errf(400, "Отрицательные значения в тарифе «%s»", t.Name)
		}
		if t.Kind != "proxy" {
			t.Kind = "vpn"
		}
		if t.ID == "" {
			t.ID = RandomHex(4)
		}
	}
	if len([]rune(req.Welcome)) > 2000 {
		return errf(400, "Приветствие слишком длинное")
	}
	var restart bool
	a.Store.Update(func(d *Data) error {
		old := d.Settings.Shop
		if strings.Contains(req.BotToken, "••••") {
			req.BotToken = old.BotToken
		}
		if strings.Contains(req.ProviderToken, "••••") {
			req.ProviderToken = old.ProviderToken
		}
		if strings.Contains(req.CryptoToken, "••••") {
			req.CryptoToken = old.CryptoToken
		}
		req.BotToken = strings.TrimSpace(req.BotToken)
		req.BotUsername = old.BotUsername
		restart = old.BotToken != req.BotToken || old.Enabled != req.Enabled
		d.Settings.Shop = req
		return nil
	})
	if restart {
		a.StartShop()
		time.Sleep(1500 * time.Millisecond)
	}
	return a.apiShop(w, r)
}

func (a *App) apiShopOrders(w http.ResponseWriter, r *http.Request) error {
	var list []Order
	a.Store.Read(func(d *Data) {
		for i := len(d.Orders) - 1; i >= 0 && len(list) < 500; i-- {
			list = append(list, *d.Orders[i])
		}
	})
	if list == nil {
		list = []Order{}
	}
	ok(w, list)
	return nil
}

func (a *App) apiShopOrderConfirm(w http.ResponseWriter, r *http.Request) error {
	if err := a.fulfillOrder(r.PathValue("id")); err != nil {
		return err
	}
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiShopOrderReject(w http.ResponseWriter, r *http.Request) error {
	a.rejectOrder(r.PathValue("id"))
	ok(w, map[string]any{"ok": true})
	return nil
}

func (a *App) apiShopUsers(w http.ResponseWriter, r *http.Request) error {
	var list []map[string]any
	now := nowMs()
	a.Store.Read(func(d *Data) {
		for i := len(d.ShopUsers) - 1; i >= 0; i-- {
			s := d.ShopUsers[i]
			item := map[string]any{"tgId": s.TgID, "username": s.Username, "firstName": s.FirstName, "joinedAt": s.JoinedAt,
				"lastSeen": s.LastSeen, "blocked": s.Blocked, "trialUsed": s.TrialUsed}
			if c := d.ClientByID(s.ClientID); c != nil {
				item["client"] = c.Name
				item["clientId"] = c.ID
				item["status"] = c.Status(now)
				item["expiresAt"] = c.ExpiresAt
			}
			spent := 0
			for _, o := range d.Orders {
				if o.TgID == s.TgID && o.Status == "paid" && o.Currency == "RUB" {
					spent += o.Amount
				}
			}
			item["spent"] = spent
			list = append(list, item)
		}
	})
	if list == nil {
		list = []map[string]any{}
	}
	ok(w, list)
	return nil
}

func (a *App) apiShopPromos(w http.ResponseWriter, r *http.Request) error {
	var list []Promo
	a.Store.Read(func(d *Data) {
		for _, p := range d.Promos {
			list = append(list, *p)
		}
	})
	if list == nil {
		list = []Promo{}
	}
	ok(w, list)
	return nil
}

func (a *App) apiShopPromoSave(w http.ResponseWriter, r *http.Request) error {
	var req Promo
	if err := readJSON(r, &req); err != nil {
		return err
	}
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	if req.Code == "" {
		req.Code = strings.ToUpper(RandomString(8))
	}
	if req.Percent < 0 || req.Percent > 100 || req.Days < 0 || (req.Percent == 0 && req.Days == 0) {
		return errf(400, "Укажите скидку (1–100%%) или бонусные дни")
	}
	a.Store.Update(func(d *Data) error {
		for _, p := range d.Promos {
			if p.Code == req.Code {
				p.Percent, p.Days, p.MaxUses, p.ExpiresAt, p.Enabled = req.Percent, req.Days, req.MaxUses, req.ExpiresAt, req.Enabled
				return nil
			}
		}
		req.CreatedAt = nowMs()
		d.Promos = append(d.Promos, &req)
		return nil
	})
	ok(w, req)
	return nil
}

func (a *App) apiShopPromoDelete(w http.ResponseWriter, r *http.Request) error {
	code := r.PathValue("code")
	a.Store.Update(func(d *Data) error {
		for i, p := range d.Promos {
			if p.Code == code {
				d.Promos = append(d.Promos[:i], d.Promos[i+1:]...)
				break
			}
		}
		return nil
	})
	ok(w, map[string]any{"ok": true})
	return nil
}

var broadcastState struct {
	sync.Mutex
	Running bool
	Sent    int
	Failed  int
	Total   int
}

func (a *App) apiShopBroadcast(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Text     string `json:"text"`
		Audience string `json:"audience"` // all | active | expired | never
		Button   bool   `json:"button"`
	}
	if err := readJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Text) == "" {
		return errf(400, "Введите текст рассылки")
	}
	token := a.shopToken()
	if token == "" {
		return errf(400, "Бот-магазин не запущен")
	}
	broadcastState.Lock()
	if broadcastState.Running {
		broadcastState.Unlock()
		return errf(409, "Рассылка уже идёт")
	}
	var ids []int64
	now := nowMs()
	a.Store.Read(func(d *Data) {
		for _, s := range d.ShopUsers {
			if s.Blocked {
				continue
			}
			c := d.ClientByID(s.ClientID)
			switch req.Audience {
			case "active":
				if c == nil || c.Status(now) != StActive {
					continue
				}
			case "expired":
				if c == nil || c.Status(now) == StActive {
					continue
				}
			case "never":
				if c != nil {
					continue
				}
			}
			ids = append(ids, s.TgID)
		}
	})
	broadcastState.Running, broadcastState.Sent, broadcastState.Failed, broadcastState.Total = true, 0, 0, len(ids)
	broadcastState.Unlock()
	go func() {
		var markup any
		if req.Button {
			markup = kb([]btn{{"text": "🛒 Купить / продлить", "callback_data": "buy"}})
		}
		for _, id := range ids {
			p := map[string]any{"chat_id": id, "text": req.Text, "disable_web_page_preview": true}
			if markup != nil {
				p["reply_markup"] = markup
			}
			err := tgCall(token, "sendMessage", p, nil)
			broadcastState.Lock()
			if err != nil {
				broadcastState.Failed++
			} else {
				broadcastState.Sent++
			}
			broadcastState.Unlock()
			time.Sleep(40 * time.Millisecond)
		}
		broadcastState.Lock()
		broadcastState.Running = false
		s, f := broadcastState.Sent, broadcastState.Failed
		broadcastState.Unlock()
		a.Store.Update(func(d *Data) error {
			d.AddEvent("info", fmt.Sprintf("Рассылка в боте: доставлено %d, ошибок %d", s, f), "")
			return nil
		})
	}()
	ok(w, map[string]any{"total": len(ids)})
	return nil
}

func (a *App) apiShopBroadcastStatus(w http.ResponseWriter, r *http.Request) error {
	broadcastState.Lock()
	defer broadcastState.Unlock()
	ok(w, map[string]any{"running": broadcastState.Running, "sent": broadcastState.Sent, "failed": broadcastState.Failed, "total": broadcastState.Total})
	return nil
}

var _ = url.QueryEscape
