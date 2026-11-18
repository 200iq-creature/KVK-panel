<template>
  <Drawer :open="open" cls="fdw" width="min(640px, calc(100% - 24px))" eyebrow="домены · бесплатно" title="Создать бесплатный домен" :sub="`Без покупки домена и без аккаунтов. Имя будет указывать на ${srvName} (${info?.ip || serverIp || '…'}), сертификат Let's Encrypt выпустится сам.`" @close="$emit('close')">
    <div v-if="!info" class="fdw-load"><span v-if="!infoErr" class="spin"></span><span v-else class="bad-t">{{ infoErr }}</span></div>
    <template v-else>
      <div class="fld">
        <label class="lbl">Сервис</label>
        <div class="fdw-cards">
          <button v-for="p in cards" :key="p.id" type="button" class="fdw-card" :class="{ on: f.provider === p.id }" @click="pick(p.id)">
            <span class="fdw-rd"></span>
            <span class="fdw-ct">
              <b>{{ p.name }}<i class="fdw-tag" :class="p.kind === 'token' ? 'own' : 'fast'">{{ p.kind === 'token' ? 'своё имя' : 'мгновенно' }}</i></b>
              <small>{{ DESC[p.id] }}</small>
              <code>{{ example(p) }}</code>
            </span>
          </button>
        </div>
      </div>

      <!-- sslip.io / nip.io -->
      <template v-if="prov?.kind === 'ip'">
        <div class="fld">
          <label class="lbl">Слово в начале имени <span class="fdw-mu">— необязательно</span></label>
          <input v-model="f.label" class="inp mono" maxlength="30" placeholder="например: cdn, site, vpn">
        </div>
        <div class="fld">
          <label class="lbl">Запись IP в имени</label>
          <Seg v-model="f.hex" :options="[[false, info.ipDashed || 'с дефисами'], [true, (info.ipHex || 'hex') + ' · короче']]" />
        </div>
        <div class="fdw-prev"><span>Получится</span><code>https://{{ preview }}</code></div>
        <div class="i-hint"><Icon n="info" /><span>Ничего регистрировать не нужно: {{ prov.name }} сам отвечает IP-адресом, который записан в имени. Если сервер сменит IP — создайте новое имя здесь же.</span></div>
      </template>

      <!-- freemyip.com -->
      <template v-else-if="prov?.id === 'freemyip'">
        <div class="fdw-steps">
          <div class="fdw-st">
            <i>1</i>
            <div>
              <b>Придумайте имя</b>
              <div class="fdw-in"><input v-model="f.want" class="inp mono" maxlength="40" placeholder="moy-sait" @keydown.enter="check"><span>.freemyip.com</span><button class="btn" type="button" :disabled="!wantLabel || busy.check" @click="check"><span v-if="busy.check" class="spin"></span>Проверить</button></div>
              <span v-if="avail && avail.name === wantLabel" class="i-stt" :class="avail.state === 'free' ? 'ok' : avail.state === 'taken' ? 'bad' : 'mu'">{{ avail.state === 'free' ? 'свободно' : avail.state === 'taken' ? 'занято — попробуйте другое' : 'не удалось проверить — проверка будет на их странице' }}</span>
            </div>
          </div>
          <div class="fdw-st">
            <i>2</i>
            <div>
              <b>Займите его — один клик</b>
              <small>Откроется страница freemyip.com с вашим именем. Нажмите там <q>Claim it!</q> — без регистрации, почты и пароля.</small>
              <a class="btn btn-p fdw-go" :class="{ dis: !wantLabel || avail?.state === 'taken' }" :href="claimFreemyip" target="_blank" rel="noopener noreferrer" @click="claimed = true"><Icon n="ext" />Занять {{ wantLabel ? wantLabel + '.freemyip.com' : 'имя' }}</a>
            </div>
          </div>
          <div class="fdw-st" :class="{ dim: !claimed && !f.token }">
            <i>3</i>
            <div>
              <b>Вставьте ссылку, которую показал сайт</b>
              <small>После нажатия freemyip.com покажет ссылку вида <code>https://freemyip.com/update?token=…&amp;domain=….freemyip.com</code> — скопируйте её целиком.</small>
              <input v-model="f.token" class="inp mono" placeholder="https://freemyip.com/update?token=…" @paste="pasted">
            </div>
          </div>
        </div>
      </template>

      <!-- myaddr.tools -->
      <template v-else-if="prov?.id === 'myaddr'">
        <div class="fdw-steps">
          <div class="fdw-st">
            <i>1</i>
            <div>
              <b>Займите имя на myaddr.tools</b>
              <small>Введите там имя (от 6 символов, начинается с буквы), дождитесь галочки проверки и нажмите <q>Claim</q>. Регистрации нет.</small>
              <a class="btn btn-p fdw-go" href="https://myaddr.tools/claim" target="_blank" rel="noopener noreferrer" @click="claimed = true"><Icon n="ext" />Открыть myaddr.tools</a>
            </div>
          </div>
          <div class="fdw-st" :class="{ dim: !claimed && !f.token }">
            <i>2</i>
            <div>
              <b>Вставьте секретный ключ</b>
              <small>Нажмите <q>Copy</q> рядом с <q>Your secret key</q> и вставьте сюда. Имя панель узнает по ключу сама. Ключ нужно вставить в течение часа — иначе myaddr.tools освободит имя.</small>
              <input v-model="f.token" class="inp mono" placeholder="43 символа" autocomplete="off" spellcheck="false">
            </div>
          </div>
        </div>
      </template>

      <template v-if="prov">
        <div v-if="prov.kind === 'token'" class="i-hint"><Icon n="refresh" /><span>Панель сама направит имя на этот сервер и будет продлевать его — {{ prov.id === 'myaddr' ? 'myaddr.tools удаляет имена, которые не обновлялись 90 дней' : 'freemyip.com требует обновления раз в год' }}.</span></div>
        <div class="fdw-tg">
          <TgRow v-model="f.useHost" label="Подставлять в ссылки клиентов" :hint="`сейчас в ссылках: ${publicHost || '—'}`" />
          <TgRow v-model="f.forPanel" label="Открывать панель по этому домену с HTTPS" hint="после выпуска сертификата панель перезапустится на https://" />
        </div>
      </template>
    </template>
    <template #foot>
      <span v-if="busy.create" class="fdw-wait">проверяем DNS — до 30 секунд…</span>
      <button class="btn" type="button" @click="$emit('close')">Отмена</button>
      <button class="btn btn-p" type="button" :disabled="!canCreate || busy.create" @click="create"><span v-if="busy.create" class="spin"></span>Создать домен</button>
    </template>
  </Drawer>
</template>
<script setup>
import { reactive, ref, computed, watch } from 'vue'
import Drawer from './Drawer.vue'
import Icon from './Icon.vue'
import Seg from './Seg.vue'
import Opt from './Opt.vue'
import TgRow from './TgRow.vue'
import { sget as get, spost as post, sput as put, srvName, srv } from '../lib/server.js'
import { ok, fail } from '../lib/ui.js'

const props = defineProps({ open: Boolean, serverIp: String, publicHost: String, hasPanelCert: Boolean })
const emit = defineEmits(['close', 'created'])
const DESC = {
  sslip: 'Имя с IP сервера внутри. Ничего не нужно регистрировать.',
  nip: 'То же самое, запасной сервис — если sslip.io недоступен.',
  freemyip: 'Любое свободное имя. Один клик «Claim it!» на их сайте.',
  myaddr: 'Любое имя от 6 символов. Один клик «Claim» на их сайте.',
}
const info = ref(null)
const infoErr = ref('')
const claimed = ref(false)
const avail = ref(null)
const busy = reactive({ check: false, create: false })
const f = reactive({ provider: 'sslip', label: '', hex: false, want: '', token: '', useHost: false, forPanel: false })

const isIpHost = (h) => !h || /^[\d.]+$|:/.test(h)
watch(() => props.open, (v) => {
  if (!v) return
  claimed.value = false; avail.value = null
  Object.assign(f, { provider: 'sslip', label: '', hex: false, want: '', token: '', useHost: isIpHost(props.publicHost), forPanel: !props.hasPanelCert })
  info.value = null; infoErr.value = ''
  get('domains/free').then(d => { info.value = d }).catch(e => { infoErr.value = srv.id && /404|not found/i.test(e.message || '') ? 'Обновите панель на этом сервере до 1.0.6 — там появились бесплатные домены' : (e.message || String(e)) })
})

const cards = computed(() => info.value?.providers || [])
const prov = computed(() => cards.value.find(p => p.id === f.provider))
function pick(id) { if (f.provider !== id) { f.provider = id; f.token = ''; claimed.value = false } }
const clean = (s, max) => (s || '').toLowerCase().trim().replace(/[^a-z0-9-]+/g, '-').replace(/-{2,}/g, '-').slice(0, max).replace(/^-+|-+$/g, '')
const preview = computed(() => {
  if (!info.value || !prov.value) return ''
  const part = f.hex ? info.value.ipHex : info.value.ipDashed
  const l = clean(f.label, 30)
  return (l ? l + '-' : '') + part + '.' + prov.value.suffix
})
function example(p) {
  const i = info.value
  if (p.kind === 'ip') return (i.ipDashed ? 'site-' + i.ipDashed : 'site-1-2-3-4') + '.' + p.suffix
  return 'moy-sait.' + p.suffix
}
const wantLabel = computed(() => clean(f.want.replace(/\.freemyip\.com$/i, ''), 40))
const claimFreemyip = computed(() => 'https://freemyip.com/main?subdomain=' + encodeURIComponent(wantLabel.value))
async function check() {
  if (!wantLabel.value) return
  busy.check = true
  try { avail.value = await get('domains/free/check?name=' + encodeURIComponent(wantLabel.value)) } catch (e) { fail(e) } finally { busy.check = false }
}
function pasted(e) {
  const t = e.clipboardData?.getData('text') || ''
  const m = t.match(/domain=([a-z0-9-]+)\.freemyip\.com/i)
  if (m && !f.want) f.want = m[1]
}
const canCreate = computed(() => {
  const p = prov.value
  if (!p || !info.value) return false
  if (p.kind === 'ip') return !!info.value.ipDashed
  if (p.id === 'freemyip') return /token=|^[0-9a-f]{16,}$/i.test(f.token.trim()) && (!!wantLabel.value || /domain=/.test(f.token))
  return f.token.trim().length >= 40
})
async function create() {
  busy.create = true
  try {
    const p = prov.value
    const body = { provider: p.id, label: f.label, hex: f.hex, name: p.id === 'freemyip' ? wantLabel.value : '', token: f.token.trim(), forPanel: f.forPanel }
    const r = await post('domains/free', body)
    if (f.useHost) { try { await put('host', { host: r.name }) } catch (e) { fail(e) } }
    ok(r.warning || `${r.name} создан — выпускаем сертификат, обычно до минуты` + (f.forPanel ? ', затем панель перейдёт на HTTPS' : ''))
    emit('created', r)
  } catch (e) { fail(e) } finally { busy.create = false }
}
</script>
