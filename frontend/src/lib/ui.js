import { reactive } from 'vue'

export const toasts = reactive([])
let tid = 0
export function toast(text, kind = 'ok', ms = 3600) {
  const id = ++tid
  toasts.push({ id, text, kind })
  setTimeout(() => { const i = toasts.findIndex(t => t.id === id); if (i >= 0) toasts.splice(i, 1) }, ms)
}
export const ok = (t) => toast(t, 'ok')
export const fail = (e) => toast(e && e.message ? e.message : String(e), 'bad', 6000)

export const dialog = reactive({ open: false, title: '', text: '', danger: false, okText: 'OK', cancelText: 'Отмена', input: null, inputLabel: '', inputType: 'text', placeholder: '', resolve: null })

export function confirm(title, text = '', { danger = false, okText = 'Подтвердить', cancelText = 'Отмена' } = {}) {
  return new Promise((resolve) => Object.assign(dialog, { open: true, title, text, danger, okText, cancelText, input: null, resolve }))
}
export function prompt(title, { text = '', value = '', label = '', okText = 'Сохранить', type = 'text', placeholder = '' } = {}) {
  return new Promise((resolve) => Object.assign(dialog, { open: true, title, text, danger: false, okText, cancelText: 'Отмена', input: String(value ?? ''), inputLabel: label, inputType: type, placeholder, resolve }))
}

export async function copy(text, msg = 'Скопировано') {
  try {
    if (navigator.clipboard && window.isSecureContext) await navigator.clipboard.writeText(text)
    else {
      const ta = document.createElement('textarea')
      ta.value = text; ta.style.position = 'fixed'; ta.style.opacity = '0'
      document.body.appendChild(ta); ta.select(); document.execCommand('copy'); ta.remove()
    }
    toast(msg, 'ok', 2000)
  } catch { toast('Не удалось скопировать', 'bad') }
}

// run an async action with busy flag + toast on error
export async function act(state, key, fn, okMsg) {
  if (state[key]) return
  state[key] = true
  try {
    const r = await fn()
    if (okMsg) ok(okMsg)
    return r
  } catch (e) { fail(e); throw e } finally { state[key] = false }
}

export const theme = reactive({ mode: localStorage.getItem('kvk-theme') || 'dark' })
export function applyTheme(mode) {
  theme.mode = mode
  try { localStorage.setItem('kvk-theme', mode) } catch {}
  const dark = mode === 'dark' || mode === 'hacker' || (mode === 'system' && matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.className = ['h1', dark ? 'dark' : '', mode === 'hacker' ? 'hack' : ''].filter(Boolean).join(' ')
}
