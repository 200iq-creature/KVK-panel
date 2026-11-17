export function plural(n, one, few, many) {
  const a = Math.abs(n) % 100, b = a % 10
  if (a > 10 && a < 20) return many
  if (b > 1 && b < 5) return few
  if (b === 1) return one
  return many
}
export const pn = (n, one, few, many) => `${fmtNum(n)} ${plural(n, one, few, many)}`

export function fmtNum(n, d = 0) {
  if (n === null || n === undefined || isNaN(n)) return '0'
  return Number(n).toLocaleString('ru-RU', { maximumFractionDigits: d, minimumFractionDigits: 0 })
}

export function bytes(b, d) {
  b = Number(b) || 0
  const u = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ', 'ПБ']
  let i = 0
  while (Math.abs(b) >= 1024 && i < u.length - 1) { b /= 1024; i++ }
  const dd = d !== undefined ? d : (i === 0 ? 0 : b < 10 ? 2 : b < 100 ? 1 : 0)
  return `${b.toLocaleString('ru-RU', { maximumFractionDigits: dd })}\u00a0${u[i]}`
}

export function gb(b) { return (Number(b) || 0) / 1073741824 }

const MON = ['янв', 'фев', 'мар', 'апр', 'мая', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек']
const pad = (n) => String(n).padStart(2, '0')

export function dt(ms, withYear) {
  if (!ms) return '—'
  const d = new Date(ms)
  const now = new Date()
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  const sameDay = d.toDateString() === now.toDateString()
  if (sameDay) return `сегодня ${time}`
  const y = new Date(now); y.setDate(now.getDate() - 1)
  if (d.toDateString() === y.toDateString()) return `вчера ${time}`
  const yr = withYear || d.getFullYear() !== now.getFullYear() ? `.${d.getFullYear()}` : ''
  return `${pad(d.getDate())}.${pad(d.getMonth() + 1)}${yr} ${time}`
}
export function date(ms) {
  if (!ms) return '—'
  const d = new Date(ms)
  return `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${d.getFullYear()}`
}
export function dayLabel(iso) {
  const d = new Date(iso + 'T00:00:00')
  return `${d.getDate()} ${MON[d.getMonth()]}`
}
export function ago(ms) {
  if (!ms) return 'никогда'
  const s = Math.max(0, (Date.now() - ms) / 1000)
  if (s < 45) return 'только что'
  if (s < 3600) return pn(Math.round(s / 60), 'минуту', 'минуты', 'минут') + ' назад'
  if (s < 86400) return pn(Math.round(s / 3600), 'час', 'часа', 'часов') + ' назад'
  if (s < 86400 * 30) return pn(Math.round(s / 86400), 'день', 'дня', 'дней') + ' назад'
  return dt(ms, true)
}
export function left(ms) {
  if (!ms) return '∞'
  const s = (ms - Date.now()) / 1000
  if (s <= 0) return 'истёк'
  if (s < 3600) return `${Math.max(1, Math.round(s / 60))}\u00a0мин`
  if (s < 86400) return `${Math.round(s / 3600)}\u00a0ч`
  return `${Math.floor(s / 86400)}\u00a0дн`
}
export function daysLeft(ms) { return ms ? Math.ceil((ms - Date.now()) / 86400000) : Infinity }
export function uptime(sec) {
  sec = Math.floor(sec || 0)
  if (sec < 60) return `${sec}\u00a0с`
  if (sec < 3600) return `${Math.floor(sec / 60)}\u00a0мин`
  if (sec < 86400) return `${Math.floor(sec / 3600)}\u00a0ч ${Math.floor(sec % 3600 / 60)}\u00a0мин`
  return `${Math.floor(sec / 86400)}\u00a0дн ${Math.floor(sec % 86400 / 3600)}\u00a0ч`
}
export function mbit(bps) { return ((Number(bps) || 0) * 8 / 1e6) }
export function toLocalInput(ms) {
  if (!ms) return ''
  const d = new Date(ms)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
export function fromLocalInput(s) { return s ? new Date(s).getTime() : 0 }

export const flag = (cc) => cc && cc.length === 2 ? String.fromCodePoint(...[...cc.toUpperCase()].map(c => 127397 + c.charCodeAt(0))) : ''

export const PROTO = {
  vless: 'VLESS', vmess: 'VMess', trojan: 'Trojan', shadowsocks: 'Shadowsocks', hysteria2: 'Hysteria2', socks: 'SOCKS5', http: 'HTTP',
}
export const NET = { tcp: 'TCP', ws: 'WebSocket', grpc: 'gRPC', xhttp: 'XHTTP', httpupgrade: 'HTTPUpgrade', hysteria: 'QUIC', '': 'TCP' }
export const SEC = { none: 'без защиты', tls: 'TLS', reality: 'Reality' }
