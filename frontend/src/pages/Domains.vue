<template>
  <div class="pg-domains">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="vw-eb">сертификаты TLS · IP сервера <span class="i-mono">{{ data?.serverIp || '—' }}</span></span><h1>Домены</h1><p>Домены и IP-адреса с сертификатами — для HTTPS панели и инбаундов с TLS (Trojan, VLESS TLS, Hysteria2). Let's Encrypt выпускается и продлевается сам.</p></div>
        <div class="i-hd-a"><button class="btn btn-p" type="button" @click="openAdd"><Icon n="plus" />Добавить домен или IP</button></div>
      </div>
      <Loading v-if="!data" :error="error" @retry="reload" />
      <template v-else>
        <div class="i-mt">
          <Metric label="Сертификатов" :value="data.domains.length" :note="`${data.domains.filter(d => d.status === 'active').length} действующих`" />
          <Metric label="Истекают скоро" :value="soon" :note="soon ? 'меньше 14 дней' : 'всё в порядке'" :note-tone="soon ? 'wa' : 'ok'" />
          <Metric label="HTTPS панели" :value="panelDom ? panelDom.name : 'выключен'" :note="panelDom ? 'до ' + date(panelDom.expiresAt) : 'включите кнопкой «Для панели»'" :note-tone="panelDom ? 'ok' : 'bad'" />
          <Metric label="Адрес в ссылках" :value="data.publicHost" note="домен или IP в подписках клиентов" />
        </div>
        <section class="i-pn">
          <div class="i-ph"><h2>Домены и IP <span class="i-cnt">{{ data.domains.length }}</span></h2><span>проверка продления — каждые 6 часов</span></div>
          <div class="tbl-w">
            <table class="tbl">
              <thead><tr><th>Имя</th><th>Сертификат</th><th>Статус</th><th>Действует до</th><th>Используется</th><th></th></tr></thead>
              <tbody>
                <tr v-for="d in data.domains" :key="d.id">
                  <td><b class="i-mono">{{ d.name }}</b><span class="badge b-mute" style="margin-left:6px">{{ d.kind === 'ip' ? 'IP' : 'домен' }}</span></td>
                  <td>{{ MODE[d.mode] }}<div v-if="d.mode === 'letsencrypt'" class="muted" style="font-size:12px">{{ d.autoRenew ? 'автопродление' : 'без автопродления' }}</div></td>
                  <td><span class="i-stt" :class="d.issuing ? 'wa' : d.status === 'active' ? (d.daysLeft < 14 ? 'wa' : 'ok') : d.status === 'error' ? 'bad' : 'mu'">{{ d.issuing ? 'выпускается…' : ST[d.status] || d.status }}</span><div v-if="d.error" class="bad-t" style="font-size:12px;max-width:320px">{{ d.error }}</div></td>
                  <td class="nw">{{ d.expiresAt ? date(d.expiresAt) + ` · ${d.daysLeft} дн.` : '—' }}</td>
                  <td><span v-if="d.panel" class="badge b-info">панель</span> <span v-for="u in d.usedBy" :key="u" class="badge b-mute" style="margin-right:4px">{{ u }}</span><span v-if="!d.panel && !d.usedBy?.length" class="muted">—</span></td>
                  <td class="r"><span class="acts">
                    <button v-if="d.mode !== 'custom'" class="btn btn-sm btn-g" type="button" :disabled="d.issuing" @click="issue(d)">{{ d.status === 'active' ? 'Перевыпустить' : 'Выпустить' }}</button>
                    <button v-if="!d.panel && d.status === 'active'" class="btn btn-sm" type="button" @click="forPanel(d)"><Icon n="lock" />Для панели</button>
                    <button v-if="d.panel" class="btn btn-sm btn-g" type="button" @click="panelOff">Выключить HTTPS</button>
                    <button v-if="d.kind === 'domain' && data.publicHost !== d.name" class="btn btn-sm btn-g" type="button" title="Использовать в ссылках клиентов" @click="setHost(d.name)">В ссылки</button>
                    <button class="btn btn-sm btn-g btn-ic" type="button" title="Удалить" @click="remove(d)"><Icon n="trash" /></button>
                  </span></td>
                </tr>
              </tbody>
            </table>
            <Empty v-if="!data.domains.length" icon="dom" title="Доменов пока нет" text="Добавьте домен (A-запись на IP сервера) или сам IP — для IP подойдёт самоподписанный сертификат или Let's Encrypt для IP" />
          </div>
        </section>
        <div class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>Адрес сервера в ссылках</h2></div>
            <div class="i-pb">
              <p class="hint">Что подставляется в ссылки подписки и конфиги клиентов. Обычно — домен с сертификатом или IP сервера.</p>
              <div class="row" style="flex-wrap:nowrap"><input v-model="host" class="inp mono" :placeholder="data.serverIp"><button class="btn" type="button" :disabled="!host.trim() || host === data.publicHost" @click="setHost(host)">Сохранить</button></div>
              <div class="row"><button class="btn btn-sm btn-g" type="button" @click="host = data.serverIp">IP сервера: {{ data.serverIp }}</button></div>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Как получить сертификат</h2></div>
            <div class="i-pb">
              <ol class="steps">
                <li><b>Let's Encrypt для домена</b> — создайте A-запись домена на {{ data.serverIp }}; порт 80 должен быть свободен и открыт (панель сама ответит на проверку).</li>
                <li><b>Let's Encrypt для IP</b> — короткий сертификат (≈6 дней) на публичный IP, продлевается автоматически.</li>
                <li><b>Самоподписанный</b> — мгновенно, без домена. Клиенты подключаются по отпечатку сертификата (pinSHA256); браузер панели покажет предупреждение.</li>
                <li><b>Свой</b> — вставьте fullchain и ключ (например, Cloudflare Origin).</li>
              </ol>
            </div>
          </section>
        </div>
      </template>
    </div>
    <Drawer :open="add.open" title="Добавить домен или IP" eyebrow="домены" @close="add.open = false">
      <div class="col">
        <div class="fld"><label class="lbl">Домен или IP</label><input v-model="add.name" class="inp mono" placeholder="vpn.example.com"><div class="row" style="margin-top:6px"><button class="btn btn-sm btn-g" type="button" @click="add.name = data.serverIp">IP этого сервера</button></div></div>
        <div class="fld"><label class="lbl">Сертификат</label><Opt v-model="add.mode" :options="[{ v: 'letsencrypt', l: `Let's Encrypt`, s: 'бесплатно, продлевается автоматически' }, { v: 'selfsigned', l: 'Самоподписанный', s: 'сразу, без проверки домена' }, { v: 'custom', l: 'Свой сертификат', s: 'вставить PEM' }]" /></div>
        <div v-if="add.mode === 'letsencrypt'" class="fld"><label class="lbl">E-mail (необязательно)</label><input v-model="add.email" class="inp" type="email" placeholder="для писем Let's Encrypt"></div>
        <div v-if="add.mode === 'letsencrypt' && isIp" class="i-hint"><Icon n="info" /><span>Для IP Let's Encrypt выдаёт короткие сертификаты (около 6 дней) — панель продлевает их автоматически.</span></div>
        <template v-if="add.mode === 'custom'">
          <div class="fld"><label class="lbl">Сертификат (fullchain.pem)</label><textarea v-model="add.cert" class="inp mono" rows="5" placeholder="-----BEGIN CERTIFICATE-----"></textarea></div>
          <div class="fld"><label class="lbl">Приватный ключ</label><textarea v-model="add.key" class="inp mono" rows="4" placeholder="-----BEGIN PRIVATE KEY-----"></textarea></div>
        </template>
        <TgRow v-model="add.forPanel" bold label="Включить HTTPS для панели" hint="панель перезапустится и откроется по https://" />
      </div>
      <template #foot><button class="btn" type="button" @click="add.open = false">Отмена</button><button class="btn btn-p" type="button" :disabled="busy.add || !add.name.trim()" @click="doAdd"><span v-if="busy.add" class="spin"></span>Добавить</button></template>
    </Drawer>
  </div>
</template>
<script setup>
import '../styles/Domains.css'
import { ref, reactive, computed, watch } from 'vue'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import Metric from '../components/Metric.vue'
import Empty from '../components/Empty.vue'
import Drawer from '../components/Drawer.vue'
import Opt from '../components/Opt.vue'
import TgRow from '../components/TgRow.vue'
import { get, post, put, del } from '../lib/api.js'
import { useLoad } from '../lib/useLoad.js'
import { ok, fail, confirm } from '../lib/ui.js'
import { date } from '../lib/fmt.js'
import { loadMeta } from '../lib/meta.js'

const MODE = { letsencrypt: "Let's Encrypt", selfsigned: 'самоподписанный', custom: 'свой' }
const ST = { active: 'действует', pending: 'ожидает выпуска', error: 'ошибка', expired: 'истёк' }
const { data, error, reload } = useLoad(() => get('domains'), { interval: 5000 })
const host = ref('')
watch(data, (d, old) => { if (d && !old) host.value = d.publicHost })
const busy = reactive({ add: false })
const add = reactive({ open: false, name: '', mode: 'letsencrypt', email: '', cert: '', key: '', forPanel: false })
const isIp = computed(() => /^[\d.]+$|:/.test(add.name.trim()))
const soon = computed(() => (data.value?.domains || []).filter(d => d.status === 'active' && d.daysLeft < 14 && d.mode !== 'letsencrypt').length + (data.value?.domains || []).filter(d => d.mode === 'letsencrypt' && d.status === 'active' && d.daysLeft < 3).length)
const panelDom = computed(() => (data.value?.domains || []).find(d => d.panel))
function openAdd() { Object.assign(add, { open: true, name: '', mode: 'letsencrypt', email: '', cert: '', key: '', forPanel: !panelDom.value }) }
function goTo(url, msg) { ok(msg); setTimeout(() => { location.href = url }, 3000) }
async function doAdd() {
  busy.add = true
  try {
    await post('domains', { name: add.name.trim(), mode: add.mode, email: add.email.trim(), cert: add.cert, key: add.key, forPanel: add.forPanel })
    add.open = false
    if (add.forPanel && add.mode !== 'letsencrypt') { const d = await get('domains'); goTo(d.panelUrl.replace(/^http:/, 'https:'), 'HTTPS включён, панель перезапускается…') }
    else { ok(add.mode === 'letsencrypt' ? 'Выпускаем сертификат… обычно до минуты' + (add.forPanel ? ', затем панель перейдёт на HTTPS' : '') : 'Добавлено'); reload(true) }
  } catch (e) { fail(e) } finally { busy.add = false }
}
async function issue(d) { try { await post(`domains/${d.id}/issue`); ok('Выпуск запущен'); reload(true) } catch (e) { fail(e) } }
async function forPanel(d) {
  if (!await confirm(`Включить HTTPS панели с ${d.name}?`, d.mode === 'selfsigned' ? 'Сертификат самоподписанный — браузер покажет предупреждение, его нужно будет принять. Панель перезапустится.' : 'Панель перезапустится и откроется по https://.', { okText: 'Включить' })) return
  try { const r = await post(`domains/${d.id}/panel`); goTo(r.url, 'Панель перезапускается на HTTPS…') } catch (e) { fail(e) }
}
async function panelOff() {
  if (!await confirm('Выключить HTTPS панели?', 'Панель откроется по http:// — пароль будет передаваться без шифрования.', { danger: true, okText: 'Выключить' })) return
  try { const r = await post('domains-panel-off'); goTo(r.url, 'Панель перезапускается…') } catch (e) { fail(e) }
}
async function setHost(h) { try { const r = await put('host', { host: h.trim() }); host.value = r.host; ok('Адрес в ссылках: ' + r.host + '. Клиентам нужно обновить подписку.'); reload(true); loadMeta() } catch (e) { fail(e) } }
async function remove(d) { if (await confirm(`Удалить ${d.name}?`, 'Сертификат и ключ будут удалены с сервера.', { danger: true, okText: 'Удалить' })) { try { await del('domains/' + d.id); reload(true) } catch (e) { fail(e) } } }
</script>
