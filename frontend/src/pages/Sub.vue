<template>
  <div class="pg-sub">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="vw-eb">подписка клиентов · <span class="i-mono">{{ data?.host }}</span></span><h1>Подписка</h1></div>
        <div class="i-hd-a">
          <a v-if="data?.example" class="btn" :href="data.example" target="_blank" rel="noopener"><Icon n="ext" />Страница в браузере</a>
          <button v-if="data?.example" class="btn" type="button" @click="copy(data.example, 'Ссылка-пример скопирована')"><Icon n="copy" />Скопировать ссылку-пример</button>
        </div>
      </div>
      <Loading v-if="!s" :error="error" @retry="load" />
      <div v-else class="i-g2 vw-subg">
        <div class="vw-col">
          <section class="i-pn">
            <div class="i-ph"><h2>Название и описание</h2><span :class="dirty ? 'wa-t' : ''">{{ dirty ? 'не сохранено' : 'сохранено' }}</span></div>
            <div class="i-pb">
              <p class="vw-lead">Как называется подписка в приложении (Happ, v2rayTun, Hiddify, Streisand) и что написано под ней.</p>
              <div class="fld"><label class="lbl">Название подписки</label><input v-model="s.title" class="inp" placeholder="напр. My VPN" maxlength="60"><p class="hint">заголовок профиля в приложении (пусто — название ноды «{{ data.node }}»)</p></div>
              <div class="fld"><label class="lbl">Описание в приложении</label><textarea v-model="s.description" class="inp" maxlength="200" placeholder="Например: продление — в боте @my_vpn_bot"></textarea><div class="vw-cnt"><p class="hint">До 200 символов. Появится после обновления подписки в приложении.</p><span class="i-mono">{{ s.description.length }} / 200</span></div></div>
              <div class="fld"><label class="lbl">Имя конфигов</label><input v-model="s.nameFormat" class="inp mono" placeholder="{name} · {label}"><p class="hint"><code class="vw-tok">{name}</code> — имя клиента, <code class="vw-tok">{node}</code> — название сервера, <code class="vw-tok">{label}</code> — название инбаунда. Пример: «{{ preview[0]?.name }}»</p></div>
              <div class="vw-foot"><button class="btn" type="button" :disabled="busy.save" @click="save">Сохранить</button></div>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Автовыбор сервера в приложении</h2><span>{{ s.autoSelect ? 'включён' : 'выключен' }}</span></div>
            <div class="i-pb">
              <p class="vw-lead">Приложение проверяет соединение через серверы и выбирает доступный с меньшей задержкой. Ручной выбор остаётся.</p>
              <div class="vw-tgl"><TgRow v-model="s.autoSelect" bold label="Добавлять «Автовыбор» в подписку" hint="группа появится в приложении над списком конфигураций" /></div>
              <div class="fld"><label class="lbl">Название группы в приложении</label><input v-model="s.autoName" class="inp" placeholder="пусто — «Автовыбор»"></div>
              <div class="fld"><label class="lbl">Серверы для автовыбора — домен или IP из конфига, каждый с новой строки</label><textarea v-model="hostsText" class="inp mono" placeholder="nl1.example.com"></textarea><p class="hint">Пустой список — все VLESS-серверы клиента.</p></div>
              <div class="i-hint"><Icon n="info" /><span>Работает в JSON-подписке (Happ, v2rayTun, v2rayNG — балансировщик Xray) и в Clash Meta (группа url-test). В группу входят только VLESS-конфиги.</span></div>
              <div class="vw-foot"><button class="btn" type="button" :disabled="busy.save" @click="save">Сохранить</button></div>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Внешние ссылки</h2><span>{{ extCount }} / 50</span></div>
            <div class="i-pb">
              <p class="vw-lead">Серверы других хостеров и панелей. Ссылка добавляется в конец подписки <b>всем активным клиентам</b>, истёкшим — нет.</p>
              <div class="fld"><label class="lbl">Ссылки vless://, vmess://, trojan://, ss://, hysteria2:// — каждая с новой строки</label><textarea v-model="s.externalLinks" class="inp mono" rows="5" placeholder="vless://{uuid}@my.other-host.com:443?security=reality&amp;…#{name} · Другой хостер"></textarea>
                <p class="hint"><code class="vw-tok">{uuid}</code> — UUID клиента, <code class="vw-tok">{password}</code> — пароль клиента (Trojan/Hysteria2), <code class="vw-tok">{name}</code> — имя клиента. Строка с <code class="vw-tok">#</code> в начале — комментарий.</p></div>
              <div class="vw-foot"><button class="btn" type="button" :disabled="busy.save" @click="save">Сохранить</button></div>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Страница подписки в браузере</h2><span>{{ s.pageView === 'classic' ? 'классическая' : 'простая' }}</span></div>
            <div class="i-pb">
              <p class="vw-lead">Если открыть ссылку подписки в браузере, клиент увидит страницу со сроком, трафиком, QR-кодом и кнопками «Добавить в приложение».</p>
              <Opt v-model="s.pageView" :options="[{ v: 'simple', l: 'Простая', s: 'одна кнопка и QR' }, { v: 'classic', l: 'Классическая', s: 'плюс все конфиги списком' }]" />
              <div class="two">
                <div class="fld"><label class="lbl">Заголовок страницы</label><input v-model="s.pageTitle" class="inp" placeholder="пусто — название подписки"></div>
                <div class="fld"><label class="lbl">Ссылка на поддержку</label><input v-model="s.pageSupportUrl" class="inp" placeholder="https://t.me/support"></div>
              </div>
              <div class="fld"><label class="lbl">Логотип (URL картинки)</label><input v-model="s.pageLogoUrl" class="inp" placeholder="https://…/logo.png"></div>
              <div class="fld"><label class="lbl">Текст на странице</label><textarea v-model="s.pageText" class="inp" rows="3" placeholder="Инструкция или объявление для клиентов"></textarea></div>
              <div class="vw-foot"><a v-if="data.example" class="btn btn-g" :href="data.example" target="_blank" rel="noopener"><Icon n="ext" />Открыть пример</a><button class="btn" type="button" :disabled="busy.save" @click="save">Сохранить</button></div>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Для опытных</h2><span>маршрутизация, обновление, анти-DPI, свой JSON</span></div>
            <div class="i-pb vw-dets">
              <details class="det">
                <summary><span><b>Маршрутизация в Happ</b><small>{{ s.happRoutingRu ? 'РФ напрямую' : 'выключена' }}</small></span><Icon n="chevD" /></summary>
                <div class="det-b">
                  <TgRow v-model="s.happRoutingRu" bold label="Российские сайты и приложения напрямую" hint="профиль маршрутизации Happ и те же правила в JSON-подписке" />
                  <p class="hint">Сайты в зонах .ru, .su и .рф и российские IP открываются напрямую, всё остальное — через VPN. Включится после обновления подписки.</p>
                  <div class="vw-foot"><button class="btn" type="button" :disabled="busy.save" @click="save">Сохранить</button></div>
                </div>
              </details>
              <details class="det">
                <summary><span><b>Автообновление и напоминание о продлении</b><small>обновление каждые {{ s.updateHours || 12 }} ч{{ s.renewRemind ? ' · напоминание включено' : '' }}</small></span><Icon n="chevD" /></summary>
                <div class="det-b">
                  <div class="fld"><label class="lbl">Обновлять подписку в приложении каждые N часов</label><input v-model.number="s.updateHours" class="inp" type="number" min="0" max="168" placeholder="12"><p class="hint">От 1 до 168. 0 — по умолчанию, 12 часов.</p></div>
                  <TgRow v-model="s.renewRemind" bold label="Напоминать о продлении в Happ" hint="истёкшим и тем, у кого подписка закончится в ближайшие сутки" />
                  <div class="fld"><label class="lbl">Ссылка для продления</label><input v-model="s.renewUrl" class="inp" maxlength="300" placeholder="https://t.me/your_bot"><p class="hint">Пусто — ссылка на бот-магазин, если он настроен.</p></div>
                  <div class="vw-foot"><button class="btn" type="button" :disabled="busy.save" @click="save">Сохранить</button></div>
                </div>
              </details>
              <details class="det">
                <summary><span><b>Анти-DPI: фрагментация</b><small>{{ s.fragment ? 'включена' : 'выключена' }}</small></span><Icon n="chevD" /></summary>
                <div class="det-b">
                  <TgRow v-model="s.fragment" bold label="Дробить TLS-приветствие на стороне клиента" hint="Happ, v2rayTun, v2rayNG — они получают JSON-подписку" />
                  <div class="three">
                    <div class="fld"><label class="lbl">packets</label><input v-model="s.fragPackets" class="inp mono" placeholder="tlshello"></div>
                    <div class="fld"><label class="lbl">length, байт</label><input v-model="s.fragLength" class="inp mono" placeholder="100-200"></div>
                    <div class="fld"><label class="lbl">interval, мс</label><input v-model="s.fragInterval" class="inp mono" placeholder="10-20"></div>
                  </div>
                  <div class="vw-foot"><button class="btn" type="button" :disabled="busy.save" @click="save">Сохранить</button></div>
                </div>
              </details>
              <details class="det">
                <summary><span><b>Свой шаблон JSON для Happ / Xray</b><small>{{ s.jsonTemplate.trim() ? 'свой' : 'стандартный' }}</small></span><Icon n="chevD" /></summary>
                <div class="det-b">
                  <p class="hint">Накладывается на стандартные конфиги JSON-подписки. Ключи dns, log, inbounds, policy — заменяют стандартные; routing.rules — добавляются перед стандартными правилами (outboundTag «proxy» — выбранный сервер); outbounds — добавляются (теги proxy, direct, block заняты).</p>
                  <textarea v-model="s.jsonTemplate" class="inp mono" rows="10" :placeholder="TPL"></textarea>
                  <div class="vw-foot"><button class="btn btn-g" type="button" @click="s.jsonTemplate = TPL">Пример</button><button class="btn btn-g" type="button" :disabled="!s.jsonTemplate" @click="s.jsonTemplate = ''">Сбросить к стандартному</button><button class="btn" type="button" :disabled="busy.save" @click="save">Сохранить шаблон</button></div>
                </div>
              </details>
            </div>
          </section>
        </div>
        <div class="vw-col vw-stick">
          <section class="i-pn">
            <div class="i-ph"><h2>Как выглядит в приложении</h2><span class="i-pt"><Icon n="phone" />Happ</span></div>
            <div class="i-pb">
              <div class="vw-stage">
                <div class="phone">
                  <div class="ph-top"><span>Подписки</span><span>{{ pn(preview.length + (s.autoSelect ? 1 : 0), 'конфигурация', 'конфигурации', 'конфигураций') }} · обновлено сейчас</span></div>
                  <div class="ph-card"><span class="ph-t">{{ s.title || data.node }}</span><div class="ph-m"><span>12&nbsp;ГБ из 100&nbsp;ГБ</span><span>до {{ date(Date.now() + 30 * 864e5) }}</span></div><div class="ph-bar"><i style="width:12%"></i></div><span v-if="s.description" class="ph-d">{{ s.description }}</span></div>
                  <span class="ph-g">Конфигурации</span>
                  <div v-if="s.autoSelect" class="ph-row"><b>⚡ {{ s.autoName || 'Автовыбор' }}</b><span>auto</span></div>
                  <div v-for="(l, i) in preview.slice(0, 6)" :key="i" class="ph-row"><b>{{ l.name }}</b><span>{{ 30 + i * 9 }} мс</span></div>
                </div>
              </div>
              <p class="vw-sm vw-mu">Клиент «{{ data.exampleClient || 'Иван' }}», нода «{{ data.node }}». Изменения видны после обновления подписки в приложении.</p>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Ссылка подписки</h2><span>пример</span></div>
            <div class="i-pb">
              <div v-if="data.example" class="vw-link"><code>{{ data.example }}</code><button class="i-cp" type="button" @click="copy(data.example)"><Icon n="copy" /></button></div>
              <p v-else class="hint">Создайте клиента — здесь появится пример его ссылки.</p>
              <div class="i-kv">
                <Kr k="Base64" :v="fmtUrl('')" code copyable />
                <Kr k="JSON (Xray)" :v="fmtUrl('json')" code copyable />
                <Kr k="Clash Meta" :v="fmtUrl('clash')" code copyable />
              </div>
              <div class="i-acts"><router-link class="btn btn-sm" to="/domains"><Icon n="dom" />Свой домен для подписки</router-link><router-link class="btn btn-sm btn-g" to="/clients"><Icon n="clients" />Ссылки клиентов</router-link></div>
              <p class="vw-sm vw-mu">У каждого клиента своя ссылка — она в карточке клиента. Приложения сами выбирают формат по User-Agent; путь и порт подписки меняются в «Настройках».</p>
            </div>
          </section>
        </div>
      </div>
    </div>
  </div>
</template>
<script setup>
import '../styles/SubPage.css'
import { ref, reactive, computed, onMounted } from 'vue'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import TgRow from '../components/TgRow.vue'
import Opt from '../components/Opt.vue'
import Kr from '../components/Kr.vue'
import { get, put } from '../lib/api.js'
import { copy, ok, fail } from '../lib/ui.js'
import { pn, date } from '../lib/fmt.js'

const TPL = '{\n  "dns": {"servers": ["https://1.1.1.1/dns-query", "8.8.8.8"]},\n  "routing": {\n    "domainStrategy": "IPIfNonMatch",\n    "rules": [\n      {"type": "field", "domain": ["geosite:category-ads-all"], "outboundTag": "block"}\n    ]\n  }\n}'
const data = ref(null)
const s = ref(null)
const saved = ref('')
const error = ref('')
const busy = reactive({ save: false })
const inbounds = ref([])
const hostsText = computed({ get: () => (s.value.autoHosts || []).join('\n'), set: (v) => { s.value.autoHosts = v.split(/\s+/).map(x => x.trim()).filter(Boolean) } })
const dirty = computed(() => s.value && JSON.stringify(s.value) !== saved.value)
const extCount = computed(() => (s.value?.externalLinks || '').split('\n').filter(l => l.trim() && !l.trim().startsWith('#')).length)
const preview = computed(() => {
  const name = data.value?.exampleClient || 'Иван'
  const tpl = s.value?.nameFormat || '{name} · {label}'
  const labels = inbounds.value.filter(i => i.enabled).map(i => i.remark)
  return (labels.length ? labels : ['Reality', 'XHTTP']).map(label => ({ name: tpl.replace('{name}', name).replace('{node}', data.value?.node || '').replace('{label}', label) }))
})
function fmtUrl(f) { return data.value?.example ? data.value.example + (f ? '?format=' + f : '') : '—' }
async function load() {
  error.value = ''
  try {
    const r = await get('sub')
    data.value = r
    const sub = { ...r.sub, autoHosts: r.sub.autoHosts || [], externalLinks: r.sub.externalLinks || '', jsonTemplate: r.sub.jsonTemplate || '', description: r.sub.description || '' }
    s.value = sub; saved.value = JSON.stringify(sub)
  } catch (e) { error.value = e.message }
}
async function save() {
  busy.save = true
  try { await put('sub', s.value); saved.value = JSON.stringify(s.value); ok('Подписка сохранена') ; const r = await get('sub'); data.value = r } catch (e) { fail(e) } finally { busy.save = false }
}
onMounted(async () => { load(); try { inbounds.value = (await get('inbounds')).inbounds } catch {} })
</script>
