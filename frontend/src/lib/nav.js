export const NAV = [
  { label: 'Панель', items: [
    { id: 'overview', label: 'Обзор', icon: 'overview', to: '/', desc: 'клиенты, трафик, связка, события' },
    { id: 'clients', label: 'Клиенты', icon: 'clients', to: '/clients', desc: 'клиенты, теги, массовые действия', count: true },
    { id: 'sub', label: 'Подписка', icon: 'sub', to: '/sub', desc: 'название, автовыбор, внешние ссылки, страница в браузере' },
  ] },
  { label: 'Продажи', items: [
    { id: 'shop', label: 'Бот-магазин', icon: 'shop', to: '/shop', desc: 'тарифы, оплаты, промокоды, рассылка' },
    { id: 'tgp', label: 'Telegram-прокси', icon: 'tgp', to: '/tgproxy', desc: 'MTProto и SOCKS5, продажа доступов' },
  ] },
  { label: 'Сервер', items: [
    { id: 'link', label: 'Связка серверов', icon: 'link', to: '/link', desc: 'одна подписка на несколько серверов' },
    { id: 'bak', label: 'Бэкапы', icon: 'bak', to: '/backups', desc: 'архивы, восстановление, переезд' },
    { id: 'sec', label: 'Безопасность', icon: 'sec', to: '/security', desc: '2FA, журнал входов, HTTPS' },
    { id: 'set', label: 'Настройки', icon: 'set', to: '/settings', desc: 'тема, порты, API, Xray, Geo-базы' },
  ] },
  { label: 'Для опытных', items: [
    { id: 'inb', label: 'Инбаунды', icon: 'inb', to: '/inbounds', desc: 'подключения: протоколы, порты, транспорт' },
    { id: 'flt', label: 'Фильтрация', icon: 'flt', to: '/filter', desc: 'реклама, опасные сайты, 18+' },
    { id: 'warp', label: 'WARP', icon: 'warp', to: '/warp', desc: 'Cloudflare WARP как выход сервера' },
    { id: 'mig', label: 'Миграция', icon: 'mig', to: '/migrate', desc: 'переезд с 3x-ui и Remnawave, экспорт' },
    { id: 'dom', label: 'Домены', icon: 'dom', to: '/domains', desc: 'домены и SSL-сертификаты для панели и подписки' },
    { id: 'logs', label: 'Логи', icon: 'logs', to: '/logs', desc: 'журнал Xray: подключения и ошибки' },
  ] },
]
export const NAV_FLAT = NAV.flatMap(g => g.items)
