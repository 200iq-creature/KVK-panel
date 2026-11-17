# KVK Panel

Веб-панель для Xray-core: клиенты, подписки, инбаунды (VLESS Reality / XHTTP / WS / gRPC, VMess, Trojan, Shadowsocks 2022, Hysteria2), бот-магазин, Telegram-прокси (MTProto/SOCKS), связка серверов, бэкапы, 2FA, фильтрация рекламы, WARP, миграция с 3x-ui/Remnawave, сертификаты Let's Encrypt для доменов и IP, логи. Версию Xray можно выбрать, обновить и откатить прямо из панели.

## Установка

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/200iq-creature/KVK-panel/main/install.sh)
```

Установщик:
- скачивает бинарник из GitHub Releases (если релиза нет — собирает из исходников временным Go);
- задаёт случайные порт, секретный путь, логин и пароль и выводит их в конце;
- открывает порт панели (и 80/443 для сертификатов) в ufw / firewalld / iptables;
- ставит последнюю версию Xray и geo-файлы;
- создаёт службу systemd `kvk-panel` и меню управления `kvk`.

Переменные окружения: `KVK_PORT`, `KVK_USER`, `KVK_PASS`, `KVK_PATH`, `KVK_XRAY_VERSION`, `KVK_FROM_SOURCE=1`.

Обновление: `kvk update` или кнопка в «Настройках». Удаление: `kvk uninstall`.

## Меню `kvk`

Наберите `kvk` на сервере. В меню можно посмотреть данные для входа, сбросить логин и пароль, сменить порт или путь, отключить 2FA или HTTPS, управлять службой (start, stop, restart, автозапуск), смотреть логи, обновить панель, сменить версию Xray, обновить geo-файлы, сделать бэкап или восстановить его, открыть порт, включить BBR и удалить панель.

Те же действия доступны командами: `kvk start|stop|restart|status|info|reset|update|xray [VER]|geo|backup|restore FILE|bbr|uninstall`.

## Файлы

| Путь | Что там |
|---|---|
| `/usr/local/kvk-panel/kvk-panel` | бинарник панели (веб-интерфейс встроен) |
| `/usr/local/kvk-panel/bin` | xray, geoip.dat, geosite.dat |
| `/etc/kvk-panel` | база `db.json`, сертификаты, бэкапы, логи |

## Разработка

```bash
cd frontend && npm install && npx vite build   # собирает web/dist (коммитится — встраивается через go:embed)
go build -o kvk-panel ./cmd/kvk-panel
./kvk-panel run --data ./data --bin ./bin       # затем ./kvk-panel info
```

Релиз: поднимите `VERSION`, создайте тег `vX.Y.Z` — GitHub Actions соберёт бинарники для amd64/arm64/armv7.
