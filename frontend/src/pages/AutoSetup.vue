<template>
  <div class="pg-wz">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div>
          <span class="wz-eb">{{ w ? pn(w.servers.length, 'сервер', 'сервера', 'серверов') : '…' }} · домены · все протоколы · клиент main</span>
          <h1>Автонастройка</h1>
          <p>Пошаговая настройка всей связки: у каждого сервера свой сайт на своём домене, подключения всех протоколов и транспортов, клиент <b>main</b> со всеми подключениями. Панель и подписка — на своих доменах.</p>
        </div>
      </div>

      <ol class="wz-bar">
        <li v-for="(t, i) in STEPS" :key="t" :class="{ on: step === i + 1, done: step > i + 1 }">
          <button type="button" :disabled="!canGo(i + 1)" @click="step = i + 1"><i><Icon v-if="step > i + 1" n="check" /><template v-else>{{ i + 1 }}</template></i><span>{{ t }}</span></button>
        </li>
      </ol>

      <Loading v-if="!w && !err" />
      <div v-else-if="err" class="err-box">{{ err }}</div>
      <template v-else>
        <div v-if="w.isNode" class="i-hint wa"><Icon n="info" /><span>Этот сервер подключён к основной панели как связанный. Обычно автонастройку запускают на основной — там видны все серверы связки.</span></div>

        <!-- 1. domains -->
        <template v-if="step === 1">
          <section class="i-pn">
            <div class="i-ph"><h2>Домены</h2><span class="muted">можно создать прямо здесь — бесплатно и без регистрации</span></div>
            <div class="i-pb">
              <TgRow v-model="oneDom" bold label="Один домен для всего" :hint="`панель, подписка и сайт сервера «${mainSrv.name}» — на одном домене${w.servers.length > 1 ? '; у остальных серверов всё равно свои домены — они ведут на их IP' : ''}`" />

              <div class="wz-blk">
                <div class="wz-bh"><span class="wz-ic"><Icon n="lock" /></span><span><b>{{ oneDom ? 'Домен панели, подписки и сайта' : 'Домен панели' }}</b><small>по нему вы будете входить в панель</small></span></div>
                <div v-if="w.panel.onDomain" class="i-hint ok"><Icon n="check" /><span>Панель уже на домене <b>{{ w.panel.domain }}</b> — её адрес не меняется.</span></div>
                <DomPick v-else v-model="panelPick" :domains="mainSrv.domains" :ip="mainSrv.ip" />
              </div>

              <div v-if="!oneDom" class="wz-blk">
                <div class="wz-bh"><span class="wz-ic"><Icon n="sub" /></span><span><b>Домен подписки</b><small>адрес ссылок подписки для клиентов</small></span></div>
                <Seg v-model="subMode" :options="[{ v: 'panel', l: 'Как у панели' }, { v: 'own', l: 'Отдельный домен' }]" />
                <DomPick v-if="subMode === 'own'" v-model="subPick" :domains="mainSrv.domains" :ip="mainSrv.ip" />
              </div>

              <div class="wz-blk">
                <div class="wz-bh"><span class="wz-ic"><Icon n="globe" /></span><span><b>Сайт на каждом сервере</b><small>домен ведёт на IP своего сервера — сайт и Reality работают на самом этом сервере</small></span></div>
                <div class="wz-srvs">
                  <div v-for="s in w.servers" :key="s.id" class="wz-srv">
                    <div class="wz-sh">
                      <span class="wz-sn"><b>{{ s.name }}</b><small>{{ s.main ? 'этот сервер' : 'связанный' }} · {{ s.ip || 'IP неизвестен' }}<template v-if="s.site.length"> · сейчас сайт: {{ s.site.join(', ') }}</template></small></span>
                      <span v-if="s.error" class="i-stt sm bad">нет связи</span>
                      <span v-else-if="!s.ready" class="i-stt sm wa">версия {{ s.version || '?' }}</span>
                    </div>
                    <div v-if="s.error" class="hint bad-t">{{ s.error }}</div>
                    <div v-else-if="!s.ready" class="i-hint wa"><Icon n="warn" /><span>На сервере панель {{ s.version || 'старой версии' }} — обновите её до {{ ver }} (<code>kvk update</code>), иначе сервер будет пропущен.</span></div>
                    <div v-if="s.main && oneDom" class="hint">тот же домен, что у панели{{ mainName ? ': ' + mainName : '' }}</div>
                    <DomPick v-else-if="!s.error" v-model="sitePick[s.id]" :domains="s.domains" :ip="s.ip" direct />
                  </div>
                </div>
              </div>
              <div class="i-hint"><Icon n="info" /><span><b>Почему у каждого сервера свой домен.</b> Reality маскируется под сайт: в подключении указан домен сайта (SNI), и сервер честно отвечает сайтом на этот домен. Поэтому SNI каждого сервера — домен его собственного сайта, и этот домен должен вести прямо на IP сервера, без прокси Cloudflare.</span></div>
              <div class="ft"><span class="grow"></span><span v-if="step1Err" class="muted small">{{ step1Err }}</span><button class="btn btn-p" type="button" :disabled="!!step1Err" @click="step = 2">Далее<Icon n="chevR" /></button></div>
            </div>
          </section>
        </template>

        <!-- 2. link group -->
        <template v-if="step === 2">
          <section class="i-pn">
            <div class="i-ph"><h2>Другие серверы</h2><span class="muted">необязательно</span></div>
            <div class="i-pb">
              <p class="hint">Добавьте в связку другие панели KVK — автонастройка создаст сайт, домен и все подключения и на них, а клиент <b>main</b> появится на каждом сервере. Ключ связки — на втором сервере в разделе «Связка серверов».</p>
              <div class="wz-list">
                <div v-for="s in w.servers" :key="s.id" class="wz-li">
                  <span class="wz-ic"><Icon :n="s.main ? 'cpu' : 'link'" /></span>
                  <span class="wz-sn"><b>{{ s.name }}</b><small>{{ s.main ? 'этот сервер' : 'в связке' }} · {{ s.ip || '—' }} · KVK {{ s.version || '?' }}</small></span>
                  <span class="i-stt sm" :class="s.error ? 'bad' : s.ready ? 'ok' : 'wa'">{{ s.error ? 'нет связи' : s.ready ? 'готов' : 'нужно обновить' }}</span>
                </div>
              </div>
              <div class="wz-add">
                <div class="fld"><label class="lbl">Ключ связки второго сервера</label><textarea v-model="add.key" class="inp mono" rows="2" placeholder="ключ из «Связка серверов» другого сервера"></textarea></div>
                <div class="row"><input v-model="add.name" class="inp grow" maxlength="40" placeholder="название (необязательно) — например, Германия"><button class="btn" type="button" :disabled="busy.add || !add.key.trim()" @click="addServer"><span v-if="busy.add" class="spin"></span><Icon v-else n="plus" />Добавить сервер</button></div>
              </div>
              <div class="ft">
                <button class="btn btn-g" type="button" @click="step = 1"><Icon n="chevR" class="flip" />Назад</button>
                <span class="grow"></span>
                <button class="btn" type="button" @click="step = 3">Мне это не надо</button>
                <button class="btn btn-p" type="button" @click="step = 3">Продолжить<Icon n="chevR" /></button>
              </div>
            </div>
          </section>
        </template>

        <!-- 3. confirm -->
        <template v-if="step === 3">
          <div class="wz-c2">
            <section class="i-pn">
              <div class="i-ph"><h2>{{ panelChanges ? 'Адрес панели изменится' : 'Адрес панели' }}</h2></div>
              <div class="i-pb">
                <template v-if="panelChanges">
                  <div class="wz-url old"><span class="lbl">сейчас</span><code>{{ w.panel.url }}</code></div>
                  <div class="wz-url new"><span class="lbl">после настройки</span><code>{{ newPanelUrl }}</code><button class="btn btn-sm" type="button" @click="copy(newPanelUrl, 'Ссылка скопирована')"><Icon n="copy" />Копировать</button></div>
                  <div class="i-hint wa"><Icon n="warn" /><span><b>Сохраните новую ссылку.</b> Логин и пароль останутся прежними. Когда всё будет настроено, панель покажет эту ссылку ещё раз и перейдёт на неё по кнопке — командная строка не понадобится.</span></div>
                </template>
                <template v-else>
                  <div class="wz-url new"><span class="lbl">{{ w.panel.onDomain ? 'панель уже на домене' : 'не меняется' }}</span><code>{{ w.panel.url }}</code></div>
                  <p class="hint">{{ w.panel.onDomain ? 'Панель уже работает на домене — автонастройка его не трогает.' : 'Адрес панели останется прежним.' }}</p>
                </template>
                <div class="fld"><label class="lbl">E-mail для Let's Encrypt <span class="muted">· необязательно</span></label><input v-model="email" class="inp" type="email" placeholder="для писем об истечении сертификатов"></div>
              </div>
            </section>
            <section class="i-pn">
              <div class="i-ph"><h2>Что будет сделано</h2></div>
              <div class="i-pb">
                <ul class="wz-plan">
                  <li v-for="s in planServers" :key="s.id"><Icon n="globe" /><span><span><b>{{ s.name }}</b> — сайт на <code>{{ s.dom || '…' }}</code>, SNI <code>{{ s.dom || '…' }}</code></span><small>VLESS Reality · XHTTP и WebSocket за сайтом · gRPC и XHTTP Reality · Trojan · VMess · HTTPUpgrade · Shadowsocks · Hysteria2 · TUIC · AmneziaWG · MASQUE — имена «{{ s.name }} VLESS Reality»…</small></span></li>
                  <li><Icon n="user" /><span><span><b>Клиент main</b> — все подключения всех серверов</span><small>{{ w.main ? 'уже есть — получит новые подключения' : 'будет создан, бессрочно и без лимита' }}</small></span></li>
                  <li><Icon n="sub" /><span><span><b>Подписка</b> — <code>{{ subHost }}</code></span></span></li>
                  <li><Icon n="lock" /><span><span><b>Панель</b> — {{ panelChanges ? 'переедет на ' + mainName + ' по кнопке в конце' : 'адрес не меняется' }}</span></span></li>
                </ul>
                <div v-if="skipped.length" class="i-hint wa"><Icon n="warn" /><span>Будут пропущены: {{ skipped.join(', ') }}</span></div>
                <div class="ft">
                  <button class="btn btn-g" type="button" @click="step = 2"><Icon n="chevR" class="flip" />Назад</button>
                  <span class="grow"></span>
                  <button class="btn btn-p" type="button" :disabled="busy.run" @click="run"><span v-if="busy.run" class="spin"></span><Icon v-else n="bolt" />Настроить всё</button>
                </div>
              </div>
            </section>
          </div>
        </template>

        <!-- 4. progress / result -->
        <template v-if="step === 4">
          <section v-if="done && !st.error" class="i-pn wz-done">
            <div class="i-pb">
              <div class="wz-dh"><span class="wz-big"><Icon n="check" /></span><span><b>Всё готово</b><small v-if="res.panelUrl">Настроенная панель ждёт вас по новой ссылке. Логин и пароль — прежние.</small><small v-else>Серверы настроены. Ссылки клиента main — ниже.</small></span></div>
              <div v-if="res.panelUrl" class="wz-url new big"><span class="lbl">новый адрес панели</span><code>{{ res.panelUrl }}</code><button class="btn btn-sm" type="button" @click="copy(res.panelUrl, 'Ссылка скопирована')"><Icon n="copy" />Копировать</button></div>
              <div v-if="res.subUrl" class="wz-url"><span class="lbl">подписка клиента main</span><code>{{ res.subUrl }}</code><button class="btn btn-sm btn-g btn-ic" type="button" title="QR-код" @click="qr = res.subUrl"><Icon n="qr" /></button><button class="btn btn-sm" type="button" @click="copy(res.subUrl)"><Icon n="copy" />Копировать</button></div>
              <div class="ft">
                <button class="btn btn-g" type="button" @click="again">Настроить заново</button>
                <router-link class="btn btn-g" to="/inbounds">Инбаунды</router-link>
                <span class="grow"></span>
                <button v-if="res.panelUrl" class="btn btn-p" type="button" :disabled="busy.sw" @click="switchPanel"><span v-if="busy.sw" class="spin"></span><Icon v-else n="ext" />{{ busy.sw ? `Переходим через ${left} с…` : 'Перейти на новый адрес' }}</button>
              </div>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Ход настройки</h2><span class="i-stt" :class="st.running ? 'wa' : st.error ? 'bad' : 'ok'">{{ st.running ? 'идёт настройка' : st.error ? 'не завершена' : 'готово' }}</span></div>
            <div class="i-pb">
              <ol class="wz-steps">
                <li v-for="(s, i) in st.steps || []" :key="s.key" :class="s.status">
                  <i><span v-if="s.status === 'run'" class="spin"></span><Icon v-else-if="s.status === 'ok'" n="check" /><Icon v-else-if="s.status === 'warn'" n="warn" /><Icon v-else-if="s.status === 'bad'" n="x" /><template v-else>{{ i + 1 }}</template></i>
                  <span><b>{{ s.title }}</b><small v-if="s.detail">{{ s.detail }}</small></span>
                </li>
              </ol>
              <div v-if="st.error" class="err-box">{{ st.error }}</div>
              <div v-for="x in st.warnings || []" :key="x" class="i-hint wa"><Icon n="warn" /><span>{{ x }}</span></div>
              <div v-if="st.running" class="hint">Можно закрыть страницу — настройка продолжится на сервере.</div>
              <div v-else-if="st.error" class="ft"><span class="grow"></span><button class="btn" type="button" @click="again">Изменить и повторить</button></div>
            </div>
          </section>
        </template>
      </template>
    </div>

    <Drawer :open="!!qr" title="Подписка main" eyebrow="QR-код" width="min(420px, calc(100% - 24px))" @close="qr = ''">
      <div v-if="qr" style="display:flex;flex-direction:column;align-items:center;gap:12px"><Qr :text="qr" :size="300" /><code style="word-break:break-all;font-size:12px">{{ qr }}</code><button class="btn" type="button" @click="copy(qr)"><Icon n="copy" />Копировать</button></div>
    </Drawer>
  </div>
</template>
<script setup>
import { ref, reactive, computed, onMounted, onBeforeUnmount } from 'vue'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import TgRow from '../components/TgRow.vue'
import Seg from '../components/Seg.vue'
import Qr from '../components/Qr.vue'
import Drawer from '../components/Drawer.vue'
import DomPick from '../components/DomPick.vue'
import { get, post } from '../lib/api.js'
import { ok, fail, copy } from '../lib/ui.js'
import { pn } from '../lib/fmt.js'
import { loadServers } from '../lib/server.js'
import { meta } from '../lib/meta.js'

const STEPS = ['Домены', 'Серверы', 'Адрес панели', 'Настройка']
const w = ref(null)
const err = ref('')
const step = ref(1)
const oneDom = ref(false)
const panelPick = ref({ mode: 'free', label: 'panel', id: '', name: '' })
const subMode = ref('panel')
const subPick = ref({ mode: 'free', label: 'sub', id: '', name: '' })
const sitePick = reactive({})
const email = ref('')
const add = reactive({ key: '', name: '' })
const busy = reactive({ add: false, run: false, sw: false })
const st = ref({})
const qr = ref('')
const left = ref(4)
const ver = computed(() => meta.version || '1.0.7')

const mainSrv = computed(() => w.value?.servers.find(s => s.main) || { domains: [], ip: '', name: '' })
const dashed = (ip) => /^\d+\.\d+\.\d+\.\d+$/.test(ip || '') ? ip.replaceAll('.', '-') : ''
const DOM_RE = /^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{1,62}$/
function nameOf(pick, s) {
  if (!pick || !s) return ''
  if (pick.mode === 'free') return dashed(s.ip) ? (pick.label ? pick.label + '-' : '') + dashed(s.ip) + '.sslip.io' : ''
  if (pick.mode === 'exist') return s.domains.find(d => d.id === pick.id)?.name || ''
  return pick.name
}
function pickErr(pick, s) {
  if (pick.mode === 'own' && !DOM_RE.test(pick.name || '')) return 'введите домен'
  if (pick.mode === 'exist' && !pick.id) return 'выберите домен'
  if (pick.mode === 'free' && !dashed(s.ip)) return 'IP сервера неизвестен — выберите «Свой домен»'
  return ''
}
// the domain shared by panel / sub / main site when «Один домен для всего» is on
const mainPick = computed(() => {
  if (!w.value?.panel.onDomain) return panelPick.value
  const d = mainSrv.value.domains.find(x => x.name === w.value.panel.domain)
  return d ? { mode: 'exist', id: d.id } : { mode: 'own', name: w.value.panel.domain }
})
const mainName = computed(() => w.value?.panel.onDomain ? w.value.panel.domain : nameOf(panelPick.value, mainSrv.value))
const panelChanges = computed(() => w.value && !w.value.panel.onDomain)
const newPanelUrl = computed(() => {
  const p = w.value.panel
  return 'https://' + mainName.value + (p.port === 443 ? '' : ':' + p.port) + '/' + p.basePath + '/'
})
const subHost = computed(() => {
  if (oneDom.value || subMode.value === 'panel') return (panelChanges.value ? mainName.value : (w.value.panel.domain || 'адрес панели')) + ' (как у панели)'
  return nameOf(subPick.value, mainSrv.value)
})
const usable = computed(() => (w.value?.servers || []).filter(s => !s.error && s.ready))
const skipped = computed(() => (w.value?.servers || []).filter(s => s.error || !s.ready).map(s => s.name + (s.error ? ' (нет связи)' : ' (обновите до ' + ver.value + ')')))
const planServers = computed(() => usable.value.map(s => ({ id: s.id, name: s.name, dom: s.main && oneDom.value ? mainName.value : nameOf(sitePick[s.id], s) })))
const step1Err = computed(() => {
  if (!w.value) return ''
  if (panelChanges.value) { const e = pickErr(panelPick.value, mainSrv.value); if (e) return 'Панель: ' + e }
  if (!oneDom.value && subMode.value === 'own') { const e = pickErr(subPick.value, mainSrv.value); if (e) return 'Подписка: ' + e }
  for (const s of usable.value) {
    if (s.main && oneDom.value) continue
    const e = pickErr(sitePick[s.id] || {}, s); if (e) return s.name + ': ' + e
  }
  if (!usable.value.length) return 'Нет доступных серверов'
  return ''
})
const done = computed(() => !st.value.running && st.value.finished > 0)
const res = computed(() => st.value.result || {})
const canGo = (n) => n === 4 ? !!(st.value.steps?.length) : !st.value.running && (n === 1 || !step1Err.value)

function defaults() {
  for (const s of w.value.servers) {
    if (sitePick[s.id]) continue
    const cur = s.site.length && s.domains.find(d => d.name === s.site[0])
    sitePick[s.id] = cur ? { mode: 'exist', id: cur.id, label: 'site', name: '' } : { mode: 'free', label: 'site', id: '', name: '' }
  }
}
async function load() {
  err.value = ''
  try {
    w.value = await get('wizard')
    st.value = w.value.state || {}
    if (!email.value) email.value = w.value.email || ''
    defaults()
  } catch (e) { err.value = e.message }
}
async function addServer() {
  busy.add = true
  try {
    const k = add.key.trim()
    await post('nodes', { url: k, name: add.name.trim() })  // url: ключ связки (base64 url|token) или адрес панели
    ok('Сервер добавлен в связку'); add.key = ''; add.name = ''
    loadServers(true); await load()
  } catch (e) { fail(e) } finally { busy.add = false }
}
function body() {
  const sites = usable.value.map(s => ({ server: s.id, domain: s.main && oneDom.value ? mainPick.value : sitePick[s.id] }))
  return {
    panel: panelChanges.value ? panelPick.value : { mode: 'keep' },
    sub: oneDom.value || subMode.value === 'panel' ? { mode: 'panel' } : subPick.value,
    sites, email: email.value.trim(),
  }
}
async function run() {
  busy.run = true
  try { await post('wizard/run', body()); step.value = 4; poll() } catch (e) { fail(e) } finally { busy.run = false }
}
let pt
async function poll() {
  clearTimeout(pt)
  try {
    st.value = await get('wizard/state')
    if (st.value.running) { pt = setTimeout(poll, 1200); return }
    loadServers(true)
  } catch { pt = setTimeout(poll, 2500) }
}
async function switchPanel() {
  busy.sw = true
  try {
    const r = await post('wizard/switch')
    left.value = 4
    const t = setInterval(() => { left.value--; if (left.value <= 0) { clearInterval(t); location.href = r.url } }, 1000)
  } catch (e) { fail(e); busy.sw = false }
}
function again() { step.value = 1; st.value = { ...st.value, steps: st.value.running ? st.value.steps : [] }; load() }
onMounted(async () => {
  await load()
  if (st.value.running) { step.value = 4; poll() } else if (st.value.finished && st.value.result && !st.value.error && (Date.now() - st.value.finished) < 3600e3) step.value = 4
})
onBeforeUnmount(() => clearTimeout(pt))
</script>
<style>
.pg-wz{display:contents}
.h1 .pg-wz .wz-eb{display:block;font-size:var(--fs-md);color:var(--muted);margin-bottom:8px}
.h1 .wz-bar{list-style:none;margin:0;padding:0;display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:8px}
.h1 .wz-bar button{width:100%;display:flex;align-items:center;gap:10px;padding:10px 12px;border-radius:var(--r-lg);border:1px solid var(--line);background:var(--surface);color:var(--muted);font:inherit;font-size:var(--fs-sm);cursor:pointer;text-align:left}
.h1 .wz-bar button:disabled{cursor:default}
.h1 .wz-bar i{width:26px;height:26px;border-radius:50%;display:grid;place-items:center;font-style:normal;font-weight:600;background:var(--surface3);color:var(--muted);flex:none}
.h1 .wz-bar i .ic{width:14px;height:14px}
.h1 .wz-bar span{font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.h1 .wz-bar li.on button{border-color:var(--accent);box-shadow:inset 0 0 0 1px var(--accent);color:var(--strong)}
.h1 .wz-bar li.on i{background:var(--accent);color:#fff}
.h1 .wz-bar li.done i{background:var(--okSoft);color:var(--ok)}
.h1 .wz-bar li.done button{color:var(--text)}
.h1 .wz-blk{display:flex;flex-direction:column;gap:12px;padding:16px 0 4px;border-top:1px solid var(--line2)}
.h1 .wz-blk>.i-seg{align-self:flex-start}
.h1 .wz-bh{display:flex;align-items:center;gap:12px;min-width:0}
.h1 .wz-bh>span:last-child{display:flex;flex-direction:column;line-height:1.35;min-width:0}
.h1 .wz-bh b{font-weight:600;color:var(--strong)}.h1 .wz-bh small{color:var(--muted);font-size:var(--fs-sm)}
.h1 .wz-ic{width:36px;height:36px;border-radius:var(--r-md);display:grid;place-items:center;background:var(--accentSoft);color:var(--accentText);flex:none}
.h1 .wz-ic .ic{width:18px;height:18px}
.h1 .wz-srvs{display:flex;flex-direction:column;gap:10px}
.h1 .wz-srv{display:flex;flex-direction:column;gap:10px;padding:12px 14px;border-radius:var(--r-lg);border:1px solid var(--line);background:var(--surface2);min-width:0}
.h1 .wz-sh{display:flex;align-items:center;gap:10px;min-width:0}
.h1 .wz-sn{display:flex;flex-direction:column;flex:1;min-width:0;line-height:1.35}
.h1 .wz-sn b{font-weight:600;color:var(--strong)}.h1 .wz-sn small{color:var(--muted);font-size:var(--fs-xs);overflow-wrap:anywhere}
.h1 .dpk{display:flex;flex-direction:column;gap:8px;min-width:0}
.h1 .dpk>.i-seg{align-self:flex-start}
.h1 .dpk-b{display:flex;flex-direction:column;gap:6px;min-width:0}
.h1 .dpk-b .hint{margin:0}
.h1 .dpk-row{display:flex;align-items:center;gap:0;min-width:0;border:1px solid var(--edge);border-radius:var(--r-sm);background:var(--inpBg);overflow:hidden}
.h1 .dpk-row{cursor:text}
.h1 .dpk-row .inp.dpk-lbl{border:0;border-radius:0;width:auto;flex:none;background:transparent;padding-right:0;color:var(--accentText);font-weight:600}
.h1 .dpk-suf{flex:1;padding:0 12px 0 0;color:var(--muted);font-size:var(--fs-sm);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.h1 .wz-list{display:flex;flex-direction:column}
.h1 .wz-li{display:flex;align-items:center;gap:12px;padding:10px 0;border-top:1px solid var(--line2)}
.h1 .wz-li:first-child{border-top:0}
.h1 .wz-add{display:flex;flex-direction:column;gap:10px;padding:14px;border-radius:var(--r-lg);border:1px dashed var(--edge3)}
.h1 .wz-c2{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:18px;align-items:start}
.h1 .wz-url{display:flex;align-items:center;flex-wrap:wrap;gap:6px 10px;padding:12px 14px;border-radius:var(--r-md);background:var(--surface2);border:1px solid var(--line);min-width:0}
.h1 .wz-url .lbl{flex-basis:100%;margin:0;font-size:var(--fs-xs);color:var(--muted)}
.h1 .wz-url code{flex:1;min-width:0;font-family:var(--mono);font-size:var(--fs-sm);color:var(--strong);overflow-wrap:anywhere}
.h1 .wz-url.old code{text-decoration:line-through;color:var(--muted)}
.h1 .wz-url.new{border-color:var(--accent);background:var(--accentSoft)}
.h1 .wz-url.big code{font-size:var(--fs-md);font-weight:600}
.h1 .wz-plan{list-style:none;margin:0;padding:0;display:flex;flex-direction:column;gap:12px}
.h1 .wz-plan li{display:flex;gap:10px;align-items:flex-start;font-size:var(--fs-sm);line-height:1.45}
.h1 .wz-plan li>.ic{width:16px;height:16px;color:var(--accentText);flex:none;margin-top:2px}
.h1 .wz-plan li>span{display:flex;flex-direction:column;gap:2px;min-width:0}
.h1 .wz-plan b{font-weight:600;color:var(--strong)}.h1 .wz-plan small{color:var(--muted);font-size:var(--fs-xs)}
.h1 .wz-plan code{font-family:var(--mono);color:var(--strong);overflow-wrap:anywhere}
.h1 .wz-done{border-color:var(--ok)}
.h1 .wz-done>.i-pb{padding-top:20px}
.h1 .wz-dh{display:flex;align-items:center;gap:14px}
.h1 .wz-dh>span:last-child{display:flex;flex-direction:column;gap:2px}
.h1 .wz-dh b{font-size:var(--fs-lg);font-weight:600;color:var(--strong)}.h1 .wz-dh small{color:var(--muted);font-size:var(--fs-sm)}
.h1 .wz-big{width:44px;height:44px;border-radius:50%;display:grid;place-items:center;background:var(--okSoft);color:var(--ok);flex:none}
.h1 .wz-big .ic{width:22px;height:22px}
.h1 .wz-steps{list-style:none;margin:0;padding:0;display:flex;flex-direction:column;gap:2px}
.h1 .wz-steps li{display:flex;gap:12px;align-items:flex-start;padding:8px 0}
.h1 .wz-steps li>i{width:28px;height:28px;border-radius:50%;display:grid;place-items:center;font-style:normal;font-size:var(--fs-sm);font-weight:600;flex:none;background:var(--surface3);color:var(--muted)}
.h1 .wz-steps li>i .ic{width:15px;height:15px}
.h1 .wz-steps li.run>i{background:var(--accentSoft);color:var(--accentText)}
.h1 .wz-steps li.ok>i{background:var(--okSoft);color:var(--ok)}
.h1 .wz-steps li.warn>i{background:var(--warnSoft);color:var(--warn)}
.h1 .wz-steps li.bad>i{background:var(--badSoft);color:var(--bad)}
.h1 .wz-steps li.skip{opacity:.55}
.h1 .wz-steps li>span{display:flex;flex-direction:column;gap:2px;padding-top:4px;min-width:0}
.h1 .wz-steps b{font-weight:600;color:var(--strong)}.h1 .wz-steps small{color:var(--muted);font-size:var(--fs-sm);overflow-wrap:anywhere}
.h1 .pg-wz .ic.flip{transform:rotate(180deg)}
@media (max-width:1000px){.h1 .wz-c2{grid-template-columns:1fr}}
@media (max-width:700px){.h1 .wz-bar{grid-template-columns:repeat(2,minmax(0,1fr))}}
</style>
