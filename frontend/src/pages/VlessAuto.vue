<template>
  <div class="pg-auto">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div>
          <span class="vw-eb">автонастройка · Reality + фейк-сайт + CDN</span>
          <h1>VLESS за минуту</h1>
          <p>Укажите свои домены — панель сама поднимет сайт-прикрытие, создаст VLESS Reality и запасной вход через Cloudflare CDN, выпустит сертификаты, откроет порты и выдаст готовые ссылки. Вручную ничего настраивать не нужно.</p>
        </div>
        <div v-if="d?.settings?.enabled && !editing" class="i-hd-a row">
          <button class="btn" type="button" :disabled="busy.check" @click="selfCheck"><span v-if="busy.check" class="spin"></span><Icon v-else n="shield" />Проверить работу</button>
          <button class="btn btn-p" type="button" @click="startEdit"><Icon n="edit" />Перенастроить</button>
        </div>
      </div>

      <Loading v-if="!d" />
      <template v-else>
        <!-- live progress -->
        <section v-if="st.steps?.length && (st.running || showRun)" class="i-pn">
          <div class="i-ph"><h2>Ход настройки</h2><span class="i-stt" :class="st.running ? 'wa' : st.error ? 'bad' : 'ok'">{{ st.running ? 'идёт настройка' : st.error ? 'не завершена' : 'готово' }}</span></div>
          <div class="i-pb">
            <ol class="au-steps">
              <li v-for="(s, i) in st.steps" :key="s.key" :class="s.status">
                <i><span v-if="s.status === 'run'" class="spin"></span><Icon v-else-if="s.status === 'ok'" n="check" /><Icon v-else-if="s.status === 'warn'" n="warn" /><Icon v-else-if="s.status === 'bad'" n="x" /><template v-else>{{ i + 1 }}</template></i>
                <span><b>{{ s.title }}</b><small v-if="s.detail">{{ s.detail }}</small></span>
              </li>
            </ol>
            <div v-if="st.error" class="err-box">{{ st.error }}</div>
            <div v-for="w in st.warnings || []" :key="w" class="i-hint wa"><Icon n="warn" /><span>{{ w }}</span></div>
            <div v-if="!st.running" class="ft"><span class="grow"></span><button class="btn btn-sm btn-g" type="button" @click="showRun = false">Скрыть</button></div>
          </div>
        </section>

        <!-- current setup -->
        <template v-if="d.settings.enabled && !editing">
          <div class="i-c2">
            <section class="i-pn">
              <div class="i-ph"><h2>Подключения</h2><span v-if="d.sampleClient" class="muted">ссылки клиента «{{ d.sampleClient }}»</span></div>
              <div class="i-pb">
                <div v-if="!d.links.length" class="i-hint wa"><Icon n="info" /><span>У автоинбаундов пока нет клиентов. <router-link to="/clients">Создайте клиента</router-link> и выдайте ему «Reality» и «CDN».</span></div>
                <div v-for="l in d.links" :key="l.link" class="au-link">
                  <span class="au-ic" :class="l.security === 'reality' ? '' : 'cdn'"><Icon :n="l.security === 'reality' ? 'bolt' : 'globe'" /></span>
                  <span class="au-lt"><b>{{ l.security === 'reality' ? 'Reality · напрямую' : 'Через Cloudflare CDN' }}</b><small class="mono">{{ l.link.slice(0, 64) }}…</small></span>
                  <button class="btn btn-sm btn-g btn-ic" type="button" title="QR-код" @click="qr = l"><Icon n="qr" /></button>
                  <button class="btn btn-sm" type="button" @click="copy(l.link)"><Icon n="copy" />Копировать</button>
                </div>
                <div v-if="d.subUrl" class="au-sub">
                  <span class="lbl">Подписка (все подключения сразу)</span>
                  <div class="row"><code class="grow">{{ d.subUrl }}</code><button class="btn btn-sm btn-g btn-ic" type="button" title="QR-код" @click="qr = { name: 'Подписка', link: d.subUrl }"><Icon n="qr" /></button><button class="btn btn-sm" type="button" @click="copy(d.subUrl)"><Icon n="copy" />Копировать</button></div>
                </div>
                <p class="hint">Reality — основной быстрый вход. CDN-вход идёт через Cloudflare и работает, даже если IP сервера заблокируют. Все клиенты в разделе <router-link to="/clients">«Клиенты»</router-link> получают оба подключения в подписке.</p>
              </div>
            </section>

            <section class="i-pn">
              <div class="i-ph"><h2>Состояние</h2><span class="i-stt" :class="d.site.running ? 'ok' : 'bad'">{{ d.site.running ? 'сайт работает' : 'сайт остановлен' }}</span></div>
              <div class="i-pb">
                <div class="i-kv">
                  <Kr k="Сервер" :v="d.serverIp" code />
                  <Kr k="SNI Reality" :v="d.settings.sni" code />
                  <Kr k="Порт" :v="d.settings.port" />
                  <Kr k="CDN-домены" :v="(d.settings.cdnDomains || []).join(', ') || 'выключено'" />
                  <Kr k="HTTP → HTTPS" :v="d.site.port80 ? 'порт 80 обслуживает панель' : 'порт 80 недоступен'" />
                  <Kr k="Настроено" :v="dt(d.settings.setupAt)" />
                </div>
                <div v-if="d.site.error" class="err-box">{{ d.site.error }}</div>
                <div class="au-certs">
                  <div v-for="c in d.certs" :key="c.name" class="au-cert">
                    <code>{{ c.name }}</code>
                    <span class="i-stt sm" :class="c.issuing ? 'wa' : c.status === 'active' ? 'ok' : c.status === 'error' ? 'bad' : 'mu'">{{ c.issuing ? 'выпускается…' : c.status === 'active' ? 'SSL до ' + date(c.expiresAt) : c.status === 'error' ? 'нет сертификата' : 'ожидает' }}</span>
                    <button v-if="c.id && c.status !== 'active' && !c.issuing" class="btn btn-sm btn-g" type="button" @click="reissue(c)">Выпустить</button>
                    <small v-if="c.error && c.status !== 'active'" class="bad-t">{{ c.error }}</small>
                  </div>
                </div>
                <div v-if="checks" class="au-checks">
                  <div v-for="c in checks.checks" :key="c" class="au-chk" :class="/не |ошибка|недоступ|закрыт/i.test(c) ? 'bad' : 'ok'"><Icon :n="/не |ошибка|недоступ|закрыт/i.test(c) ? 'warn' : 'check'" />{{ c }}</div>
                </div>
                <div class="ft">
                  <button class="btn btn-sm btn-danger-g" type="button" @click="remove"><Icon n="trash" />Удалить автонастройку</button>
                  <span class="grow"></span>
                  <router-link class="btn btn-sm btn-g" to="/inbounds">Инбаунды</router-link>
                  <router-link class="btn btn-sm btn-g" to="/domains">Домены</router-link>
                </div>
              </div>
            </section>
          </div>
        </template>

        <!-- wizard -->
        <template v-if="!d.settings.enabled || editing">
          <div class="au-grid">
            <section class="i-pn">
              <div class="i-ph"><h2><span class="vw-num">1</span> Домены</h2><span class="muted">от 1 до 10 · чем больше, тем надёжнее</span></div>
              <div class="i-pb">
                <p class="hint">Подойдут любые домены и поддомены, которыми вы управляете. На всех будет один и тот же сайт-прикрытие — так сервер выглядит как обычный сайт. Один из доменов станет «маской» (SNI) для Reality.</p>
                <div class="au-add">
                  <input v-model="newDom" class="inp mono" placeholder="example.com или site.example.com" @keydown.enter.prevent="addDomain" @paste="onPaste">
                  <button class="btn" type="button" :disabled="!newDom.trim()" @click="addDomain"><Icon n="plus" />Добавить</button>
                </div>
                <div v-if="f.domains.length" class="au-doms">
                  <div v-for="(dm, i) in f.domains" :key="dm.name" class="au-dom" :class="{ sni: f.sni === dm.name }">
                    <div class="au-dh">
                      <button class="au-rd" type="button" :class="{ on: f.sni === dm.name }" :title="'Сделать маской Reality (SNI)'" @click="f.sni = dm.name"></button>
                      <span class="au-dn"><code>{{ dm.name }}</code><small>{{ f.sni === dm.name ? 'маска Reality (SNI)' : 'нажмите кружок, чтобы сделать маской' }}</small></span>
                      <span v-if="dnsOf(dm.name)" class="au-dns" :class="DNS[dnsOf(dm.name).status]?.[1]" :title="(dnsOf(dm.name).ips || []).join(', ')">{{ DNS[dnsOf(dm.name).status]?.[0] }}</span>
                      <button class="btn btn-sm btn-g btn-ic" type="button" aria-label="Убрать" @click="f.domains.splice(i, 1); fixSni()"><Icon n="x" /></button>
                    </div>
                    <div class="au-dp">
                      <Tg v-model="dm.proxy" />
                      <span><b>Cloudflare-прокси</b> <em class="au-or" :class="{ on: dm.proxy }"></em><small>{{ dm.proxy ? 'оранжевое облако: сайт за Cloudflare, домен даёт CDN-вход' : 'серое облако / не Cloudflare: только Reality' }}</small></span>
                    </div>
                    <p v-if="dnsOf(dm.name)?.status === 'none' || dnsOf(dm.name)?.status === 'other'" class="hint wa-t">{{ dnsOf(dm.name).status === 'none' ? 'Нет A-записи.' : 'A-запись указывает не на этот сервер (' + (dnsOf(dm.name).ips || []).join(', ') + ').' }} {{ cfOn ? 'Панель исправит её через Cloudflare API.' : `Создайте A-запись на ${chk?.serverIp || d.serverIp} или включите управление DNS ниже.` }}</p>
                  </div>
                </div>
                <Empty v-else icon="globe" title="Добавьте хотя бы один домен" text="Можно вставить сразу несколько — через пробел, запятую или с новой строки." />
                <div class="i-hint"><Icon n="info" /><span><b>Совет:</b> включите <b>Cloudflare-прокси (оранжевое облако)</b> на доменах сайта. Посетители и сканеры увидят сайт за Cloudflare и не узнают IP сервера, а подписка получит запасной вход через CDN — он работает, даже если IP сервера заблокируют. Reality при этом подключается к серверу напрямую. С 2–3 доменами надёжнее: если один попадёт под блокировку, остальные продолжат работать.</span></div>
              </div>
            </section>

            <div class="au-side">
              <section class="i-pn">
                <div class="i-ph"><h2><span class="vw-num">2</span> Порт и опции</h2></div>
                <div class="i-pb">
                  <div class="fld"><label class="lbl">Порт Reality и сайта</label>
                    <div class="row"><input v-model.number="f.port" class="inp" type="number" min="1" max="65535" style="max-width:140px"><Seg v-model="f.port" :options="[{ v: 443, l: '443' }, { v: 8443, l: '8443' }, { v: 2053, l: '2053' }]" /></div>
                    <p class="hint" :class="chk?.port && !chk.port.free ? 'bad-t' : ''">{{ chk?.port ? (chk.port.free ? 'порт свободен' : chk.port.reason) : '443 — лучший выбор: так выглядит любой обычный сайт' }}{{ chk?.port?.free && !chk.port.cdnOk && f.cdn ? ' · Cloudflare не проксирует этот порт — CDN-вход не заработает, выберите 443, 2053, 2083, 2087, 2096 или 8443' : '' }}</p>
                  </div>
                  <div class="vw-tgl">
                    <TgRow v-model="f.cdn" bold label="Запасной вход через CDN" hint="VLESS XHTTP за сайтом — для доменов с оранжевым облаком" />
                    <TgRow v-model="f.grantAll" bold label="Выдать всем клиентам" :hint="`новые подключения появятся в подписке у ${d.clients} ${plural(d.clients, 'клиента', 'клиентов', 'клиентов')}`" />
                    <TgRow v-model="f.createClient" bold label="Создать первого клиента" hint="если клиентов ещё нет — чтобы сразу получить ссылку" />
                  </div>
                  <div class="fld"><label class="lbl">E-mail для Let's Encrypt <span class="muted">· необязательно</span></label><input v-model="f.email" class="inp" type="email" placeholder="для писем об истечении сертификатов"></div>
                </div>
              </section>

              <section class="i-pn">
                <div class="i-ph"><h2><span class="vw-num">3</span> Cloudflare <span class="muted small">· необязательно</span></h2><span v-if="chk?.cf" class="i-stt sm" :class="chk.cf.ok ? 'ok' : 'bad'">{{ chk.cf.ok ? 'токен работает' : 'токен не подошёл' }}</span></div>
                <div class="i-pb">
                  <TgRow v-model="f.cfManage" bold label="Настроить DNS в Cloudflare автоматически" hint="панель сама создаст A-записи, включит облако и режим SSL Full (strict)" />
                  <template v-if="f.cfManage">
                    <div class="fld"><label class="lbl">API-токен <span v-if="d.cfTokenSet" class="muted">· сохранён, можно не вводить</span></label>
                      <div class="row"><input v-model="f.cfToken" class="inp mono grow" type="password" autocomplete="off" :placeholder="d.cfTokenSet ? '••••••••  (сохранённый токен)' : 'токен с правами Zone · DNS · Edit'"><button v-if="d.cfTokenSet" class="btn btn-g btn-sm" type="button" @click="dropToken">Забыть</button></div>
                      <p class="hint">Cloudflare → My Profile → API Tokens → Create Token → шаблон «Edit zone DNS» и добавьте право <b>Zone · Zone Settings · Edit</b> (для режима SSL). Токен хранится только на этом сервере.</p>
                      <p v-if="chk?.cf && !chk.cf.ok" class="hint bad-t">{{ chk.cf.error }}</p>
                      <p v-if="chk?.cf?.ok && missingZones.length" class="hint wa-t">Токен не видит зону для: {{ missingZones.join(', ') }}</p>
                    </div>
                  </template>
                  <p v-else class="hint">Без токена создайте A-записи на <code>{{ chk?.serverIp || d.serverIp }}</code> сами и включите оранжевое облако для доменов с CDN; в Cloudflare → SSL/TLS выберите режим <b>Full</b>.</p>
                </div>
              </section>

              <section class="i-pn au-go">
                <div class="i-pb">
                  <div class="steps small">
                    <div class="step"><i>✓</i><span>сайт-прикрытие на {{ f.domains.length ? f.domains.map(x => x.name).join(', ') : 'ваших доменах' }}</span></div>
                    <div class="step"><i>✓</i><span>VLESS Reality (Vision) на порту {{ f.port }}, маска {{ f.sni || '—' }}</span></div>
                    <div v-if="f.cdn" class="step"><i>✓</i><span>VLESS XHTTP через CDN: {{ cdnList.length ? cdnList.join(', ') : 'включите облако хотя бы для одного домена' }}</span></div>
                    <div class="step"><i>✓</i><span>сертификаты Let's Encrypt и их продление, открытые порты</span></div>
                  </div>
                  <div class="ft">
                    <button v-if="editing" class="btn btn-g" type="button" @click="editing = false">Отмена</button>
                    <button class="btn" type="button" :disabled="busy.chk || !f.domains.length" @click="check"><span v-if="busy.chk" class="spin"></span><Icon v-else n="search" />Проверить</button>
                    <span class="grow"></span>
                    <button class="btn btn-p" type="button" :disabled="st.running || !f.domains.length || busy.run" @click="run"><span v-if="st.running || busy.run" class="spin"></span><Icon v-else n="bolt" />{{ st.running ? 'Настраиваю…' : 'Настроить всё' }}</button>
                  </div>
                </div>
              </section>
            </div>
          </div>
        </template>

        <!-- decoy site -->
        <section class="i-pn">
          <div class="i-ph"><h2>Сайт-прикрытие</h2><div class="row"><Seg v-model="siteTab" :options="[{ v: 'view', l: 'Просмотр' }, { v: 'edit', l: 'Свой HTML' }]" /></div></div>
          <div class="i-pb">
            <p class="hint">Это видят браузеры, поисковики и сканеры по адресу ваших доменов. По умолчанию — генеративная «студия»: рисунок, палитра и тексты зависят от домена, поэтому сайты на разных серверах не похожи друг на друга. Можно заменить своим HTML.</p>
            <div v-show="siteTab === 'view'" class="au-prev">
              <div class="au-bar"><i></i><i></i><i></i><code>https://{{ f.sni || d.settings.sni || 'ваш-домен' }}/</code><span class="grow"></span><button class="btn btn-sm btn-g btn-ic" type="button" title="Обновить" @click="frameKey++"><Icon n="refresh" /></button><a class="btn btn-sm btn-g btn-ic" :href="previewUrl" target="_blank" rel="noopener" title="Открыть отдельно"><Icon n="ext" /></a></div>
              <iframe :key="frameKey" :src="previewUrl" sandbox="allow-scripts" title="Сайт-прикрытие" loading="lazy"></iframe>
            </div>
            <div v-show="siteTab === 'edit'" class="au-edit">
              <textarea v-model="html" class="inp mono" spellcheck="false" placeholder="<!doctype html>&#10;<html>…ваша страница…</html>&#10;&#10;Оставьте пустым — будет стандартный сайт."></textarea>
              <div class="ft">
                <label class="btn btn-g"><Icon n="upload" />Загрузить .html<input type="file" accept=".html,.htm,text/html" hidden @change="loadFile"></label>
                <button class="btn btn-g" type="button" :disabled="!d.settings.customHtml && !html" @click="html = ''; saveHtml()">Вернуть стандартный</button>
                <span class="grow"></span>
                <span v-if="html !== (d.settings.customHtml || '')" class="muted small">есть несохранённые изменения</span>
                <button class="btn btn-p" type="button" :disabled="busy.html || html === (d.settings.customHtml || '')" @click="saveHtml"><span v-if="busy.html" class="spin"></span>Сохранить</button>
              </div>
              <p class="hint">Одна страница: стили и картинки лучше встроить в неё (inline / data:). Все остальные адреса сайта отвечают «404», кроме секретного пути CDN.</p>
            </div>
          </div>
        </section>
      </template>
    </div>

    <Drawer :open="!!qr" :title="qr?.name || ''" eyebrow="QR-код" width="min(420px, calc(100% - 24px))" @close="qr = null">
      <div v-if="qr" class="qr-wrap" style="display:flex;flex-direction:column;align-items:center;gap:12px"><Qr :text="qr.link" :size="300" /><code style="word-break:break-all;font-size:12px">{{ qr.link }}</code><button class="btn" type="button" @click="copy(qr.link)"><Icon n="copy" />Копировать</button></div>
    </Drawer>
  </div>
</template>
<script setup>
import { ref, reactive, computed, onMounted, onBeforeUnmount } from 'vue'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import Empty from '../components/Empty.vue'
import Tg from '../components/Tg.vue'
import TgRow from '../components/TgRow.vue'
import Seg from '../components/Seg.vue'
import Kr from '../components/Kr.vue'
import Qr from '../components/Qr.vue'
import Drawer from '../components/Drawer.vue'
import { get, post, put, del, BASE } from '../lib/api.js'
import { ok, fail, confirm, copy } from '../lib/ui.js'
import { dt, date, plural } from '../lib/fmt.js'
import { loadMeta } from '../lib/meta.js'

const DNS = { direct: ['→ этот сервер', 'ok'], cloudflare: ['☁ Cloudflare', 'cf'], other: ['другой IP', 'wa'], none: ['нет A-записи', 'wa'], invalid: ['ошибка', 'bad'] }
const d = ref(null)
const st = ref({})
const chk = ref(null)
const checks = ref(null)
const editing = ref(false)
const showRun = ref(false)
const qr = ref(null)
const newDom = ref('')
const siteTab = ref('view')
const html = ref('')
const frameKey = ref(0)
const busy = reactive({ chk: false, run: false, check: false, html: false })
const f = reactive({ domains: [], sni: '', port: 443, cdn: true, grantAll: true, createClient: true, email: '', cfManage: false, cfToken: '' })
const previewUrl = computed(() => BASE + 'api/autosetup/preview?k=' + frameKey.value)
const cfOn = computed(() => f.cfManage && (f.cfToken || d.value?.cfTokenSet))
const cdnList = computed(() => f.domains.filter(x => x.proxy).map(x => x.name))
const missingZones = computed(() => Object.entries(chk.value?.cf?.zones || {}).filter(([, z]) => !z).map(([n]) => n))
const dnsOf = (n) => (chk.value?.domains || []).find(x => x.name === n)

const norm = (s) => s.trim().toLowerCase().replace(/^https?:\/\//, '').replace(/[/:].*$/, '').replace(/\.$/, '')
function addDomain() {
  for (const raw of newDom.value.split(/[\s,;]+/)) {
    const n = norm(raw)
    if (!n) continue
    if (!/^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{1,62}$/.test(n)) { fail('Некорректный домен: ' + n); continue }
    if (f.domains.some(x => x.name === n)) continue
    if (f.domains.length >= 10) { fail('Не больше 10 доменов'); break }
    f.domains.push({ name: n, proxy: true })
  }
  newDom.value = ''
  fixSni()
}
function onPaste(e) { const t = e.clipboardData?.getData('text') || ''; if (/[\s,;]/.test(t.trim())) { e.preventDefault(); newDom.value = t; addDomain() } }
function fixSni() { if (!f.domains.some(x => x.name === f.sni)) f.sni = f.domains[0]?.name || '' }

async function load() {
  try {
    const r = await get('autosetup')
    d.value = r; st.value = r.state || {}
    if (!editing.value && !f.domains.length) fillForm()
    if (html.value === '' || html.value === (d.value.settings.customHtml || '')) html.value = r.settings.customHtml || ''
  } catch (e) { fail(e) }
}
function fillForm() {
  const s = d.value.settings
  if (s.domains?.length) {
    f.domains = s.domains.map(n => ({ name: n, proxy: (s.cdnDomains || []).includes(n) }))
    f.sni = s.sni || s.domains[0]; f.port = s.port || 443; f.cdn = s.cdn ?? true; f.email = s.email || ''
  }
  f.cfManage = !!d.value.cfTokenSet
}
function startEdit() { fillForm(); editing.value = true; chk.value = null }

async function check() {
  busy.chk = true
  try {
    chk.value = await post('autosetup/check', { domains: f.domains.map(x => x.name), port: Number(f.port) || 443, cfToken: f.cfManage ? f.cfToken : '' })
    for (const x of chk.value.domains) { const dm = f.domains.find(y => y.name === x.name); if (dm && x.status === 'cloudflare') dm.proxy = true }
  } catch (e) { fail(e) } finally { busy.chk = false }
}
async function run() {
  if (!f.domains.length) return
  if (f.cdn && !cdnList.value.length && !await confirm('CDN-вход не заработает', 'Ни для одного домена не включён Cloudflare-прокси. Продолжить только с Reality?', { okText: 'Продолжить' })) return
  if (d.value.settings.enabled && !await confirm('Перенастроить?', 'Автоинбаунды будут пересозданы с новыми параметрами — ключи Reality сохранятся, ссылки клиентов обновятся в подписке.', { okText: 'Перенастроить' })) return
  busy.run = true
  try {
    await post('autosetup/run', { domains: f.domains, sni: f.sni, port: Number(f.port) || 443, cdn: f.cdn, grantAll: f.grantAll, createClient: f.createClient, email: f.email, cfToken: f.cfToken, cfManage: f.cfManage })
    showRun.value = true
    poll()
  } catch (e) { fail(e) } finally { busy.run = false }
}
let pt
async function poll() {
  clearTimeout(pt)
  try {
    const prev = st.value.running
    st.value = await get('autosetup/state')
    if (st.value.running) { pt = setTimeout(poll, 1000); return }
    if (prev !== false) {
      showRun.value = true
      await load(); loadMeta(); frameKey.value++
      if (!st.value.error) { editing.value = false; ok('Готово — ссылки ниже') }
    }
  } catch { pt = setTimeout(poll, 2000) }
}
async function selfCheck() { busy.check = true; try { checks.value = await post('autosetup/selfcheck'); checks.value.ok ? ok('Всё работает') : fail('Есть проблемы — смотрите список') } catch (e) { fail(e) } finally { busy.check = false } }
async function reissue(c) { try { await post(`domains/${c.id}/issue`); ok('Выпуск запущен'); setTimeout(load, 1500) } catch (e) { fail(e) } }
async function remove() {
  if (!await confirm('Удалить автонастройку?', 'Инбаунды «Reality» и «CDN» будут удалены, сайт-прикрытие остановлен. Клиенты и сертификаты останутся.', { danger: true, okText: 'Удалить' })) return
  try { await del('autosetup'); ok('Автонастройка удалена'); f.domains = []; await load(); loadMeta() } catch (e) { fail(e) }
}
async function dropToken() { try { await del('autosetup/token'); f.cfToken = ''; await load(); ok('Токен удалён') } catch (e) { fail(e) } }
async function saveHtml() {
  busy.html = true
  try { await put('autosetup/site', { customHtml: html.value }); d.value.settings.customHtml = html.value; frameKey.value++; ok(html.value ? 'Страница сохранена' : 'Стандартный сайт включён'); siteTab.value = 'view' } catch (e) { fail(e) } finally { busy.html = false }
}
function loadFile(e) {
  const file = e.target.files?.[0]; e.target.value = ''
  if (!file) return
  if (file.size > 2 << 20) { fail('Файл больше 2 МБ'); return }
  const r = new FileReader(); r.onload = () => { html.value = String(r.result || '') }; r.readAsText(file)
}
onMounted(async () => { await load(); if (st.value.running) { showRun.value = true; poll() } })
onBeforeUnmount(() => clearTimeout(pt))
</script>
<style>
.pg-auto{display:contents}
.h1 .pg-auto .vw-eb{display:block;font-size:var(--fs-md);color:var(--muted);margin-bottom:8px}
.h1 .pg-auto .vw-num{display:inline-grid;place-items:center;width:26px;height:26px;border-radius:var(--r-sm);background:var(--accent);color:#fff;font-size:var(--fs-sm);font-weight:600;margin-right:6px;vertical-align:1px}
.h1 .pg-auto .i-ph h2{display:flex;align-items:center;gap:4px}
.h1 .au-grid{display:grid;grid-template-columns:minmax(0,1.25fr) minmax(0,1fr);gap:18px;align-items:start}
.h1 .au-side{display:flex;flex-direction:column;gap:18px;min-width:0}
.h1 .au-add{display:flex;gap:8px}.h1 .au-add .inp{flex:1}
.h1 .au-doms{display:flex;flex-direction:column;gap:8px}
.h1 .au-dom{display:flex;flex-direction:column;gap:10px;padding:12px 14px;border-radius:var(--r-lg);border:1px solid var(--line);background:var(--surface)}
.h1 .au-dom.sni{border-color:var(--accent);box-shadow:inset 0 0 0 1px var(--accent)}
.h1 .au-dh{display:flex;align-items:center;gap:12px;min-width:0}
.h1 .au-rd{width:20px;height:20px;border-radius:50%;border:2px solid var(--edge3);background:var(--surface);padding:0;cursor:pointer;flex:none}
.h1 .au-rd.on{border:6px solid var(--accent)}
.h1 .au-dn{display:flex;flex-direction:column;flex:1;min-width:0;line-height:1.3}
.h1 .au-dn code{font-family:var(--mono);font-weight:500;color:var(--strong);overflow:hidden;text-overflow:ellipsis}
.h1 .au-dn small{font-size:var(--fs-xs);color:var(--muted)}
.h1 .au-dns{font-size:var(--fs-xs);padding:3px 9px;border-radius:99px;background:var(--surface3);color:var(--muted);white-space:nowrap}
.h1 .au-dns.ok{background:var(--okSoft);color:var(--ok)}.h1 .au-dns.wa{background:var(--warnSoft);color:var(--warn)}.h1 .au-dns.bad{background:var(--badSoft);color:var(--bad)}.h1 .au-dns.cf{background:rgba(244,129,32,.14);color:#d56a0c}
.h1 .au-dp{display:flex;align-items:center;gap:12px;padding-left:32px;font-size:var(--fs-sm)}
.h1 .au-dp>span{display:flex;flex-wrap:wrap;align-items:center;gap:0 6px}.h1 .au-dp b{font-weight:600;color:var(--strong)}.h1 .au-dp small{flex-basis:100%;color:var(--muted);font-size:var(--fs-xs)}
.h1 .au-or{width:18px;height:11px;border-radius:99px;background:var(--surface3);display:inline-block;position:relative}
.h1 .au-or.on{background:#f48120}
.h1 .au-dom>.hint{padding-left:32px}
.h1 .au-steps{list-style:none;margin:0;padding:0;display:flex;flex-direction:column;gap:2px}
.h1 .au-steps li{display:flex;gap:12px;align-items:flex-start;padding:8px 0}
.h1 .au-steps li>i{width:28px;height:28px;border-radius:50%;display:grid;place-items:center;font-style:normal;font-size:var(--fs-sm);font-weight:600;flex:none;background:var(--surface3);color:var(--muted)}
.h1 .au-steps li>i .ic{width:15px;height:15px}
.h1 .au-steps li.run>i{background:var(--accentSoft);color:var(--accentText)}
.h1 .au-steps li.ok>i{background:var(--okSoft);color:var(--ok)}
.h1 .au-steps li.warn>i{background:var(--warnSoft);color:var(--warn)}
.h1 .au-steps li.bad>i{background:var(--badSoft);color:var(--bad)}
.h1 .au-steps li.skip{opacity:.55}
.h1 .au-steps li>span{display:flex;flex-direction:column;gap:2px;padding-top:4px;min-width:0}
.h1 .au-steps b{font-weight:600;color:var(--strong)}.h1 .au-steps small{color:var(--muted);font-size:var(--fs-sm);overflow-wrap:anywhere}
.h1 .au-link{display:flex;align-items:center;gap:12px;padding:12px 0;border-top:1px solid var(--line2);min-width:0}
.h1 .au-link:first-child{border-top:0;padding-top:0}
.h1 .au-ic{width:40px;height:40px;border-radius:var(--r-md);display:grid;place-items:center;background:var(--accentSoft);color:var(--accentText);flex:none}
.h1 .au-ic.cdn{background:rgba(244,129,32,.14);color:#d56a0c}
.h1 .au-lt{display:flex;flex-direction:column;flex:1;min-width:0}.h1 .au-lt b{font-weight:600;color:var(--strong)}.h1 .au-lt small{color:var(--muted);font-size:var(--fs-xs);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-family:var(--mono)}
.h1 .au-sub{display:flex;flex-direction:column;gap:6px;padding:12px 14px;border-radius:var(--r-md);background:var(--surface2);border:1px solid var(--line)}
.h1 .au-sub code{font-family:var(--mono);font-size:var(--fs-sm);color:var(--strong);overflow-wrap:anywhere;min-width:0}
.h1 .au-certs{display:flex;flex-direction:column}
.h1 .au-cert{display:flex;flex-wrap:wrap;align-items:center;gap:6px 12px;padding:9px 0;border-top:1px solid var(--line2)}
.h1 .au-cert code{font-family:var(--mono);color:var(--strong);flex:1;min-width:0;overflow-wrap:anywhere}
.h1 .au-cert small{flex-basis:100%;font-size:var(--fs-xs)}
.h1 .au-checks{display:flex;flex-direction:column;gap:6px}
.h1 .au-chk{display:flex;gap:8px;align-items:flex-start;font-size:var(--fs-sm);padding:8px 10px;border-radius:var(--r-sm)}
.h1 .au-chk.ok{background:var(--okSoft)}.h1 .au-chk.bad{background:var(--badSoft)}.h1 .au-chk .ic{width:16px;height:16px;flex:none;margin-top:1px}.h1 .au-chk.ok .ic{color:var(--ok)}.h1 .au-chk.bad .ic{color:var(--bad)}
.h1 .au-go .steps{gap:6px}.h1 .au-go .step>i{width:22px;height:22px;font-size:12px;background:var(--okSoft);color:var(--ok)}.h1 .au-go .step>span{padding-top:1px;font-size:var(--fs-sm)}
.h1 .au-prev{border:1px solid var(--line);border-radius:var(--r-lg);overflow:hidden;background:var(--surface2)}
.h1 .au-bar{display:flex;align-items:center;gap:6px;padding:6px 8px 6px 12px;border-bottom:1px solid var(--line)}
.h1 .au-bar>i{width:10px;height:10px;border-radius:50%;background:var(--surface3)}
.h1 .au-bar code{margin-left:8px;font-family:var(--mono);font-size:var(--fs-xs);color:var(--muted);padding:3px 10px;border-radius:99px;background:var(--surface);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.h1 .au-prev iframe{display:block;width:100%;height:min(560px,70vh);border:0;background:#fff}
.h1 .au-edit{display:flex;flex-direction:column;gap:10px}
.h1 .au-edit textarea{min-height:340px;font-size:12.5px;line-height:1.5;white-space:pre;tab-size:2}
@media (max-width:1000px){.h1 .au-grid{grid-template-columns:1fr}}
@media (max-width:600px){.h1 .au-dp{padding-left:0}.h1 .au-dom>.hint{padding-left:0}.h1 .au-link{flex-wrap:wrap}}
</style>
