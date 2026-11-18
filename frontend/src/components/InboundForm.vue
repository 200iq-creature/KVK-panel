<template>
  <div class="col">
    <div class="fl"><span class="lbl">Протокол</span>
      <Opt v-model="f.protocol" :options="PROTOS" :disabled="edit" />
      <p v-if="edit" class="hint">Протокол существующего инбаунда не меняется — создайте новый.</p>
    </div>
    <div class="two">
      <div class="fld"><label class="lbl">Название</label><input v-model="f.remark" class="inp" maxlength="40" placeholder="напр. Reality"></div>
      <div class="fld"><label class="lbl">Порт</label>
        <div class="row" style="flex-wrap:nowrap"><input v-model.number="f.port" class="inp" type="number" min="1" max="65535" placeholder="авто"><button class="btn btn-ic" type="button" title="Случайный свободный порт" @click="randomPort"><Icon n="refresh" /></button></div>
        <p class="hint" :class="portMsg.ok ? 'ok-t' : portMsg.text ? 'bad-t' : ''">{{ portMsg.text || portKind }}</p>
      </div>
    </div>
    <template v-if="!special">
      <div class="fl"><span class="lbl">Транспорт</span><Seg v-model="f.network" :options="nets" /></div>
      <div class="fl"><span class="lbl">Защита</span><Seg v-model="f.security" :options="secs" /></div>
    </template>
    <div v-else-if="f.protocol === 'hysteria2'" class="i-hint"><Icon n="info" /><span>Hysteria2 работает поверх QUIC (UDP) и всегда с TLS — выберите сертификат ниже. Для IP без домена подойдёт самоподписанный сертификат из раздела «Домены»: клиенты получат его отпечаток (pinSHA256).</span></div>
    <div v-else-if="f.protocol === 'tuic'" class="i-hint"><Icon n="info" /><span>TUIC v5 работает поверх QUIC (UDP) и всегда с TLS. Сервер встроен в панель и обновляется вместе с ней; трафик идёт через Xray, поэтому лимиты, статистика, WARP и фильтрация работают как обычно. Клиенты: Hiddify, NekoBox, Karing, sing-box, v2rayN, Clash Meta. Для самоподписанного сертификата в ссылке будет allow_insecure.</span></div>
    <div v-else-if="f.protocol === 'amneziawg'" class="i-hint"><Icon n="info" /><span>AmneziaWG — WireGuard с обфускацией рукопожатия: DPI не узнаёт протокол. Сервер встроен в панель и работает без модуля ядра; трафик идёт через Xray (лимиты, статистика, WARP, фильтрация). Клиенты: AmneziaVPN, AmneziaWG, Clash Meta — каждый получает свой .conf и QR-код в карточке клиента и на странице подписки.</span></div>
    <div v-else-if="f.protocol === 'masque'" class="i-hint"><Icon n="info" /><span>MASQUE (CONNECT-IP поверх HTTP/3 или HTTP/2) выглядит как обычный HTTPS-трафик. Работает в ядре Xray и всегда с TLS. Клиенты — приложения на свежем Xray (26.x): конфиг выдаётся в JSON-подписке и в карточке клиента. Для HTTP/3 нужен открытый UDP-порт, для HTTP/2 — TCP.</span></div>

    <template v-if="f.security === 'reality' && !special">
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

    <template v-if="(f.security === 'tls' && !special) || tlsAlways">
      <div class="fld"><label class="lbl">Сертификат</label>
        <select v-model="f.certId" class="inp"><option value="">— выберите —</option><option v-for="d in domains" :key="d.id" :value="d.id" :disabled="d.status !== 'active'">{{ d.name }} · {{ d.mode === 'selfsigned' ? 'самоподписанный' : d.mode === 'letsencrypt' ? "Let's Encrypt" : 'свой' }}{{ domUse(d) }}{{ d.status !== 'active' ? ' (не выпущен)' : '' }}</option></select>
        <p class="hint">Нет нужного? <router-link to="/domains">Добавьте домен или IP</router-link> — сертификат выпустится сам.</p>
      </div>
      <div class="two">
        <div class="fld"><label class="lbl">SNI для клиентов</label><input v-model="f.tlsServerName" class="inp mono" placeholder="домен сертификата"></div>
        <div v-if="!special" class="fld"><label class="lbl">ALPN</label><input v-model="f.alpnText" class="inp mono" placeholder="h2,http/1.1"></div>
        <div v-else-if="f.protocol === 'tuic'" class="fld"><label class="lbl">ALPN</label><input v-model="f.alpnText" class="inp mono" placeholder="h3"></div>
      </div>
    </template>

    <template v-if="f.protocol === 'tuic'">
      <div class="fl"><span class="lbl">Контроль перегрузки</span><Seg v-model="f.tuicCongestion" :options="[['bbr', 'BBR'], ['cubic', 'CUBIC'], ['new_reno', 'NewReno']]" /><p class="hint">BBR быстрее на каналах с потерями, CUBIC и NewReno — бережнее к сети.</p></div>
      <div class="fl"><span class="lbl">Передача UDP</span><Seg v-model="f.tuicUdpMode" :options="[['native', 'native'], ['quic', 'quic']]" /><p class="hint">native — датаграммы QUIC (быстрее), quic — потоки (надёжнее, если провайдер режет датаграммы). Режим выбирает клиент, значение попадает в ссылку.</p></div>
      <TgRow v-model="f.tuicZeroRtt" label="0-RTT" hint="Быстрее открываются соединения, но рукопожатие уязвимо к повтору. По умолчанию выключено." />
    </template>

    <template v-if="f.protocol === 'masque'">
      <div class="fl"><span class="lbl">Версия HTTP</span><Seg v-model="f.masqueHttp" :options="[['h3', 'HTTP/3 · UDP'], ['h2', 'HTTP/2 · TCP'], ['both', 'обе']]" /><p class="hint">«Обе» — сервер слушает TCP и UDP на одном порту, клиенты подключаются по HTTP/3.</p></div>
      <div class="two">
        <div class="fld"><label class="lbl">Путь</label><input v-model="f.path" class="inp mono" placeholder="/.well-known/masque/ip/*/*/"></div>
        <div class="fld"><label class="lbl">MTU туннеля</label><input v-model.number="f.mtu" class="inp" type="number" min="0" max="65535" placeholder="по умолчанию"><p class="hint">0 — по умолчанию; иначе 1280–65535</p></div>
      </div>
      <div class="fld"><label class="lbl">Пул адресов клиентов</label><input v-model="f.poolText" class="inp mono" placeholder="10.14.0.1/24, fd14::1/64"><p class="hint">не больше одной IPv4 и одной IPv6 подсети; адрес выдаётся на время подключения</p></div>
    </template>

    <template v-if="f.protocol === 'amneziawg'">
      <div class="two">
        <div class="fld"><label class="lbl">Подсеть туннеля</label><input v-model="f.awg.network" class="inp mono" placeholder="10.66.0.0/16"><p class="hint">сервер — первый адрес, клиенты — по порядку</p></div>
        <div class="fld"><label class="lbl">DNS для клиентов</label><input v-model="f.awgDns" class="inp mono" placeholder="1.1.1.1, 1.0.0.1"></div>
      </div>
      <div class="two">
        <div class="fld"><label class="lbl">MTU</label><input v-model.number="f.mtu" class="inp" type="number" min="1280" max="1500" placeholder="1280"></div>
        <div class="fld"><label class="lbl">Приватный ключ сервера</label><input v-model="f.awg.privateKey" class="inp mono" placeholder="пусто — сгенерируем"><p v-if="f.awg.publicKey" class="hint">Публичный: <span class="i-mono">{{ f.awg.publicKey }}</span></p></div>
      </div>
      <details class="det">
        <summary><span><b>Обфускация</b><small>Jc {{ f.awg.jc }} · S1 {{ f.awg.s1 }} · S2 {{ f.awg.s2 }} — параметры попадают в конфиги клиентов</small></span><Icon n="chevD" /></summary>
        <div class="det-b">
          <div class="i-hint"><Icon n="info" /><span>Значения сгенерированы случайно и совместимы со всеми клиентами AmneziaWG. После изменения клиентам нужно заново импортировать конфиг.</span></div>
          <div class="three">
            <div class="fld"><label class="lbl">Jc</label><input v-model.number="f.awg.jc" class="inp" type="number" min="0" max="128"></div>
            <div class="fld"><label class="lbl">Jmin</label><input v-model.number="f.awg.jmin" class="inp" type="number" min="0" max="1280"></div>
            <div class="fld"><label class="lbl">Jmax</label><input v-model.number="f.awg.jmax" class="inp" type="number" min="0" max="1280"></div>
          </div>
          <div class="three">
            <div class="fld"><label class="lbl">S1</label><input v-model.number="f.awg.s1" class="inp" type="number" min="0" max="1280"></div>
            <div class="fld"><label class="lbl">S2</label><input v-model.number="f.awg.s2" class="inp" type="number" min="0" max="1280"></div>
            <div class="fld"><label class="lbl">S3 / S4</label><div class="row" style="flex-wrap:nowrap"><input v-model.number="f.awg.s3" class="inp" type="number" min="0" max="1280" placeholder="0"><input v-model.number="f.awg.s4" class="inp" type="number" min="0" max="1280" placeholder="0"></div><p class="hint">AmneziaWG 2.0, 0 — выкл.</p></div>
          </div>
          <div class="two">
            <div class="fld"><label class="lbl">H1</label><input v-model="f.awg.h1" class="inp mono"></div>
            <div class="fld"><label class="lbl">H2</label><input v-model="f.awg.h2" class="inp mono"></div>
          </div>
          <div class="two">
            <div class="fld"><label class="lbl">H3</label><input v-model="f.awg.h3" class="inp mono"></div>
            <div class="fld"><label class="lbl">H4</label><input v-model="f.awg.h4" class="inp mono"></div>
          </div>
          <div v-for="k in ['i1', 'i2', 'i3', 'i4', 'i5']" :key="k" class="fld"><label class="lbl">{{ k.toUpperCase() }}</label><input v-model="f.awg[k]" class="inp mono" placeholder="необязательно, напр. <b 0x…>"></div>
          <div class="row"><button class="btn btn-sm" type="button" @click="regenObf"><Icon n="refresh" />Сгенерировать заново</button></div>
        </div>
      </details>
    </template>

    <template v-if="!special">
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
    <div v-if="f.security !== 'reality' && f.security !== 'tls' && !special" class="i-hint wa"><Icon n="warn" /><span>Без защиты трафик легко распознаётся. Новые клиенты Xray не подключаются к VLESS без TLS/Reality напрямую — используйте такой инбаунд только за CDN или для теста.</span></div>

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
        <div v-if="f.security !== 'none' && !special" class="fld"><label class="lbl">Отпечаток (fp)</label><select v-model="f.fingerprint" class="inp"><option value="">по умолчанию панели</option><option v-for="x in ['chrome', 'firefox', 'safari', 'ios', 'edge', 'android', 'random', 'randomized']" :key="x">{{ x }}</option></select></div>
        <TgRow v-if="!['tuic', 'amneziawg'].includes(f.protocol)" v-model="f.sniffing" label="Sniffing" hint="Определять домен по трафику — нужно для фильтрации и правил по доменам" />
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
import { sget as get } from '../lib/server.js'
import { fail } from '../lib/ui.js'

const p = defineProps({ inbound: Object, edit: Boolean })
const PROTOS = [
  { v: 'vless', l: 'VLESS', s: 'рекомендуется' }, { v: 'vmess', l: 'VMess' }, { v: 'trojan', l: 'Trojan' }, { v: 'shadowsocks', l: 'Shadowsocks' },
  { v: 'hysteria2', l: 'Hysteria2', s: 'UDP / QUIC' }, { v: 'tuic', l: 'TUIC v5', s: 'UDP / QUIC · встроен в панель' },
  { v: 'amneziawg', l: 'AmneziaWG', s: 'WireGuard с обфускацией · встроен в панель' }, { v: 'masque', l: 'MASQUE', s: 'HTTP/3 · ядро Xray' },
]
function masqueHttpOf(alpn) { const a = alpn || []; const h2 = a.includes('h2'), h3 = a.includes('h3') || !h2; return h2 && h3 ? 'both' : h2 ? 'h2' : 'h3' }
const rnd = (lo, hi) => lo + Math.floor(Math.random() * (hi - lo + 1))
function obf() {
  const o = { jc: rnd(3, 6), jmin: rnd(40, 60), s1: rnd(15, 150), s3: 0, s4: 0, i1: '', i2: '', i3: '', i4: '', i5: '' }
  o.jmax = o.jmin + rnd(20, 70)
  do { o.s2 = rnd(15, 150) } while (o.s1 + 56 === o.s2)
  const seen = new Set()
  for (const k of ['h1', 'h2', 'h3', 'h4']) {
    let v
    do { v = rnd(5, 2147483647) } while (seen.has(v))
    seen.add(v); o[k] = String(v)
  }
  return o
}
function newAwg() { return { network: '10.66.0.0/16', privateKey: '', publicKey: '', ...obf() } }
const SS = ['2022-blake3-aes-128-gcm', '2022-blake3-aes-256-gcm', '2022-blake3-chacha20-poly1305', 'aes-128-gcm', 'aes-256-gcm', 'chacha20-ietf-poly1305', 'xchacha20-ietf-poly1305']
const i = p.inbound || {}
const f = reactive({
  protocol: i.protocol || 'vless', remark: i.remark || '', port: i.port || null, network: i.network || 'tcp', security: i.security || 'reality', flow: i.flow ?? 'xtls-rprx-vision',
  realityTarget: i.realityTarget || 'www.microsoft.com:443', sniText: (i.realitySni || []).join(', '), privateKey: i.privateKey || '', publicKey: i.publicKey || '', sidText: (i.shortIds || []).join(', '), spiderX: i.spiderX || '/',
  fingerprint: i.fingerprint || '', certId: i.certId || '', tlsServerName: i.tlsServerName || '', alpnText: (i.alpn || []).join(','),
  path: i.path || '', host: i.host || '', serviceName: i.serviceName || '', xhttpMode: i.xhttpMode || 'auto', headerType: i.headerType || '',
  ssMethod: i.ssMethod || '2022-blake3-aes-128-gcm', ssPassword: i.ssPassword || '', obfsPassword: i.obfsPassword || '',
  externalHost: i.externalHost || '', externalPort: i.externalPort || null, listen: i.listen || '', tag: i.tag || '', sniffing: i.id ? !!i.sniffing : true, grant: true,
  tuicCongestion: i.tuicCongestion || 'bbr', tuicUdpMode: i.tuicUdpMode || 'native', tuicZeroRtt: !!i.tuicZeroRtt,
  masqueHttp: masqueHttpOf(i.alpn), mtu: i.mtu || null, poolText: (i.masquePool || []).join(', '),
  awg: { ...(i.awg || newAwg()) }, awgDns: ((i.awg && i.awg.dns) || ['1.1.1.1', '1.0.0.1']).join(', '),
})
if (i.protocol === 'tuic' && !f.alpnText) f.alpnText = 'h3'
const special = computed(() => ['hysteria2', 'tuic', 'amneziawg', 'masque'].includes(f.protocol))
const tlsAlways = computed(() => ['hysteria2', 'tuic', 'masque'].includes(f.protocol))
const portKind = computed(() => {
  if (['hysteria2', 'tuic', 'amneziawg'].includes(f.protocol)) return 'UDP-порт'
  if (f.protocol === 'masque') return f.masqueHttp === 'h2' ? 'TCP-порт' : f.masqueHttp === 'both' ? 'TCP + UDP порт' : 'UDP-порт'
  if (f.protocol === 'shadowsocks') return 'TCP + UDP порт'
  return 'TCP-порт'
})
const domains = ref([])
const domUse = (d) => [d.site ? (d.sni ? 'сайт-прикрытие, маска Reality' : 'сайт-прикрытие') : '', d.panelOf ? `панель «${d.panelOf}»` : ''].filter(Boolean).map(s => ' — ' + s).join('')
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
watch(() => [f.port, f.protocol, f.masqueHttp], ([v]) => {
  clearTimeout(pt); portMsg.text = ''
  if (!v) return
  pt = setTimeout(async () => {
    try { const r = await get(`tools/port?port=${v}${i.id ? '&except=' + i.id : ''}&protocol=${f.protocol}&alpn=${f.protocol === 'masque' ? ({ h3: 'h3', h2: 'h2', both: 'h3,h2' })[f.masqueHttp] : ''}`); portMsg.ok = r.free; portMsg.text = r.free ? 'порт свободен' : (r.reason || 'порт занят') } catch {}
  }, 400)
})
async function randomPort() { try { const r = await get('tools/port?random=1'); f.port = r.port } catch (e) { fail(e) } }
function regenObf() { Object.assign(f.awg, obf()) }
watch(() => f.protocol, (v) => { if (v === 'tuic' && !f.alpnText) f.alpnText = 'h3'; if (v !== 'tuic' && f.alpnText === 'h3') f.alpnText = '' })
async function genKeys() { try { const r = await get('tools/x25519'); f.privateKey = r.privateKey; f.publicKey = r.publicKey; if (!f.sidText) f.sidText = r.shortId } catch (e) { fail(e) } }
onMounted(async () => { try { domains.value = (await get('domains')).domains } catch {} })
const list = (s) => s.split(/[,\s]+/).map(x => x.trim()).filter(Boolean)

function body() {
  const b = {
    ...i, protocol: f.protocol, remark: f.remark.trim(), port: Number(f.port) || 0, network: special.value ? 'tcp' : f.network,
    security: f.protocol === 'amneziawg' ? 'none' : tlsAlways.value ? 'tls' : f.security, flow: f.flow, realityTarget: f.realityTarget.trim(), realitySni: list(f.sniText),
    privateKey: f.privateKey.trim(), shortIds: list(f.sidText).map(s => s.toLowerCase()), spiderX: f.spiderX, fingerprint: f.fingerprint, certId: f.certId, tlsServerName: f.tlsServerName.trim(),
    alpn: list(f.alpnText), path: f.path.trim(), host: f.host.trim(), serviceName: f.serviceName.trim(), xhttpMode: f.network === 'xhttp' ? f.xhttpMode : '', headerType: f.headerType,
    ssMethod: f.ssMethod, ssPassword: f.ssPassword.trim(), obfsPassword: f.obfsPassword.trim(), externalHost: f.externalHost.trim(), externalPort: Number(f.externalPort) || 0,
    listen: f.listen.trim(), tag: f.tag.trim(), sniffing: f.sniffing,
    tuicCongestion: f.tuicCongestion, tuicUdpMode: f.tuicUdpMode, tuicZeroRtt: f.tuicZeroRtt, mtu: Number(f.mtu) || 0,
    masquePool: list(f.poolText), awg: null,
  }
  if (f.protocol === 'masque') b.alpn = ({ h3: ['h3'], h2: ['h2'], both: ['h3', 'h2'] })[f.masqueHttp]
  if (f.protocol === 'amneziawg') {
    const a = f.awg, n = (x) => Number(x) || 0
    b.awg = { ...a, network: (a.network || '').trim(), privateKey: (a.privateKey || '').trim(), dns: list(f.awgDns), jc: n(a.jc), jmin: n(a.jmin), jmax: n(a.jmax), s1: n(a.s1), s2: n(a.s2), s3: n(a.s3), s4: n(a.s4) }
    for (const k of ['h1', 'h2', 'h3', 'h4', 'i1', 'i2', 'i3', 'i4', 'i5']) b.awg[k] = String(a[k] ?? '').trim()
  }
  delete b.clientList; delete b.clients; delete b.online; delete b.certName
  return b
}
defineExpose({ body, grant: () => f.grant })
</script>
