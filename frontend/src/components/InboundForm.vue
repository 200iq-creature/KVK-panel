<template>
  <div class="col">
    <div class="fl"><span class="lbl">Протокол</span>
      <Opt v-model="f.protocol" :options="[{ v: 'vless', l: 'VLESS', s: 'рекомендуется' }, { v: 'vmess', l: 'VMess' }, { v: 'trojan', l: 'Trojan' }, { v: 'shadowsocks', l: 'Shadowsocks' }, { v: 'hysteria2', l: 'Hysteria2', s: 'UDP / QUIC' }]" :disabled="edit" />
      <p v-if="edit" class="hint">Протокол существующего инбаунда не меняется — создайте новый.</p>
    </div>
    <div class="two">
      <div class="fld"><label class="lbl">Название</label><input v-model="f.remark" class="inp" maxlength="40" placeholder="напр. Reality"></div>
      <div class="fld"><label class="lbl">Порт</label>
        <div class="row" style="flex-wrap:nowrap"><input v-model.number="f.port" class="inp" type="number" min="1" max="65535" placeholder="авто"><button class="btn btn-ic" type="button" title="Случайный свободный порт" @click="randomPort"><Icon n="refresh" /></button></div>
        <p class="hint" :class="portMsg.ok ? 'ok-t' : portMsg.text ? 'bad-t' : ''">{{ portMsg.text || (f.protocol === 'hysteria2' ? 'UDP-порт' : 'TCP-порт') }}</p>
      </div>
    </div>
    <template v-if="f.protocol !== 'hysteria2'">
      <div class="fl"><span class="lbl">Транспорт</span><Seg v-model="f.network" :options="nets" /></div>
      <div class="fl"><span class="lbl">Защита</span><Seg v-model="f.security" :options="secs" /></div>
    </template>
    <div v-else class="i-hint"><Icon n="info" /><span>Hysteria2 работает поверх QUIC (UDP) и всегда с TLS — выберите сертификат ниже. Для IP без домена подойдёт самоподписанный сертификат из раздела «Домены»: клиенты получат его отпечаток (pinSHA256).</span></div>

    <template v-if="f.security === 'reality' && f.protocol !== 'hysteria2'">
      <div class="i-hint"><Icon n="shield" /><span>Reality маскирует подключение под чужой сайт: DPI видит обычный TLS к нему. Цель должна поддерживать TLS 1.3 и HTTP/2 и открываться с сервера.</span></div>
      <div class="two">
        <div class="fld"><label class="lbl">Сайт для маскировки (dest)</label><input v-model="f.realityTarget" class="inp mono" placeholder="www.microsoft.com:443"></div>
        <div class="fld"><label class="lbl">SNI (serverNames)</label><input v-model="f.sniText" class="inp mono" placeholder="www.microsoft.com"><p class="hint">через запятую; пусто — домен из dest</p></div>
      </div>
      <div class="row"><span class="hint">Быстрый выбор:</span><button v-for="s in ['www.microsoft.com', 'www.apple.com', 'dl.google.com', 'www.samsung.com', 'github.com']" :key="s" class="btn btn-sm btn-g" type="button" @click="f.realityTarget = s + ':443'; f.sniText = s">{{ s }}</button></div>
      <div class="fld"><label class="lbl">Приватный ключ</label><div class="row" style="flex-wrap:nowrap"><input v-model="f.privateKey" class="inp mono" placeholder="пусто — сгенерируем"><button class="btn" type="button" @click="genKeys">Сгенерировать</button></div><p v-if="f.publicKey" class="hint">Публичный (pbk): <span class="i-mono">{{ f.publicKey }}</span></p></div>
      <div class="two">
        <div class="fld"><label class="lbl">shortId</label><input v-model="f.sidText" class="inp mono" placeholder="авто"><p class="hint">hex, через запятую</p></div>
        <div class="fld"><label class="lbl">spiderX</label><input v-model="f.spiderX" class="inp mono" placeholder="/"></div>
      </div>
    </template>

    <template v-if="f.security === 'tls' || f.protocol === 'hysteria2'">
      <div class="fld"><label class="lbl">Сертификат</label>
        <select v-model="f.certId" class="inp"><option value="">— выберите —</option><option v-for="d in domains" :key="d.id" :value="d.id" :disabled="d.status !== 'active'">{{ d.name }} · {{ d.mode === 'selfsigned' ? 'самоподписанный' : d.mode === 'acme' ? "Let's Encrypt" : 'свой' }}{{ d.status !== 'active' ? ' (не выпущен)' : '' }}</option></select>
        <p class="hint">Нет нужного? <router-link to="/domains">Добавьте домен или IP</router-link> — сертификат выпустится сам.</p>
      </div>
      <div class="two">
        <div class="fld"><label class="lbl">SNI для клиентов</label><input v-model="f.tlsServerName" class="inp mono" placeholder="домен сертификата"></div>
        <div v-if="f.protocol !== 'hysteria2'" class="fld"><label class="lbl">ALPN</label><input v-model="f.alpnText" class="inp mono" placeholder="h2,http/1.1"></div>
      </div>
    </template>

    <template v-if="f.protocol !== 'hysteria2'">
      <div v-if="['ws', 'xhttp', 'httpupgrade'].includes(f.network)" class="two">
        <div class="fld"><label class="lbl">Путь</label><input v-model="f.path" class="inp mono" placeholder="авто"></div>
        <div class="fld"><label class="lbl">Host</label><input v-model="f.host" class="inp mono" placeholder="необязательно"></div>
      </div>
      <div v-if="f.network === 'xhttp'" class="fld"><label class="lbl">Режим XHTTP</label><Seg v-model="f.xhttpMode" :options="[['auto', 'auto'], ['packet-up', 'packet-up'], ['stream-up', 'stream-up'], ['stream-one', 'stream-one']]" /></div>
      <div v-if="f.network === 'grpc'" class="fld"><label class="lbl">serviceName</label><input v-model="f.serviceName" class="inp mono" placeholder="авто"></div>
      <div v-if="f.network === 'tcp' && f.security === 'none'" class="fld"><label class="lbl">Маскировка заголовка</label><Seg v-model="f.headerType" :options="[['', 'нет'], ['http', 'HTTP']]" /></div>
      <div v-if="f.protocol === 'vless' && f.network === 'tcp' && f.security !== 'none'" class="fld"><label class="lbl">Flow</label><Seg v-model="f.flow" :options="[['xtls-rprx-vision', 'xtls-rprx-vision'], ['', 'без flow']]" /></div>
    </template>

    <div v-if="f.protocol === 'shadowsocks'" class="two">
      <div class="fld"><label class="lbl">Метод</label><select v-model="f.ssMethod" class="inp"><option v-for="m in SS" :key="m">{{ m }}</option></select></div>
      <div class="fld"><label class="lbl">Ключ сервера</label><input v-model="f.ssPassword" class="inp mono" placeholder="авто"><p class="hint">для 2022 — base64 нужной длины; пусто — сгенерируем</p></div>
    </div>
    <div v-if="f.protocol === 'hysteria2'" class="fld"><label class="lbl">Пароль obfs (salamander)</label><input v-model="f.obfsPassword" class="inp mono" placeholder="пусто — без обфускации"><p class="hint">Прячет QUIC от DPI. Клиенты получат пароль в ссылке.</p></div>
    <div v-if="f.security !== 'reality' && f.security !== 'tls' && f.protocol !== 'hysteria2'" class="i-hint wa"><Icon n="warn" /><span>Без защиты трафик легко распознаётся. Новые клиенты Xray не подключаются к VLESS без TLS/Reality напрямую — используйте такой инбаунд только за CDN или для теста.</span></div>

    <details class="det">
      <summary><span><b>Дополнительно</b><small>адрес для клиентов, listen, тег, sniffing</small></span><Icon n="chevD" /></summary>
      <div class="det-b">
        <div class="two">
          <div class="fld"><label class="lbl">Адрес в ссылках</label><input v-model="f.externalHost" class="inp mono" placeholder="по умолчанию — host сервера"><p class="hint">домен CDN или другой IP</p></div>
          <div class="fld"><label class="lbl">Порт в ссылках</label><input v-model.number="f.externalPort" class="inp" type="number" min="0" max="65535" placeholder="тот же"></div>
        </div>
        <div class="two">
          <div class="fld"><label class="lbl">Listen</label><input v-model="f.listen" class="inp mono" placeholder="0.0.0.0"></div>
          <div class="fld"><label class="lbl">Тег</label><input v-model="f.tag" class="inp mono" placeholder="авто"></div>
        </div>
        <div v-if="f.security !== 'none'" class="fld"><label class="lbl">Отпечаток (fp)</label><select v-model="f.fingerprint" class="inp"><option value="">по умолчанию панели</option><option v-for="x in ['chrome', 'firefox', 'safari', 'ios', 'edge', 'android', 'random', 'randomized']" :key="x">{{ x }}</option></select></div>
        <TgRow v-model="f.sniffing" label="Sniffing" hint="Определять домен по трафику — нужно для фильтрации и правил по доменам" />
      </div>
    </details>
    <TgRow v-if="!edit" v-model="f.grant" label="Выдать всем клиентам" hint="Действующие клиенты сразу получат это подключение в подписке" />
  </div>
</template>
<script setup>
import { reactive, computed, watch, onMounted, ref } from 'vue'
import Icon from './Icon.vue'
import Seg from './Seg.vue'
import Opt from './Opt.vue'
import TgRow from './TgRow.vue'
import { get } from '../lib/api.js'
import { fail } from '../lib/ui.js'

const p = defineProps({ inbound: Object, edit: Boolean })
const SS = ['2022-blake3-aes-128-gcm', '2022-blake3-aes-256-gcm', '2022-blake3-chacha20-poly1305', 'aes-128-gcm', 'aes-256-gcm', 'chacha20-ietf-poly1305', 'xchacha20-ietf-poly1305']
const i = p.inbound || {}
const f = reactive({
  protocol: i.protocol || 'vless', remark: i.remark || '', port: i.port || null, network: i.network || 'tcp', security: i.security || 'reality', flow: i.flow ?? 'xtls-rprx-vision',
  realityTarget: i.realityTarget || 'www.microsoft.com:443', sniText: (i.realitySni || []).join(', '), privateKey: i.privateKey || '', publicKey: i.publicKey || '', sidText: (i.shortIds || []).join(', '), spiderX: i.spiderX || '/',
  fingerprint: i.fingerprint || '', certId: i.certId || '', tlsServerName: i.tlsServerName || '', alpnText: (i.alpn || []).join(','),
  path: i.path || '', host: i.host || '', serviceName: i.serviceName || '', xhttpMode: i.xhttpMode || 'auto', headerType: i.headerType || '',
  ssMethod: i.ssMethod || '2022-blake3-aes-128-gcm', ssPassword: i.ssPassword || '', obfsPassword: i.obfsPassword || '',
  externalHost: i.externalHost || '', externalPort: i.externalPort || null, listen: i.listen || '', tag: i.tag || '', sniffing: i.id ? !!i.sniffing : true, grant: true,
})
const domains = ref([])
const portMsg = reactive({ ok: false, text: '' })
const nets = computed(() => [['tcp', 'TCP'], ['xhttp', 'XHTTP'], ['grpc', 'gRPC'], ['ws', 'WebSocket'], ['httpupgrade', 'HTTPUpgrade']])
const secs = computed(() => {
  const s = [['none', 'без защиты'], ['tls', 'TLS']]
  if (f.protocol !== 'vmess' && !['ws', 'httpupgrade'].includes(f.network)) s.push(['reality', 'Reality'])
  return s
})
watch(() => f.protocol, (v) => { if (v === 'vmess' && f.security === 'reality') f.security = 'tls'; if (v === 'shadowsocks' && f.security === 'reality') f.security = 'none' })
watch(() => f.network, (v) => { if (['ws', 'httpupgrade'].includes(v) && f.security === 'reality') f.security = 'tls' })
let pt
watch(() => f.port, (v) => {
  clearTimeout(pt); portMsg.text = ''
  if (!v) return
  pt = setTimeout(async () => {
    try { const r = await get(`tools/port?port=${v}${i.id ? '&except=' + i.id : ''}`); portMsg.ok = r.free; portMsg.text = r.free ? 'порт свободен' : (r.reason || 'порт занят') } catch {}
  }, 400)
})
async function randomPort() { try { const r = await get('tools/port?random=1'); f.port = r.port } catch (e) { fail(e) } }
async function genKeys() { try { const r = await get('tools/x25519'); f.privateKey = r.privateKey; f.publicKey = r.publicKey; if (!f.sidText) f.sidText = r.shortId } catch (e) { fail(e) } }
onMounted(async () => { try { domains.value = (await get('domains')).domains } catch {} })
const list = (s) => s.split(/[,\s]+/).map(x => x.trim()).filter(Boolean)

function body() {
  const b = {
    ...i, protocol: f.protocol, remark: f.remark.trim(), port: Number(f.port) || 0, network: f.protocol === 'hysteria2' ? 'tcp' : f.network,
    security: f.protocol === 'hysteria2' ? 'tls' : f.security, flow: f.flow, realityTarget: f.realityTarget.trim(), realitySni: list(f.sniText),
    privateKey: f.privateKey.trim(), shortIds: list(f.sidText).map(s => s.toLowerCase()), spiderX: f.spiderX, fingerprint: f.fingerprint, certId: f.certId, tlsServerName: f.tlsServerName.trim(),
    alpn: list(f.alpnText), path: f.path.trim(), host: f.host.trim(), serviceName: f.serviceName.trim(), xhttpMode: f.network === 'xhttp' ? f.xhttpMode : '', headerType: f.headerType,
    ssMethod: f.ssMethod, ssPassword: f.ssPassword.trim(), obfsPassword: f.obfsPassword.trim(), externalHost: f.externalHost.trim(), externalPort: Number(f.externalPort) || 0,
    listen: f.listen.trim(), tag: f.tag.trim(), sniffing: f.sniffing,
  }
  delete b.clientList; delete b.clients; delete b.online; delete b.certName
  return b
}
defineExpose({ body, grant: () => f.grant })
</script>
