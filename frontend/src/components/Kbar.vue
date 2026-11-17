<template>
  <Teleport to="body">
    <template v-if="open">
      <button class="kveil" type="button" aria-label="Закрыть" @click="close"></button>
      <div class="card kbox" role="dialog" aria-modal="true" aria-label="Поиск">
        <div class="kin"><Icon n="search" /><input ref="inp" v-model="q" placeholder="Клиент, раздел или действие…" @keydown.down.prevent="move(1)" @keydown.up.prevent="move(-1)" @keydown.enter.prevent="run(items[sel])" @keydown.esc="close"><span class="kbd">Esc</span></div>
        <div class="klist">
          <template v-for="(it, i) in items" :key="it.key">
            <div v-if="i === 0 || items[i - 1].group !== it.group" class="kgl">{{ it.group }}</div>
            <button class="kit" :class="{ on: i === sel }" type="button" @mouseenter="sel = i" @click="run(it)">
              <span class="kic"><Icon :n="it.icon" /></span><span><b>{{ it.label }}</b><small>{{ it.desc }}</small></span>
            </button>
          </template>
          <div v-if="!items.length" class="empty"><b>Ничего не нашлось</b><span class="hint">Попробуйте имя клиента, UUID или название раздела</span></div>
        </div>
      </div>
    </template>
  </Teleport>
</template>
<script setup>
import { ref, computed, watch, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import Icon from './Icon.vue'
import { NAV_FLAT } from '../lib/nav.js'
import { get, post } from '../lib/api.js'
import { ok, fail } from '../lib/ui.js'
import { left } from '../lib/fmt.js'

const p = defineProps({ open: Boolean })
const emit = defineEmits(['update:open'])
const router = useRouter()
const q = ref('')
const sel = ref(0)
const inp = ref(null)
const clients = ref([])
const close = () => emit('update:open', false)

const ACTIONS = [
  { key: 'a-new-client', label: 'Создать клиента', desc: 'новый клиент со всеми инбаундами', icon: 'plus', go: () => router.push('/clients?new=1') },
  { key: 'a-new-inb', label: 'Новый инбаунд', desc: 'добавить подключение', icon: 'inb', go: () => router.push('/inbounds?new=1') },
  { key: 'a-backup', label: 'Создать бэкап', desc: 'архив базы и сертификатов', icon: 'bak', go: async () => { try { await post('backups'); ok('Бэкап создан') } catch (e) { fail(e) } } },
  { key: 'a-xray', label: 'Перезапустить Xray', desc: 'применить конфиг заново', icon: 'refresh', go: async () => { try { await post('xray/restart'); ok('Xray перезапущен') } catch (e) { fail(e) } } },
  { key: 'a-sync', label: 'Синхронизировать связку', desc: 'отправить клиентов на серверы связки', icon: 'link', go: async () => { try { await post('nodes/sync'); ok('Синхронизировано') } catch (e) { fail(e) } } },
]

const items = computed(() => {
  const s = q.value.trim().toLowerCase()
  const m = (t) => !s || t.toLowerCase().includes(s)
  const out = []
  clients.value.filter(c => s && (m(c.name) || m(c.uuid) || m(c.telegram || '') || m(c.subId))).slice(0, 6)
    .forEach(c => out.push({ key: 'c' + c.id, group: 'Клиенты', label: c.name, desc: `${c.status === 'active' ? 'активен' : c.status} · осталось ${left(c.expiresAt)}`, icon: 'user', go: () => router.push('/clients?open=' + c.id) }))
  NAV_FLAT.filter(n => m(n.label) || m(n.desc)).forEach(n => out.push({ key: n.id, group: 'Разделы', label: n.label, desc: n.desc, icon: n.icon, go: () => router.push(n.to) }))
  ACTIONS.filter(a => m(a.label) || m(a.desc)).forEach(a => out.push({ ...a, group: 'Действия' }))
  return out
})
function move(d) { const n = items.value.length; if (n) sel.value = (sel.value + d + n) % n }
function run(it) { if (!it) return; close(); it.go() }
watch(q, () => { sel.value = 0 })
watch(() => p.open, async (v) => {
  if (!v) return
  q.value = ''; sel.value = 0
  await nextTick(); inp.value && inp.value.focus()
  try { clients.value = await get('clients') } catch {}
})
</script>
