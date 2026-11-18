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
        <section v-if="data.nodes.length" class="i-pn">
          <div class="i-ph">
            <h2>Обновление панелей</h2>
            <span class="lk-ua">
              <span v-if="upd.list" class="i-stt" :class="updCount ? 'wa' : updating ? 'wa' : 'ok'">{{ updating ? 'обновляются…' : updCount ? `обновления: ${updCount}` : 'все актуальны' }}</span>
              <button class="btn btn-sm btn-g" type="button" :disabled="upd.checking" @click="checkUpd(false)"><span v-if="upd.checking" class="spin"></span><Icon v-else n="refresh" />Проверить</button>
              <button class="btn btn-sm btn-p" type="button" :disabled="!updCount || upd.busy || updating" @click="runUpd(upd.list.filter(x => x.update && !x.updating))"><span v-if="upd.busy" class="spin"></span><Icon v-else n="download" />Обновить все{{ updCount ? ` (${updCount})` : '' }}</button>
            </span>
          </div>
          <div class="i-pb lk-ub">
            <p class="hint">Каждый сервер обновляется сам, из своей ветки — так же, как кнопкой в его «Настройках». Бэкап делается перед обновлением, Xray продолжает работать. Этот сервер обновляется последним: панель перезапустится, страница перезагрузится сама.</p>
            <div v-if="!upd.list" class="lk-ul"><div class="lk-ur"><span class="muted">{{ upd.checking ? 'Проверяем версии на всех серверах…' : upd.error || '—' }}</span></div></div>
            <div v-else class="lk-ul">
              <div v-for="x in upd.list" :key="x.id || 'self'" class="lk-ur">
                <span class="lk-un"><b>{{ flag(x.country) }} {{ x.name }}</b><small>{{ x.main ? 'этот сервер' : 'сервер связки' }}<template v-if="x.branch"> · ветка {{ x.branch }}</template></small></span>
                <span class="lk-uv"><span class="i-mono">v{{ x.current || '?' }}</span><template v-if="x.update"> · доступна <b class="i-mono">v{{ x.latest }}</b></template></span>
                <span class="lk-us">
                  <span v-if="x.updating" class="i-stt wa"><span class="spin"></span>обновляется</span>
                  <span v-else-if="x.done" class="i-stt ok">обновлён</span>
                  <span v-else-if="x.error" class="i-stt bad" :title="x.error">ошибка</span>
                  <span v-else-if="x.update" class="i-stt wa">есть обновление</span>
                  <span v-else-if="x.latest" class="i-stt ok">актуальная</span>
                  <span v-else class="i-stt mu">—</span>
                </span>
                <span class="lk-ub2"><button v-if="x.update && !x.updating" class="btn btn-sm" type="button" :disabled="upd.busy" @click="runUpd([x])">Обновить</button></span>
                <span v-if="x.error && !x.updating" class="lk-ue">{{ x.error }}</span>
              </div>
            </div>
          </div>
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
        <div class="lk-2">
          <div class="fld"><label class="lbl">Название</label><input v-model="ed.name" class="inp"></div>
          <div class="fld"><label class="lbl">Страна (код)</label><span class="lk-cc"><input v-model="ed.country" class="inp mono" maxlength="2" placeholder="DE" @input="ed.country = ed.country.toUpperCase().replace(/[^A-Z]/g, '')"><i>{{ flag(ed.country) }}</i></span></div>
        </div>
        <span class="hint lk-cch">Код из двух букв (DE, NL, FI) — флаг в подписке и в связке. Сохраняется и на самом сервере.</span>
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
        <div class="lk-2">
          <div class="fld"><label class="lbl">Название</label><input v-model="add.name" class="inp" placeholder="пусто — как на сервере"></div>
          <div class="fld"><label class="lbl">Страна (код)</label><span class="lk-cc"><input v-model="add.country" class="inp mono" maxlength="2" :placeholder="add.test?.info?.country || 'как на сервере'" @input="add.country = add.country.toUpperCase().replace(/[^A-Z]/g, '')"><i>{{ flag(add.country || add.test?.info?.country) }}</i></span></div>
        </div>
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
import { ref, reactive, computed, watch, onBeforeUnmount } from 'vue'
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
const ed = reactive({ open: false, node: null, name: '', url: '', token: '', enabled: true, country: '' })
const pf = reactive({ open: false, node: null, data: null, error: '', dns: true, warns: [] })
const add = reactive({ open: false, url: '', token: '', name: '', country: '', test: null })
const showKey = ref(false)
// panel updates of the whole link group
const upd = reactive({ list: null, checking: false, busy: false, error: '' })
const updCount = computed(() => (upd.list || []).filter(x => x.update && !x.updating).length)
const updating = computed(() => (upd.list || []).some(x => x.updating))
let updTimer = null
async function checkUpd(quick) {
  if (!quick) upd.checking = true
  try {
    const r = await get('nodes/updates' + (quick ? '?quick=1' : ''))
    const prev = Object.fromEntries((upd.list || []).map(x => [x.id, x]))
    upd.list = r.servers.map(x => (quick && prev[x.id] && !x.latest ? { ...x, latest: prev[x.id].latest } : x))
    upd.error = ''
  } catch (e) { if (!quick) { upd.error = e.message; fail(e) } } finally { upd.checking = false }
}
async function runUpd(list) {
  if (!list.length) return
  const self = list.some(x => x.main)
  const names = list.map(x => x.name).join(', ')
  if (!await confirm(list.length > 1 ? `Обновить ${pn(list.length, 'сервер', 'сервера', 'серверов')}?` : `Обновить «${list[0].name}»?`, `${names}. Каждый сервер сделает бэкап, скачает новую версию и перезапустит панель (около минуты). Xray продолжит работать.${self ? ' Этот сервер обновится последним — страница перезагрузится сама.' : ''}`, { okText: 'Обновить' })) return
  upd.busy = true
  try {
    const r = await post('nodes/update', { ids: list.filter(x => !x.main).map(x => x.id), self })
    const bad = (r.results || []).filter(x => !x.ok)
    if (bad.length) fail(bad.map(x => `${x.name || x.id}: ${x.error}`).join('; '))
    const okN = (r.results || []).length - bad.length
    if (okN) ok(`Обновление запущено: ${okN}`)
    if (r.self) { setTimeout(() => location.reload(), 70000) }
    await checkUpd(true)
  } catch (e) { fail(e) } finally { upd.busy = false }
}
watch(updating, (v) => { clearInterval(updTimer); if (v) updTimer = setInterval(() => checkUpd(true), 8000) })
watch(() => data.value && data.value.nodes.length, (n, o) => { if (n && !o && !upd.list && !upd.checking) checkUpd(false) }, { immediate: true })
onBeforeUnmount(() => clearInterval(updTimer))
async function sync() { busy.sync = true; try { data.value = await post('nodes/sync'); ok('Синхронизировано') } catch (e) { fail(e) } finally { busy.sync = false } }
async function test() { busy.test = true; add.test = null; try { add.test = await post('nodes/test', { url: add.url, token: add.token }) } catch (e) { fail(e) } finally { busy.test = false } }
async function doAdd() {
  busy.add = true
  try {
    const r = await post('nodes', { url: add.url, token: add.token, name: add.name, country: add.country })
    if (r && r.warning) fail(r.warning); else ok('Сервер добавлен в связку')
    Object.assign(add, { open: false, url: '', token: '', name: '', country: '', test: null }); reload(true); loadServers(true)
  } catch (e) { fail(e) } finally { busy.add = false }
}
function openEdit(n) { Object.assign(ed, { open: true, node: n, name: n.name, url: '', token: '', enabled: n.enabled, country: n.country || '' }) }
async function saveEdit() {
  const b = {}
  if (ed.name.trim() && ed.name.trim() !== ed.node.name) b.name = ed.name.trim()
  if (ed.url.trim()) b.url = ed.url.trim()
  if (ed.token.trim()) b.token = ed.token.trim()
  if (ed.enabled !== ed.node.enabled) b.enabled = ed.enabled
  if (ed.country.length === 1) return fail('Код страны — две латинские буквы, например DE')
  if (ed.country !== (ed.node.country || '')) b.country = ed.country
  if (!Object.keys(b).length) { ed.open = false; return }
  busy.ed = true
  try {
    const r = await put('nodes/' + ed.node.id, b)
    ed.open = false; reload(true); loadServers(true)
    if (r && r.warning) fail(r.warning)
    if (r && r.replaced) { ok('Адрес сменён — теперь восстановите конфигурацию'); const d = await get('nodes'); data.value = d; const n = d.nodes.find(x => x.id === ed.node.id); if (n) openProf(n) } else if (!r || !r.warning) ok('Сохранено')
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
<style>
.h1 .lk-2{display:grid;grid-template-columns:minmax(0,1fr) 150px;gap:12px}
.h1 .lk-cc{position:relative;display:block}
.h1 .lk-cc .inp{width:100%;padding-right:40px;text-transform:uppercase}
.h1 .lk-cc i{position:absolute;right:12px;top:50%;transform:translateY(-50%);font-style:normal;font-size:20px;line-height:1;pointer-events:none}
.h1 .lk-cch{margin-top:-6px}
.h1 .lk-ua{display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.h1 .lk-ub{padding-top:0}
.h1 .lk-ub>.hint{margin:0 0 12px}
.h1 .lk-ul{border:1px solid var(--line);border-radius:var(--r-md,10px);overflow:hidden}
.h1 .lk-ur{display:grid;grid-template-columns:minmax(0,1.4fr) minmax(0,1fr) 150px 110px;gap:12px;align-items:center;padding:10px 14px;border-top:1px solid var(--line2);font-size:var(--fs-sm)}
.h1 .lk-ur:first-child{border-top:0}
.h1 .lk-un{display:flex;flex-direction:column;min-width:0;line-height:1.3}
.h1 .lk-un b{color:var(--strong);font-weight:600}
.h1 .lk-un small{color:var(--muted);font-size:var(--fs-xs)}
.h1 .lk-uv b{color:var(--strong)}
.h1 .lk-us .i-stt{display:inline-flex;align-items:center;gap:6px}
.h1 .lk-us .spin{width:12px;height:12px}
.h1 .lk-ub2{justify-self:end}
.h1 .lk-ue{grid-column:1/-1;color:var(--bad);font-size:var(--fs-xs);margin-top:-4px;overflow-wrap:anywhere}
@media (max-width:760px){.h1 .lk-ur{grid-template-columns:minmax(0,1fr) auto}.h1 .lk-uv{grid-column:1}.h1 .lk-us{grid-column:2;grid-row:1;justify-self:end}.h1 .lk-ub2{grid-column:2;grid-row:2}}
</style>
