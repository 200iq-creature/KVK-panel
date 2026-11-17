import { ref, onMounted, onBeforeUnmount } from 'vue'

// load(fn) → { data, error, loading, reload }; optional polling interval (ms)
export function useLoad(fn, { interval = 0, immediate = true } = {}) {
  const data = ref(null)
  const error = ref('')
  const loading = ref(false)
  let timer = null
  let alive = true
  async function reload(silent) {
    if (!silent) loading.value = true
    try {
      const r = await fn()
      if (alive) { data.value = r; error.value = '' }
    } catch (e) {
      if (alive && (!silent || !data.value)) error.value = e.message
    } finally { if (alive) loading.value = false }
  }
  onMounted(() => {
    if (immediate) reload()
    if (interval) timer = setInterval(() => { if (!document.hidden) reload(true) }, interval)
  })
  onBeforeUnmount(() => { alive = false; clearInterval(timer) })
  return { data, error, loading, reload }
}
