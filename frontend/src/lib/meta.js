import { reactive } from 'vue'
import { get } from './api.js'

// shared panel metadata (node name, host, counters) for headers and nav
export const meta = reactive({ loaded: false, node: '', host: '', country: '', version: '', clients: 0, activeClients: 0, inbounds: 0, xray: {}, user: '', subPath: 'sub', tags: [], main: null, defaultFp: 'chrome', https: false })

export async function loadMeta() {
  try { Object.assign(meta, await get('meta'), { loaded: true }) } catch { /* handled by api */ }
}
