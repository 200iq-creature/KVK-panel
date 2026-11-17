<template>
  <div class="pg-clients">
    <div class="pagein i-det">
      <div class="i-hd">
        <div><h1>Клиенты</h1><p>{{ meta.node }} · {{ pn(list.length, 'клиент', 'клиента', 'клиентов') }} · {{ counts.online }} онлайн</p></div>
        <div class="i-hd-a">
          <span class="ae-mw">
            <button class="btn" type="button" @click="ioMenu = !ioMenu"><Icon n="file" />Импорт и экспорт<Icon n="chevD" /></button>
            <template v-if="ioMenu">
              <button class="i-veil" type="button" aria-label="Закрыть" @click="ioMenu = false"></button>
              <div class="card i-menu" role="menu" style="position:absolute;top:calc(100% + 6px);right:0;z-index:35;padding:6px;display:flex;flex-direction:column">
                <button type="button" @click="exp('json')"><Icon n="download" />Экспорт JSON</button>
                <button type="button" @click="exp('csv')"><Icon n="download" />Экспорт CSV (Excel)</button>
                <button type="button" @click="ioMenu = false; imp.open = true"><Icon n="upload" />Импорт JSON / CSV</button>
              </div>
            </template>
          </span>
          <button class="btn btn-p" type="button" @click="openNew"><Icon n="plus" />Новый клиент</button>
        </div>
      </div>
      <Loading v-if="!loaded" :error="error" @retry="reload" />
      <section v-else class="i-pn">
        <div class="ae-tb">
          <label class="i-ls"><Icon n="search" /><input v-model="q" placeholder="Имя, uuid, Telegram, IP, заметка"></label>
          <div class="i-chf">
            <button v-for="fl in FILTERS" :key="fl[0]" type="button" :class="{ on: filter === fl[0] }" @click="filter = fl[0]">{{ fl[1] }}<i v-if="counts[fl[0]]"> {{ counts[fl[0]] }}</i></button>
          </div>
        </div>
        <div v-if="allTags.length" class="ae-tb2">
          <span class="ae-lab">Теги</span>
          <div class="i-chf"><button v-for="t in allTags" :key="t" type="button" :class="{ on: tagF === t }" @click="tagF = tagF === t ? '' : t">{{ t }}</button></div>
        </div>
        <div v-if="sel.length" class="ae-bulk">
          <b>Выбрано: {{ sel.length }}</b>
          <button class="btn btn-sm" type="button" @click="bulk('extend')">Продлить</button>
          <button class="btn btn-sm" type="button" @click="bulk('reset-traffic')">Сбросить трафик</button>
          <button class="btn btn-sm" type="button" @click="bulk('ban')">Забанить</button>
          <button class="btn btn-sm" type="button" @click="bulk('unban')">Разбанить</button>
          <button class="btn btn-sm" type="button" @click="bulk('freeze')">Заморозить</button>
          <button class="btn btn-sm" type="button" @click="bulk('unfreeze')">Разморозить</button>
          <button class="btn btn-sm" type="button" @click="bulk('add-tag')">+ тег</button>
          <button class="btn btn-sm" type="button" @click="bulk('remove-tag')">− тег</button>
          <button class="btn btn-sm" type="button" @click="bulk('grant-all')">Выдать все инбаунды</button>
          <button class="btn btn-sm btn-danger-g btn-g" type="button" @click="bulk('delete')"><Icon n="trash" />Удалить</button>
          <button class="btn btn-sm btn-g" type="button" @click="sel = []">Снять выбор</button>
        </div>
        <div class="ae-vl">
          <div class="ae-vl-in">
            <div class="grid-h ae-vh" style="padding:10px 20px;font-size:var(--fs-sm);color:var(--muted);font-weight:600">
              <button class="chk" :class="{ on: pageItems.length && pageItems.every(c => sel.includes(c.id)) }" type="button" aria-label="Выбрать все" @click="toggleAll"><Icon n="check" /></button>
              <span>Клиент</span><span>Статус</span><span>Осталось</span><span>Трафик</span><span>Устройства</span><span></span>
            </div>
            <div v-for="c in pageItems" :key="c.id" class="row">
              <div class="grid-h" @click="open = c.id">
                <button class="chk" :class="{ on: sel.includes(c.id) }" type="button" aria-label="Выбрать" @click.stop="toggle(c.id)"><Icon n="check" /></button>
                <span class="ae-cl"><span class="ae-cl-n"><b>{{ c.name }}</b><span v-for="t in c.tags.slice(0, 2)" :key="t" class="badge b-mute" style="height:20px;font-size:11px">{{ t }}</span></span><span class="ae-sub">{{ c.telegram || 'без Telegram' }}{{ c.note ? ' · ' + c.note : '' }}</span></span>
                <span class="ae-stt"><span class="i-stt" :class="STATUS[c.status]?.[1]">{{ STATUS[c.status]?.[0] || c.status }}</span><span v-if="c.online" class="badge b-ok ae-on"><span class="dot"></span>онлайн</span></span>
                <span class="small" :class="{ 'wa-t': c.expiresAt && daysLeft(c.expiresAt) <= 3 && daysLeft(c.expiresAt) > 0 }">{{ left(c.expiresAt) }}</span>
                <span class="ae-trf"><span class="small">{{ bytes(c.up + c.down) }} · {{ c.trafficLimit ? bytes(c.trafficLimit) : 'безлимит' }}</span><span v-if="c.trafficLimit" class="i-mpr" :class="tone(c)"><i :style="{ width: usage(c) + '%' }"></i></span></span>
                <span class="small">{{ c.devices }} / {{ c.deviceLimit || '∞' }}</span>
                <button class="btn btn-sm btn-g btn-ic" type="button" title="Скопировать ссылку подписки" @click.stop="copy(c.subUrl, 'Ссылка подписки скопирована')"><Icon n="copy" /></button>
              </div>
            </div>
            <Empty v-if="!filtered.length" icon="clients" :title="list.length ? 'Никого не нашли' : 'Клиентов пока нет'" :text="list.length ? 'Измените поиск или фильтр' : 'Создайте первого — у него будет своя ссылка подписки'">
              <button v-if="!list.length" class="btn btn-sm" type="button" @click="openNew"><Icon n="plus" />Новый клиент</button>
            </Empty>
          </div>
        </div>
        <div v-if="filtered.length" class="ae-pager">
          <span>{{ (page - 1) * PER + 1 }}–{{ Math.min(page * PER, filtered.length) }} из {{ filtered.length }}</span>
          <div class="ae-acts"><span class="ae-sub">стр. {{ page }} из {{ pages }}</span><button class="btn btn-sm" type="button" :disabled="page <= 1" @click="page--">Назад</button><button class="btn btn-sm" type="button" :disabled="page >= pages" @click="page++">Вперёд</button></div>
        </div>
      </section>
    </div>

    <Drawer :open="creating" title="Новый клиент" eyebrow="клиенты" @close="creating = false">
      <ClientForm v-if="creating" ref="form" :inbounds="inbounds" :all-tags="allTags" />
      <template #foot>
        <button class="btn" type="button" @click="creating = false">Отмена</button>
        <button class="btn btn-p" type="button" :disabled="busy.create" @click="create"><span v-if="busy.create" class="spin"></span>Создать</button>
      </template>
    </Drawer>

    <Drawer :open="imp.open" title="Импорт клиентов" eyebrow="клиенты" @close="imp.open = false">
      <p class="note">JSON — как в экспорте этой панели. CSV — с заголовком: <span class="i-mono">name,uuid,subId,expires,trafficLimitGB,deviceLimit,telegram,tags,banned,note</span> (дата — <span class="i-mono">ГГГГ-ММ-ДД [ЧЧ:ММ]</span> МСК, теги через «;»). Клиенты с уже существующим UUID или именем пропускаются.</p>
      <input ref="file" type="file" accept=".json,.csv,text/csv,application/json" hidden @change="readFile">
      <button class="btn" type="button" @click="file.click()"><Icon n="upload" />Выбрать файл</button>
      <textarea v-model="imp.data" class="inp mono" rows="12" placeholder="или вставьте содержимое сюда"></textarea>
      <template #foot>
        <button class="btn" type="button" @click="imp.open = false">Отмена</button>
        <button class="btn btn-p" type="button" :disabled="busy.imp || !imp.data.trim()" @click="doImport"><span v-if="busy.imp" class="spin"></span>Импортировать</button>
      </template>
    </Drawer>

    <ClientDrawer :id="open" :inbounds="inbounds" :all-tags="allTags" @close="closeClient" @changed="reload(true)" />
  </div>
</template>
<script setup>
import '../styles/Clients.css'
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import Empty from '../components/Empty.vue'
import Drawer from '../components/Drawer.vue'
import ClientForm from '../components/ClientForm.vue'
import ClientDrawer from '../components/ClientDrawer.vue'
import { get, post, download } from '../lib/api.js'
import { meta, loadMeta } from '../lib/meta.js'
import { useLoad } from '../lib/useLoad.js'
import { copy, ok, fail, confirm, prompt } from '../lib/ui.js'
import { bytes, left, pn, daysLeft } from '../lib/fmt.js'
import { STATUS } from '../lib/client.js'

const route = useRoute()
const router = useRouter()
const PER = 50
const FILTERS = [['all', 'Все'], ['active', 'Активные'], ['soon', '≤ 3 дней'], ['expired', 'Истёк'], ['limited', 'Лимит'], ['banned', 'Бан'], ['frozen', 'Заморожен'], ['online', 'Онлайн']]
const { data, error, reload } = useLoad(() => get('clients'), { interval: 10000 })
const inbounds = ref([])
const list = computed(() => data.value || [])
const loaded = computed(() => data.value !== null)
const q = ref('')
const filter = ref(typeof route.query.f === 'string' ? route.query.f : 'all')
const tagF = ref('')
const page = ref(1)
const sel = ref([])
const open = ref(typeof route.query.open === 'string' ? route.query.open : '')
const creating = ref(false)
const ioMenu = ref(false)
const form = ref(null)
const file = ref(null)
const imp = reactive({ open: false, data: '' })
const busy = reactive({ create: false, imp: false })

const allTags = computed(() => [...new Set(list.value.flatMap(c => c.tags || []))].sort())
const match = (c, f) => {
  switch (f) {
    case 'all': return true
    case 'soon': return c.status === 'active' && c.expiresAt && daysLeft(c.expiresAt) <= 3
    case 'online': return c.online
    default: return c.status === f
  }
}
const counts = computed(() => Object.fromEntries(FILTERS.map(f => [f[0], list.value.filter(c => match(c, f[0])).length])))
const filtered = computed(() => {
  const s = q.value.trim().toLowerCase()
  return list.value.filter(c => match(c, filter.value) && (!tagF.value || (c.tags || []).includes(tagF.value)) &&
    (!s || [c.name, c.uuid, c.telegram, c.note, c.subId, ...(c.ips || [])].some(x => (x || '').toLowerCase().includes(s))))
})
const pages = computed(() => Math.max(1, Math.ceil(filtered.value.length / PER)))
const pageItems = computed(() => filtered.value.slice((page.value - 1) * PER, page.value * PER))
watch([q, filter, tagF], () => { page.value = 1 })
watch(pages, (n) => { if (page.value > n) page.value = n })
const usage = (c) => c.trafficLimit ? Math.min(100, Math.round((c.up + c.down) / c.trafficLimit * 100)) : 0
const tone = (c) => usage(c) > 90 ? 'bad' : usage(c) > 70 ? 'wa' : 'ok'
const toggle = (id) => { sel.value = sel.value.includes(id) ? sel.value.filter(x => x !== id) : [...sel.value, id] }
function toggleAll() {
  const ids = pageItems.value.map(c => c.id)
  sel.value = ids.every(id => sel.value.includes(id)) ? sel.value.filter(id => !ids.includes(id)) : [...new Set([...sel.value, ...ids])]
}
async function loadInbounds() { try { inbounds.value = (await get('inbounds')).inbounds } catch {} }
function openNew() { creating.value = true; loadInbounds() }
function closeClient() { open.value = ''; if (route.query.open) router.replace({ query: {} }) }
async function create() {
  busy.create = true
  try {
    const r = await post('clients', form.value.body())
    ok(`Клиент ${r.name || ''} создан`)
    creating.value = false
    await reload(true); loadMeta()
    if (r.id) open.value = r.id
  } catch (e) { fail(e) } finally { busy.create = false }
}
async function bulk(action) {
  const n = sel.value.length
  const body = { ids: sel.value, action }
  if (action === 'extend') {
    const v = await prompt(`Продлить ${pn(n, 'клиента', 'клиентов', 'клиентов')}`, { label: 'Дней', value: '30', type: 'number', okText: 'Продлить' })
    if (v === null) return
    body.days = Number(v)
  } else if (action === 'add-tag' || action === 'remove-tag') {
    const v = await prompt(action === 'add-tag' ? 'Добавить тег' : 'Убрать тег', { label: 'Тег', placeholder: 'напр. vip' })
    if (!v) return
    body.tag = v.trim()
  } else if (action === 'delete') {
    if (!await confirm(`Удалить ${pn(n, 'клиента', 'клиентов', 'клиентов')}?`, 'Ссылки и подписки перестанут работать. Отменить нельзя.', { danger: true, okText: 'Удалить' })) return
  }
  try { const r = await post('clients/bulk', body); ok(`Готово: ${r && r.count !== undefined ? r.count : n}`); sel.value = []; await reload(true); loadMeta() } catch (e) { fail(e) }
}
async function exp(fmt) { ioMenu.value = false; try { await download('clients/export?format=' + fmt, 'clients.' + fmt) } catch (e) { fail(e) } }
function readFile(e) { const f = e.target.files[0]; if (!f) return; const r = new FileReader(); r.onload = () => { imp.data = String(r.result) }; r.readAsText(f); e.target.value = '' }
async function doImport() {
  busy.imp = true
  try { const r = await post('clients/import', { data: imp.data }); ok(`Добавлено: ${r.added}, пропущено: ${r.skipped}`); imp.open = false; imp.data = ''; await reload(true); loadMeta() } catch (e) { fail(e) } finally { busy.imp = false }
}
onMounted(() => { loadInbounds(); if (route.query.new) { openNew(); router.replace({ query: {} }) } })
</script>
