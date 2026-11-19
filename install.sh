#!/usr/bin/env bash
# KVK Panel installer
#   install:   bash <(curl -fsSL https://raw.githubusercontent.com/200iq-creature/KVK-panel/main/install.sh)
#   update:    curl -fsSL https://raw.githubusercontent.com/200iq-creature/KVK-panel/main/install.sh | bash -s -- update
#   uninstall: curl -fsSL https://raw.githubusercontent.com/200iq-creature/KVK-panel/main/install.sh | bash -s -- uninstall
#   branch:    curl -fsSL https://raw.githubusercontent.com/200iq-creature/KVK-panel/main/install.sh | bash -s -- update BRANCH
#              (the branch is remembered: later `kvk update` and the panel button keep using it;
#               `kvk update main` switches back)
# Environment overrides: KVK_PORT, KVK_USER, KVK_PASS, KVK_PATH, KVK_XRAY_VERSION, KVK_FROM_SOURCE=1, KVK_BRANCH
set -euo pipefail

REPO="200iq-creature/KVK-panel"
INSTALL_DIR="/usr/local/kvk-panel"
BIN="$INSTALL_DIR/kvk-panel"
XRAY_DIR="$INSTALL_DIR/bin"
DATA_DIR="/etc/kvk-panel"
BRANCH_FILE="$DATA_DIR/branch"
# Branch this copy of the script was published in. When the script is run
# straight from a branch URL without arguments, that branch is used if its
# VERSION is newer than the one of the remembered branch (after merging into
# main the versions are equal, so main wins).
SCRIPT_BRANCH="arena/3b5bfb45-kvk-panel"
# branch priority: `update BRANCH` argument > KVK_BRANCH > newer SCRIPT_BRANCH > remembered branch > main
BRANCH="${KVK_BRANCH:-}"
if [ "${1:-}" = "update" ] || [ "${1:-}" = "install" ]; then [ -n "${2:-}" ] && BRANCH="$2"; fi
SAVED_BRANCH="$(tr -d ' \n\r' 2>/dev/null <"$BRANCH_FILE" || true)"
SAVED_BRANCH="${SAVED_BRANCH:-main}"

branch_version() {
  curl -fsSL --max-time 15 -H 'Cache-Control: no-cache' "https://raw.githubusercontent.com/$REPO/$1/VERSION" 2>/dev/null | tr -d ' \n\r' || true
}
# version_gt A B — true when A > B (dotted numbers)
version_gt() { [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -1)" = "$1" ]; }

if [ -z "$BRANCH" ]; then
  BRANCH="$SAVED_BRANCH"
  if [ "$SCRIPT_BRANCH" != "$SAVED_BRANCH" ] && command -v curl >/dev/null 2>&1; then
    sv="$(branch_version "$SCRIPT_BRANCH")"
    bv="$(branch_version "$SAVED_BRANCH")"
    if [ -n "$sv" ] && version_gt "$sv" "${bv:-0}"; then BRANCH="$SCRIPT_BRANCH"; fi
  fi
fi
SERVICE="kvk-panel"
GO_VERSION="1.25.5"

red() { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
yellow() { printf '\033[33m%s\033[0m\n' "$*"; }
bold() { printf '\033[1m%s\033[0m\n' "$*"; }
die() { red "Ошибка: $*"; exit 1; }

case "$BRANCH" in
  *[!A-Za-z0-9._/-]* | "" | -*) die "недопустимое имя ветки: $BRANCH" ;;
esac

[ "$(id -u)" -eq 0 ] || die "запустите от root (sudo -i)"
[ "$(uname -s)" = "Linux" ] || die "поддерживается только Linux"
command -v systemctl >/dev/null 2>&1 || die "нужен systemd"

case "$(uname -m)" in
  x86_64 | amd64) ARCH=amd64 ;;
  aarch64 | arm64) ARCH=arm64 ;;
  armv7l | armv7) ARCH=armv7 ;;
  *) die "архитектура $(uname -m) не поддерживается" ;;
esac

install_deps() {
  local need=()
  for c in curl tar; do command -v "$c" >/dev/null 2>&1 || need+=("$c"); done
  command -v ss >/dev/null 2>&1 || need+=("iproute2")
  [ -f /etc/ssl/certs/ca-certificates.crt ] || [ -f /etc/pki/tls/certs/ca-bundle.crt ] || need+=("ca-certificates")
  [ ${#need[@]} -eq 0 ] && return 0
  yellow "Устанавливаем: ${need[*]}"
  if command -v apt-get >/dev/null 2>&1; then
    DEBIAN_FRONTEND=noninteractive apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "${need[@]}" >/dev/null
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y -q "${need[@]/iproute2/iproute}"
  elif command -v yum >/dev/null 2>&1; then
    yum install -y -q "${need[@]/iproute2/iproute}"
  elif command -v apk >/dev/null 2>&1; then
    apk add --no-cache "${need[@]}" bash
  elif command -v pacman >/dev/null 2>&1; then
    pacman -Sy --noconfirm "${need[@]}"
  else
    die "не удалось установить ${need[*]} — поставьте вручную"
  fi
}

latest_tag() {
  curl -fsSL --max-time 20 "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null |
    grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/' || true
}

# download_release DEST — prebuilt binary from GitHub Releases
download_release() {
  local tag tmp
  tag="$(latest_tag)"
  [ -n "$tag" ] || return 1
  tmp="$(mktemp -d)"
  if ! curl -fL --progress-bar --max-time 300 -o "$tmp/kvk.tar.gz" \
    "https://github.com/$REPO/releases/download/$tag/kvk-panel-linux-$ARCH.tar.gz"; then
    rm -rf "$tmp"
    return 1
  fi
  tar -xzf "$tmp/kvk.tar.gz" -C "$tmp" || { rm -rf "$tmp"; return 1; }
  [ -f "$tmp/kvk-panel" ] || { rm -rf "$tmp"; return 1; }
  install -m 0755 "$tmp/kvk-panel" "$1"
  rm -rf "$tmp"
  green "Скачан релиз $tag"
}

# build_from_source DEST — compile with a temporary Go toolchain (web UI is prebuilt in the repo)
build_from_source() {
  local tmp goarch="$ARCH" goarm=""
  [ "$ARCH" = "armv7" ] && goarch=arm && goarm=7
  tmp="$(mktemp -d)"
  yellow "Собираем из исходников ветки $BRANCH (1–3 минуты)…"
  local gotar="go${GO_VERSION}.linux-${goarch/arm/armv6l}.tar.gz"
  [ "$goarch" = "arm64" ] && gotar="go${GO_VERSION}.linux-arm64.tar.gz"
  curl -fL --progress-bar --max-time 600 -o "$tmp/go.tgz" "https://go.dev/dl/$gotar" ||
    curl -fL --progress-bar --max-time 600 -o "$tmp/go.tgz" "https://dl.google.com/go/$gotar" ||
    die "не удалось скачать Go"
  tar -xzf "$tmp/go.tgz" -C "$tmp"
  curl -fL --progress-bar --max-time 300 -o "$tmp/src.tgz" "https://codeload.github.com/$REPO/tar.gz/refs/heads/$BRANCH" ||
    die "не удалось скачать исходники ветки $BRANCH (есть ли такая ветка?)"
  mkdir -p "$tmp/src"
  tar -xzf "$tmp/src.tgz" -C "$tmp/src" --strip-components=1
  local ver
  ver="$(tr -d ' \n\r' <"$tmp/src/VERSION" 2>/dev/null || echo dev)"
  (
    cd "$tmp/src"
    export PATH="$tmp/go/bin:$PATH" GOPATH="$tmp/gopath" GOCACHE="$tmp/gocache" GOFLAGS=-mod=mod CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" GOARM="$goarm"
    go build -trimpath -ldflags "-s -w -X github.com/200iq-creature/KVK-panel/internal/app.Version=$ver -X github.com/200iq-creature/KVK-panel/internal/app.Branch=$BRANCH" -o "$tmp/kvk-panel" ./cmd/kvk-panel
  ) || die "сборка не удалась"
  install -m 0755 "$tmp/kvk-panel" "$1"
  rm -rf "$tmp"
  green "Собрана версия $ver (ветка $BRANCH)"
}

# remote_version — VERSION file of the selected branch
remote_version() { branch_version "$BRANCH"; }

fetch_binary() {
  local dest="$1" tag want
  # releases are cut from main only; other branches are always built from source
  if [ "${KVK_FROM_SOURCE:-0}" != "1" ] && [ "$BRANCH" = "main" ]; then
    tag="$(latest_tag)"
    want="$(remote_version)"
    # use the release only when it is not older than the code in main
    if [ -n "$tag" ] && { [ -z "$want" ] || [ "${tag#v}" = "$want" ]; } && download_release "$dest"; then
      return 0
    fi
  fi
  build_from_source "$dest"
}

save_branch() {
  mkdir -p "$DATA_DIR"
  echo "$BRANCH" >"$BRANCH_FILE"
}

random_port() {
  local p
  for _ in $(seq 1 50); do
    p=$((RANDOM % 50000 + 10000))
    if ! ss -Hltn 2>/dev/null | awk '{print $4}' | grep -qE "[:.]$p\$"; then
      echo "$p"
      return
    fi
  done
  echo 2053
}

random_str() { tr -dc 'A-Za-z0-9' </dev/urandom 2>/dev/null | head -c "$1" || true; }

open_port() {
  local port="$1" proto="${2:-tcp}"
  if command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active"; then
    ufw allow "$port/$proto" >/dev/null 2>&1 && echo "  ufw: открыт $port/$proto"
  fi
  if command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
    firewall-cmd --permanent --add-port="$port/$proto" >/dev/null 2>&1
    firewall-cmd --reload >/dev/null 2>&1 && echo "  firewalld: открыт $port/$proto"
  fi
  if command -v iptables >/dev/null 2>&1 && ! command -v ufw >/dev/null 2>&1 && ! command -v firewall-cmd >/dev/null 2>&1; then
    # plain iptables with a restrictive INPUT policy
    if iptables -S INPUT 2>/dev/null | grep -qE '^-P INPUT (DROP|REJECT)'; then
      iptables -C INPUT -p "$proto" --dport "$port" -j ACCEPT 2>/dev/null ||
        iptables -I INPUT -p "$proto" --dport "$port" -j ACCEPT && echo "  iptables: открыт $port/$proto"
    fi
  fi
}

write_service() {
  cat >/etc/systemd/system/$SERVICE.service <<EOF
[Unit]
Description=KVK Panel (Xray)
Documentation=https://github.com/$REPO
After=network-online.target nss-lookup.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=$BIN run --data $DATA_DIR --bin $XRAY_DIR
Restart=on-failure
RestartSec=5
LimitNOFILE=1048576
Environment=KVK_DATA=$DATA_DIR KVK_BIN=$XRAY_DIR

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
}

write_menu() {
  cat >/usr/local/bin/kvk <<'MENU'
#!/usr/bin/env bash
# KVK Panel management menu
BIN=/usr/local/kvk-panel/kvk-panel
DATA=/etc/kvk-panel
XBIN=/usr/local/kvk-panel/bin
SVC=kvk-panel
REPO=200iq-creature/KVK-panel
P() { "$BIN" "$@" --data "$DATA" --bin "$XBIN"; }
red() { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
yellow() { printf '\033[33m%s\033[0m\n' "$*"; }
[ "$(id -u)" -eq 0 ] || { red "Запустите от root"; exit 1; }

status_line() {
  echo "KVK Panel v$("$BIN" version 2>/dev/null || echo '?') · ветка $(cat "$DATA/branch" 2>/dev/null || echo main)"
  if systemctl is-active --quiet $SVC; then green "Панель: работает"; else red "Панель: остановлена"; fi
  if pgrep -f "$XBIN/xray" >/dev/null 2>&1; then green "Xray:   работает ($("$XBIN/xray" version 2>/dev/null | head -1 | awk '{print $2}'))"; else yellow "Xray:   не запущен"; fi
  if systemctl is-enabled --quiet $SVC 2>/dev/null; then echo "Автозапуск: включён"; else echo "Автозапуск: выключен"; fi
}

# offline changes: stop the service so it does not overwrite db.json, then start it again
offline() {
  local was=0
  systemctl is-active --quiet $SVC && was=1 && systemctl stop $SVC
  P "$@"
  local rc=$?
  [ $was -eq 1 ] && systemctl start $SVC
  return $rc
}

open_port() {
  local port="$1" proto="${2:-tcp}"
  if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then ufw allow "$port/$proto"; fi
  if command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
    firewall-cmd --permanent --add-port="$port/$proto" && firewall-cmd --reload
  fi
}

enable_bbr() {
  if sysctl net.ipv4.tcp_congestion_control 2>/dev/null | grep -q bbr; then green "BBR уже включён"; return; fi
  modprobe tcp_bbr 2>/dev/null
  grep -q '^net.core.default_qdisc' /etc/sysctl.conf || echo 'net.core.default_qdisc=fq' >>/etc/sysctl.conf
  grep -q '^net.ipv4.tcp_congestion_control' /etc/sysctl.conf || echo 'net.ipv4.tcp_congestion_control=bbr' >>/etc/sysctl.conf
  sysctl -p >/dev/null 2>&1
  if sysctl net.ipv4.tcp_congestion_control | grep -q bbr; then green "BBR включён"; else red "Ядро не поддерживает BBR"; fi
}

# kvk update [BRANCH] — without an argument the remembered branch is used
update_panel() {
  local b="${1:-$(tr -d ' \n\r' 2>/dev/null <"$DATA/branch")}"
  b="${b:-main}"
  echo "Ветка: $b"
  curl -fsSL "https://raw.githubusercontent.com/$REPO/$b/install.sh" -o /tmp/kvk-install.sh || { red "Не удалось скачать install.sh ветки $b"; return 1; }
  bash /tmp/kvk-install.sh update "$b"
}

uninstall() {
  read -rp "Удалить панель, Xray и все данные (клиенты, настройки)? Введите yes: " a
  [ "$a" = "yes" ] || { echo "Отменено"; return; }
  read -rp "Сохранить бэкап в /root перед удалением? [Y/n]: " b
  if [ "${b:-y}" != "n" ]; then f=$(P backup 2>/dev/null) && cp "$f" /root/ && green "Бэкап: /root/$(basename "$f")"; fi
  systemctl disable --now $SVC 2>/dev/null
  rm -f /etc/systemd/system/$SVC.service
  systemctl daemon-reload
  rm -rf /usr/local/kvk-panel "$DATA" /usr/local/bin/kvk-panel
  green "Панель удалена"
  rm -f /usr/local/bin/kvk
  exit 0
}

xray_menu() {
  echo "Текущая версия: $("$XBIN/xray" version 2>/dev/null | head -1 | awk '{print $2}')"
  echo "Последние версии Xray:"
  curl -fsSL --max-time 15 "https://api.github.com/repos/XTLS/Xray-core/releases?per_page=10" 2>/dev/null |
    grep '"tag_name"' | sed -E 's/.*"(v[^"]+)".*/  \1/'
  read -rp "Версия (например v25.10.15, пусто — последняя): " v
  offline xray-install "${v:-latest}" && green "Готово"
}

cmd_menu() {
  clear
  echo "==================== KVK Panel ===================="
  echo "  версия $(P version 2>/dev/null)"
  status_line
  cat <<EOF
---------------------------------------------------
  1. Данные для входа (адрес, порт, путь, логин)
  2. Сбросить логин и пароль
  3. Сменить логин
  4. Сменить пароль
  5. Сменить порт панели
  6. Сменить секретный путь
  7. Отключить 2FA
  8. Выключить HTTPS панели
---------------------------------------------------
  9. Запустить        10. Остановить
 11. Перезапустить    12. Статус службы
 13. Логи панели      14. Логи Xray (ошибки)
 15. Автозапуск вкл   16. Автозапуск выкл
---------------------------------------------------
 17. Обновить панель
 18. Установить / сменить версию Xray
 19. Обновить geo-файлы
 20. Создать бэкап    21. Восстановить бэкап
 22. Перевыпустить API-токен
 23. Открыть порт в firewall
 24. Включить BBR
 25. Ключ связки серверов (API-токен)
 26. Удалить панель
  0. Выход
===================================================
EOF
  read -rp "Выберите пункт: " n
  case "$n" in
    1) P info ;;
    2) offline reset-creds ;;
    3) read -rp "Новый логин: " u; offline set-user "$u" ;;
    4) read -rsp "Новый пароль: " p; echo; offline set-pass "$p" ;;
    5) read -rp "Новый порт (пусто — случайный): " p
       [ -z "$p" ] && p=$((RANDOM % 50000 + 10000))
       offline set-port "$p" && open_port "$p" && P info ;;
    6) read -rp "Новый путь (пусто — случайный): " p; offline set-path $p && P info ;;
    7) offline 2fa-off ;;
    8) offline https-off && P info ;;
    9) systemctl start $SVC && green "Запущена" ;;
    10) systemctl stop $SVC && yellow "Остановлена (Xray тоже остановлен)" ;;
    11) systemctl restart $SVC && green "Перезапущена" ;;
    12) systemctl status $SVC --no-pager ;;
    13) journalctl -u $SVC -n 100 --no-pager ;;
    14) tail -n 100 "$DATA/logs/error.log" 2>/dev/null || echo "пусто" ;;
    15) systemctl enable $SVC && green "Автозапуск включён" ;;
    16) systemctl disable $SVC && yellow "Автозапуск выключен" ;;
    17) update_panel ;;
    18) xray_menu ;;
    19) read -rp "Источник [loyalsoldier/runetfreedom/v2fly, пусто — текущий]: " s; offline geo-update $s ;;
    20) P backup ;;
    21) read -rp "Путь к .zip: " f; offline restore "$f" ;;
    22) offline reset-token ;;
    23) read -rp "Порт: " p; read -rp "Протокол [tcp/udp]: " pr; open_port "$p" "${pr:-tcp}" ;;
    24) enable_bbr ;;
    25) P apikey ;;
    26) uninstall ;;
    0) exit 0 ;;
    *) red "Нет такого пункта" ;;
  esac
  echo
  read -rp "Enter — вернуться в меню…" _
  cmd_menu
}

case "${1:-}" in
  "") cmd_menu ;;
  start | stop | restart | status) systemctl "$1" $SVC ;;
  enable | disable) systemctl "$1" $SVC ;;
  log | logs) journalctl -u $SVC -n 200 --no-pager -f ;;
  info) P info ;;
  update) shift; update_panel "${1:-}" ;;
  version) "$BIN" version ;;
  uninstall) uninstall ;;
  xray) shift; offline xray-install "${1:-latest}" ;;
  geo) shift; offline geo-update "$@" ;;
  backup) P backup ;;
  restore) shift; offline restore "$1" ;;
  reset) offline reset-creds ;;
  apikey | key | api) P apikey ;;
  bbr) enable_bbr ;;
  *) cat <<EOF
Использование: kvk [команда]
  (без команды)   интерактивное меню
  start|stop|restart|status|enable|disable
  log             журнал панели
  info            адрес и логин
  reset           сбросить логин и пароль
  apikey          ключ связки серверов и API-токен
  update [ВЕТКА]  обновить панель (ветка запоминается, по умолчанию main)
  version         версия панели
  xray [VER]      установить версию Xray
  geo [SRC]       обновить geo-файлы
  backup          создать бэкап
  restore FILE    восстановить бэкап
  bbr             включить BBR
  uninstall       удалить панель
EOF
  ;;
esac
MENU
  chmod +x /usr/local/bin/kvk
  ln -sf "$BIN" /usr/local/bin/kvk-panel
}

P() { "$BIN" "$@" --data "$DATA_DIR" --bin "$XRAY_DIR"; }

do_install() {
  if [ -f "$DATA_DIR/db.json" ] && [ -x "$BIN" ]; then
    yellow "Панель уже установлена — выполняем обновление."
    do_update
    return
  fi
  bold "Установка KVK Panel"
  install_deps
  mkdir -p "$INSTALL_DIR" "$XRAY_DIR" "$DATA_DIR"
  chmod 700 "$DATA_DIR"
  fetch_binary "$BIN"
  save_branch

  local port user pass path
  port="${KVK_PORT:-$(random_port)}"
  user="${KVK_USER:-admin$(random_str 4)}"
  pass="${KVK_PASS:-$(random_str 16)}"
  path="${KVK_PATH:-$(random_str 14)}"
  P set-port "$port" >/dev/null
  P set-path "$path" >/dev/null
  P reset-creds "$user" "$pass" >/dev/null

  bold "Устанавливаем Xray…"
  P xray-install "${KVK_XRAY_VERSION:-latest}" || yellow "Xray не скачался — установите его позже в «Настройках» панели или командой kvk xray"
  bold "Скачиваем geo-файлы…"
  P geo-update >/dev/null 2>&1 || yellow "geo-файлы не скачались — обновите их позже в «Настройках»"

  write_service
  write_menu
  systemctl enable --now $SERVICE >/dev/null 2>&1 || true

  bold "Открываем порты в firewall…"
  open_port "$port" tcp
  open_port 80 tcp
  open_port 443 tcp

  sleep 2
  systemctl is-active --quiet $SERVICE || { red "Служба не запустилась:"; journalctl -u $SERVICE -n 30 --no-pager; exit 1; }

  local url
  url="$(P info-json 2>/dev/null | grep '"url"' | sed -E 's/.*"url": *"([^"]+)".*/\1/' || true)"
  echo
  green "================ KVK Panel установлена ================"
  echo "  Адрес:   $url"
  echo "  Логин:   $user"
  echo "  Пароль:  $pass"
  echo "-------------------------------------------------------"
  echo "  Меню управления:  kvk"
  echo "  Сохраните эти данные — пароль больше не будет показан"
  echo "  (сбросить можно командой: kvk reset)"
  green "======================================================="
  if command -v ufw >/dev/null 2>&1 && ! ufw status 2>/dev/null | grep -q "Status: active"; then
    echo "Firewall ufw выключен — порт $port и так доступен."
  fi
  yellow "Если у провайдера есть внешний firewall (security group) — откройте в нём порт $port/tcp."
}

do_update() {
  [ -x "$BIN" ] || die "панель не установлена — запустите установку без аргументов"
  bold "Обновление KVK Panel"
  install_deps
  local old new tmp
  old="$("$BIN" version 2>/dev/null || echo '?')"
  P backup >/dev/null 2>&1 && echo "Бэкап перед обновлением создан" || true
  tmp="$(mktemp)"
  echo "Текущая версия: $old · ветка: $BRANCH"
  fetch_binary "$tmp"
  new="$("$tmp" version 2>/dev/null || echo '?')"
  save_branch
  systemctl stop $SERVICE 2>/dev/null || true
  cp -f "$BIN" "$BIN.prev" 2>/dev/null || true
  install -m 0755 "$tmp" "$BIN"
  rm -f "$tmp"
  write_service
  write_menu
  systemctl enable $SERVICE >/dev/null 2>&1
  systemctl start $SERVICE
  sleep 2
  if ! systemctl is-active --quiet $SERVICE; then
    red "Новая версия не запустилась — откатываемся на $old"
    install -m 0755 "$BIN.prev" "$BIN"
    systemctl start $SERVICE
    exit 1
  fi
  if [ "$old" = "$new" ]; then
    yellow "Версия не изменилась: $new (ветка $BRANCH) — в ветке нет новых изменений"
  else
    green "Готово: $old → $new (ветка $BRANCH)"
  fi
  P info
}

do_uninstall() {
  systemctl disable --now $SERVICE 2>/dev/null || true
  rm -f /etc/systemd/system/$SERVICE.service
  systemctl daemon-reload
  rm -rf "$INSTALL_DIR" /usr/local/bin/kvk /usr/local/bin/kvk-panel
  if [ "${KVK_KEEP_DATA:-0}" = "1" ]; then
    yellow "Данные сохранены в $DATA_DIR"
  else
    rm -rf "$DATA_DIR"
  fi
  green "KVK Panel удалена"
}

case "${1:-install}" in
  install) do_install ;;
  update) do_update ;;
  uninstall) do_uninstall ;;
  *) die "неизвестная команда: $1 (install | update | uninstall)" ;;
esac
