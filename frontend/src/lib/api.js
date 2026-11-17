import { reactive } from 'vue'

export const BASE = (window.__BASE__ || '/').replace(/\/?$/, '/')

const ERR = {
  unauthorized: 'Сессия истекла — войдите в панель заново',
  invalid_credentials: 'Неверный логин или пароль',
  too_many_attempts: 'Слишком много попыток входа, подождите',
  csrf: 'Запрос отклонён (CSRF) — обновите страницу',
}

export class ApiError extends Error {
  constructor(status, msg, data) { super(ERR[msg] || msg || 'Ошибка ' + status); this.status = status; this.code = msg; this.data = data }
}

export const session = reactive({ authed: null, user: '', node: '', version: '' })

let onUnauthorized = null
export function setUnauthorizedHandler(fn) { onUnauthorized = fn }

export async function api(method, path, body, opts = {}) {
  const headers = { 'X-KVK': '1' }
  let payload
  if (body instanceof FormData) payload = body
  else if (body !== undefined) { headers['Content-Type'] = 'application/json'; payload = JSON.stringify(body) }
  let res
  try {
    res = await fetch(BASE + 'api/' + path.replace(/^\//, ''), { method, headers, body: payload, credentials: 'same-origin', signal: opts.signal })
  } catch (e) {
    if (e.name === 'AbortError') throw e
    throw new ApiError(0, 'Нет связи с панелью')
  }
  if (opts.raw) {
    if (!res.ok) throw new ApiError(res.status, (await res.json().catch(() => ({}))).error)
    return res
  }
  const text = await res.text()
  let data = null
  try { data = text ? JSON.parse(text) : null } catch { data = text }
  if (!res.ok) {
    const msg = data && data.error ? data.error : (typeof data === 'string' ? data.slice(0, 200) : '')
    if (res.status === 401 && msg === 'unauthorized' && !opts.noRedirect) {
      session.authed = false
      onUnauthorized && onUnauthorized()
    }
    throw new ApiError(res.status, msg, data)
  }
  return data
}

export const get = (p, o) => api('GET', p, undefined, o)
export const post = (p, b, o) => api('POST', p, b === undefined ? {} : b, o)
export const put = (p, b, o) => api('PUT', p, b, o)
export const del = (p, o) => api('DELETE', p, undefined, o)

export function apiUrl(p) { return BASE + 'api/' + p.replace(/^\//, '') }

export async function download(path, fallbackName) {
  const res = await api('GET', path, undefined, { raw: true })
  const blob = await res.blob()
  let name = fallbackName
  const cd = res.headers.get('Content-Disposition') || ''
  const m = cd.match(/filename\*?=(?:UTF-8'')?"?([^";]+)"?/i)
  if (m) name = decodeURIComponent(m[1])
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = name
  document.body.appendChild(a)
  a.click()
  setTimeout(() => { URL.revokeObjectURL(a.href); a.remove() }, 1000)
}
export const LOGO = BASE + 'logo.svg'
