<template>
  <div class="pg-inbounds">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="vw-eb">{{ srvName }} · xray {{ xr.version || '—' }} · {{ pn(list.length, 'инбаунд', 'инбаунда', 'инбаундов') }}</span><h1>Инбаунды</h1></div>
        <div class="i-hd-a"><router-link class="btn" to="/site"><Icon n="bolt" />Автонастройка VLESS</router-link><button class="btn btn-p" type="button" @click="openForm(null)"><Icon n="plus" />Новый инбаунд</button></div>
      </div>
      <ServerPicker />
      <Loading v-if="!data" :error="error" @retry="reload" />
      <template v-else>
        <div class="i-mt">
          <Metric label="Инбаунды" :value="list.length" :pct="list.length ? enabled / list.length * 100 : 0" tone="ok" :note="`включено ${enabled} из ${list.length}`" />
          <Metric label="Клиенты в инбаундах" :value="remote ? list.reduce((m, i) => Math.max(m, i.clients), 0) : meta.clients" :note="`${list.reduce((s, i) => s + i.clients, 0)} подключений — у клиента своё в каждом`" />
          <Metric label="Онлайн" :value="list.reduce((s, i) => s + (i.online || 0), 0)" note="подключений за последние 2 минуты" />
          <Metric label="Xray" :value="xr.version || '—'" :note="xr.running ? 'изменения применяются сразу, без разрыва остальных клиентов' : (xr.error || 'не запущен')" :note-tone="xr.running ? '' : 'bad'" />
        </div>
        <section class="i-pn">
          <div class="i-ph"><h2>Все инбаунды <span class="i-cnt">{{ list.length }}</span></h2><span>трафик — с момента создания</span></div>
          <div class="vw-ord"><Icon n="info" /><span><b>Порядок строк — это порядок конфигов в подписке клиентов и в «Автовыборе»:</b> <span class="vw-mu">меняйте его стрелками у строки.</span></span></div>
          <div class="vw-tl">
            <div class="vw-tl-in">
              <div class="vw-ib vw-th"><span>Протокол</span><span>Название</span><span>Порт</span><span>Транспорт</span><span>Клиенты</span><span>Статус</span><span></span></div>
              <div v-for="(i, idx) in list" :key="i.id" class="vw-row vw-hasmv" :class="{ open: openId === i.id }">
                <div class="vw-ib">
                  <span class="vw-pr">
                    <span class="vw-mv"><button class="btn btn-g btn-ic" type="button" :disabled="idx === 0" title="Выше" @click="move(i, -1)"><Icon n="up" /></button><button class="btn btn-g btn-ic" type="button" :disabled="idx === list.length - 1" title="Ниже" @click="move(i, 1)"><Icon n="down" /></button></span>
                    <span class="vw-ic"><Icon :n="i.protocol === 'amneziawg' ? 'shield' : ['hysteria2', 'tuic'].includes(i.protocol) ? 'bolt' : i.security === 'reality' ? 'shield' : i.security === 'tls' ? 'lock' : 'inb'" /></span>
                    <span class="vw-id"><b>{{ PROTO[i.protocol] }}</b><small>{{ secLabel(i) }}</small></span>
                  </span>
                  <span class="vw-c vw-strong">{{ i.remark }}</span>
                  <span class="vw-c i-mono">{{ i.port }}</span>
                  <span class="vw-c">{{ netLabel(i) }}</span>
                  <span class="vw-c"><b class="vw-strong">{{ i.clients }}</b><span v-if="i.online" class="vw-mu"> · {{ i.online }} онлайн</span></span>
                  <span class="vw-br"></span>
                  <span class="vw-st"><button class="i-tg" :class="{ on: i.enabled }" type="button" :title="i.enabled ? 'Выключить' : 'Включить'" @click="toggle(i)"><span></span></button><span class="i-stt" :class="i.embError && i.enabled ? 'bad' : i.enabled ? 'ok' : 'mu'" :title="i.embError ? 'Не запустился: ' + i.embError : ''">{{ i.embError && i.enabled ? 'ошибка' : i.enabled ? 'вкл' : 'выкл' }}</span></span>
                  <span class="vw-ra"><button class="btn btn-sm btn-g" type="button" @click="openForm(i)"><Icon n="edit" />Изменить</button><button class="btn btn-sm btn-g btn-ic" type="button" :title="openId === i.id ? 'Свернуть' : 'Подробнее'" @click="openId = openId === i.id ? '' : i.id"><Icon :n="openId === i.id ? 'up' : 'chevD'" /></button></span>
                </div>
                <div v-if="openId === i.id" class="vw-exp">
                  <div class="vw-xc">
                    <span class="vw-xt">Параметры</span>
                    <div class="i-kv vw-kvb">
                      <div class="i-kr vw-k2"><span>тег</span><code>{{ i.tag }}</code></div>
                      <template v-if="i.security === 'reality'">
                        <div class="i-kr vw-k2"><span>маскировка (dest)</span><code>{{ i.realityTarget }}</code></div>
                        <div class="i-kr vw-k2"><span>SNI</span><code>{{ (i.realitySni || []).join(', ') || i.realityTarget.split(':')[0] }}</code></div>
                        <div class="i-kr vw-k2"><span>ключ (pbk)</span><code>{{ i.publicKey }}</code></div>
                        <div class="i-kr vw-k2"><span>shortId</span><code>{{ (i.shortIds || []).join(', ') }}</code></div>
                      </template>
                      <div v-if="i.certName" class="i-kr vw-k2"><span>сертификат</span><code>{{ i.certName }}</code></div>
                      <div v-if="i.flow" class="i-kr vw-k2"><span>flow</span><code>{{ i.flow }}</code></div>
                      <div v-if="i.path" class="i-kr vw-k2"><span>путь</span><code>{{ i.path }}</code></div>
                      <div v-if="i.serviceName" class="i-kr vw-k2"><span>serviceName</span><code>{{ i.serviceName }}</code></div>
                      <div v-if="i.ssMethod" class="i-kr vw-k2"><span>метод</span><code>{{ i.ssMethod }}</code></div>
                      <div v-if="i.obfsPassword" class="i-kr vw-k2"><span>obfs</span><code>salamander</code></div>
                      <div v-if="i.embError && i.enabled" class="i-kr vw-k2"><span>ошибка запуска</span><code class="bad-t">{{ i.embError }}</code></div>
                      <template v-if="i.protocol === 'tuic'">
                        <div class="i-kr vw-k2"><span>контроль перегрузки</span><code>{{ i.tuicCongestion }}</code></div>
                        <div class="i-kr vw-k2"><span>UDP relay</span><code>{{ i.tuicUdpMode }}{{ i.tuicZeroRtt ? ' · 0-RTT' : '' }}</code></div>
                      </template>
                      <template v-if="i.protocol === 'masque'">
                        <div class="i-kr vw-k2"><span>HTTP</span><code>{{ masqueNet(i) }}</code></div>
                        <div class="i-kr vw-k2"><span>пул адресов</span><code>{{ (i.masquePool || []).join(', ') }}</code></div>
                      </template>
                      <template v-if="i.protocol === 'amneziawg' && i.awg">
                        <div class="i-kr vw-k2"><span>подсеть</span><code>{{ i.awg.network }}</code></div>
                        <div class="i-kr vw-k2"><span>публичный ключ</span><code>{{ i.awg.publicKey }}</code></div>
                        <div class="i-kr vw-k2"><span>Jc / Jmin / Jmax</span><code>{{ i.awg.jc }} / {{ i.awg.jmin }} / {{ i.awg.jmax }}</code></div>
                        <div class="i-kr vw-k2"><span>S1 / S2</span><code>{{ i.awg.s1 }} / {{ i.awg.s2 }}</code></div>
                      </template>
                      <div class="i-kr vw-k2"><span>трафик</span><code>↑ {{ bytes(i.up) }} · ↓ {{ bytes(i.down) }}</code></div>
                    </div>
                  </div>
                  <div class="vw-xc">
                    <span class="vw-xt">Клиенты инбаунда · {{ i.clients }}</span>
                    <div class="vw-cl">
                      <div v-for="c in (i.clientList || []).slice(0, showAll[i.id] ? 9999 : 8)" :key="c.id" class="vw-xr"><span><b>{{ c.name }}</b> <span class="vw-mu">{{ c.online ? 'онлайн' : STATUS[c.status]?.[0] }}</span></span><button v-if="!remote" class="btn btn-sm btn-g" type="button" @click="removeClient(i, c)">Убрать</button></div>
                      <button v-if="(i.clientList || []).length > 8 && !showAll[i.id]" class="btn btn-sm btn-g" type="button" @click="showAll[i.id] = true">Показать всех ({{ i.clientList.length }})</button>
                    </div>
                    <div v-if="!remote" class="vw-add"><input v-model="addName[i.id]" class="inp" placeholder="имя клиента (пусто = новый)" @keydown.enter="addClient(i)"><button class="btn btn-sm" type="button" @click="addClient(i)"><Icon n="plus" />Добавить</button></div>
                    <p v-else class="hint">Клиенты живут на основном сервере: доступ к этому инбаунду выдаётся в карточке клиента («Изменить» → «Инбаунды») или кнопками «Выдать всем» / «Забрать у всех».</p>
                  </div>
                  <div class="vw-xc vw-xa">
                    <span class="vw-xt">Действия</span>
                    <button class="btn btn-sm" type="button" @click="exampleLink(i)">Скопировать пример ссылки</button>
                    <button class="btn btn-sm" type="button" :disabled="!!i.front || i.auto === 'cdn'" :title="i.front || i.auto === 'cdn' ? 'Работает за сайтом-прикрытием — на другом сервере запустите «Автонастройку»' : 'Создать такой же инбаунд на другом сервере связки'" @click="openCopy(i)"><Icon n="copy" />Копировать на сервер</button>
                    <button class="btn btn-sm" type="button" @click="grant(i, true)">Выдать всем</button>
                    <button class="btn btn-sm btn-g" type="button" @click="grant(i, false)">Забрать у всех</button>
                    <button class="btn btn-sm btn-g vw-bad" type="button" @click="remove(i)">Удалить инбаунд</button>
                  </div>
                </div>
              </div>
              <Empty v-if="!list.length" icon="inb" title="Инбаундов пока нет" text="Начните с VLESS Reality на порту 443 — он лучше всего проходит блокировки">
                <div class="row"><router-link class="btn btn-sm btn-p" to="/site"><Icon n="bolt" />Настроить автоматически</router-link><button class="btn btn-sm" type="button" @click="openForm(null)"><Icon n="plus" />Новый инбаунд</button></div>
              </Empty>
            </div>
          </div>
        </section>
        <div class="i-alrt info"><Icon n="info" /><span><b>Новый Xray не запускает VLESS без защиты к публичному серверу:</b> <span class="muted">для прямого подключения — Reality (маскировка под сайт) или TLS. После смены порта, защиты или obfs клиентам нужно обновить подписку.</span></span><router-link class="btn btn-sm" to="/sub">Настроить подписку</router-link></div>
      </template>
    </div>
    <Drawer :open="cp.open" title="Копировать инбаунд" :sub="cp.inbound ? cp.inbound.remark : ''" eyebrow="инбаунды" @close="cp.open = false">
      <div v-if="cp.inbound" class="col">
        <p class="note">На выбранных серверах появится такой же инбаунд: протокол, транспорт, порт и настройки. То, что принадлежит серверу «{{ srvName }}», заменится своим: ключи Reality и AmneziaWG будут новые, маской Reality станет сайт сервера, а сертификат — домен этого сервера. Если порт там занят, будет выбран свободный. Название «{{ srvName }} …» поменяется на имя нового сервера.</p>
        <div class="fld">
          <label class="lbl">На какие серверы</label>
          <div class="cpy-l">
            <button v-for="t in copyTargets" :key="t.id || 'self'" type="button" class="cpy-s" :class="{ on: cp.to.includes(t.id), bad: t.bad }" @click="toggleTo(t.id)">
              <span class="cpy-c"><Icon n="check" /></span>
              <span class="cpy-n"><b>{{ t.name }}</b><small>{{ t.note }}</small></span>
            </button>
          </div>
        </div>
        <TgRow v-model="cp.grant" bold label="Выдать тем же клиентам" :hint="cp.grant ? `копия достанется тем, у кого есть «${cp.inbound.remark}» (${pn(cp.inbound.clients, 'клиент', 'клиента', 'клиентов')})` : 'копию не получит никто — выдайте её в карточке клиента'" />
        <div v-if="cp.results.length" class="list-rows">
          <div v-for="r in cp.results" :key="r.server + r.name" class="lr" style="flex-wrap:wrap">
            <span class="lr-t"><b>{{ r.name || r.server }}</b><small :class="r.ok ? '' : 'bad-t'">{{ r.ok ? `создан «${r.remark}» на порту ${r.port}` : r.error }}<template v-if="r.warning"> · {{ r.warning }}</template></small></span>
            <span class="i-stt" :class="r.ok ? (r.warning ? 'wa' : 'ok') : 'bad'">{{ r.ok ? 'готово' : 'ошибка' }}</span>
          </div>
        </div>
      </div>
      <template #foot>
        <button class="btn" type="button" @click="cp.open = false">{{ cp.results.length ? 'Закрыть' : 'Отмена' }}</button>
        <button class="btn btn-p" type="button" :disabled="busy.copy || !cp.to.length" @click="doCopy"><span v-if="busy.copy" class="spin"></span><Icon v-else n="copy" />Скопировать{{ cp.to.length > 1 ? ` (${cp.to.length})` : '' }}</button>
      </template>
    </Drawer>
    <Drawer :open="form.open" :title="form.inbound ? 'Изменить инбаунд' : 'Новый инбаунд'" :sub="form.inbound ? form.inbound.remark : ''" eyebrow="инбаунды" width="min(640px, calc(100% - 24px))" @close="form.open = false">
      <InboundForm v-if="form.open" ref="fref" :inbound="form.inbound" :edit="!!form.inbound" />
      <template #foot>
        <button class="btn" type="button" @click="form.open = false">Отмена</button>
        <button class="btn btn-p" type="button" :disabled="busy.save" @click="save"><span v-if="busy.save" class="spin"></span>{{ form.inbound ? 'Сохранить' : 'Создать' }}</button>
      </template>
    </Drawer>
  </div>
</template>
<script setup>
import '../styles/Inbounds.css'
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import Empty from '../components/Empty.vue'
import Metric from '../components/Metric.vue'
import Drawer from '../components/Drawer.vue'
import InboundForm from '../components/InboundForm.vue'
import ServerPicker from '../components/ServerPicker.vue'
import TgRow from '../components/TgRow.vue'
import { post } from '../lib/api.js'
import { srv, sget as get, spost, sput as put, sdel as del, srvName, remote } from '../lib/server.js'
import { meta, loadMeta } from '../lib/meta.js'
import { useLoad } from '../lib/useLoad.js'
import { ok, fail, confirm, copy } from '../lib/ui.js'
import { pn, bytes, PROTO, NET, flag } from '../lib/fmt.js'
import { STATUS } from '../lib/client.js'

const route = useRoute()
const router = useRouter()
const { data, error, reload } = useLoad(() => get('inbounds'), { interval: 15000 })
watch(() => srv.id, () => { data.value = null; error.value = ''; openId.value = ''; reload() })
const list = computed(() => data.value?.inbounds || [])
const xr = computed(() => data.value?.xray || {})
const enabled = computed(() => list.value.filter(i => i.enabled).length)
const openId = ref('')
const addName = reactive({})
const showAll = reactive({})
const form = reactive({ open: false, inbound: null })
const fref = ref(null)
const busy = reactive({ save: false, copy: false })
const cp = reactive({ open: false, inbound: null, to: [], grant: true, results: [] })
const copyTargets = computed(() => [
  ...(srv.id ? [{ id: '', name: `${flag(meta.country)} ${srv.self || 'Этот сервер'}`.trim(), note: 'этот сервер' }] : []),
  ...srv.nodes.filter(n => n.id !== srv.id).map(n => ({ id: n.id, name: `${flag(n.country)} ${n.name}`.trim(), note: n.lastError ? 'нет связи: ' + n.lastError : 'v' + (n.version || '?'), bad: !!n.lastError })),
  { id: srv.id, name: `${srvName.value}`, note: 'этот же сервер — копия с другим портом' },
])
function openCopy(i) { Object.assign(cp, { open: true, inbound: i, to: [], grant: true, results: [] }); if (copyTargets.value.length === 2) cp.to = [copyTargets.value[0].id] }
const toggleTo = (id) => { cp.to = cp.to.includes(id) ? cp.to.filter(x => x !== id) : [...cp.to, id] }
async function doCopy() {
  busy.copy = true
  try {
    const r = await post('inbounds-copy', { from: srv.id, id: cp.inbound.id, to: cp.to, grant: cp.grant })
    cp.results = r.results || []
    const bad = cp.results.filter(x => !x.ok).length
    if (r.copied) ok(`Скопировано: ${r.copied}` + (bad ? `, ошибок: ${bad}` : ''))
    else fail('Не скопировано — причины в списке')
    cp.to = cp.to.filter(id => cp.results.some(x => x.server === id && !x.ok))
    await reload(true); loadMeta()
  } catch (e) { fail(e) } finally { busy.copy = false }
}
const netLabel = (i) => ({ hysteria2: 'QUIC', tuic: 'QUIC', amneziawg: 'UDP', masque: masqueNet(i) })[i.protocol] || NET[i.network]
const masqueNet = (i) => { const a = i.alpn || []; const h2 = a.includes('h2'), h3 = a.includes('h3') || !h2; return h2 && h3 ? 'HTTP/3 + HTTP/2' : h2 ? 'HTTP/2' : 'HTTP/3' }
const secLabel = (i) => i.protocol === 'tuic' ? `TLS · ${i.tuicCongestion || 'bbr'}` : i.protocol === 'masque' ? 'TLS · CONNECT-IP' : i.protocol === 'amneziawg' ? 'обфускация AmneziaWG' : i.protocol === 'hysteria2' ? `TLS${i.obfsPassword ? ' · salamander' : ''}` : i.security === 'reality' ? 'Reality · маскировка под сайт' : i.security === 'tls' ? 'TLS' : 'без защиты'

function openForm(i) { form.inbound = i ? JSON.parse(JSON.stringify(i)) : null; form.open = true }
async function save() {
  busy.save = true
  try {
    const b = fref.value.body()
    if (form.inbound) { await put('inbounds/' + form.inbound.id, b); ok('Инбаунд сохранён') } else { await spost('inbounds' + (fref.value.grant() ? '?grant=1' : ''), b); ok('Инбаунд создан' + (remote.value ? ' на сервере «' + srvName.value + '»' : '')) }
    form.open = false; await reload(true); loadMeta()
  } catch (e) { fail(e) } finally { busy.save = false }
}
async function run(fn, msg) { try { await fn(); if (msg) ok(msg); await reload(true) } catch (e) { fail(e) } }
const toggle = (i) => run(() => spost(`inbounds/${i.id}/toggle`), i.enabled ? 'Выключен' : 'Включён')
const move = (i, dir) => run(() => spost(`inbounds/${i.id}/move`, { dir }))
// clients of linked servers live here, so their access is granted on this panel
const grantCall = (i, all) => remote.value ? post(`nodes/${srv.id}/grant`, { inbound: i.id, all }) : spost(`inbounds/${i.id}/grant`, { all })
const grant = async (i, all) => { if (all || await confirm(`Забрать «${i.remark}» у всех клиентов?`, 'Подключение пропадёт из подписок после обновления.', { danger: true, okText: 'Забрать' })) run(async () => { await grantCall(i, all); if (remote.value) await new Promise(r => setTimeout(r, 1500)) }, all ? 'Выдано всем клиентам' : 'Забрано у всех') }
const removeClient = (i, c) => run(() => del(`inbounds/${i.id}/clients/${c.id}`), `${c.name} убран из инбаунда`)
async function addClient(i) { const name = (addName[i.id] || '').trim(); await run(() => spost(`inbounds/${i.id}/clients`, { name }), 'Клиент добавлен'); addName[i.id] = ''; loadMeta() }
async function exampleLink(i) { try { const r = await get(`inbounds/${i.id}/link`); copy(r.link, 'Пример ссылки скопирован') } catch (e) { fail(e) } }
async function remove(i) {
  if (!await confirm(`Удалить «${i.remark}»?`, `Порт ${i.port} освободится, у ${pn(i.clients, 'клиента', 'клиентов', 'клиентов')} пропадёт это подключение.`, { danger: true, okText: 'Удалить' })) return
  run(() => del('inbounds/' + i.id), 'Инбаунд удалён').then(loadMeta)
}
onMounted(() => { if (route.query.new) { openForm(null); router.replace({ query: {} }) } })
</script>
<style>
.h1 .cpy-l{display:flex;flex-direction:column;gap:6px}
.h1 .cpy-s{display:flex;align-items:center;gap:12px;width:100%;padding:10px 12px;border:1px solid var(--line);border-radius:var(--r-md,10px);background:var(--surface);color:var(--text);font:inherit;text-align:left;cursor:pointer}
.h1 .cpy-s:hover{border-color:var(--faint)}
.h1 .cpy-s.on{border-color:var(--accent);background:var(--surface2)}
.h1 .cpy-c{flex:none;width:18px;height:18px;border-radius:var(--r-xs,5px);border:1.5px solid var(--faint);display:inline-flex;align-items:center;justify-content:center;color:transparent}
.h1 .cpy-c .ic{width:12px;height:12px}
.h1 .cpy-s.on .cpy-c{background:#5b54ee;border-color:#5b54ee;color:#fff}
.h1 .cpy-n{display:flex;flex-direction:column;min-width:0;line-height:1.3}
.h1 .cpy-n b{color:var(--strong);font-weight:600}
.h1 .cpy-n small{color:var(--muted);font-size:var(--fs-xs);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.h1 .cpy-s.bad .cpy-n small{color:var(--bad)}
</style>
