import { reactive, computed, watch } from 'vue'
import { api, get, apiUrl } from './api.js'

// The server whose domains / inbounds / site / filter / WARP are being edited.
// '' — this server; otherwise the id of a linked server (calls go through
// the main panel: /api/nodes/{id}/api/...).
const saved = (() => { try { return sessionStorage.getItem('kvk-srv') || '' } catch { return '' } })()
export const srv = reactive({ id: saved, nodes: [], self: '', main: null, loaded: false })
watch(() => srv.id, (v) => { try { sessionStorage.setItem('kvk-srv', v) } catch {} })

export const spath = (p) => srv.id ? `nodes/${srv.id}/api/${p.replace(/^\//, '')}` : p
export const sget = (p, o) => api('GET', spath(p), undefined, o)
export const spost = (p, b, o) => api('POST', spath(p), b === undefined ? {} : b, o)
export const sput = (p, b, o) => api('PUT', spath(p), b, o)
export const sdel = (p, o) => api('DELETE', spath(p), undefined, o)
export const surl = (p) => apiUrl(spath(p))

export const curNode = computed(() => srv.nodes.find(n => n.id === srv.id) || null)
export const srvName = computed(() => curNode.value ? curNode.value.name : (srv.self || 'этот сервер'))
export const remote = computed(() => !!srv.id)

let pending = null
export function loadServers(force) {
  if (srv.loaded && !force) return Promise.resolve()
  if (pending) return pending
  pending = get('nodes').then((r) => {
    srv.nodes = (r.nodes || []).filter(n => n.enabled)
    srv.self = r.self?.name || ''
    srv.main = r.main || null
    if (srv.id && !srv.nodes.some(n => n.id === srv.id)) srv.id = ''
    srv.loaded = true
  }).catch(() => {}).finally(() => { pending = null })
  return pending
}
