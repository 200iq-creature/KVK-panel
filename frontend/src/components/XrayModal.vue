<template>
  <Drawer :open="xrayUi.open" title="Ядро Xray" eyebrow="версия · обновление · откат" width="min(620px, calc(100% - 24px))" @close="xrayUi.open = false">
    <div class="xm">
      <section class="xm-cur">
        <div class="xm-v">
          <span class="i-stt" :class="st.running ? 'ok' : st.installed ? 'bad' : 'mu'">{{ st.running ? 'работает' : st.installed ? 'остановлено' : 'не установлено' }}</span>
          <b>{{ st.version ? 'v' + st.version : '—' }}</b>
          <small>{{ st.running ? 'аптайм ' + uptime(st.uptime) + (st.pid ? ' · PID ' + st.pid : '') : (st.error || 'клиенты не могут подключиться') }}</small>
        </div>
        <div class="row">
          <button class="btn btn-sm" type="button" :disabled="busy.x" @click="act('restart')"><Icon n="refresh" />Перезапустить</button>
          <button v-if="st.running" class="btn btn-sm btn-g" type="button" :disabled="busy.x" @click="act('stop')"><Icon n="stop" />Стоп</button>
          <button v-else class="btn btn-sm btn-g" type="button" :disabled="busy.x || !st.installed" @click="act('start')"><Icon n="play" />Запустить</button>
        </div>
      </section>

      <div v-if="xs?.prev" class="i-hint"><Icon n="clock" /><span>Предыдущая версия <b>{{ xs.prev }}</b> сохранена — можно вернуться одним нажатием.</span><button class="btn btn-sm" type="button" :disabled="busy.inst" @click="rollback">Откатить</button></div>
      <div v-if="latest && !isCur(latest.tag)" class="i-hint ok"><Icon n="up" /><span>Доступна новая версия <b>{{ latest.tag }}</b> от {{ date(latest.published) }}.</span><button class="btn btn-sm btn-p" type="button" :disabled="busy.inst" @click="install(latest.tag)">Обновить</button></div>

      <div class="xm-lh">
        <b>Все версии</b>
        <span class="grow"></span>
        <TgRow v-model="showPre" label="pre-release" />
        <button class="btn btn-sm btn-g btn-ic" type="button" title="Обновить список" :disabled="busy.ver" @click="loadVersions"><span v-if="busy.ver" class="spin"></span><Icon v-else n="refresh" /></button>
      </div>
      <div v-if="verErr" class="i-hint bad"><Icon n="warn" /><span>Не удалось получить список версий с GitHub: {{ verErr }}</span></div>
      <div v-else-if="!vers && busy.ver" class="xm-load"><span class="spin"></span>Загружаю список релизов…</div>
      <div v-else class="xm-list thin-sb">
        <button v-for="r in releases" :key="r.tag" class="xm-r" :class="{ cur: isCur(r.tag), sel: pick === r.tag }" type="button" @click="pick = r.tag">
          <span class="xm-rd"></span>
          <b>{{ r.tag }}</b>
          <em v-if="isCur(r.tag)" class="xm-tg ok">установлена</em>
          <em v-else-if="r.tag === xs?.prev" class="xm-tg">предыдущая</em>
          <em v-if="r.prerelease" class="xm-tg wa">pre-release</em>
          <span class="grow"></span>
          <small>{{ date(r.published) }}</small>
        </button>
        <p v-if="vers && !releases.length" class="hint">Релизов не найдено.</p>
      </div>
      <p class="hint">Перед установкой текущий бинарник сохраняется. Если новая версия не запустится с вашим конфигом, панель автоматически вернёт прежнюю. Подключения клиентов прервутся на пару секунд.</p>
    </div>
    <template #foot>
      <router-link class="btn btn-g" to="/settings?tab=xray" @click="xrayUi.open = false"><Icon n="set" />Журнал и config.json</router-link>
      <span class="grow"></span>
      <button class="btn btn-p" type="button" :disabled="!pick || isCur(pick) || busy.inst" @click="install(pick)">
        <span v-if="busy.inst" class="spin"></span><Icon v-else n="download" />
        {{ busy.inst ? 'Устанавливаю…' : !pick ? 'Выберите версию' : isCur(pick) ? 'Уже установлена' : (older(pick) ? 'Откатить на ' : 'Установить ') + pick }}
      </button>
    </template>
  </Drawer>
</template>
<script setup>
import { ref, reactive, computed, watch, onBeforeUnmount } from 'vue'
import Drawer from './Drawer.vue'
import Icon from './Icon.vue'
import TgRow from './TgRow.vue'
import { get, post } from '../lib/api.js'
import { ok, fail, confirm } from '../lib/ui.js'
import { uptime, date } from '../lib/fmt.js'
import { loadMeta } from '../lib/meta.js'
import { xrayUi } from '../lib/xrayui.js'

const xs = ref(null)
const st = computed(() => xs.value?.status || {})
const vers = ref(null)
const verErr = ref('')
const showPre = ref(false)
const pick = ref('')
const busy = reactive({ x: false, ver: false, inst: false })
const releases = computed(() => (vers.value?.releases || []).filter(r => showPre.value || !r.prerelease || isCur(r.tag)))
const latest = computed(() => (vers.value?.releases || []).find(r => !r.prerelease))
const norm = (t) => String(t || '').replace(/^v/, '')
const isCur = (t) => !!st.value.version && norm(t) === norm(st.value.version)
const idx = (t) => (vers.value?.releases || []).findIndex(r => r.tag === t)
const older = (t) => { const a = idx(t), c = (vers.value?.releases || []).findIndex(r => isCur(r.tag)); return a >= 0 && c >= 0 && a > c }

async function loadStatus() { try { xs.value = await get('xray/status') } catch {} }
async function loadVersions() { busy.ver = true; verErr.value = ''; try { vers.value = await get('xray/versions') } catch (e) { verErr.value = e.message } finally { busy.ver = false } }
async function act(a) {
  busy.x = true
  try { await post('xray/' + a); ok({ restart: 'Xray перезапущен', stop: 'Xray остановлен', start: 'Xray запущен' }[a]); await loadStatus(); loadMeta() } catch (e) { fail(e) } finally { busy.x = false }
}
async function install(tag) {
  const down = older(tag)
  if (!await confirm(`${down ? 'Откатить' : 'Установить'} Xray ${tag}?`, 'Xray перезапустится — подключения клиентов прервутся на пару секунд. Если версия не запустится, вернётся текущая.', { okText: down ? 'Откатить' : 'Установить' })) return
  busy.inst = true
  try { const r = await post('xray-install', { version: tag }); ok('Установлена ' + (r?.version || tag)); pick.value = ''; await loadStatus(); loadMeta() } catch (e) { fail(e) } finally { busy.inst = false }
}
async function rollback() {
  if (!await confirm(`Откатить Xray на ${xs.value.prev}?`, 'Xray перезапустится.', { okText: 'Откатить' })) return
  busy.inst = true
  try { await post('xray-rollback'); ok('Откат выполнен'); await loadStatus(); loadMeta() } catch (e) { fail(e) } finally { busy.inst = false }
}
let t
watch(() => xrayUi.open, (v) => {
  clearInterval(t)
  if (!v) return
  loadStatus(); if (!vers.value) loadVersions()
  t = setInterval(loadStatus, 5000)
})
onBeforeUnmount(() => clearInterval(t))
</script>
<style>
.h1 .xm{display:flex;flex-direction:column;gap:14px}
.h1 .xm-cur{display:flex;align-items:center;gap:14px;flex-wrap:wrap;padding:16px;border-radius:var(--r-lg);background:var(--surface2);border:1px solid var(--line)}
.h1 .xm-v{display:flex;flex-direction:column;gap:2px;flex:1;min-width:180px}
.h1 .xm-v b{font-size:var(--fs-2xl);font-weight:500;letter-spacing:-.02em;color:var(--strong);font-family:var(--mono)}
.h1 .xm-v small{color:var(--muted);font-size:var(--fs-sm)}
.h1 .xm .i-hint{display:flex;align-items:center;gap:10px}
.h1 .xm .i-hint>span{flex:1}
.h1 .xm-lh{display:flex;align-items:center;gap:10px}
.h1 .xm-lh b{color:var(--strong);font-weight:600}
.h1 .xm-lh .i-tgr{font-size:var(--fs-sm)}
.h1 .xm-load{display:flex;align-items:center;gap:10px;color:var(--muted);padding:16px 0}
.h1 .xm-list{display:flex;flex-direction:column;gap:4px;max-height:min(46vh,420px);overflow:auto;padding-right:2px}
.h1 .xm-r{display:flex;align-items:center;gap:10px;padding:10px 12px;border-radius:var(--r-md);border:1px solid var(--line2);background:var(--surface);color:var(--text);font:inherit;text-align:left;cursor:pointer}
.h1 .xm-r:hover{background:var(--surface2)}
.h1 .xm-r b{font-family:var(--mono);font-weight:500;color:var(--strong)}
.h1 .xm-r small{color:var(--muted);font-size:var(--fs-sm)}
.h1 .xm-rd{width:16px;height:16px;border-radius:50%;border:1.5px solid var(--edge3);flex:none}
.h1 .xm-r.sel{border-color:var(--accent);box-shadow:inset 0 0 0 1px var(--accent)}
.h1 .xm-r.sel .xm-rd{border:5px solid var(--accent)}
.h1 .xm-r.cur{background:var(--okSoft)}
.h1 .xm-tg{font-style:normal;font-size:var(--fs-xs);padding:2px 8px;border-radius:99px;background:var(--surface3);color:var(--muted)}
.h1 .xm-tg.ok{background:var(--okSoft);color:var(--ok)}
.h1 .xm-tg.wa{background:var(--warnSoft);color:var(--warn)}
.h1 .tx-xr{display:inline-flex;align-items:center;gap:8px;height:var(--ctlSm);padding:0 12px;border-radius:99px;border:1px solid var(--line);background:var(--btnBg);color:var(--text);font:inherit;font-size:var(--fs-sm);cursor:pointer;white-space:nowrap}
.h1 .tx-xr:hover{background:var(--btnHover);border-color:var(--edge3)}
.h1 .tx-xr i{width:8px;height:8px;border-radius:50%;background:var(--muted)}
.h1 .tx-xr i.ok{background:var(--okDot)}
.h1 .tx-xr i.bad{background:var(--badDot)}
.h1 .tx-xr b{font-family:var(--mono);font-weight:500;color:var(--strong)}
@media (max-width:700px){.h1 .tx-xr span{display:none}}
</style>
