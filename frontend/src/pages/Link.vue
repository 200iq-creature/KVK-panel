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
              <thead><tr><th>Сервер</th><th>Адрес</th><th>Клиенты</th><th>Инбаунды</th><th>CPU</th><th>Пинг</th><th>Синхр.</th><th>Статус</th><th></th></tr></thead>
              <tbody>
                <tr>
                  <td><b>{{ flag(data.self.country) }} {{ data.self.name }}</b> <span class="badge b-info">этот</span></td>
                  <td class="i-mono" style="font-size:12px">{{ data.self.url }}</td>
                  <td>{{ data.self.clients }}</td><td>{{ data.self.inbounds }}</td><td>—</td><td>—</td><td>—</td>
                  <td><span class="i-stt ok">{{ data.main ? 'нода' : 'основной' }}</span></td><td></td>
                </tr>
                <tr v-for="n in data.nodes" :key="n.id">
                  <td><b>{{ flag(n.country) }} {{ n.name }}</b><div class="muted" style="font-size:12px">v{{ n.version || '?' }}</div></td>
                  <td class="i-mono" style="font-size:12px;max-width:260px;overflow:hidden;text-overflow:ellipsis">{{ n.url }}</td>
                  <td>{{ n.clients }}</td><td>{{ n.inbounds }}</td>
                  <td>{{ n.cpu ? Math.round(n.cpu) + '%' : '—' }}</td>
                  <td>{{ n.pingMs ? n.pingMs + ' мс' : '—' }}</td>
                  <td class="nw">{{ n.lastSync ? ago(n.lastSync) : '—' }}</td>
                  <td><span class="i-stt" :class="!n.enabled ? 'mu' : n.lastError ? 'bad' : 'ok'" :title="n.lastError">{{ !n.enabled ? 'выключен' : n.lastError ? 'ошибка' : 'в связке' }}</span></td>
                  <td class="r"><span class="acts">
                    <button class="btn btn-sm btn-g" type="button" @click="toggle(n)">{{ n.enabled ? 'Выключить' : 'Включить' }}</button>
                    <button class="btn btn-sm btn-g" type="button" @click="rename(n)">Переименовать</button>
                    <button class="btn btn-sm btn-g" type="button" @click="retoken(n)">Токен</button>
                    <button class="btn btn-sm btn-g btn-ic" type="button" title="Удалить из связки" @click="remove(n)"><Icon n="trash" /></button>
                  </span></td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="data.nodes.length" class="i-pb" style="padding-top:0"><div class="i-hint"><Icon n="info" /><span>Инбаунды подключённых серверов при первой синхронизации выдаются всем клиентам и сразу попадают в их подписки. Кому какие оставить, выбирается в карточке клиента → «Инбаунды» (группа «Сервер «…»») или в боте-магазине для новых клиентов. Новый инбаунд на сервере получают клиенты, у которых уже есть доступ к этому серверу.</span></div></div>
          <div v-for="n in data.nodes.filter(x => x.enabled && x.lastError)" :key="'e' + n.id" class="i-pb" style="padding-top:0"><div class="i-hint bad"><Icon n="warn" /><span><b>{{ n.name }}:</b> {{ n.lastError }}</span></div></div>
        </section>
        <div class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>Ключ этого сервера</h2><span>для подключения к основному</span></div>
            <div class="i-pb">
              <p class="hint">Чтобы сделать этот сервер нодой другого — вставьте ключ на основном сервере в «Добавить сервер». Ключ содержит адрес панели и API-токен: не публикуйте его.</p>
              <div class="vw-link"><code>{{ showKey ? data.linkKey : '•'.repeat(48) }}</code><button class="i-cp" type="button" @click="showKey = !showKey"><Icon n="eye" /></button><button class="i-cp" type="button" @click="copy(data.linkKey, 'Ключ скопирован')"><Icon n="copy" /></button></div>
              <p class="hint">Сменить токен (и ключ) можно в «Настройках» → API.</p>
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
import { get, post, put, del } from '../lib/api.js'
import { useLoad } from '../lib/useLoad.js'
import { ok, fail, confirm, prompt, copy } from '../lib/ui.js'
import { ago, flag, pn } from '../lib/fmt.js'
import { loadMeta } from '../lib/meta.js'

const { data, error, reload } = useLoad(() => get('nodes'), { interval: 15000 })
const busy = reactive({ sync: false, test: false, add: false })
const add = reactive({ open: false, url: '', token: '', name: '', test: null })
const showKey = ref(false)
async function sync() { busy.sync = true; try { data.value = await post('nodes/sync'); ok('Синхронизировано') } catch (e) { fail(e) } finally { busy.sync = false } }
async function test() { busy.test = true; add.test = null; try { add.test = await post('nodes/test', { url: add.url, token: add.token }) } catch (e) { fail(e) } finally { busy.test = false } }
async function doAdd() {
  busy.add = true
  try { await post('nodes', { url: add.url, token: add.token, name: add.name }); ok('Сервер добавлен в связку'); Object.assign(add, { open: false, url: '', token: '', name: '', test: null }); reload(true) } catch (e) { fail(e) } finally { busy.add = false }
}
async function upd(n, b, msg) { try { await put('nodes/' + n.id, b); ok(msg); reload(true) } catch (e) { fail(e) } }
const toggle = (n) => upd(n, { enabled: !n.enabled }, n.enabled ? 'Сервер выключен' : 'Сервер включён')
async function rename(n) { const v = await prompt('Название сервера', { value: n.name }); if (v && v.trim()) upd(n, { name: v.trim() }, 'Переименован') }
async function retoken(n) { const v = await prompt('Новый API-токен сервера', { placeholder: 'если на сервере сменили токен', label: 'Токен' }); if (v && v.trim()) upd(n, { token: v.trim() }, 'Токен обновлён') }
async function remove(n) {
  if (!await confirm(`Убрать «${n.name}» из связки?`, 'Его конфиги пропадут из подписок клиентов, а клиенты, полученные им отсюда, будут с него удалены.', { danger: true, okText: 'Убрать' })) return
  try { await del('nodes/' + n.id); ok('Сервер убран'); reload(true) } catch (e) { fail(e) }
}
async function detach() {
  if (!await confirm('Отвязать от основного сервера?', 'Клиенты, пришедшие с основного, будут удалены с этого сервера.', { danger: true, okText: 'Отвязать' })) return
  try { const r = await post('main/detach'); ok(`Отвязан, удалено клиентов: ${r.removed}`); reload(true); loadMeta() } catch (e) { fail(e) }
}
</script>
