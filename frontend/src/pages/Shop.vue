<template>
  <div class="pg-shop">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="vw-eb">продажи · Telegram-бот</span><h1>Бот-магазин</h1><p>Клиенты сами покупают и продлевают VPN в Telegram: тарифы, оплата Stars, картой (ЮKassa и др. через Telegram Payments), CryptoBot или вручную, промокоды и рассылка.</p></div>
        <div class="i-hd-a">
          <a v-if="d?.bot.username" class="btn" :href="'https://t.me/' + d.bot.username" target="_blank" rel="noopener"><Icon n="send" />@{{ d.bot.username }}</a>
          <button v-if="s" class="btn btn-p" type="button" :disabled="busy.save || !dirty" @click="save"><span v-if="busy.save" class="spin"></span><Icon v-else n="check" />Сохранить</button>
        </div>
      </div>
      <Loading v-if="!s" :error="error" @retry="load" />
      <template v-else>
        <div class="i-mt">
          <Metric label="Пользователей бота" :value="d.stats.users" :note="`${d.stats.active} с активной подпиской`" />
          <Metric label="Оплачено заказов" :value="d.stats.paid" :note="d.stats.review ? `${d.stats.review} ждут проверки` : 'все проверены'" :note-tone="d.stats.review ? 'wa' : ''" />
          <Metric label="Выручка за 30 дней" :value="fmtNum(d.stats.revenue30) + ' ₽'" :note="`${fmtNum(d.stats.stars30)} ⭐ Stars`" />
          <Metric label="Бот" :value="d.bot.running ? 'работает' : 'остановлен'" :note="d.bot.error || (d.bot.username ? '@' + d.bot.username : 'укажите токен')" :note-tone="d.bot.error ? 'bad' : d.bot.running ? 'ok' : ''" />
        </div>
        <div class="i-tabs" role="tablist" style="margin-top:0">
          <button v-for="t in TABS" :key="t[0]" type="button" :class="{ on: tab === t[0] }" @click="tab = t[0]">{{ t[1] }}<span v-if="t[0] === 'orders' && d.stats.review" class="i-cnt" style="margin-left:6px">{{ d.stats.review }}</span></button>
        </div>

        <div v-if="tab === 'bot'" class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>Бот</h2><span class="i-stt" :class="d.bot.running ? 'ok' : d.bot.error ? 'bad' : 'mu'">{{ d.bot.running ? 'работает' : d.bot.error ? 'ошибка' : 'выключен' }}</span></div>
            <div class="i-pb">
              <TgRow v-model="s.enabled" bold label="Включить бот-магазин" hint="Бот начнёт отвечать сразу после сохранения" />
              <div class="fld"><label class="lbl">Токен бота</label><input v-model="s.botToken" class="inp mono" placeholder="123456:ABC… от @BotFather" autocomplete="off"><p class="hint">Создайте бота в <a href="https://t.me/BotFather" target="_blank" rel="noopener">@BotFather</a> → /newbot и вставьте токен.</p></div>
              <div v-if="d.bot.error" class="err-box">{{ d.bot.error }}</div>
              <div class="fld"><label class="lbl">Приветствие</label><textarea v-model="s.welcome" class="inp" rows="4" maxlength="2000"></textarea></div>
              <div class="fld"><label class="lbl">Поддержка</label><input v-model="s.support" class="inp" placeholder="@support_username или https://…"></div>
              <TgRow v-model="s.remind" bold label="Напоминать о продлении" hint="за 3 дня и за 1 день до конца подписки, и в день окончания" />
              <div v-if="!d.tgAdmin" class="i-hint wa"><Icon n="warn" /><span>Укажите свой Telegram ID в <router-link to="/security">«Безопасности» → Telegram</router-link> — бот будет присылать уведомления об оплатах и заявки на ручную проверку.</span></div>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Что получает покупатель</h2></div>
            <div class="i-pb">
              <div class="fld"><label class="lbl">Инбаунды для новых клиентов из бота · {{ s.inbounds.length || 'все включённые' }}</label>
                <div class="list-rows">
                  <label v-for="i in inbounds" :key="i.id" class="lr" style="cursor:pointer"><button class="chk" :class="{ on: s.inbounds.includes(i.id) }" type="button" @click.prevent="toggleArr(s.inbounds, i.id)"><Icon n="check" /></button><span class="lr-t"><b>{{ i.remark }}</b><small>{{ i.node ? "сервер «" + i.node + "» · " : "" }}{{ PROTO[i.protocol] || i.protocol }} · порт {{ i.port }}</small></span></label>
                </div>
                <p class="hint">Ничего не отмечено — все включённые инбаунды.</p>
              </div>
              <div class="two">
                <div class="fld"><label class="lbl">Пробный период, дней</label><input v-model.number="s.trialDays" class="inp" type="number" min="0"><p class="hint">0 — без пробного</p></div>
                <div class="fld"><label class="lbl">Трафик пробного, ГБ</label><input v-model.number="s.trialGb" class="inp" type="number" min="0"><p class="hint">0 — безлимит</p></div>
              </div>
            </div>
          </section>
        </div>

        <section v-if="tab === 'tariffs'" class="i-pn">
          <div class="i-ph"><h2>Тарифы <span class="i-cnt">{{ s.tariffs.length }}</span></h2><button class="btn btn-sm" type="button" @click="addTariff"><Icon n="plus" />Тариф</button></div>
          <div class="tbl-w">
            <table class="tbl">
              <thead><tr><th>Вкл</th><th>Название</th><th>Что</th><th>Дней</th><th>Трафик, ГБ</th><th>Устройств</th><th>Цена, ₽</th><th>Stars ⭐</th><th></th></tr></thead>
              <tbody>
                <tr v-for="(t, i) in s.tariffs" :key="t.id || i">
                  <td><Tg v-model="t.enabled" /></td>
                  <td><input v-model="t.name" class="inp" style="min-width:140px"></td>
                  <td><select v-model="t.kind" class="inp" style="min-width:110px"><option value="vpn">VPN</option><option value="proxy">TG-прокси</option></select></td>
                  <td><input v-model.number="t.days" class="inp" type="number" min="1" style="width:84px"></td>
                  <td><input v-model.number="t.trafficGb" class="inp" type="number" min="0" style="width:96px" :disabled="t.kind === 'proxy'"></td>
                  <td><input v-model.number="t.devices" class="inp" type="number" min="0" style="width:84px" :disabled="t.kind === 'proxy'"></td>
                  <td><input v-model.number="t.price" class="inp" type="number" min="0" style="width:96px"></td>
                  <td><input v-model.number="t.stars" class="inp" type="number" min="0" style="width:96px"></td>
                  <td class="r"><span class="acts"><button class="btn btn-sm btn-g btn-ic" type="button" :disabled="i === 0" @click="swap(i, -1)"><Icon n="up" /></button><button class="btn btn-sm btn-g btn-ic" type="button" @click="s.tariffs.splice(i, 1)"><Icon n="trash" /></button></span></td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="i-pb"><p class="hint">0 в трафике и устройствах — без ограничений. «TG-прокси» выдаёт доступ к Telegram-прокси этой панели. Не забудьте нажать «Сохранить» вверху.</p></div>
        </section>

        <div v-if="tab === 'pay'" class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>Telegram Stars</h2><span class="i-stt" :class="s.payStars ? 'ok' : 'mu'">{{ s.payStars ? 'включено' : 'выключено' }}</span></div>
            <div class="i-pb"><TgRow v-model="s.payStars" bold label="Принимать Stars ⭐" hint="Оплата внутри Telegram без настройки — цена в колонке «Stars» тарифа" /></div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Карта (Telegram Payments)</h2><span class="i-stt" :class="s.providerToken ? 'ok' : 'mu'">{{ s.providerToken ? 'настроено' : 'нет токена' }}</span></div>
            <div class="i-pb"><div class="fld"><label class="lbl">Provider token</label><input v-model="s.providerToken" class="inp mono" placeholder="от @BotFather → Payments (ЮKassa, Smart Glocal…)"><p class="hint">Цена в рублях из колонки «Цена». Пусто — оплата картой выключена.</p></div></div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>CryptoBot</h2><span class="i-stt" :class="s.cryptoToken ? 'ok' : 'mu'">{{ s.cryptoToken ? 'настроено' : 'нет токена' }}</span></div>
            <div class="i-pb"><div class="fld"><label class="lbl">API-токен Crypto Pay</label><input v-model="s.cryptoToken" class="inp mono" placeholder="из @CryptoBot → Crypto Pay → Create App"><p class="hint">Счёт выставляется в рублях, клиент платит USDT/TON/BTC. Оплата проверяется автоматически.</p></div></div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Ручная оплата</h2><span class="i-stt" :class="s.payManual ? 'ok' : 'mu'">{{ s.payManual ? 'включено' : 'выключено' }}</span></div>
            <div class="i-pb">
              <TgRow v-model="s.payManual" bold label="Перевод по реквизитам" hint="Клиент присылает скриншот/номер операции, вы подтверждаете в «Заказах» или кнопкой в Telegram" />
              <div class="fld"><label class="lbl">Реквизиты и инструкция</label><textarea v-model="s.manualText" class="inp" rows="3" placeholder="Перевод на карту 0000 0000 0000 0000 (Сбер), в комментарии — ничего"></textarea></div>
            </div>
          </section>
        </div>

        <section v-if="tab === 'orders'" class="i-pn">
          <div class="i-ph"><h2>Заказы <span class="i-cnt">{{ orders.length }}</span></h2><Seg v-model="ordF" :options="[['all', 'Все'], ['review', 'На проверке'], ['paid', 'Оплачены'], ['pending', 'Ожидают']]" /></div>
          <div class="tbl-w">
            <table class="tbl">
              <thead><tr><th>Дата</th><th>Покупатель</th><th>Тариф</th><th>Сумма</th><th>Способ</th><th>Статус</th><th></th></tr></thead>
              <tbody>
                <tr v-for="o in ordersF" :key="o.id">
                  <td class="nw">{{ dt(o.createdAt) }}</td>
                  <td>{{ o.user || o.tgId }}</td>
                  <td>{{ o.tariff }}</td>
                  <td class="nw"><b>{{ o.currency === 'XTR' ? o.amount + ' ⭐' : fmtNum(o.amount) + ' ₽' }}</b></td>
                  <td>{{ METHOD[o.method] || o.method }}</td>
                  <td><span class="i-stt" :class="{ paid: 'ok', review: 'wa', rejected: 'bad', pending: 'mu', canceled: 'mu' }[o.status]">{{ OST[o.status] || o.status }}</span></td>
                  <td class="r"><span v-if="o.status === 'review' || o.status === 'pending'" class="acts"><button class="btn btn-sm" type="button" @click="orderAct(o, 'confirm')">Подтвердить</button><button class="btn btn-sm btn-g" type="button" @click="orderAct(o, 'reject')">Отклонить</button></span></td>
                </tr>
              </tbody>
            </table>
            <Empty v-if="!ordersF.length" icon="shop" title="Заказов нет" />
          </div>
        </section>

        <section v-if="tab === 'users'" class="i-pn">
          <div class="i-ph"><h2>Пользователи бота <span class="i-cnt">{{ users.length }}</span></h2></div>
          <div class="tbl-w">
            <table class="tbl">
              <thead><tr><th>Пользователь</th><th>Telegram ID</th><th>Клиент</th><th>Подписка</th><th>Потратил</th><th>Пришёл</th></tr></thead>
              <tbody>
                <tr v-for="u in users" :key="u.tgId">
                  <td><b>{{ u.firstName || '—' }}</b> <span v-if="u.username" class="muted">@{{ u.username }}</span><span v-if="u.blocked" class="badge b-bad" style="margin-left:6px">заблокировал бота</span></td>
                  <td class="i-mono">{{ u.tgId }}</td>
                  <td><router-link v-if="u.clientId" :to="'/clients?open=' + u.clientId">{{ u.client }}</router-link><span v-else class="muted">—</span></td>
                  <td><span v-if="u.status" class="i-stt" :class="STATUS[u.status]?.[1]">{{ STATUS[u.status]?.[0] }} · {{ left(u.expiresAt) }}</span><span v-else class="muted">{{ u.trialUsed ? 'был пробный' : 'нет' }}</span></td>
                  <td class="nw">{{ fmtNum(u.spent) }} ₽</td>
                  <td class="nw">{{ dt(u.joinedAt) }}</td>
                </tr>
              </tbody>
            </table>
            <Empty v-if="!users.length" icon="user" title="Пока никто не писал боту" />
          </div>
        </section>

        <div v-if="tab === 'promos'" class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>Промокоды <span class="i-cnt">{{ promos.length }}</span></h2></div>
            <div class="tbl-w">
              <table class="tbl">
                <thead><tr><th>Код</th><th>Даёт</th><th>Использован</th><th>До</th><th></th></tr></thead>
                <tbody>
                  <tr v-for="p in promos" :key="p.code">
                    <td><b class="i-mono">{{ p.code }}</b><span v-if="!p.enabled" class="badge b-mute" style="margin-left:6px">выкл</span></td>
                    <td>{{ p.percent ? `−${p.percent}%` : '' }}{{ p.percent && p.days ? ' · ' : '' }}{{ p.days ? `+${p.days} дн.` : '' }}</td>
                    <td>{{ p.used }} / {{ p.maxUses || '∞' }}</td>
                    <td class="nw">{{ p.expiresAt ? date(p.expiresAt) : '—' }}</td>
                    <td class="r"><span class="acts"><button class="btn btn-sm btn-g" type="button" @click="editPromo(p)">Изменить</button><button class="btn btn-sm btn-g btn-ic" type="button" @click="delPromo(p)"><Icon n="trash" /></button></span></td>
                  </tr>
                </tbody>
              </table>
              <Empty v-if="!promos.length" icon="tag" title="Промокодов нет" text="Создайте скидку или бонусные дни справа" />
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>{{ promo.edit ? 'Изменить промокод' : 'Новый промокод' }}</h2></div>
            <div class="i-pb">
              <div class="fld"><label class="lbl">Код</label><input v-model="promo.code" class="inp mono" placeholder="SALE20" :disabled="promo.edit" style="text-transform:uppercase"></div>
              <div class="two">
                <div class="fld"><label class="lbl">Скидка, %</label><input v-model.number="promo.percent" class="inp" type="number" min="0" max="100"></div>
                <div class="fld"><label class="lbl">Бонусных дней</label><input v-model.number="promo.days" class="inp" type="number" min="0"><p class="hint">добавятся к покупке, или сразу — если скидки нет</p></div>
              </div>
              <div class="two">
                <div class="fld"><label class="lbl">Максимум использований</label><input v-model.number="promo.maxUses" class="inp" type="number" min="0" placeholder="0 — без лимита"></div>
                <div class="fld"><label class="lbl">Действует до</label><input v-model="promo.until" class="inp" type="date"></div>
              </div>
              <TgRow v-model="promo.enabled" label="Активен" />
              <div class="ft"><button v-if="promo.edit" class="btn btn-g" type="button" @click="resetPromo">Отмена</button><span class="grow"></span><button class="btn btn-p" type="button" @click="savePromo">Сохранить промокод</button></div>
            </div>
          </section>
        </div>

        <div v-if="tab === 'broadcast'" class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>Рассылка</h2><span v-if="bc.running" class="i-stt wa">идёт</span></div>
            <div class="i-pb">
              <div class="fld"><label class="lbl">Кому</label><Seg v-model="bcf.audience" :options="[['all', 'Всем'], ['active', 'С подпиской'], ['expired', 'Истёкшим'], ['never', 'Не покупали']]" /></div>
              <div class="fld"><label class="lbl">Текст</label><textarea v-model="bcf.text" class="inp" rows="6" maxlength="3500" placeholder="Можно HTML: <b>жирный</b>, <i>курсив</i>, <a href='…'>ссылка</a>"></textarea></div>
              <TgRow v-model="bcf.button" label="Кнопка «Купить / продлить» под сообщением" />
              <div class="ft"><span class="hint">Не больше ~25 сообщений в секунду — лимит Telegram.</span><button class="btn btn-p" type="button" :disabled="bc.running || !bcf.text.trim()" @click="sendBc"><Icon n="send" />Отправить</button></div>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Статус</h2></div>
            <div class="i-pb">
              <div class="i-kv"><Kr k="Отправлено" :v="bc.sent" /><Kr k="Ошибок" :v="bc.failed" /><Kr k="Всего" :v="bc.total" /></div>
              <div v-if="bc.total" class="i-mpr ok"><i :style="{ width: Math.round((bc.sent + bc.failed) / bc.total * 100) + '%' }"></i></div>
            </div>
          </section>
        </div>
      </template>
    </div>
  </div>
</template>
<script setup>
import '../styles/Shop.css'
import { ref, reactive, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import Metric from '../components/Metric.vue'
import Empty from '../components/Empty.vue'
import Tg from '../components/Tg.vue'
import TgRow from '../components/TgRow.vue'
import Seg from '../components/Seg.vue'
import Kr from '../components/Kr.vue'
import { get, put, post, del } from '../lib/api.js'
import { ok, fail, confirm } from '../lib/ui.js'
import { fmtNum, dt, date, left, PROTO } from '../lib/fmt.js'
import { STATUS } from '../lib/client.js'

const TABS = [['bot', 'Бот'], ['tariffs', 'Тарифы'], ['pay', 'Оплата'], ['orders', 'Заказы'], ['users', 'Пользователи'], ['promos', 'Промокоды'], ['broadcast', 'Рассылка']]
const METHOD = { stars: 'Stars', card: 'Карта', crypto: 'CryptoBot', manual: 'Вручную', trial: 'Пробный', promo: 'Промокод', admin: 'Админ', panel: 'Панель' }
const OST = { paid: 'оплачен', review: 'на проверке', pending: 'ожидает оплаты', rejected: 'отклонён', canceled: 'отменён' }
const d = ref(null)
const s = ref(null)
const saved = ref('')
const error = ref('')
const tab = ref('bot')
const inbounds = ref([])
const orders = ref([])
const users = ref([])
const promos = ref([])
const bc = ref({ running: false, sent: 0, failed: 0, total: 0 })
const bcf = reactive({ audience: 'all', text: '', button: true })
const ordF = ref('all')
const busy = reactive({ save: false })
const promo = reactive({ edit: false, code: '', percent: 10, days: 0, maxUses: 0, until: '', enabled: true })
const dirty = computed(() => s.value && JSON.stringify(s.value) !== saved.value)
const ordersF = computed(() => orders.value.filter(o => ordF.value === 'all' || o.status === ordF.value))
const toggleArr = (arr, v) => { const i = arr.indexOf(v); i >= 0 ? arr.splice(i, 1) : arr.push(v) }

async function load() {
  error.value = ''
  try {
    const r = await get('shop'); d.value = r
    if (!dirty.value) { const st = { ...r.settings, tariffs: r.settings.tariffs || [], inbounds: r.settings.inbounds || [] }; s.value = st; saved.value = JSON.stringify(st) }
  } catch (e) { error.value = e.message }
}
async function save() {
  busy.save = true
  try { await put('shop', s.value); saved.value = JSON.stringify(s.value); ok('Сохранено'); setTimeout(load, 1500) } catch (e) { fail(e) } finally { busy.save = false }
}
function addTariff() { s.value.tariffs.push({ id: '', name: 'Новый тариф', kind: 'vpn', days: 30, trafficGb: 0, devices: 3, price: 199, stars: 120, enabled: true }) }
function swap(i, dlt) { const t = s.value.tariffs; [t[i], t[i + dlt]] = [t[i + dlt], t[i]] }
async function loadTab(t) {
  try {
    if (t === 'orders') orders.value = await get('shop/orders')
    if (t === 'users') users.value = await get('shop/users')
    if (t === 'promos') promos.value = await get('shop/promos')
    if (t === 'broadcast') bc.value = await get('shop/broadcast')
  } catch (e) { fail(e) }
}
watch(tab, loadTab)
async function orderAct(o, a) {
  if (a === 'reject' && !await confirm('Отклонить заказ?', 'Покупатель получит сообщение, что оплата не подтверждена.', { danger: true, okText: 'Отклонить' })) return
  try { await post(`shop/orders/${o.id}/${a}`); ok(a === 'confirm' ? 'Оплата подтверждена — доступ выдан' : 'Заказ отклонён'); loadTab('orders'); load() } catch (e) { fail(e) }
}
function resetPromo() { Object.assign(promo, { edit: false, code: '', percent: 10, days: 0, maxUses: 0, until: '', enabled: true }) }
function editPromo(p) { Object.assign(promo, { edit: true, code: p.code, percent: p.percent, days: p.days, maxUses: p.maxUses, until: p.expiresAt ? new Date(p.expiresAt).toISOString().slice(0, 10) : '', enabled: p.enabled }) }
async function savePromo() {
  try {
    await post('shop/promos', { code: promo.code.trim().toUpperCase(), percent: Number(promo.percent) || 0, days: Number(promo.days) || 0, maxUses: Number(promo.maxUses) || 0, expiresAt: promo.until ? new Date(promo.until + 'T23:59:59').getTime() : 0, enabled: promo.enabled })
    ok('Промокод сохранён'); resetPromo(); loadTab('promos')
  } catch (e) { fail(e) }
}
async function delPromo(p) { if (await confirm(`Удалить промокод ${p.code}?`, '', { danger: true, okText: 'Удалить' })) { try { await del('shop/promos/' + encodeURIComponent(p.code)); loadTab('promos') } catch (e) { fail(e) } } }
async function sendBc() {
  if (!await confirm('Отправить рассылку?', 'Сообщение уйдёт выбранным пользователям бота. Отменить нельзя.', { okText: 'Отправить' })) return
  try { await post('shop/broadcast', { ...bcf }); ok('Рассылка запущена'); bcf.text = ''; loadTab('broadcast') } catch (e) { fail(e) }
}
let timer
onMounted(async () => {
  load()
  try { const r = await get('inbounds'); inbounds.value = [...r.inbounds, ...(r.nodeInbounds || [])] } catch {}
  timer = setInterval(() => { if (tab.value === 'broadcast') loadTab('broadcast'); if (!dirty.value) load() }, 5000)
})
onBeforeUnmount(() => clearInterval(timer))
</script>
