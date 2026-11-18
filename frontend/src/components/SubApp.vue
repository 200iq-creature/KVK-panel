<template>
  <div class="sp-root">
    <div class="sp-in">
      <div v-if="loading" class="empty" style="padding-top:20vh"><span class="spin"></span></div>
      <div v-else-if="err" class="empty" style="padding-top:20vh"><span class="empty-ic"><Icon n="warn" /></span><b>Подписка не найдена</b><span class="hint">{{ err }}</span></div>
      <template v-else>
        <header class="sp-hd" style="display:flex;align-items:center;gap:12px">
          <img :src="d.logo || LOGO_URL" alt="" style="width:40px;height:40px;border-radius:10px;object-fit:contain">
          <div style="flex:1;min-width:0"><h1 style="margin:0;font-size:22px">{{ d.title || 'VPN' }}</h1><span class="muted">{{ d.name }}</span></div>
          <button class="btn btn-ic btn-g" type="button" :title="dark ? 'Светлая тема' : 'Тёмная тема'" @click="toggleTheme"><Icon :n="dark ? 'sun' : 'moon'" /></button>
        </header>

        <section class="i-pn">
          <div class="i-pb">
            <div class="row" style="justify-content:space-between;align-items:center">
              <span class="i-stt" :class="STATUS[d.status]?.[1] || 'mu'" style="font-size:15px">{{ STATUS[d.status]?.[0] || d.status }}</span>
              <span class="strong">{{ d.expiresAt ? (d.status === 'expired' ? 'истекла ' + date(d.expiresAt) : 'до ' + date(d.expiresAt) + ' · ' + left(d.expiresAt)) : 'бессрочно' }}</span>
            </div>
            <template v-if="d.showTraffic">
              <div class="row" style="justify-content:space-between;margin-top:14px"><span class="muted">Трафик</span><span><b>{{ bytes(d.up + d.down) }}</b><span class="muted">&nbsp;{{ d.limit ? 'из ' + bytes(d.limit) : '· без лимита' }}</span></span></div>
              <div v-if="d.limit" class="i-mpr" :class="pct > 90 ? 'bad' : pct > 70 ? 'wa' : 'ok'" style="margin-top:8px"><i :style="{ width: pct + '%' }"></i></div>
            </template>
            <div v-if="d.status !== 'active' && d.renewUrl" class="i-alrt wa" style="margin-top:14px"><Icon n="clock" /><span><b>Подписка не активна.</b> <span class="muted">Продлите её, чтобы VPN снова заработал.</span></span><a class="btn btn-sm btn-p" :href="d.renewUrl" target="_blank" rel="noopener">Продлить</a></div>
          </div>
        </section>

        <div v-if="d.text" class="i-hint" style="white-space:pre-line"><Icon n="info" /><span>{{ d.text }}</span></div>

        <section class="i-pn">
          <div class="i-ph"><h2>Подключить</h2><span>{{ PLATFORMS.find(p => p[0] === platform)?.[1] }}</span></div>
          <div class="i-pb">
            <div class="i-seg" style="margin-bottom:14px;flex-wrap:wrap">
              <button v-for="p in PLATFORMS" :key="p[0]" type="button" :class="{ on: platform === p[0] }" @click="platform = p[0]">{{ p[1] }}</button>
            </div>
            <ol class="steps">
              <li>Установите приложение: <template v-for="(a, i) in apps" :key="a.name"><a :href="a.store" target="_blank" rel="noopener">{{ a.name }}</a><span v-if="i < apps.length - 1">, </span></template>.</li>
              <li>Нажмите кнопку — подписка добавится в приложение сама.</li>
              <li>Включите VPN. Подписка обновляется автоматически.</li>
            </ol>
            <div class="row" style="margin-top:12px">
              <a v-for="a in apps" :key="a.name" class="btn" :class="a === apps[0] ? 'btn-p' : ''" :href="a.link(subUrl)"><Icon n="plus" />Добавить в {{ a.name }}</a>
            </div>
            <div class="vw-link" style="margin-top:16px;display:flex;align-items:center;gap:8px"><code class="i-mono" style="flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">{{ subUrl }}</code><button class="btn btn-sm" type="button" @click="copyText(subUrl, 'Ссылка подписки скопирована')"><Icon n="copy" />Копировать</button></div>
            <div style="display:flex;justify-content:center;margin-top:16px"><Qr :text="subUrl" :size="220" /></div>
            <p class="hint" style="text-align:center">Отсканируйте QR в приложении на другом устройстве</p>
          </div>
        </section>

        <section class="i-pn">
          <div class="i-ph"><h2>Форматы подписки</h2><span>если приложение не добавило всё</span></div>
          <div class="i-pb">
            <div class="list-rows">
              <div v-for="f in formats" :key="f.id" class="lr">
                <span class="lr-t"><b>{{ f.name }}</b><small>{{ f.note }}</small></span>
                <span class="acts">
                  <a v-if="f.add" class="btn btn-sm btn-g" :href="f.add">{{ f.addLabel }}</a>
                  <button class="btn btn-sm btn-g btn-ic" type="button" title="QR" @click="qr = qr === 'f' + f.id ? -1 : 'f' + f.id"><Icon n="qr" /></button>
                  <button class="btn btn-sm btn-g btn-ic" type="button" title="Копировать ссылку" @click="copyText(f.url, 'Ссылка скопирована')"><Icon n="copy" /></button>
                </span>
                <div v-if="qr === 'f' + f.id" style="flex-basis:100%;display:flex;justify-content:center;padding:8px 0"><Qr :text="f.url" :size="220" /></div>
              </div>
            </div>
          </div>
        </section>

        <section v-if="awg.length" class="i-pn">
          <div class="i-ph"><h2>AmneziaWG <span class="i-cnt">{{ awg.length }}</span></h2><span>отдельное приложение</span></div>
          <div class="i-pb">
            <p class="hint" style="margin:0 0 10px">Установите <a href="https://amnezia.org/downloads" target="_blank" rel="noopener">AmneziaVPN</a> или AmneziaWG, затем импортируйте файл конфигурации или отсканируйте QR-код.</p>
            <div class="list-rows">
              <div v-for="(l, i) in awg" :key="'a' + i" class="lr">
                <span class="lr-t"><b>{{ l.name }}</b><small>AmneziaWG{{ l.node ? ' · ' + l.node : '' }}</small></span>
                <span class="acts"><button class="btn btn-sm btn-g btn-ic" type="button" title="Скачать .conf" @click="dl(l)"><Icon n="download" /></button><button class="btn btn-sm btn-g btn-ic" type="button" title="QR" @click="qr = qr === 'a' + i ? -1 : 'a' + i"><Icon n="qr" /></button><button class="btn btn-sm btn-g btn-ic" type="button" title="Копировать" @click="copyText(l.config, 'Конфиг скопирован')"><Icon n="copy" /></button></span>
                <div v-if="qr === 'a' + i" style="flex-basis:100%;display:flex;justify-content:center;padding:8px 0"><Qr :text="l.config" :size="260" /></div>
              </div>
            </div>
          </div>
        </section>

        <section v-if="d.view === 'classic' && plain.length" class="i-pn">
          <div class="i-ph"><h2>Конфигурации <span class="i-cnt">{{ plain.length }}</span></h2><button class="btn btn-sm btn-g" type="button" @click="copyText(plain.map(l => l.link).join('\n'), 'Все ссылки скопированы')"><Icon n="copy" />Все</button></div>
          <div class="list-rows" style="padding:0 16px 16px">
            <div v-for="(l, i) in plain" :key="i" class="lr">
              <span class="lr-t"><b>{{ l.name }}</b><small>{{ (l.protocol || '').toUpperCase() }}{{ l.node ? ' · ' + l.node : '' }}</small></span>
              <span class="acts"><button class="btn btn-sm btn-g btn-ic" type="button" title="QR" @click="qr = qr === i ? -1 : i"><Icon n="qr" /></button><button class="btn btn-sm btn-g btn-ic" type="button" title="Копировать" @click="copyText(l.link)"><Icon n="copy" /></button></span>
              <div v-if="qr === i" style="flex-basis:100%;display:flex;justify-content:center;padding:8px 0"><Qr :text="l.link" :size="200" /></div>
            </div>
          </div>
        </section>

        <footer class="row" style="justify-content:center;gap:16px">
          <a v-if="d.support" class="btn btn-g" :href="d.support" target="_blank" rel="noopener"><Icon n="send" />Поддержка</a>
          <a v-if="d.renewUrl && d.status === 'active'" class="btn btn-g" :href="d.renewUrl" target="_blank" rel="noopener"><Icon n="clock" />Продлить</a>
        </footer>
        <div v-if="toast" class="i-alrt" style="position:fixed;left:50%;bottom:24px;transform:translateX(-50%);z-index:50;box-shadow:0 8px 30px rgba(0,0,0,.2)"><Icon n="check" /><span>{{ toast }}</span></div>
      </template>
    </div>
  </div>
</template>
<script setup>
import { ref, computed, onMounted } from 'vue'
import Icon from './Icon.vue'
import Qr from './Qr.vue'
import { bytes, date, left } from '../lib/fmt.js'
import { STATUS } from '../lib/client.js'

const LOGO_URL = new URL('logo.svg', document.baseURI).href
const enc = encodeURIComponent
const APPS = {
  happ: { name: 'Happ', link: (u) => 'happ://add/' + u, stores: { ios: 'https://apps.apple.com/app/happ-proxy-utility/id6504287215', android: 'https://play.google.com/store/apps/details?id=com.happproxy', windows: 'https://github.com/Happ-proxy/happ-desktop/releases', macos: 'https://apps.apple.com/app/happ-proxy-utility/id6504287215' } },
  v2raytun: { name: 'v2rayTun', link: (u) => 'v2raytun://import/' + u, stores: { ios: 'https://apps.apple.com/app/v2raytun/id6476628951', android: 'https://play.google.com/store/apps/details?id=com.v2raytun.android', windows: 'https://v2raytun.com/', macos: 'https://apps.apple.com/app/v2raytun/id6476628951' } },
  hiddify: { name: 'Hiddify', link: (u) => 'hiddify://import/' + u, stores: { ios: 'https://apps.apple.com/app/hiddify-proxy-vpn/id6596777532', android: 'https://play.google.com/store/apps/details?id=app.hiddify.com', windows: 'https://github.com/hiddify/hiddify-app/releases', macos: 'https://github.com/hiddify/hiddify-app/releases', linux: 'https://github.com/hiddify/hiddify-app/releases' } },
  streisand: { name: 'Streisand', link: (u) => 'streisand://import/' + u, stores: { ios: 'https://apps.apple.com/app/streisand/id6450534064', macos: 'https://apps.apple.com/app/streisand/id6450534064' } },
  v2rayng: { name: 'v2rayNG', link: (u) => 'v2rayng://install-config?url=' + enc(u), stores: { android: 'https://github.com/2dust/v2rayNG/releases' } },
  flclash: { name: 'FlClash', link: (u) => 'clash://install-config?url=' + enc(u.includes('?') ? u + '&format=clash' : u + '?format=clash'), stores: { windows: 'https://github.com/chen08209/FlClash/releases', linux: 'https://github.com/chen08209/FlClash/releases', macos: 'https://github.com/chen08209/FlClash/releases', android: 'https://github.com/chen08209/FlClash/releases' } },
}
const BY = { ios: ['happ', 'v2raytun', 'streisand', 'hiddify'], android: ['happ', 'v2raytun', 'hiddify', 'v2rayng'], windows: ['happ', 'hiddify', 'flclash', 'v2raytun'], macos: ['happ', 'v2raytun', 'hiddify', 'streisand'], linux: ['hiddify', 'flclash'] }
const PLATFORMS = [['ios', 'iPhone / iPad'], ['android', 'Android'], ['windows', 'Windows'], ['macos', 'macOS'], ['linux', 'Linux']]
function detect() {
  const ua = navigator.userAgent
  if (/iPhone|iPad|iPod/i.test(ua) || (/Macintosh/.test(ua) && navigator.maxTouchPoints > 1)) return 'ios'
  if (/Android/i.test(ua)) return 'android'
  if (/Windows/i.test(ua)) return 'windows'
  if (/Mac OS X/i.test(ua)) return 'macos'
  return 'linux'
}
const platform = ref(detect())
const apps = computed(() => BY[platform.value].map(k => ({ ...APPS[k], store: APPS[k].stores[platform.value] })))
const d = ref(null)
const err = ref('')
const awg = computed(() => (d.value?.links || []).filter(l => l.configType === 'awg' && l.config))
const plain = computed(() => (d.value?.links || []).filter(l => l.link))
function dl(l) {
  const name = (l.name || 'awg').replace(/[^a-zA-Z0-9_=+.-]+/g, '_').replace(/^_+|_+$/g, '').slice(0, 15) || 'awg'
  const a = document.createElement('a')
  a.href = URL.createObjectURL(new Blob([l.config], { type: 'text/plain' }))
  a.download = name + (l.configType === 'awg' ? '.conf' : '.json')
  document.body.appendChild(a); a.click(); a.remove()
  setTimeout(() => URL.revokeObjectURL(a.href), 2000)
}
const loading = ref(true)
const qr = ref(-1)
const toast = ref('')
const dark = ref(document.documentElement.classList.contains('dark'))
const subUrl = computed(() => d.value?.subUrl || location.href.split('?')[0])
const withFmt = (f) => subUrl.value + (subUrl.value.includes('?') ? '&' : '?') + 'format=' + f
const hasMasque = computed(() => (d.value?.links || []).some(l => l.configType === 'xray-json'))
const hasTuic = computed(() => (d.value?.links || []).some(l => l.protocol === 'tuic'))
const formats = computed(() => {
  const json = withFmt('json'), clash = withFmt('clash'), sb = withFmt('singbox')
  return [
    { id: 'json', name: 'JSON (Xray)', url: json, add: 'happ://add/' + json, addLabel: 'В Happ',
      note: 'всё в одном файле' + (hasMasque.value ? ', включая MASQUE' : '') + ' — Happ, v2RayTun, INCY, v2rayN' },
    { id: 'singbox', name: 'sing-box', url: sb, add: 'sing-box://import-remote-profile?url=' + enc(sb) + '#' + enc(d.value?.title || 'VPN'), addLabel: 'В sing-box',
      note: 'готовый конфиг sing-box 1.12+' + (hasTuic.value ? ', с TUIC' : '') + ' (без XHTTP, MASQUE, AWG) — sing-box (SFA/SFI/SFM/SFW), Karing, Throne, NekoBox' },
    { id: 'clash', name: 'Clash / Mihomo', url: clash, add: 'clash://install-config?url=' + enc(clash), addLabel: 'В FlClash',
      note: (hasTuic.value || awg.value.length ? 'включая ' + [hasTuic.value && 'TUIC', awg.value.length && 'AmneziaWG'].filter(Boolean).join(' и ') + ' — ' : '') + 'FlClash, Clash Verge, Mihomo' },
    { id: 'links', name: 'Ссылки (base64)', url: withFmt('base64'), note: 'Hiddify, v2rayNG, Streisand, Throne и другие' + (hasMasque.value ? ' — без MASQUE' : '') },
  ]
})
const pct = computed(() => d.value?.limit ? Math.min(100, Math.round((d.value.up + d.value.down) / d.value.limit * 100)) : 0)
function toggleTheme() { dark.value = !dark.value; document.documentElement.classList.toggle('dark', dark.value); try { localStorage.setItem('kvk-sub-theme', dark.value ? 'dark' : 'light') } catch {} }
async function copyText(t, msg = 'Скопировано') {
  try { await navigator.clipboard.writeText(t) } catch { const ta = document.createElement('textarea'); ta.value = t; document.body.appendChild(ta); ta.select(); document.execCommand('copy'); ta.remove() }
  toast.value = msg; setTimeout(() => { toast.value = '' }, 1800)
}
onMounted(async () => {
  try {
    const r = await fetch(location.pathname + '?format=info', { headers: { Accept: 'application/json' } })
    if (!r.ok) throw new Error(r.status === 404 ? 'Проверьте ссылку или обратитесь в поддержку' : 'Ошибка ' + r.status)
    d.value = await r.json()
    document.title = (d.value.title || 'Подписка') + ' · ' + d.value.name
  } catch (e) { err.value = e.message } finally { loading.value = false }
})
</script>
