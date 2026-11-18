<template>
  <div class="pg-domains">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="vw-eb">{{ srvName }} · сертификаты TLS · IP сервера <span class="i-mono">{{ data?.serverIp || '—' }}</span></span><h1>Домены</h1><p>Домены и IP-адреса с сертификатами — для сайта-прикрытия, HTTPS панели и инбаундов с TLS (Trojan, VLESS TLS, Hysteria2). Let's Encrypt выпускается и продлевается сам.<template v-if="srv.nodes.length"> Домены связанных серверов добавляются здесь же — выберите сервер.</template></p></div>
        <div class="i-hd-a"><button class="btn" type="button" @click="openAdd"><Icon n="plus" />Свой домен или IP</button><button class="btn btn-p" type="button" @click="free.open = true"><Icon n="star" />Бесплатный домен</button></div>
      </div>
      <ServerPicker />
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
              <thead><tr><th>Имя</th><th>Сертификат</th><th>Используется</th><th></th></tr></thead>
              <tbody>
                <tr v-for="d in data.domains" :key="d.id">
                  <td class="dm-nm"><b class="i-mono">{{ d.name }}</b><span class="badge" :class="d.provider ? 'b-info' : 'b-mute'">{{ d.provider ? PROV[d.provider] || d.provider : d.kind === 'ip' ? 'IP' : 'домен' }}</span>
                    <div v-if="d.dyn" class="muted dm-dyn">{{ d.dynIp ? '→ ' + d.dynIp : '' }}<span v-if="d.dynIp && d.dynIp !== data.serverIp" class="wa-t"> · не этот сервер</span><span v-if="d.dynError" class="bad-t"> · {{ d.dynError }}</span></div></td>
                  <td><div class="dm-mode">{{ MODE[d.mode] }}<template v-if="d.mode === 'letsencrypt'"> · {{ d.autoRenew ? 'автопродление' : 'без автопродления' }}</template></div><span class="i-stt" :class="d.issuing ? 'wa' : d.status === 'active' ? (d.daysLeft < 14 ? 'wa' : 'ok') : d.status === 'error' ? 'bad' : 'mu'">{{ d.issuing ? 'выпускается…' : ST[d.status] || d.status }}</span><div v-if="d.expiresAt" class="muted nw" style="font-size:12px">до {{ date(d.expiresAt) }} · {{ d.daysLeft }} дн.</div><div v-if="d.error" class="bad-t dm-err" :title="d.error">{{ d.error }}</div></td>
                  <td class="dm-use"><span v-if="d.panelOf" class="badge b-info" style="margin-right:4px" :title="d.panel ? 'HTTPS панели' : 'адрес панели'">панель «{{ d.panelOf }}»</span><span v-if="d.site" class="badge b-ok" style="margin-right:4px">{{ d.sni ? 'сайт-прикрытие · маска Reality' : 'сайт-прикрытие' }}</span><span v-for="u in d.usedBy" :key="u" class="badge b-mute" style="margin-right:4px">{{ u }}</span><span v-if="!d.panelOf && !d.site && !d.usedBy?.length" class="muted">—</span></td>
                  <td class="r"><span class="acts">
                    <button v-if="d.dyn && d.dynIp !== data.serverIp" class="btn btn-sm btn-ic dm-pt" type="button" :title="`Направить ${d.name} на этот сервер (${data.serverIp})`" @click="point(d)"><Icon n="globe" /></button>
                    <button v-if="d.mode !== 'custom'" class="btn btn-sm btn-g btn-ic" type="button" :disabled="d.issuing" :title="d.status === 'active' ? 'Перевыпустить сертификат' : 'Выпустить сертификат'" @click="issue(d)"><Icon n="refresh" /></button>
                    <button v-if="!d.panel && d.status === 'active'" class="btn btn-sm btn-g btn-ic" type="button" title="Открывать панель по этому домену (HTTPS)" @click="forPanel(d)"><Icon n="lock" /></button>
                    <button v-if="d.panel" class="btn btn-sm btn-g btn-ic" type="button" title="Выключить HTTPS панели" @click="panelOff"><Icon n="power" /></button>
                    <button v-if="d.kind === 'domain' && data.publicHost !== d.name" class="btn btn-sm btn-g btn-ic" type="button" title="Подставлять в ссылки клиентов" @click="setHost(d.name)"><Icon n="link" /></button>
                    <button class="btn btn-sm btn-g btn-ic" type="button" title="Удалить" @click="remove(d)"><Icon n="trash" /></button>
                  </span></td>
                </tr>
              </tbody>
            </table>
            <Empty v-if="!data.domains.length" icon="dom" title="Доменов пока нет" text="Нет своего домена — нажмите «Бесплатный домен»: имя появится за секунды, без покупки и регистрации. Свой домен (A-запись на IP сервера) или сам IP добавляются кнопкой «Свой домен или IP»." />
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
                <li><b>Бесплатный домен</b> — sslip.io / nip.io мгновенно, или своё имя на freemyip.com / myaddr.tools в один клик. Панель сама направит его на сервер и выпустит сертификат.</li>
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
    <FreeDomain :open="free.open" :server-ip="data?.serverIp" :public-host="data?.publicHost" :has-panel-cert="!!panelDom" @close="free.open = false" @created="onFree" />
    <Drawer :open="add.open" title="Добавить домен или IP" eyebrow="домены" @close="add.open = false">
      <div class="col">
        <div class="fld"><label class="lbl">Домен или IP</label><input v-model="add.name" class="inp mono" placeholder="vpn.example.com"><div class="row" style="margin-top:6px"><button class="btn btn-sm btn-g" type="button" @click="add.name = data.serverIp">IP сервера «{{ srvName }}»</button></div></div>
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
import ServerPicker from '../components/ServerPicker.vue'
import FreeDomain from '../components/FreeDomain.vue'
import '../styles/FreeDomain.css'
import { srv, sget as get, spost as post, sput as put, sdel as del, srvName, remote } from '../lib/server.js'
import { useLoad } from '../lib/useLoad.js'
import { ok, fail, confirm } from '../lib/ui.js'
import { date } from '../lib/fmt.js'
import { loadMeta } from '../lib/meta.js'

const MODE = { letsencrypt: "Let's Encrypt", selfsigned: 'самоподписанный', custom: 'свой' }
const PROV = { sslip: 'sslip.io', nip: 'nip.io', freemyip: 'freemyip.com', myaddr: 'myaddr.tools' }
const free = reactive({ open: false })
function onFree() { free.open = false; reload(true); loadMeta() }
async function point(d) {
  if (!await confirm(`Направить ${d.name} на ${data.value.serverIp}?`, d.dynIp ? `Сейчас имя указывает на ${d.dynIp}. Клиенты с этим адресом начнут подключаться к «${srvName.value}».` : '', { okText: 'Направить' })) return
  try { await post(`domains/${d.id}/point`); ok('Готово — сертификат перевыпустится через полминуты'); reload(true) } catch (e) { fail(e) }
}
const ST = { active: 'действует', pending: 'ожидает выпуска', error: 'ошибка', expired: 'истёк' }
const { data, error, reload } = useLoad(() => get('domains'), { interval: 5000 })
const host = ref('')
watch(data, (d, old) => { if (d && !old) host.value = d.publicHost })
watch(() => srv.id, () => { data.value = null; error.value = ''; reload() })
const busy = reactive({ add: false })
const add = reactive({ open: false, name: '', mode: 'letsencrypt', email: '', cert: '', key: '', forPanel: false })
const isIp = computed(() => /^[\d.]+$|:/.test(add.name.trim()))
const soon = computed(() => (data.value?.domains || []).filter(d => d.status === 'active' && d.daysLeft < 14 && d.mode !== 'letsencrypt').length + (data.value?.domains || []).filter(d => d.mode === 'letsencrypt' && d.status === 'active' && d.daysLeft < 3).length)
const panelDom = computed(() => (data.value?.domains || []).find(d => d.panel))
function openAdd() { Object.assign(add, { open: true, name: '', mode: 'letsencrypt', email: '', cert: '', key: '', forPanel: !panelDom.value }) }
// HTTPS of a linked server's panel: its address on this panel follows by itself
function goTo(url, msg) { if (remote.value) { ok(msg.replace('Панель', `Панель «${srvName.value}»`)); setTimeout(() => reload(true), 3000); return } ok(msg); setTimeout(() => { location.href = url }, 3000) }
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
async function remove(d) { if (await confirm(`Удалить ${d.name}?`, 'Сертификат и ключ будут удалены с сервера.' + (d.dyn ? ` Имя останется за вами на ${PROV[d.provider]} — его можно добавить снова той же ссылкой или ключом.` : ''), { danger: true, okText: 'Удалить' })) { try { await del('domains/' + d.id); reload(true) } catch (e) { fail(e) } } }
</script>
