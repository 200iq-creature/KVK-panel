export const NAV = [
  { label: 'Панель', items: [
    { id: 'overview', label: 'Обзор', icon: 'overview', to: '/', desc: 'клиенты, трафик, связка, события' },
    { id: 'inb', label: 'Инбаунды', icon: 'inb', to: '/inbounds', desc: 'подключения этого и связанных серверов: протоколы, порты, транспорт' },
    { id: 'clients', label: 'Клиенты', icon: 'clients', to: '/clients', desc: 'клиенты, теги, доступ к серверам, массовые действия', count: true },
    { id: 'sub', label: 'Подписка', icon: 'sub', to: '/sub', desc: 'название, автовыбор, форматы, страница в браузере' },
    { id: 'auto', label: 'Автонастройка', icon: 'auto', to: '/autosetup', desc: 'по шагам: домены, сайты серверов, все протоколы, клиент main' },
  ] },
  { label: 'Сервер', items: [
    { id: 'link', label: 'Связка серверов', icon: 'link', to: '/link', desc: 'серверы, сохранённые конфигурации, восстановление' },
    { id: 'dom', label: 'Домены', icon: 'dom', to: '/domains', desc: 'домены и SSL-сертификаты всех серверов' },
    { id: 'flt', label: 'Фильтрация', icon: 'flt', to: '/filter', desc: 'реклама, опасные сайты, 18+' },
    { id: 'site', label: 'Сайт', icon: 'globe', to: '/site', desc: 'сайт-прикрытие: домены, страница, сертификаты' },
    { id: 'warp', label: 'WARP', icon: 'warp', to: '/warp', desc: 'Cloudflare WARP как выход сервера' },
  ] },
  { label: 'Дополнительно', items: [
    { id: 'set', label: 'Настройки', icon: 'set', to: '/settings', desc: 'тема, порты, API, Xray, Geo-базы' },
    { id: 'sec', label: 'Безопасность', icon: 'sec', to: '/security', desc: '2FA, журнал входов, HTTPS · в «Настройках»', under: 'set' },
    { id: 'bak', label: 'Бэкапы', icon: 'bak', to: '/backups', desc: 'архивы, восстановление, переезд · в «Настройках»', under: 'set' },
    { id: 'mig', label: 'Миграция', icon: 'mig', to: '/migrate', desc: 'переезд с 3x-ui и Remnawave, экспорт' },
    { id: 'logs', label: 'Логи', icon: 'logs', to: '/logs', desc: 'журнал Xray: подключения и ошибки' },
  ] },
  { label: 'Продажи', items: [
    { id: 'shop', label: 'Бот-магазин', icon: 'shop', to: '/shop', desc: 'тарифы, оплаты, промокоды, рассылка' },
    { id: 'tgp', label: 'Telegram-прокси', icon: 'tgp', to: '/tgproxy', desc: 'MTProto и SOCKS5, продажа доступов' },
  ] },
]
export const NAV_FLAT = NAV.flatMap(g => g.items)
// sidebar: sections opened from another one (Безопасность, Бэкапы — from Настройки)
export const NAV_SIDE = NAV.map(g => ({ ...g, items: g.items.filter(i => !i.under) }))
export const navOf = (id) => NAV_FLAT.find(i => i.id === id)?.under || id
