<template>
  <div class="pg-inbounds">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="vw-eb">xray {{ xr.version || '—' }} · {{ pn(list.length, 'инбаунд', 'инбаунда', 'инбаундов') }}</span><h1>Инбаунды</h1></div>
        <div class="i-hd-a"><button class="btn btn-p" type="button" @click="openForm(null)"><Icon n="plus" />Новый инбаунд</button></div>
      </div>
      <Loading v-if="!data" :error="error" @retry="reload" />
      <template v-else>
        <div class="i-mt">
          <Metric label="Инбаунды" :value="list.length" :pct="list.length ? enabled / list.length * 100 : 0" tone="ok" :note="`включено ${enabled} из ${list.length}`" />
          <Metric label="Клиенты в инбаундах" :value="meta.clients" :note="`${list.reduce((s, i) => s + i.clients, 0)} подключений — у клиента своё в каждом`" />
          <Metric label="Онлайн" :value="list.reduce((s, i) => s + (i.online || 0), 0)" note="подключений за последние 2 минуты" />
          <Metric label="Xray" :value="xr.version || '—'" :note="xr.running ? 'изменения применяются сразу, без разрыва остальных клиентов' : (xr.error || 'не запущен')" :note-tone="xr.running ? '' : 'bad'" />
        </div>
        <section class="i-pn">
          <div class="i-ph"><h2>Все инбаунды <span class="i-cnt">{{ list.length }}</span></h2><span>трафик — с момента создания</span></div>
          <div class="vw-ord"><Icon n="info" /><span><b>Порядок строк — это порядок конфигов в подписке клиентов и в «Автовыборе»:</b> <span class="vw-mu">меняйте его стрелками у строки.</span></span></div>
          <div class="vw-tl">
            <div class="vw-tl-in">
              <div class="vw-ib vw-th"><span>Протокол</span><span>Название</span><span>Порт</span><span>Транспорт</span><span>Клиенты</span><span>Статус</span><span></span></div>
              <div v-for="(i, idx) in list" :key="i.id" class="vw-row vw-hasmv" :class="{ open: openId === i.id }">
                <div class="vw-ib">
                  <span class="vw-pr">
                    <span class="vw-mv"><button class="btn btn-g btn-ic" type="button" :disabled="idx === 0" title="Выше" @click="move(i, -1)"><Icon n="up" /></button><button class="btn btn-g btn-ic" type="button" :disabled="idx === list.length - 1" title="Ниже" @click="move(i, 1)"><Icon n="down" /></button></span>
                    <span class="vw-ic"><Icon :n="i.security === 'reality' ? 'shield' : i.security === 'tls' ? 'lock' : i.protocol === 'hysteria2' ? 'bolt' : 'inb'" /></span>
                    <span class="vw-id"><b>{{ PROTO[i.protocol] }}</b><small>{{ secLabel(i) }}</small></span>
                  </span>
                  <span class="vw-c vw-strong">{{ i.remark }}</span>
                  <span class="vw-c i-mono">{{ i.port }}</span>
                  <span class="vw-c">{{ i.protocol === 'hysteria2' ? 'QUIC' : NET[i.network] }}</span>
                  <span class="vw-c"><b class="vw-strong">{{ i.clients }}</b><span v-if="i.online" class="vw-mu"> · {{ i.online }} онлайн</span></span>
                  <span class="vw-br"></span>
                  <span class="vw-st"><button class="i-tg" :class="{ on: i.enabled }" type="button" :title="i.enabled ? 'Выключить' : 'Включить'" @click="toggle(i)"><span></span></button><span class="i-stt" :class="i.enabled ? 'ok' : 'mu'">{{ i.enabled ? 'вкл' : 'выкл' }}</span></span>
                  <span class="vw-ra"><button class="btn btn-sm btn-g" type="button" @click="openForm(i)"><Icon n="edit" />Изменить</button><button class="btn btn-sm btn-g btn-ic" type="button" :title="openId === i.id ? 'Свернуть' : 'Подробнее'" @click="openId = openId === i.id ? '' : i.id"><Icon :n="openId === i.id ? 'up' : 'chevD'" /></button></span>
                </div>
                <div v-if="openId === i.id" class="vw-exp">
                  <div class="vw-xc">
                    <span class="vw-xt">Параметры</span>
                    <div class="i-kv vw-kvb">
                      <div class="i-kr vw-k2"><span>тег</span><code>{{ i.tag }}</code></div>
                      <template v-if="i.security === 'reality'">
                        <div class="i-kr vw-k2"><span>маскировка (dest)</span><code>{{ i.realityTarget }}</code></div>
                        <div class="i-kr vw-k2"><span>SNI</span><code>{{ (i.realitySni || []).join(', ') || i.realityTarget.split(':')[0] }}</code></div>
                        <div class="i-kr vw-k2"><span>ключ (pbk)</span><code>{{ i.publicKey }}</code></div>
                        <div class="i-kr vw-k2"><span>shortId</span><code>{{ (i.shortIds || []).join(', ') }}</code></div>
                      </template>
                      <div v-if="i.certName" class="i-kr vw-k2"><span>сертификат</span><code>{{ i.certName }}</code></div>
                      <div v-if="i.flow" class="i-kr vw-k2"><span>flow</span><code>{{ i.flow }}</code></div>
                      <div v-if="i.path" class="i-kr vw-k2"><span>путь</span><code>{{ i.path }}</code></div>
                      <div v-if="i.serviceName" class="i-kr vw-k2"><span>serviceName</span><code>{{ i.serviceName }}</code></div>
                      <div v-if="i.ssMethod" class="i-kr vw-k2"><span>метод</span><code>{{ i.ssMethod }}</code></div>
                      <div v-if="i.obfsPassword" class="i-kr vw-k2"><span>obfs</span><code>salamander</code></div>
                      <div class="i-kr vw-k2"><span>трафик</span><code>↑ {{ bytes(i.up) }} · ↓ {{ bytes(i.down) }}</code></div>
                    </div>
                  </div>
                  <div class="vw-xc">
                    <span class="vw-xt">Клиенты инбаунда · {{ i.clients }}</span>
                    <div class="vw-cl">
                      <div v-for="c in (i.clientList || []).slice(0, showAll[i.id] ? 9999 : 8)" :key="c.id" class="vw-xr"><span><b>{{ c.name }}</b> <span class="vw-mu">{{ c.online ? 'онлайн' : STATUS[c.status]?.[0] }}</span></span><button class="btn btn-sm btn-g" type="button" @click="removeClient(i, c)">Убрать</button></div>
                      <button v-if="(i.clientList || []).length > 8 && !showAll[i.id]" class="btn btn-sm btn-g" type="button" @click="showAll[i.id] = true">Показать всех ({{ i.clientList.length }})</button>
                    </div>
                    <div class="vw-add"><input v-model="addName[i.id]" class="inp" placeholder="имя клиента (пусто = новый)" @keydown.enter="addClient(i)"><button class="btn btn-sm" type="button" @click="addClient(i)"><Icon n="plus" />Добавить</button></div>
                  </div>
                  <div class="vw-xc vw-xa">
                    <span class="vw-xt">Действия</span>
                    <button class="btn btn-sm" type="button" @click="exampleLink(i)">Скопировать пример ссылки</button>
                    <button class="btn btn-sm" type="button" @click="grant(i, true)">Выдать всем</button>
                    <button class="btn btn-sm btn-g" type="button" @click="grant(i, false)">Забрать у всех</button>
                    <button class="btn btn-sm btn-g vw-bad" type="button" @click="remove(i)">Удалить инбаунд</button>
                  </div>
                </div>
              </div>
              <Empty v-if="!list.length" icon="inb" title="Инбаундов пока нет" text="Начните с VLESS Reality на порту 443 — он лучше всего проходит блокировки">
                <button class="btn btn-sm" type="button" @click="openForm(null)"><Icon n="plus" />Новый инбаунд</button>
              </Empty>
            </div>
          </div>
        </section>
        <div class="i-alrt info"><Icon n="info" /><span><b>Новый Xray не запускает VLESS без защиты к публичному серверу:</b> <span class="muted">для прямого подключения — Reality (маскировка под сайт) или TLS. После смены порта, защиты или obfs клиентам нужно обновить подписку.</span></span><router-link class="btn btn-sm" to="/sub">Настроить подписку</router-link></div>
      </template>
    </div>
    <Drawer :open="form.open" :title="form.inbound ? 'Изменить инбаунд' : 'Новый инбаунд'" :sub="form.inbound ? form.inbound.remark : ''" eyebrow="инбаунды" width="min(640px, calc(100% - 24px))" @close="form.open = false">
      <InboundForm v-if="form.open" ref="fref" :inbound="form.inbound" :edit="!!form.inbound" />
      <template #foot>
        <button class="btn" type="button" @click="form.open = false">Отмена</button>
        <button class="btn btn-p" type="button" :disabled="busy.save" @click="save"><span v-if="busy.save" class="spin"></span>{{ form.inbound ? 'Сохранить' : 'Создать' }}</button>
      </template>
    </Drawer>
  </div>
</template>
<script setup>
import '../styles/Inbounds.css'
import { ref, reactive, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import Empty from '../components/Empty.vue'
import Metric from '../components/Metric.vue'
import Drawer from '../components/Drawer.vue'
import InboundForm from '../components/InboundForm.vue'
import { get, post, put, del } from '../lib/api.js'
import { meta, loadMeta } from '../lib/meta.js'
import { useLoad } from '../lib/useLoad.js'
import { ok, fail, confirm, copy } from '../lib/ui.js'
import { pn, bytes, PROTO, NET } from '../lib/fmt.js'
import { STATUS } from '../lib/client.js'

const route = useRoute()
const router = useRouter()
const { data, error, reload } = useLoad(() => get('inbounds'), { interval: 15000 })
const list = computed(() => data.value?.inbounds || [])
const xr = computed(() => data.value?.xray || {})
const enabled = computed(() => list.value.filter(i => i.enabled).length)
const openId = ref('')
const addName = reactive({})
const showAll = reactive({})
const form = reactive({ open: false, inbound: null })
const fref = ref(null)
const busy = reactive({ save: false })
const secLabel = (i) => i.protocol === 'hysteria2' ? `TLS${i.obfsPassword ? ' · salamander' : ''}` : i.security === 'reality' ? 'Reality · маскировка под сайт' : i.security === 'tls' ? 'TLS' : 'без защиты'

function openForm(i) { form.inbound = i ? JSON.parse(JSON.stringify(i)) : null; form.open = true }
async function save() {
  busy.save = true
  try {
    const b = fref.value.body()
    if (form.inbound) { await put('inbounds/' + form.inbound.id, b); ok('Инбаунд сохранён') } else { await post('inbounds' + (fref.value.grant() ? '?grant=1' : ''), b); ok('Инбаунд создан') }
    form.open = false; await reload(true); loadMeta()
  } catch (e) { fail(e) } finally { busy.save = false }
}
async function run(fn, msg) { try { await fn(); if (msg) ok(msg); await reload(true) } catch (e) { fail(e) } }
const toggle = (i) => run(() => post(`inbounds/${i.id}/toggle`), i.enabled ? 'Выключен' : 'Включён')
const move = (i, dir) => run(() => post(`inbounds/${i.id}/move`, { dir }))
const grant = async (i, all) => { if (all || await confirm(`Забрать «${i.remark}» у всех клиентов?`, 'Подключение пропадёт из подписок после обновления.', { danger: true, okText: 'Забрать' })) run(() => post(`inbounds/${i.id}/grant`, { all }), all ? 'Выдано всем клиентам' : 'Забрано у всех') }
const removeClient = (i, c) => run(() => del(`inbounds/${i.id}/clients/${c.id}`), `${c.name} убран из инбаунда`)
async function addClient(i) { const name = (addName[i.id] || '').trim(); await run(() => post(`inbounds/${i.id}/clients`, { name }), 'Клиент добавлен'); addName[i.id] = ''; loadMeta() }
async function exampleLink(i) { try { const r = await get(`inbounds/${i.id}/link`); copy(r.link, 'Пример ссылки скопирован') } catch (e) { fail(e) } }
async function remove(i) {
  if (!await confirm(`Удалить «${i.remark}»?`, `Порт ${i.port} освободится, у ${pn(i.clients, 'клиента', 'клиентов', 'клиентов')} пропадёт это подключение.`, { danger: true, okText: 'Удалить' })) return
  run(() => del('inbounds/' + i.id), 'Инбаунд удалён').then(loadMeta)
}
onMounted(() => { if (route.query.new) { openForm(null); router.replace({ query: {} }) } })
</script>
