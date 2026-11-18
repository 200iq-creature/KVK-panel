<template>
  <div class="pg-warp">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="w2-eb">{{ srvName }} · Cloudflare WARP</span><h1>WARP</h1><p>Выход в интернет через Cloudflare: открывает сайты, которые блокируют IP дата-центров (ChatGPT, Netflix, Google без капчи), и подменяет IP сервера на адрес Cloudflare.</p></div>
        <div class="i-hd-a">
          <button v-if="w?.registered" class="btn" type="button" :disabled="busy.refresh" @click="refresh"><span v-if="busy.refresh" class="spin"></span><Icon v-else n="refresh" />Проверить аккаунт</button>
          <button v-if="w?.registered" class="btn btn-p" type="button" :disabled="busy.save || !dirty" @click="save"><span v-if="busy.save" class="spin"></span>Сохранить</button>
        </div>
      </div>
      <ServerPicker />
      <Loading v-if="!w" :error="error" @retry="load" />
      <template v-else>
        <div class="i-mt">
          <Metric label="WARP" :value="!w.registered ? 'не подключён' : f.enabled ? 'включён' : 'выключен'" :note="w.registered ? MODE[f.mode] : 'зарегистрируйте бесплатный аккаунт'" :note-tone="w.registered && f.enabled ? 'ok' : ''" />
          <Metric label="Аккаунт" :value="w.type === 'unlimited' || w.type === 'limited' ? 'WARP+' : w.registered ? 'бесплатный' : '—'" :note="w.license ? 'ключ WARP+ применён' : 'без ключа WARP+'" />
          <Metric label="Адрес в сети WARP" :value="w.address4 || '—'" :note="w.address6 || ''" />
          <Metric label="Сейчас" :value="f.mode === 'fallback' ? (w.fallbackOn ? 'через WARP' : 'напрямую') : f.enabled ? (f.mode === 'all' ? 'весь трафик' : 'выбранные сайты') : 'напрямую'" :note="f.mode === 'fallback' ? 'резервный режим: проверка каждую минуту' : ''" :note-tone="w.fallbackOn ? 'wa' : ''" />
        </div>
        <section v-if="!w.registered" class="i-pn">
          <div class="i-ph"><h2>Подключить WARP</h2></div>
          <div class="i-pb">
            <ol class="steps"><li>Панель зарегистрирует бесплатный аккаунт Cloudflare WARP и создаст ключи WireGuard.</li><li>Xray получит исходящее подключение «warp» — включите его для всех или для выбранных сайтов.</li><li>Есть ключ WARP+? Его можно применить после регистрации.</li></ol>
            <div class="ft"><span class="grow"></span><button class="btn btn-p" type="button" :disabled="busy.reg" @click="register"><span v-if="busy.reg" class="spin"></span><Icon v-else n="bolt" />Зарегистрировать</button></div>
          </div>
        </section>
        <div v-else class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>Что пускать через WARP</h2><span class="i-stt" :class="f.enabled ? 'ok' : 'mu'">{{ f.enabled ? 'включён' : 'выключен' }}</span></div>
            <div class="i-pb">
              <TgRow v-model="f.enabled" bold label="Включить WARP" hint="применяется сразу, без переподключения клиентов" />
              <Opt v-model="f.mode" :options="[{ v: 'sites', l: 'Только выбранные сайты', s: 'остальное — напрямую с IP сервера' }, { v: 'all', l: 'Весь трафик', s: 'все клиенты выходят с IP Cloudflare' }, { v: 'fallback', l: 'Резерв', s: 'напрямую, а если прямой выход сервера перестал работать — автоматически через WARP' }]" />
              <div v-if="f.mode === 'sites'" class="fld"><label class="lbl">Сайты и списки</label><textarea v-model="sites" class="inp mono" rows="7" placeholder="openai.com&#10;geosite:netflix"></textarea>
                <p class="hint">Домен (с поддоменами), IP/подсеть или список geosite — по одному в строке.</p>
                <div class="row"><button v-for="p in PRESETS" :key="p[0]" class="btn btn-sm btn-g" type="button" @click="addPreset(p[1])"><Icon n="plus" />{{ p[0] }}</button></div>
              </div>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Подключение</h2></div>
            <div class="i-pb">
              <div class="fld"><label class="lbl">Порт Cloudflare</label><Seg v-model="f.port" :options="w.ports.map(p => [p, String(p)])" /><p class="hint">если WARP не подключается — попробуйте другой порт</p></div>
              <TgRow v-model="f.ipv6First" bold label="Предпочитать IPv6" hint="помогает, если IPv4-адрес Cloudflare попал под ограничения сайта" />
              <div class="fld"><label class="lbl">Ключ WARP+</label><div class="row" style="flex-wrap:nowrap"><input v-model="lic" class="inp mono" :placeholder="w.license || 'xxxxxxxx-xxxxxxxx-xxxxxxxx'"><button class="btn" type="button" :disabled="!lic.trim()" @click="applyLic">Применить</button></div><p class="hint">необязательно: бесплатного WARP достаточно для обхода гео-ограничений</p></div>
              <div v-if="acc" class="i-kv"><Kr k="Тип" :v="acc.type || '—'" /><Kr k="Квота" :v="acc.quota ? bytes(acc.quota) : '—'" /><Kr k="WARP+" :v="acc.warpPlus ? 'да' : 'нет'" /></div>
              <div v-if="accErr" class="i-hint bad"><Icon n="warn" /><span>{{ accErr }}</span></div>
              <div class="ft"><span class="grow"></span><button class="btn btn-danger" type="button" @click="remove">Удалить аккаунт WARP</button></div>
            </div>
          </section>
        </div>
      </template>
    </div>
  </div>
</template>
<script setup>
import '../styles/Warp.css'
import { ref, reactive, computed, onMounted, watch } from 'vue'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import Metric from '../components/Metric.vue'
import TgRow from '../components/TgRow.vue'
import Opt from '../components/Opt.vue'
import Seg from '../components/Seg.vue'
import Kr from '../components/Kr.vue'
import ServerPicker from '../components/ServerPicker.vue'
import { srv, sget as get, spost as post, sput as put, sdel as del, srvName } from '../lib/server.js'
import { ok, fail, confirm } from '../lib/ui.js'
import { bytes } from '../lib/fmt.js'

const MODE = { sites: 'только выбранные сайты', all: 'весь трафик', fallback: 'резерв при сбое прямого выхода' }
const PRESETS = [['ChatGPT / OpenAI', ['geosite:openai']], ['Netflix', ['geosite:netflix']], ['Google', ['geosite:google']], ['Spotify', ['geosite:spotify']], ['Disney+', ['geosite:disney']], ['Claude / Gemini', ['anthropic.com', 'claude.ai', 'gemini.google.com']]]
const w = ref(null)
const f = reactive({ enabled: false, mode: 'sites', port: 2408, ipv6First: false })
const sites = ref('')
const lic = ref('')
const acc = ref(null)
const accErr = ref('')
const saved = ref('')
const error = ref('')
const busy = reactive({ save: false, reg: false, refresh: false })
const body = () => ({ ...f, sites: sites.value.split('\n').map(x => x.trim()).filter(Boolean) })
const dirty = computed(() => JSON.stringify(body()) !== saved.value)
function addPreset(arr) { const cur = sites.value.split('\n').map(x => x.trim()).filter(Boolean); for (const a of arr) if (!cur.includes(a)) cur.push(a); sites.value = cur.join('\n') }
async function load(refreshAcc) {
  error.value = ''
  try {
    const r = await get('warp' + (refreshAcc ? '?refresh=1' : '')); w.value = r
    Object.assign(f, { enabled: r.enabled, mode: r.mode || 'sites', port: r.port || 2408, ipv6First: r.ipv6First }); sites.value = (r.sites || []).join('\n')
    saved.value = JSON.stringify(body())
    if (refreshAcc) { acc.value = r.account || null; accErr.value = r.accountError || '' }
  } catch (e) { error.value = e.message }
}
async function save() { busy.save = true; try { await put('warp', body()); ok('WARP применён'); load() } catch (e) { fail(e) } finally { busy.save = false } }
async function register() { busy.reg = true; try { const r = await post('warp/register'); ok('WARP зарегистрирован'); if (r.licenseError) fail(r.licenseError); load() } catch (e) { fail(e) } finally { busy.reg = false } }
async function refresh() { busy.refresh = true; await load(true); busy.refresh = false }
async function applyLic() { try { await post('warp/license', { license: lic.value.trim() }); ok('Ключ WARP+ применён'); lic.value = ''; load(true) } catch (e) { fail(e) } }
async function remove() { if (await confirm('Удалить аккаунт WARP?', 'WARP выключится, ключи будут удалены. Список сайтов сохранится.', { danger: true, okText: 'Удалить' })) { try { await del('warp'); ok('Удалено'); load() } catch (e) { fail(e) } } }
onMounted(() => load())
watch(() => srv.id, () => { w.value = null; acc.value = null; accErr.value = ''; load() })
</script>
