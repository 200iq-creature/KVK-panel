<template>
  <div class="pg-migrate">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="vw-eb">перенос клиентов</span><h1>Миграция</h1><p>Перенесите клиентов из 3x-ui или Remnawave: сохраняются UUID, сроки, лимиты трафика и ссылки подписки по subId — клиентам достаточно сменить адрес сервера или просто обновить подписку, если домен тот же.</p></div>
      </div>
      <div class="i-c2">
        <section class="i-pn">
          <div class="i-ph"><h2>Откуда переносим</h2><span v-if="st.running" class="i-stt wa">идёт перенос</span></div>
          <div class="i-pb">
            <Opt v-model="m.source" :options="[{ v: '3xui', l: '3x-ui', s: 'логин и пароль панели' }, { v: 'remnawave', l: 'Remnawave', s: 'API-токен из настроек' }]" />
            <div class="fld"><label class="lbl">Адрес панели</label><input v-model="m.url" class="inp mono" :placeholder="m.source === '3xui' ? 'https://1.2.3.4:2053/secretpath/' : 'https://panel.example.com'"><p class="hint">{{ m.source === '3xui' ? 'тот, что открываете в браузере, включая секретный путь' : 'адрес панели Remnawave' }}</p></div>
            <div v-if="m.source === '3xui'" class="two">
              <div class="fld"><label class="lbl">Логин</label><input v-model="m.username" class="inp" autocomplete="off"></div>
              <div class="fld"><label class="lbl">Пароль</label><input v-model="m.password" class="inp" type="password" autocomplete="new-password"></div>
            </div>
            <div v-else class="fld"><label class="lbl">API-токен</label><input v-model="m.token" class="inp mono" placeholder="Remnawave → Settings → API Tokens"></div>
            <TgRow v-model="m.insecure" label="Не проверять сертификат" hint="для панелей с самоподписанным HTTPS или по IP" />
            <div class="fld"><label class="lbl">Выдать перенесённым инбаунды · {{ m.inbounds.length || 'все включённые' }}</label>
              <div class="list-rows">
                <label v-for="i in inbounds" :key="i.id" class="lr" style="cursor:pointer"><button class="chk" :class="{ on: m.inbounds.includes(i.id) }" type="button" @click.prevent="toggle(i.id)"><Icon n="check" /></button><span class="lr-t"><b>{{ i.remark }}</b><small>{{ PROTO[i.protocol] }} · порт {{ i.port }}</small></span></label>
              </div>
              <p v-if="!inbounds.length" class="hint">Инбаундов ещё нет — <router-link to="/inbounds?new=1">создайте</router-link>, чтобы клиенты сразу получили подключения.</p>
            </div>
            <div v-if="test" class="i-hint"><Icon n="info" /><span>Найдено <b>{{ test.total }}</b>: будет добавлено <b>{{ test.willAdd }}</b>, уже есть {{ test.duplicates }}, истёкших (пропустим) {{ test.expired }}, отключённых (перенесём забаненными) {{ test.disabled }}, бессрочных {{ test.noExpiry }}.</span></div>
            <div class="ft">
              <button class="btn" type="button" :disabled="busy.test || !m.url" @click="doTest"><span v-if="busy.test" class="spin"></span>Проверить</button>
              <span class="grow"></span>
              <button class="btn btn-p" type="button" :disabled="st.running || !m.url" @click="run"><Icon n="mig" />Перенести</button>
            </div>
          </div>
        </section>
        <section class="i-pn">
          <div class="i-ph"><h2>Ход переноса</h2><span class="i-stt" :class="st.stage === 'done' ? 'ok' : st.stage === 'error' ? 'bad' : st.running ? 'wa' : 'mu'">{{ STAGE[st.stage] || 'не запускался' }}</span></div>
          <div class="i-pb">
            <div class="i-kv"><Kr k="Найдено" :v="st.total" /><Kr k="Добавлено" :v="st.added" /><Kr k="Пропущено" :v="st.skipped" /><Kr k="Перенесено забаненными" :v="st.banned" /></div>
            <div v-if="st.total" class="i-mpr ok"><i :style="{ width: Math.min(100, Math.round((st.added + st.skipped) / st.total * 100)) + '%' }"></i></div>
            <div v-if="st.error" class="err-box">{{ st.error }}</div>
            <pre v-if="st.log?.length" class="log-box">{{ st.log.join('\n') }}</pre>
            <div v-if="st.stage === 'done'" class="ft"><span class="grow"></span><router-link class="btn btn-sm" to="/clients">К клиентам</router-link></div>
          </div>
        </section>
      </div>
      <section class="i-pn">
        <div class="i-ph"><h2>Экспорт из этой панели</h2><span>если переезжаете отсюда</span></div>
        <div class="i-pb">
          <p class="hint">Клиенты в формате, который понимают другие панели. Полный перенос на другую KVK-панель — через <router-link to="/backups">бэкап</router-link>.</p>
          <div class="row">
            <button class="btn" type="button" @click="download('migrate/export?format=3xui', 'kvk-3xui-inbounds.json')"><Icon n="download" />Для 3x-ui (инбаунды с клиентами)</button>
            <button class="btn" type="button" @click="download('migrate/export?format=remnawave', 'kvk-remnawave-users.json')"><Icon n="download" />Для Remnawave (пользователи)</button>
            <button class="btn btn-g" type="button" @click="download('clients/export?format=csv', 'clients.csv')"><Icon n="download" />CSV</button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
<script setup>
import '../styles/Migrate.css'
import { ref, reactive, onMounted, onBeforeUnmount } from 'vue'
import Icon from '../components/Icon.vue'
import Opt from '../components/Opt.vue'
import TgRow from '../components/TgRow.vue'
import Kr from '../components/Kr.vue'
import { get, post, download } from '../lib/api.js'
import { ok, fail, confirm } from '../lib/ui.js'
import { PROTO } from '../lib/fmt.js'
import { loadMeta } from '../lib/meta.js'

const STAGE = { connect: 'подключение', import: 'импорт', done: 'готово', error: 'ошибка' }
const m = reactive({ source: '3xui', url: '', username: '', password: '', token: '', insecure: true, inbounds: [] })
const st = ref({})
const test = ref(null)
const inbounds = ref([])
const busy = reactive({ test: false })
const toggle = (id) => { const i = m.inbounds.indexOf(id); i >= 0 ? m.inbounds.splice(i, 1) : m.inbounds.push(id) }
async function status() { try { const prev = st.value.running; st.value = await get('migrate/status'); if (prev && !st.value.running) { loadMeta(); if (st.value.stage === 'done') ok(`Перенесено клиентов: ${st.value.added}`) } } catch {} }
async function doTest() { busy.test = true; test.value = null; try { test.value = await post('migrate/test', { ...m }) } catch (e) { fail(e) } finally { busy.test = false } }
async function run() {
  if (!await confirm('Начать перенос?', 'Клиенты будут добавлены в эту панель. С исходной панели ничего не удаляется.', { okText: 'Перенести' })) return
  try { await post('migrate/run', { ...m }); status() } catch (e) { fail(e) }
}
let t
onMounted(async () => { status(); t = setInterval(() => { if (st.value.running || !st.value.stage) status() }, 1500); try { inbounds.value = (await get('inbounds')).inbounds.filter(i => i.enabled) } catch {} })
onBeforeUnmount(() => clearInterval(t))
</script>
