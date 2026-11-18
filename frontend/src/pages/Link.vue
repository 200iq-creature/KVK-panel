<template>
  <div class="pg-link">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="vw-eb">несколько серверов — одна подписка</span><h1>Связка серверов</h1><p>Этот сервер — основной: клиенты и сроки создаются здесь и сами расходятся по остальным серверам. В подписке клиента — конфиги всех серверов, трафик суммируется.</p></div>
        <div class="i-hd-a">
          <button class="btn" type="button" :disabled="busy.sync" @click="sync"><span v-if="busy.sync" class="spin"></span><Icon v-else n="refresh" />Синхронизировать</button>
          <button class="btn btn-p" type="button" @click="add.open = true"><Icon n="plus" />Добавить сервер</button>
        </div>
      </div>
      <Loading v-if="!data" :error="error" @retry="reload" />
      <template v-else>
        <div v-for="n in data.nodes.filter(x => x.enabled && (x.drift || x.replaced))" :key="'d' + n.id" class="i-alrt wa">
          <Icon n="warn" />
          <span v-if="n.replaced"><b>«{{ n.name }}» — новый сервер.</b> <span class="muted">Адрес или ключ сменились, а на основном сохранена конфигурация старого сервера: инбаунды, домены, сайт, фильтрация, WARP. Восстановите её на новом сервере — клиентам ничего менять не придётся.</span></span>
          <span v-else><b>Конфигурация «{{ n.name }}» изменилась не через основной сервер.</b> <span class="muted">Она отличается от сохранённой копии. Оставьте новую или верните сохранённую.</span></span>
          <button class="btn btn-sm btn-p" type="button" @click="openProf(n)">Разобраться</button>
        </div>
        <div v-if="data.main" class="i-alrt wa"><Icon n="link" /><span><b>Этот сервер подчинён основному «{{ data.main.name }}»</b> <span class="muted">— клиенты приходят оттуда (последняя синхронизация {{ ago(data.main.lastSync) }}). Менять их лучше на основном.</span></span><button class="btn btn-sm" type="button" @click="detach">Отвязать</button></div>
        <div class="i-mt">
          <Metric label="Серверов в связке" :value="data.nodes.length + 1" :note="`этот + ${data.nodes.length}`" />
          <Metric label="Недоступны" :value="data.offline" :note="data.offline ? 'проверьте ошибки ниже' : 'все отвечают'" :note-tone="data.offline ? 'bad' : 'ok'" />
          <Metric label="Клиентов" :value="data.self.clients" note="на этом сервере, расходятся на все" />
          <Metric label="Синхронизация" :value="data.lastSync ? ago(data.lastSync) : '—'" note="автоматически каждую минуту" />
        </div>
        <section class="i-pn">
          <div class="i-ph"><h2>Серверы <span class="i-cnt">{{ data.nodes.length + 1 }}</span></h2><span>пинг — от этого сервера</span></div>
          <div class="tbl-w">
            <table class="tbl">
              <thead><tr><th>Сервер</th><th>Адрес</th><th>Клиенты</th><th>Инбаунды</th><th>CPU · пинг</th><th>Синхр.</th><th>Статус</th><th></th></tr></thead>
              <tbody>
                <tr>
                  <td><b>{{ flag(data.self.country) }} {{ data.self.name }}</b> <span class="badge b-info">этот</span></td>
                  <td class="i-mono" style="font-size:12px;max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" :title="data.self.url">{{ data.self.url }}</td>
                  <td>{{ data.self.clients }}</td><td>{{ data.self.inbounds }}</td><td>—</td><td>—</td>
                  <td><span class="i-stt ok">{{ data.main ? 'нода' : 'основной' }}</span></td><td></td>
                </tr>
                <tr v-for="n in data.nodes" :key="n.id">
                  <td><b>{{ flag(n.country) }} {{ n.name }}</b><div class="muted" style="font-size:12px">v{{ n.version || '?' }}<template v-if="n.xray"> · Xray {{ n.xray }}</template></div></td>
                  <td class="i-mono" style="font-size:12px;max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" :title="n.url">{{ n.url }}</td>
                  <td>{{ n.clients }}</td><td>{{ n.inbounds }}</td>
                  <td class="nw">{{ n.cpu ? Math.round(n.cpu) + '%' : '—' }} · {{ n.pingMs ? n.pingMs + ' мс' : '—' }}</td>
                  <td class="nw">{{ n.lastSync ? ago(n.lastSync) : '—' }}</td>
                  <td>
                    <span class="i-stt" :class="!n.enabled ? 'mu' : n.lastError ? 'bad' : 'ok'" :title="n.lastError">{{ !n.enabled ? 'выключен' : n.lastError ? 'ошибка' : 'в связке' }}</span>
                    <div class="lk-cf"><span v-if="n.replaced" class="badge b-warn">новый сервер</span><span v-else-if="n.drift" class="badge b-warn">конфиг изменён</span><span v-else-if="n.hasProfile" class="badge b-mute" :title="'копия от ' + dt(n.profileAt)">копия {{ ago(n.profileAt) }}</span><span v-else class="badge b-mute">нет копии</span></div>
                  </td>
                  <td class="r"><span class="acts">
                    <button class="btn btn-sm btn-g btn-ic" :class="{ 'lk-att': n.drift || n.replaced }" type="button" title="Сохранённая конфигурация: восстановить, скачать" aria-label="Конфигурация" @click="openProf(n)"><Icon n="db" /></button>
                    <button class="btn btn-sm btn-g btn-ic" type="button" title="Изменить: название, адрес/ключ, токен" aria-label="Изменить" @click="openEdit(n)"><Icon n="edit" /></button>
                  </span></td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="data.nodes.length" class="i-pb" style="padding-top:0"><div class="i-hint"><Icon n="info" /><span>Инбаунды, домены, сайт, фильтрация и WARP подключённых серверов настраиваются прямо здесь — выберите сервер вверху этих разделов, входить в его панель не нужно. Основной хранит копию конфигурации каждого сервера: если сервер заблокируют, поставьте панель на новый, вставьте его ключ в «Изменить» и нажмите «Восстановить». Инбаунды подключённых серверов при первой синхронизации выдаются всем клиентам и сразу попадают в их подписки. Кому какие оставить, выбирается в карточке клиента → «Инбаунды» (группа «Сервер «…»») или в боте-магазине для новых клиентов. Новый инбаунд на сервере получают клиенты, у которых уже есть доступ к этому серверу.</span></div></div>
          <div v-for="n in data.nodes.filter(x => x.enabled && x.lastError)" :key="'e' + n.id" class="i-pb" style="padding-top:0"><div class="i-hint bad"><Icon n="warn" /><span><b>{{ n.name }}:</b> {{ n.lastError }}</span></div></div>
        </section>
        <div class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>Ключ этого сервера</h2><span>для подключения к основному</span></div>
            <div class="i-pb">
              <p class="hint">Чтобы сделать этот сервер нодой другого — вставьте ключ на основном сервере в «Добавить сервер». Ключ содержит адрес панели и API-токен: не публикуйте его.</p>
              <div class="vw-link"><code>{{ showKey ? data.linkKey : '•'.repeat(48) }}</code><button class="i-cp" type="button" @click="showKey = !showKey"><Icon n="eye" /></button><button class="i-cp" type="button" @click="copy(data.linkKey, 'Ключ скопирован')"><Icon n="copy" /></button></div>
              <p class="hint">Сменить токен (и ключ) можно в «Настройках» → API. В консоли ключ показывает команда <code>kvk apikey</code>.</p>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Как это работает</h2></div>
            <div class="i-pb">
              <ol class="steps">
                <li>Установите KVK-панель на второй сервер тем же скриптом.</li>
                <li>На нём откройте «Связка серверов» и скопируйте ключ.</li>
                <li>Здесь нажмите «Добавить сервер» и вставьте ключ. Инбаунды второго сервера появятся в подписках клиентов.</li>
                <li>Клиенты, сроки, лимиты и баны синхронизируются раз в минуту. Трафик с нод суммируется в лимит клиента.</li>
              </ol>
            </div>
          </section>
        </div>
      </template>
    </div>
    <Drawer :open="ed.open" :title="ed.node ? `Сервер «${ed.node.name}»` : ''" eyebrow="связка серверов" @close="ed.open = false">
      <div v-if="ed.node" class="col">
        <div class="fld"><label class="lbl">Название</label><input v-model="ed.name" class="inp"></div>
        <TgRow v-model="ed.enabled" bold label="Сервер в связке" hint="выключенный сервер не синхронизируется, его конфиги пропадают из подписок" />
        <div class="fld"><label class="lbl">Ключ связки или адрес панели</label><textarea v-model="ed.url" class="inp mono" rows="3" :placeholder="ed.node.url"></textarea><span class="hint">Пусто — оставить адрес <span class="i-mono">{{ ed.node.url }}</span>. Вставьте ключ нового сервера (<code>kvk apikey</code> на нём), если старый заблокирован, — затем восстановите на нём конфигурацию.</span></div>
        <div v-if="!ed.url.trim() || ed.url.trim().startsWith('http')" class="fld"><label class="lbl">API-токен</label><input v-model="ed.token" class="inp mono" placeholder="пусто — оставить текущий"></div>
        <div v-if="ed.url.trim() && ed.node.hasProfile" class="i-hint"><Icon n="info" /><span>Сохранённая конфигурация «{{ ed.node.name }}» останется на основном — после смены адреса её можно будет восстановить на новом сервере одной кнопкой.</span></div>
      </div>
      <template #foot>
        <button class="btn btn-d" type="button" style="margin-right:auto" @click="remove(ed.node)"><Icon n="trash" />Убрать из связки</button>
        <button class="btn" type="button" @click="ed.open = false">Отмена</button>
        <button class="btn btn-p" type="button" :disabled="busy.ed" @click="saveEdit"><span v-if="busy.ed" class="spin"></span>Сохранить</button>
      </template>
    </Drawer>
    <Drawer :open="pf.open" :title="pf.node ? `Конфигурация «${pf.node.name}»` : ''" eyebrow="копия на основном сервере" width="min(640px, calc(100% - 24px))" @close="pf.open = false">
      <Loading v-if="!pf.data" :error="pf.error" @retry="loadProf" />
      <div v-else class="col">
        <div v-if="pf.data.replaced" class="i-hint wa"><Icon n="warn" /><span><b>Адрес сервера сменился.</b> Восстановите сохранённую конфигурацию на новом сервере — инбаунды, домены с сертификатами, сайт, фильтрация, WARP и версия Xray. Клиенты получат её при следующей синхронизации, ссылки в подписках обновятся сами.</span></div>
        <div v-else-if="pf.data.drift" class="i-hint wa"><Icon n="warn" /><span><b>Сервер изменили напрямую.</b> Его текущая конфигурация отличается от сохранённой. «Оставить как на сервере» — сохранить текущую как новую копию; «Восстановить» — вернуть сохранённую.</span></div>
        <div v-else-if="pf.data.profile" class="i-hint"><Icon n="check" /><span>Конфигурация на сервере совпадает с сохранённой.</span></div>
        <div v-if="!pf.data.profile" class="i-hint"><Icon n="info" /><span>Копия ещё не сохранена — она появится при синхронизации. Если сервер на старой версии, обновите на нём панель: <code>kvk update</code>.</span></div>
        <template v-if="pf.data.profile">
          <div class="lk-kv">
            <Kr k="Сохранена" :v="dt(pf.data.profile.at) + ' · ' + ago(pf.data.profile.at)" />
            <Kr k="Сервер" :v="pf.data.profile.name + ' · KVK ' + (pf.data.profile.version || '?') + ' · Xray ' + (pf.data.profile.xray || '—')" />
            <Kr k="Сайт-прикрытие" :v="pf.data.profile.site.enabled ? (pf.data.profile.site.domains || []).join(', ') + (pf.data.profile.site.sni ? ' · маска ' + pf.data.profile.site.sni : '') : 'выключен'" />
            <Kr k="Фильтрация · WARP" :v="(pf.data.profile.filter ? 'включена' : 'выключена') + ' · ' + (pf.data.profile.warp ? 'включён' : 'выключен')" />
          </div>
          <div>
            <h3 class="lk-h">Инбаунды <span class="i-cnt">{{ pf.data.profile.inbounds.length }}</span></h3>
            <div v-if="pf.data.profile.inbounds.length" class="tbl-w"><table class="tbl dense">
              <thead><tr><th>Название</th><th>Протокол</th><th>Порт</th><th></th></tr></thead>
              <tbody><tr v-for="i in pf.data.profile.inbounds" :key="i.id"><td>{{ i.remark }}</td><td>{{ PROTO[i.protocol] || i.protocol }} · {{ NET[i.network] || i.network }}<template v-if="i.security && i.security !== 'none'"> · {{ i.security }}</template></td><td class="i-mono">{{ i.port }}</td><td><span v-if="!i.enabled" class="badge b-mute">выкл</span></td></tr></tbody>
            </table></div>
            <p v-else class="muted">нет</p>
          </div>
          <div>
            <h3 class="lk-h">Домены <span class="i-cnt">{{ pf.data.profile.domains.length }}</span></h3>
            <div v-if="pf.data.profile.domains.length" class="lk-doms"><span v-for="d in pf.data.profile.domains" :key="d.name" class="badge" :class="d.site ? 'b-ok' : 'b-mute'"><span class="i-mono">{{ d.name }}</span><template v-if="d.site"> · сайт</template><template v-if="d.cert"> · серт.</template></span></div>
            <p v-else class="muted">нет</p>
          </div>
          <label v-if="pf.data.profile.domains.some(d => d.kind === 'domain')" class="lk-ck"><input v-model="pf.dns" type="checkbox"> <span><b>Направить DNS доменов на новый сервер</b><br><span class="muted">через Cloudflare API — нужен токен Cloudflare в настройках сайта{{ pf.data.profile.site.cfToken ? ' (сохранён)' : ' (не задан — DNS придётся сменить вручную)' }}</span></span></label>
        </template>
        <div v-if="pf.warns.length" class="i-hint wa"><Icon n="warn" /><span><b>Восстановлено с замечаниями:</b><br><template v-for="w in pf.warns" :key="w">• {{ w }}<br></template></span></div>
      </div>
      <template #foot>
        <button v-if="pf.data?.profile" class="btn btn-g" type="button" @click="downloadProf"><Icon n="download" />Скачать</button>
        <button v-if="pf.data" class="btn" type="button" :disabled="busy.acc" @click="accept"><span v-if="busy.acc" class="spin"></span>{{ pf.data.profile ? 'Оставить как на сервере' : 'Сохранить копию' }}</button>
        <button v-if="pf.data?.profile" class="btn btn-p" type="button" :disabled="busy.rest" @click="restore"><span v-if="busy.rest" class="spin"></span>Восстановить на сервере</button>
      </template>
    </Drawer>
    <Drawer :open="add.open" title="Добавить сервер" eyebrow="связка серверов" @close="add.open = false">
      <div class="col">
        <div class="fld"><label class="lbl">Ключ связки или адрес панели</label><textarea v-model="add.url" class="inp mono" rows="3" placeholder="ключ из «Связка серверов» второго сервера или https://IP:порт/путь/"></textarea></div>
        <div v-if="add.url.trim().startsWith('http')" class="fld"><label class="lbl">API-токен сервера</label><input v-model="add.token" class="inp mono" placeholder="Настройки → API на втором сервере"></div>
        <div class="fld"><label class="lbl">Название</label><input v-model="add.name" class="inp" placeholder="пусто — как на сервере"></div>
        <div v-if="add.test" class="i-hint"><Icon n="check" /><span><b>{{ flag(add.test.info.country) }} {{ add.test.info.name }}</b> отвечает · {{ add.test.pingMs }} мс · KVK {{ add.test.info.version }} · Xray {{ add.test.info.xray || '—' }} · {{ pn((add.test.info.inbounds || []).length, 'инбаунд', 'инбаунда', 'инбаундов') }}</span></div>
      </div>
      <template #foot>
        <button class="btn" type="button" :disabled="busy.test || !add.url.trim()" @click="test"><span v-if="busy.test" class="spin"></span>Проверить</button>
        <button class="btn btn-p" type="button" :disabled="busy.add || !add.url.trim()" @click="doAdd"><span v-if="busy.add" class="spin"></span>Добавить</button>
      </template>
    </Drawer>
  </div>
</template>
<script setup>
import '../styles/Link.css'
import { ref, reactive } from 'vue'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import Metric from '../components/Metric.vue'
import Drawer from '../components/Drawer.vue'
import Kr from '../components/Kr.vue'
import TgRow from '../components/TgRow.vue'
import { get, post, put, del, download } from '../lib/api.js'
import { useLoad } from '../lib/useLoad.js'
import { ok, fail, confirm, copy } from '../lib/ui.js'
import { ago, dt, flag, pn, PROTO, NET } from '../lib/fmt.js'
import { loadServers } from '../lib/server.js'
import { loadMeta } from '../lib/meta.js'

const { data, error, reload } = useLoad(() => get('nodes'), { interval: 15000 })
const busy = reactive({ sync: false, test: false, add: false, ed: false, acc: false, rest: false })
const ed = reactive({ open: false, node: null, name: '', url: '', token: '', enabled: true })
const pf = reactive({ open: false, node: null, data: null, error: '', dns: true, warns: [] })
const add = reactive({ open: false, url: '', token: '', name: '', test: null })
const showKey = ref(false)
async function sync() { busy.sync = true; try { data.value = await post('nodes/sync'); ok('Синхронизировано') } catch (e) { fail(e) } finally { busy.sync = false } }
async function test() { busy.test = true; add.test = null; try { add.test = await post('nodes/test', { url: add.url, token: add.token }) } catch (e) { fail(e) } finally { busy.test = false } }
async function doAdd() {
  busy.add = true
  try { await post('nodes', { url: add.url, token: add.token, name: add.name }); ok('Сервер добавлен в связку'); Object.assign(add, { open: false, url: '', token: '', name: '', test: null }); reload(true) } catch (e) { fail(e) } finally { busy.add = false }
}
function openEdit(n) { Object.assign(ed, { open: true, node: n, name: n.name, url: '', token: '', enabled: n.enabled }) }
async function saveEdit() {
  const b = {}
  if (ed.name.trim() && ed.name.trim() !== ed.node.name) b.name = ed.name.trim()
  if (ed.url.trim()) b.url = ed.url.trim()
  if (ed.token.trim()) b.token = ed.token.trim()
  if (ed.enabled !== ed.node.enabled) b.enabled = ed.enabled
  if (!Object.keys(b).length) { ed.open = false; return }
  busy.ed = true
  try {
    const r = await put('nodes/' + ed.node.id, b)
    ed.open = false; reload(true); loadServers(true)
    if (r && r.replaced) { ok('Адрес сменён — теперь восстановите конфигурацию'); const d = await get('nodes'); data.value = d; const n = d.nodes.find(x => x.id === ed.node.id); if (n) openProf(n) } else ok('Сохранено')
  } catch (e) { fail(e) } finally { busy.ed = false }
}
async function loadProf() { pf.error = ''; try { pf.data = await get(`nodes/${pf.node.id}/profile`) } catch (e) { pf.error = e.message } }
function openProf(n) { Object.assign(pf, { open: true, node: n, data: null, error: '', dns: true, warns: [] }); loadProf() }
const downloadProf = () => download(`nodes/${pf.node.id}/profile?download=1`, 'kvk-server.json').catch(fail)
async function accept() {
  if (pf.data.profile && !await confirm(`Сохранить текущую конфигурацию «${pf.node.name}»?`, 'Сохранённая копия заменится тем, что сейчас настроено на сервере. Клиенты потеряют доступ к инбаундам, которых на сервере больше нет.', { okText: 'Сохранить' })) return
  busy.acc = true
  try { await post(`nodes/${pf.node.id}/accept`); ok('Копия обновлена'); pf.warns = []; await loadProf(); reload(true); loadServers(true) } catch (e) { fail(e) } finally { busy.acc = false }
}
async function restore() {
  if (!await confirm(`Восстановить конфигурацию на «${pf.node.name}»?`, `Инбаунды, домены, сайт, фильтрация и WARP на сервере ${pf.node.url} будут заменены сохранёнными. Клиенты и доступ к панели не меняются.`, { okText: 'Восстановить', danger: !pf.data.replaced })) return
  busy.rest = true; pf.warns = []
  try { const r = await post(`nodes/${pf.node.id}/restore`, { dns: pf.dns }); pf.warns = r.warnings || []; ok('Конфигурация восстановлена'); await loadProf(); reload(true); loadServers(true) } catch (e) { fail(e) } finally { busy.rest = false }
}
async function remove(n) {
  if (!await confirm(`Убрать «${n.name}» из связки?`, 'Его конфиги пропадут из подписок клиентов, а клиенты, полученные им отсюда, будут с него удалены.', { danger: true, okText: 'Убрать' })) return
  try { await del('nodes/' + n.id); ok('Сервер убран'); ed.open = false; reload(true); loadServers(true) } catch (e) { fail(e) }
}
async function detach() {
  if (!await confirm('Отвязать от основного сервера?', 'Клиенты, пришедшие с основного, будут удалены с этого сервера.', { danger: true, okText: 'Отвязать' })) return
  try { const r = await post('main/detach'); ok(`Отвязан, удалено клиентов: ${r.removed}`); reload(true); loadMeta() } catch (e) { fail(e) }
}
</script>
