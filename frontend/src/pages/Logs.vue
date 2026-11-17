<template>
  <div class="pg-logs">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="w2-eb">Xray и панель</span><h1>Логи</h1><p>Журнал ошибок Xray пишется всегда. Подробную запись (уровень debug/info и журнал подключений) включайте на время — она сама выключится, чтобы не забить диск.</p></div>
        <div class="i-hd-a">
          <button v-if="kind !== 'events' && kind !== 'core'" class="btn" type="button" @click="download('logs/download?kind=' + kind, 'xray-' + kind + '.log')"><Icon n="download" />Скачать</button>
          <button v-if="kind !== 'events' && kind !== 'core'" class="btn btn-g" type="button" @click="clear"><Icon n="trash" />Очистить</button>
        </div>
      </div>
      <div class="i-mt">
        <Metric label="Запись" :value="state.recording ? 'идёт' : 'выключена'" :note="state.recording ? `до ${time(state.until)} · осталось ${left(state.until)}` : 'только ошибки (warning)'" :note-tone="state.recording ? 'wa' : ''" />
        <Metric label="Уровень" :value="state.level || 'warning'" :note="state.access ? 'журнал подключений включён' : 'без журнала подключений'" />
        <Metric label="error.log" :value="bytes(state.errorSize || 0)" note="ошибки и предупреждения" />
        <Metric label="access.log" :value="bytes(state.accessSize || 0)" note="подключения клиентов" />
      </div>
      <section class="i-pn">
        <div class="i-ph"><h2>Подробная запись</h2><span class="i-stt" :class="state.recording ? 'wa' : 'mu'">{{ state.recording ? 'идёт' : 'выключена' }}</span></div>
        <div class="i-pb">
          <div v-if="!state.recording" class="row" style="align-items:flex-end">
            <div class="fld"><label class="lbl">Длительность</label><Seg v-model="rec.minutes" :options="[[5, '5 мин'], [15, '15 мин'], [30, '30 мин'], [60, '1 час']]" /></div>
            <div class="fld"><label class="lbl">Уровень</label><Seg v-model="rec.level" :options="[['warning', 'warning'], ['info', 'info'], ['debug', 'debug']]" /></div>
            <div class="fld"><TgRow v-model="rec.access" label="Журнал подключений" hint="кто, куда и через какой инбаунд" /></div>
            <span class="grow"></span>
            <button class="btn btn-p" type="button" @click="start"><Icon n="play" />Начать запись</button>
          </div>
          <div v-else class="row"><span class="hint">Xray пишет подробный лог до {{ time(state.until) }}. После этого уровень вернётся к warning.</span><span class="grow"></span><button class="btn" type="button" @click="stop"><Icon n="stop" />Остановить</button></div>
        </div>
      </section>
      <section class="i-pn">
        <div class="i-ph">
          <div class="i-tabs" role="tablist" style="margin:0;border:0">
            <button v-for="t in TABS" :key="t[0]" type="button" :class="{ on: kind === t[0] }" @click="kind = t[0]">{{ t[1] }}</button>
          </div>
          <div class="row" style="flex-wrap:nowrap">
            <div class="i-ls"><Icon n="search" /><input v-model="q" class="inp" :placeholder="kind === 'access' ? 'email клиента, домен, IP…' : 'поиск'" @keydown.enter="load"></div>
            <TgRow v-model="follow" label="Авто" />
          </div>
        </div>
        <div class="i-pb">
          <div v-if="kind === 'events'" class="list-rows">
            <div v-for="(e, i) in eventsF" :key="i" class="lr"><span class="i-stt" :class="e.kind === 'bad' ? 'bad' : e.kind === 'warn' ? 'wa' : e.kind === 'ok' ? 'ok' : 'mu'" style="min-width:0"></span><span class="lr-t"><b style="font-weight:500">{{ e.text }}</b><small>{{ dt(e.t) }}</small></span></div>
            <Empty v-if="!eventsF.length" icon="logs" title="Событий нет" />
          </div>
          <template v-else>
            <pre ref="box" class="log-box" style="max-height:60vh;min-height:240px">{{ lines.length ? lines.join('\n') : (kind === 'access' && !state.access ? 'Журнал подключений не ведётся — включите его в «Подробной записи».' : 'пусто') }}</pre>
            <p class="hint">Показаны последние {{ lines.length }} строк{{ q ? ` с «${q}»` : '' }}.</p>
          </template>
        </div>
      </section>
    </div>
  </div>
</template>
<script setup>
import '../styles/Logs.css'
import { ref, reactive, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import Icon from '../components/Icon.vue'
import Metric from '../components/Metric.vue'
import Seg from '../components/Seg.vue'
import TgRow from '../components/TgRow.vue'
import Empty from '../components/Empty.vue'
import { get, post, download } from '../lib/api.js'
import { ok, fail, confirm } from '../lib/ui.js'
import { bytes, dt, left } from '../lib/fmt.js'

const TABS = [['error', 'Ошибки Xray'], ['access', 'Подключения'], ['core', 'Вывод Xray'], ['events', 'События панели']]
const kind = ref('error')
const q = ref('')
const follow = ref(true)
const lines = ref([])
const events = ref([])
const state = ref({})
const rec = reactive({ minutes: 15, level: 'info', access: true })
const box = ref(null)
const time = (t) => t ? new Date(t).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' }) : ''
const eventsF = computed(() => events.value.filter(e => !q.value || e.text.toLowerCase().includes(q.value.toLowerCase())))
async function loadState() { try { state.value = await get('logs/state') } catch {} }
async function load() {
  try {
    if (kind.value === 'events') { events.value = await get('events'); return }
    const r = await get(`logs?kind=${kind.value}&limit=1000&q=${encodeURIComponent(q.value)}`)
    const atBottom = !box.value || box.value.scrollTop + box.value.clientHeight >= box.value.scrollHeight - 30
    lines.value = r.lines
    if (atBottom) { await nextTick(); if (box.value) box.value.scrollTop = box.value.scrollHeight }
  } catch (e) { fail(e) }
}
watch(kind, () => { lines.value = []; load() })
let qt
watch(q, () => { clearTimeout(qt); qt = setTimeout(load, 400) })
async function start() { try { await post('logs/start', { ...rec }); ok('Запись начата'); loadState() } catch (e) { fail(e) } }
async function stop() { try { await post('logs/stop'); ok('Запись остановлена'); loadState() } catch (e) { fail(e) } }
async function clear() { if (await confirm('Очистить лог?', kind.value + '.log будет обнулён.', { danger: true, okText: 'Очистить' })) { try { await post('logs/clear', { kind: kind.value }); load(); loadState() } catch (e) { fail(e) } } }
let t
onMounted(() => { loadState(); load(); t = setInterval(() => { if (document.hidden) return; loadState(); if (follow.value) load() }, 3000) })
onBeforeUnmount(() => clearInterval(t))
</script>
