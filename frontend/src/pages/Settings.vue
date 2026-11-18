<template>
  <div class="pg-settings">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="w2-eb">KVK Panel {{ s?.version }} · Xray {{ xs?.status?.version || '—' }}</span><h1>Настройки</h1><p>Адрес панели, ядро Xray и его версии, geo-файлы, API и обновление панели. Домены и сертификаты — в разделе <router-link to="/domains">«Домены»</router-link>.</p></div>
        <div class="i-hd-a">
          <router-link class="btn" to="/security"><Icon n="sec" />Безопасность</router-link>
          <router-link class="btn" to="/backups"><Icon n="bak" />Бэкапы</router-link>
        </div>
      </div>
      <Loading v-if="!s" :error="error" @retry="load" />
      <template v-else>
        <div class="i-tabs" role="tablist" style="margin-top:0">
          <button v-for="t in TABS" :key="t[0]" type="button" :class="{ on: tab === t[0] }" @click="tab = t[0]">{{ t[1] }}</button>
        </div>

        <div v-if="tab === 'panel'" class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>Адрес панели</h2><span class="i-stt" :class="s.https ? 'ok' : 'wa'">{{ s.https ? 'HTTPS' : 'HTTP' }}</span></div>
            <div class="i-pb">
              <Kr k="Сейчас" :v="s.panelUrl" code copyable />
              <div class="two">
                <div class="fld"><label class="lbl">Порт панели</label><div class="row" style="flex-wrap:nowrap"><input v-model.number="pf.port" class="inp" type="number" min="1" max="65535"><button class="btn btn-ic" type="button" title="Случайный" @click="pf.port = 10000 + Math.floor(Math.random() * 50000)"><Icon n="refresh" /></button></div></div>
                <div class="fld"><label class="lbl">Секретный путь</label><div class="row" style="flex-wrap:nowrap"><input v-model="pf.basePath" class="inp mono"><button class="btn btn-ic" type="button" title="Случайный" @click="pf.basePath = rnd(12)"><Icon n="refresh" /></button></div></div>
              </div>
              <div class="two">
                <div class="fld"><label class="lbl">Путь подписки</label><input v-model="pf.subPath" class="inp mono"></div>
                <div class="fld"><label class="lbl">Отдельный порт подписки</label><input v-model.number="pf.subPort" class="inp" type="number" min="0" max="65535"><p class="hint">0 — тот же порт, что и панель</p></div>
              </div>
              <div class="fld"><label class="lbl">Домен подписки</label>
                <div class="row" style="flex-wrap:nowrap"><select v-model="subDom" class="inp"><option value="">как у панели</option><option v-for="d in s.domainList || []" :key="d.id" :value="d.id">{{ d.name }}{{ d.status === 'active' ? '' : ' · сертификат не готов' }}</option></select><button class="btn" type="button" :disabled="subDom === (s.subCertId || '')" @click="saveSubDom">Применить</button></div>
                <p class="hint">ссылки подписки: <code>{{ s.subUrl }}</code> · без перезапуска. Домен должен вести на этот сервер; сертификат — в разделе «Домены»</p>
              </div>
              <div class="i-hint wa"><Icon n="warn" /><span>После сохранения панель перезапустится и откроется по новому адресу. Новый порт будет открыт в ufw/firewalld автоматически. Ссылки подписок клиентов изменятся, если поменять путь или порт подписки.</span></div>
              <div class="ft"><span class="grow"></span><button class="btn btn-p" type="button" :disabled="!panelDirty" @click="savePanel">Сохранить и перезапустить</button></div>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Общие</h2></div>
            <div class="i-pb">
              <div class="two">
                <div class="fld"><label class="lbl">Название сервера</label><input v-model="g.nodeName" class="inp" maxlength="40"><p class="hint">видно в подписке и связке серверов</p></div>
                <div class="fld"><label class="lbl">Страна (код)</label><input v-model="g.countryCode" class="inp mono" maxlength="2" placeholder="NL" style="text-transform:uppercase"><p class="hint">{{ flag(g.countryCode) || 'двухбуквенный код' }}</p></div>
              </div>
              <div class="fld"><label class="lbl">Отпечаток TLS по умолчанию (fp)</label><select v-model="g.defaultFp" class="inp"><option v-for="x in FP" :key="x">{{ x }}</option></select><p class="hint">какой браузер изображает клиент при Reality/TLS</p></div>
              <TgRow v-model="g.showTrafficAll" bold label="Показывать трафик всем клиентам" hint="в приложении и на странице подписки, даже без лимита" />
              <div class="fld"><label class="lbl">Удалять клиентов, истёкших больше N дней назад</label><input v-model.number="g.autoCleanDays" class="inp" type="number" min="0"><p class="hint">0 — не удалять. Проверка раз в час.</p></div>
              <div class="fld"><label class="lbl">Сброс трафика для новых клиентов</label><select v-model="g.newResetPeriod" class="inp"><option v-for="r in RESET" :key="r[0]" :value="r[0]">{{ r[1] }}</option></select></div>
              <div v-if="['mday', 'days'].includes(g.newResetPeriod)" class="fld"><label class="lbl">{{ g.newResetPeriod === 'mday' ? 'Число месяца' : 'Каждые N дней' }}</label><input v-model.number="g.newResetDays" class="inp" type="number" min="1" :max="g.newResetPeriod === 'mday' ? 28 : 365"></div>
              <div class="ft"><button class="btn btn-g" type="button" @click="resetAll">Применить сброс ко всем клиентам</button><span class="grow"></span><button class="btn btn-p" type="button" @click="saveGeneral">Сохранить</button></div>
            </div>
          </section>
        </div>

        <template v-if="tab === 'xray'">
          <div class="i-c2">
            <section class="i-pn">
              <div class="i-ph"><h2>Ядро Xray</h2><span class="i-stt" :class="xst.running ? 'ok' : xst.installed ? 'bad' : 'mu'">{{ xst.running ? 'работает' : xst.installed ? 'остановлено' : 'не установлено' }}</span></div>
              <div class="i-pb">
                <div class="i-kv">
                  <Kr k="Версия" :v="xst.version || '—'" />
                  <Kr k="Аптайм" :v="xst.running ? uptime(xst.uptime) : '—'" />
                  <Kr k="PID" :v="xst.pid || '—'" />
                  <Kr k="Сборка" :v="xs?.asset" code />
                  <Kr v-if="xs?.prev" k="Предыдущая" :v="xs.prev" />
                </div>
                <div v-if="xst.error" class="err-box">{{ xst.error }}</div>
                <div class="ft">
                  <button class="btn" type="button" :disabled="busy.x" @click="xAct('restart')"><Icon n="refresh" />Перезапустить</button>
                  <button v-if="xst.running" class="btn btn-g" type="button" :disabled="busy.x" @click="xAct('stop')"><Icon n="stop" />Остановить</button>
                  <button v-else class="btn btn-g" type="button" :disabled="busy.x || !xst.installed" @click="xAct('start')"><Icon n="play" />Запустить</button>
                  <span class="grow"></span>
                  <button class="btn btn-g" type="button" @click="showConfig"><Icon n="file" />config.json</button>
                  <button class="btn" type="button" @click="openXray"><Icon n="up" />Сменить версию</button>
                </div>
              </div>
            </section>
            <section class="i-pn">
              <div class="i-ph"><h2>Версия Xray</h2><button class="btn btn-sm btn-g" type="button" :disabled="busy.ver" @click="loadVersions"><span v-if="busy.ver" class="spin"></span><Icon v-else n="refresh" />Обновить список</button></div>
              <div class="i-pb">
                <p class="hint">Установите новую версию или откатитесь на старую. Текущий бинарник сохраняется — если новая версия не запустится с вашим конфигом, панель автоматически вернёт прежнюю.</p>
                <div v-if="verErr" class="i-hint bad"><Icon n="warn" /><span>{{ verErr }}</span></div>
                <div class="fld"><label class="lbl">Версия</label>
                  <select v-model="ver" class="inp"><option value="">— выберите —</option><option v-for="r in releases" :key="r.tag" :value="r.tag">{{ r.tag }}{{ r.prerelease ? ' · pre-release' : '' }}{{ r.tag.replace(/^v/, '') === (xst.version || '') ? ' · установлена' : '' }} · {{ date(r.published) }}</option></select>
                  <TgRow v-model="showPre" label="Показывать pre-release" />
                </div>
                <div class="ft">
                  <button v-if="xs?.prev" class="btn btn-g" type="button" :disabled="busy.inst" @click="rollback"><Icon n="clock" />Откатить на {{ xs.prev }}</button>
                  <span class="grow"></span>
                  <button class="btn btn-p" type="button" :disabled="!ver || busy.inst" @click="install"><span v-if="busy.inst" class="spin"></span><Icon v-else n="download" />Установить {{ ver }}</button>
                </div>
              </div>
            </section>
          </div>
          <section class="i-pn">
            <div class="i-ph"><h2>Журнал Xray</h2><span>последние строки · подробные логи — в разделе <router-link to="/logs">«Логи»</router-link></span></div>
            <div class="i-pb"><pre class="log-box">{{ (xs?.log || []).join('\n') || 'пусто' }}</pre></div>
          </section>
        </template>

        <div v-if="tab === 'geo'" class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>Geo-файлы</h2><span>{{ geo?.lastUpdate ? 'обновлены ' + ago(geo.lastUpdate) : 'не загружались' }}</span></div>
            <div class="i-pb">
              <p class="hint">geoip.dat и geosite.dat нужны для правил по странам и категориям (фильтрация, WARP по сайтам, «РФ напрямую»).</p>
              <div class="list-rows">
                <div v-for="f in geo?.files || []" :key="f.name" class="lr"><span class="lr-t"><b class="i-mono">{{ f.name }}</b><small>{{ f.exists ? `${bytes(f.size)} · ${dt(f.mtime)}` : 'нет файла' }}{{ f.backup ? ' · есть резервная копия' : '' }}</small></span><span class="i-stt" :class="f.exists ? 'ok' : 'bad'">{{ f.exists ? 'есть' : 'нет' }}</span></div>
              </div>
              <div class="fld"><label class="lbl">Источник</label><Opt v-model="gs.source" :options="[{ v: 'loyalsoldier', l: 'Loyalsoldier', s: 'расширенные списки' }, { v: 'runetfreedom', l: 'runetfreedom', s: 'с российскими категориями' }, { v: 'v2fly', l: 'v2fly', s: 'официальные' }]" /></div>
              <div class="ft"><button v-if="(geo?.files || []).some(f => f.backup)" class="btn btn-g" type="button" @click="geoRollback">Откатить</button><span class="grow"></span><button class="btn btn-p" type="button" :disabled="busy.geo" @click="geoUpdate"><span v-if="busy.geo" class="spin"></span><Icon v-else n="download" />Обновить сейчас</button></div>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Автообновление</h2><span class="i-stt" :class="gs.auto ? 'ok' : 'mu'">{{ gs.auto ? 'включено' : 'выключено' }}</span></div>
            <div class="i-pb">
              <TgRow v-model="gs.auto" bold label="Обновлять geo-файлы автоматически" hint="контрольная сумма проверяется, Xray перезапускается мягко" />
              <div class="two">
                <div class="fld"><label class="lbl">Как часто</label><Seg v-model="gs.weekly" :options="[[false, 'ежедневно'], [true, 'раз в неделю']]" /></div>
                <div class="fld"><label class="lbl">Час (МСК)</label><input v-model.number="gs.hour" class="inp" type="number" min="0" max="23"></div>
              </div>
              <div class="ft"><span class="grow"></span><button class="btn btn-p" type="button" @click="geoSave">Сохранить</button></div>
            </div>
          </section>
        </div>

        <div v-if="tab === 'api'" class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>API</h2></div>
            <div class="i-pb">
              <p class="hint">Токен для связки серверов и внешних скриптов: заголовок <code class="i-mono">Authorization: Bearer &lt;токен&gt;</code>.</p>
              <Kr k="Адрес" :v="s.apiUrl" code copyable />
              <Kr k="Токен" :v="s.apiToken" code copyable secret />
              <div class="ft"><span class="grow"></span><button class="btn btn-danger" type="button" @click="newToken">Перевыпустить токен</button></div>
              <div class="i-hint"><Icon n="info" /><span>Пример: <code class="i-mono">curl -H "Authorization: Bearer …" {{ s.apiUrl }}/clients</code></span></div>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Обновление панели</h2><span class="i-stt" :class="upd?.update ? 'wa' : 'ok'">{{ upd ? (upd.update ? 'есть обновление' : 'актуальная') : '—' }}</span></div>
            <div class="i-pb">
              <div class="i-kv"><Kr k="Установлена" :v="'v' + s.version + (s.branch && s.branch !== 'main' ? ' · ветка ' + s.branch : '')" /><Kr k="Последняя" :v="(upd?.latest || s.latestVersion) ? 'v' + (upd?.latest || s.latestVersion) : '—'" /><Kr k="Обновляться из ветки" :v="upd?.branch || s.updateBranch || 'main'" code /><Kr k="Репозиторий" :v="s.repo" code /></div>
              <TgRow v-model="g.autoUpdate" bold label="Обновляться автоматически" hint="проверка раз в сутки; бэкап перед обновлением" @update:model-value="saveGeneral" />
              <div class="ft"><button class="btn btn-g" type="button" :disabled="busy.chk" @click="checkUpd"><span v-if="busy.chk" class="spin"></span>Проверить</button><span class="grow"></span><button class="btn btn-p" type="button" :disabled="!upd?.update || busy.upd" @click="doUpdate"><span v-if="busy.upd" class="spin"></span>Обновить до {{ upd?.latest }}</button></div>
              <p class="hint">Из консоли: <code class="i-mono">kvk update</code> — из запомненной ветки, <code class="i-mono">kvk update ВЕТКА</code> — переключиться на другую ветку (например, <code class="i-mono">main</code>).</p>
            </div>
          </section>
        </div>
      </template>
    </div>
    <Drawer :open="cfg.open" title="config.json Xray" eyebrow="только чтение" width="min(820px, calc(100% - 24px))" @close="cfg.open = false">
      <pre class="log-box" style="max-height:none">{{ cfg.text }}</pre>
      <template #foot><button class="btn" type="button" @click="copy(cfg.text)"><Icon n="copy" />Скопировать</button></template>
    </Drawer>
  </div>
</template>
<script setup>
import '../styles/Settings.css'
import { ref, reactive, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import TgRow from '../components/TgRow.vue'
import Seg from '../components/Seg.vue'
import Opt from '../components/Opt.vue'
import Kr from '../components/Kr.vue'
import Drawer from '../components/Drawer.vue'
import { get, post, put } from '../lib/api.js'
import { ok, fail, confirm, copy } from '../lib/ui.js'
import { uptime, date, dt, ago, bytes, flag } from '../lib/fmt.js'
import { RESET } from '../lib/client.js'
import { loadMeta } from '../lib/meta.js'
import { useRoute } from 'vue-router'
import { openXray } from '../lib/xrayui.js'

const TABS = [['panel', 'Панель'], ['xray', 'Xray'], ['geo', 'Geo-файлы'], ['api', 'API и обновления']]
const FP = ['chrome', 'firefox', 'safari', 'ios', 'edge', 'android', 'random', 'randomized']
const route = useRoute()
const tab = ref(TABS.some(t => t[0] === route.query.tab) ? route.query.tab : 'panel')
const s = ref(null)
const error = ref('')
const pf = reactive({ port: 0, basePath: '', subPath: '', subPort: 0 })
const subDom = ref('')
const g = reactive({ nodeName: '', countryCode: '', defaultFp: 'chrome', showTrafficAll: false, autoCleanDays: 0, newResetPeriod: '', newResetDays: 1, autoUpdate: false })
const xs = ref(null)
const xst = computed(() => xs.value?.status || {})
const vers = ref(null)
const verErr = ref('')
const ver = ref('')
const showPre = ref(false)
const releases = computed(() => (vers.value?.releases || []).filter(r => showPre.value || !r.prerelease))
const geo = ref(null)
const gs = reactive({ source: 'loyalsoldier', auto: false, weekly: false, hour: 4 })
const upd = ref(null)
const busy = reactive({ x: false, ver: false, inst: false, geo: false, chk: false, upd: false })
const cfg = reactive({ open: false, text: '' })
const panelDirty = computed(() => s.value && (pf.port !== s.value.panelPort || pf.basePath !== s.value.basePath || pf.subPath !== s.value.subPath || Number(pf.subPort) !== s.value.subPort))
const rnd = (n) => Array.from(crypto.getRandomValues(new Uint32Array(n)), x => 'abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789'[x % 56]).join('')

async function load() {
  error.value = ''
  try {
    const r = await get('settings'); s.value = r
    Object.assign(pf, { port: r.panelPort, basePath: r.basePath, subPath: r.subPath, subPort: r.subPort })
    subDom.value = r.subCertId || ''
    Object.assign(g, { nodeName: r.nodeName, countryCode: r.countryCode, defaultFp: r.defaultFp || 'chrome', showTrafficAll: r.showTrafficAll, autoCleanDays: r.autoCleanDays, newResetPeriod: r.newResetPeriod || '', newResetDays: r.newResetDays || 1, autoUpdate: r.autoUpdate })
  } catch (e) { error.value = e.message }
}
async function savePanel() {
  if (!await confirm('Сменить адрес панели?', 'Панель перезапустится. Запишите новый адрес — по старому она откроется как nginx 404.', { okText: 'Сменить' })) return
  try {
    const r = await post('settings/panel', { port: Number(pf.port), basePath: pf.basePath.trim(), subPath: pf.subPath.trim(), subPort: Number(pf.subPort) || 0 })
    if (r.restart) { ok('Перезапуск… откроем ' + r.url); setTimeout(() => { location.href = r.url }, 3000) } else ok('Без изменений')
  } catch (e) { fail(e) }
}
async function saveSubDom() {
  try { const r = await put('settings/subdomain', { id: subDom.value }); s.value.subCertId = subDom.value; s.value.subUrl = r.subUrl; ok('Ссылки подписки: ' + r.subUrl) } catch (e) { fail(e) }
}
async function saveGeneral() {
  try { await put('settings', { ...g, countryCode: g.countryCode.toUpperCase(), autoCleanDays: Number(g.autoCleanDays) || 0, newResetDays: Number(g.newResetDays) || 0 }); ok('Сохранено'); loadMeta() } catch (e) { fail(e) }
}
async function resetAll() {
  if (!await confirm('Применить ко всем?', 'У всех клиентов период сброса трафика станет таким, как выбран для новых (сначала сохраните).', { okText: 'Применить' })) return
  try { const r = await post('settings/reset-all'); ok(`Применено к ${r.count} клиентам`) } catch (e) { fail(e) }
}
async function loadXray() { try { xs.value = await get('xray/status') } catch {} }
async function xAct(a) { busy.x = true; try { await post('xray/' + a); ok({ restart: 'Xray перезапущен', stop: 'Xray остановлен', start: 'Xray запущен' }[a]); loadXray(); loadMeta() } catch (e) { fail(e) } finally { busy.x = false } }
async function loadVersions() { busy.ver = true; verErr.value = ''; try { vers.value = await get('xray/versions') } catch (e) { verErr.value = e.message } finally { busy.ver = false } }
async function install() {
  if (!await confirm(`Установить Xray ${ver.value}?`, 'Xray перезапустится — подключения клиентов прервутся на пару секунд.', { okText: 'Установить' })) return
  busy.inst = true
  try { const r = await post('xray-install', { version: ver.value }); ok('Установлена ' + (r.version || ver.value)); ver.value = ''; loadXray(); loadVersions(); loadMeta() } catch (e) { fail(e) } finally { busy.inst = false }
}
async function rollback() {
  if (!await confirm(`Откатить Xray на ${xs.value.prev}?`, 'Xray перезапустится.', { okText: 'Откатить' })) return
  busy.inst = true
  try { await post('xray-rollback'); ok('Откат выполнен'); loadXray(); loadMeta() } catch (e) { fail(e) } finally { busy.inst = false }
}
async function showConfig() { try { const r = await get('xray-config'); cfg.text = typeof r === 'string' ? r : JSON.stringify(r, null, 2); cfg.open = true } catch (e) { fail(e) } }
async function loadGeo() { try { const r = await get('geo'); geo.value = r; Object.assign(gs, { source: r.source || 'loyalsoldier', auto: r.auto, weekly: r.weekly, hour: r.hour }) } catch {} }
async function geoUpdate() { busy.geo = true; try { await post('geo/update', { source: gs.source }); ok('Geo-файлы обновлены'); loadGeo() } catch (e) { fail(e) } finally { busy.geo = false } }
async function geoRollback() { try { await post('geo/rollback'); ok('Восстановлены прежние файлы'); loadGeo() } catch (e) { fail(e) } }
async function geoSave() { try { await put('geo', { ...gs, hour: Number(gs.hour) || 0 }); ok('Сохранено'); loadGeo() } catch (e) { fail(e) } }
async function newToken() {
  if (!await confirm('Перевыпустить токен?', 'Связка серверов и скрипты со старым токеном перестанут работать — на основном сервере нужно будет вставить новый.', { danger: true, okText: 'Перевыпустить' })) return
  try { const r = await post('settings/token'); s.value.apiToken = r.apiToken; ok('Новый токен создан') } catch (e) { fail(e) }
}
async function checkUpd() { busy.chk = true; try { upd.value = await get('panel/update') } catch (e) { fail(e) } finally { busy.chk = false } }
async function doUpdate() {
  if (!await confirm(`Обновить панель до ${upd.value.latest}?`, 'Будет сделан бэкап, затем панель скачает новую версию и перезапустится (около минуты). Xray продолжит работать.', { okText: 'Обновить' })) return
  busy.upd = true
  try { await post('panel/update'); ok('Обновление запущено — страница перезагрузится'); setTimeout(() => location.reload(), 60000) } catch (e) { fail(e); busy.upd = false }
}
watch(tab, (t) => { if (t === 'xray') { loadXray(); if (!vers.value) loadVersions() } if (t === 'geo') loadGeo(); if (t === 'api' && !upd.value) checkUpd() })
let timer
onMounted(() => { load(); loadXray(); if (tab.value === 'xray') loadVersions(); timer = setInterval(() => { if (tab.value === 'xray') loadXray() }, 5000) })
onBeforeUnmount(() => clearInterval(timer))
</script>
