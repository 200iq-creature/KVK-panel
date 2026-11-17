<template>
  <Drawer :open="!!id" :title="c ? c.name : 'Клиент'" eyebrow="клиент" width="min(620px, calc(100% - 24px))" @close="$emit('close')">
    <Loading v-if="!c" :error="error" @retry="load" />
    <template v-else-if="mode === 'edit'">
      <ClientForm ref="form" :client="c" :inbounds="inbounds" :all-tags="allTags" edit />
    </template>
    <template v-else>
      <div class="row">
        <span class="i-stt" :class="STATUS[c.status]?.[1]">{{ STATUS[c.status]?.[0] || c.status }}</span>
        <span v-if="c.online" class="badge b-ok"><span class="dot"></span>онлайн</span>
        <span v-for="t in c.tags" :key="t" class="badge b-mute">{{ t }}</span>
      </div>
      <div class="i-kv">
        <Kr k="Осталось"><span>{{ c.expiresAt ? `${left(c.expiresAt)} · до ${dt(c.expiresAt, true)}` : 'бессрочно' }}</span></Kr>
        <Kr k="Трафик">
          <span class="col" style="gap:6px">
            <span>{{ bytes(c.up + c.down) }} {{ c.trafficLimit ? 'из ' + bytes(c.trafficLimit) : '· безлимит' }} <span class="muted small">(↑ {{ bytes(c.up) }} ↓ {{ bytes(c.down) }}; всего {{ bytes(c.totalUp + c.totalDown) }})</span></span>
            <span v-if="c.trafficLimit" class="i-mpr" :class="usage > 90 ? 'bad' : usage > 70 ? 'wa' : 'ok'"><i :style="{ width: usage + '%' }"></i></span>
          </span>
        </Kr>
        <Kr k="Устройства"><span>{{ c.devices }} / {{ c.deviceLimit || '∞' }}<span v-if="c.ips?.length" class="muted small"> · {{ c.ips.join(', ') }}</span></span></Kr>
        <Kr k="Сброс трафика"><span>{{ resetLabel }}</span></Kr>
        <Kr v-if="c.telegram" k="Telegram" :v="c.telegram" copyable />
        <Kr k="Был онлайн"><span>{{ ago(c.lastOnline) }}</span></Kr>
        <Kr k="Подписка"><span>{{ c.lastSubAt ? `${ago(c.lastSubAt)} · ${c.app}` : 'ещё не обновлялась' }}<span v-if="c.lastSubIp" class="muted small"> · {{ c.lastSubIp }}</span></span></Kr>
        <Kr k="Создан"><span>{{ dt(c.createdAt, true) }}</span></Kr>
        <Kr k="UUID" :v="c.uuid" code copyable />
        <Kr v-if="c.note" k="Заметка"><span style="white-space:pre-wrap">{{ c.note }}</span></Kr>
      </div>
      <section class="col" style="gap:8px">
        <b class="strong">Ссылка подписки</b>
        <div class="row" style="flex-wrap:nowrap"><div class="code-box grow">{{ c.subUrl }}</div><button class="i-cp" type="button" title="Скопировать" @click="copy(c.subUrl)"><Icon n="copy" /></button><button class="i-cp" type="button" title="QR-код" @click="qr = qr === c.subUrl ? '' : c.subUrl"><Icon n="qr" /></button><a class="i-cp" :href="c.subUrl" target="_blank" rel="noopener" title="Открыть страницу подписки"><Icon n="ext" /></a></div>
        <div v-if="qr === c.subUrl" class="qr-wrap"><Qr :text="c.subUrl" :size="240" /><span class="hint">Отсканируйте в Happ, v2rayTun, Hiddify, v2rayNG или Streisand</span></div>
      </section>
      <section class="col" style="gap:8px">
        <b class="strong">Конфиги · {{ links.length }}</b>
        <div class="list-rows">
          <div v-for="l in links" :key="l.link" class="lr" style="flex-wrap:wrap">
            <span class="lr-t"><b>{{ l.inbound || l.name }}</b><small>{{ l.node }} · {{ PROTO[l.protocol] || l.protocol }} · {{ NET[l.network] || l.network }} · {{ SEC[l.security] || l.security }}</small></span>
            <button class="i-cp" type="button" title="Скопировать" @click="copy(l.link)"><Icon n="copy" /></button>
            <button class="i-cp" type="button" title="QR-код" @click="qr = qr === l.link ? '' : l.link"><Icon n="qr" /></button>
            <div v-if="qr === l.link" class="qr-wrap" style="width:100%"><Qr :text="l.link" :size="240" /></div>
          </div>
          <p v-if="!links.length" class="hint">У клиента нет подключений — добавьте инбаунды в «Изменить».</p>
        </div>
      </section>
      <section class="col" style="gap:8px">
        <b class="strong">Действия</b>
        <div class="row">
          <button class="btn btn-sm" type="button" @click="extend"><Icon n="clock" />Продлить</button>
          <button class="btn btn-sm" type="button" @click="action('reset-traffic', 'Трафик сброшен')"><Icon n="refresh" />Сбросить трафик</button>
          <button v-if="c.frozen" class="btn btn-sm" type="button" @click="action('unfreeze', 'Разморожен')"><Icon n="snow" />Разморозить</button>
          <button v-else class="btn btn-sm" type="button" @click="action('freeze', 'Заморожен — срок остановлен')"><Icon n="snow" />Заморозить</button>
          <button v-if="c.banned" class="btn btn-sm" type="button" @click="action('unban', 'Разбанен')"><Icon n="check" />Разбанить</button>
          <button v-else class="btn btn-sm" type="button" @click="action('ban', 'Забанен')"><Icon n="ban" />Забанить</button>
          <button class="btn btn-sm" type="button" @click="action('grant-all', 'Выданы все инбаунды')"><Icon n="inb" />Выдать все инбаунды</button>
          <button class="btn btn-sm btn-g" type="button" @click="regen('regen-uuid', 'Новый UUID', 'Старые ссылки клиента перестанут работать. Подписка останется той же — клиенту достаточно обновить её.')">Новый UUID</button>
          <button class="btn btn-sm btn-g" type="button" @click="regen('regen-sub', 'Новая ссылка подписки', 'Старая ссылка подписки перестанет работать — клиенту нужно будет добавить новую.')">Новая ссылка подписки</button>
        </div>
      </section>
    </template>
    <template #foot>
      <template v-if="mode === 'edit'">
        <button class="btn" type="button" @click="mode = 'view'">Отмена</button>
        <button class="btn btn-p" type="button" :disabled="busy.save" @click="save"><span v-if="busy.save" class="spin"></span>Сохранить</button>
      </template>
      <template v-else-if="c">
        <button class="btn btn-g btn-danger-g" type="button" @click="remove"><Icon n="trash" />Удалить</button>
        <span class="grow"></span>
        <button class="btn btn-p" type="button" @click="mode = 'edit'"><Icon n="edit" />Изменить</button>
      </template>
    </template>
  </Drawer>
</template>
<script setup>
import { ref, reactive, computed, watch } from 'vue'
import Drawer from './Drawer.vue'
import Loading from './Loading.vue'
import Icon from './Icon.vue'
import Kr from './Kr.vue'
import Qr from './Qr.vue'
import ClientForm from './ClientForm.vue'
import { get, post, put, del } from '../lib/api.js'
import { copy, ok, fail, confirm, prompt } from '../lib/ui.js'
import { bytes, left, dt, ago, PROTO, NET, SEC } from '../lib/fmt.js'
import { STATUS, RESET } from '../lib/client.js'

const p = defineProps({ id: String, inbounds: Array, allTags: Array })
const emit = defineEmits(['close', 'changed'])
const c = ref(null)
const links = ref([])
const error = ref('')
const mode = ref('view')
const qr = ref('')
const form = ref(null)
const busy = reactive({ save: false })
const usage = computed(() => c.value && c.value.trafficLimit ? Math.min(100, Math.round((c.value.up + c.value.down) / c.value.trafficLimit * 100)) : 0)
const resetLabel = computed(() => {
  const r = RESET.find(x => x[0] === (c.value?.resetPeriod || ''))
  let s = r ? r[1] : c.value?.resetPeriod
  if (c.value?.resetPeriod === 'days') s = `каждые ${c.value.resetDays} дн.`
  if (c.value?.resetPeriod === 'mday') s = `каждый месяц ${c.value.resetDays}-го`
  return s
})

async function load() {
  if (!p.id) return
  error.value = ''
  try { const r = await get('clients/' + p.id); c.value = r.client; links.value = r.links || [] } catch (e) { error.value = e.message }
}
watch(() => p.id, (v) => { c.value = null; mode.value = 'view'; qr.value = ''; if (v) load() }, { immediate: true })

async function action(a, msg, extra = {}) {
  try { await post(`clients/${c.value.id}/action`, { action: a, ...extra }); ok(msg); await load(); emit('changed') } catch (e) { fail(e) }
}
async function extend() {
  const v = await prompt('Продлить подписку', { label: 'На сколько дней', value: '30', type: 'number', text: 'Если срок уже истёк — отсчёт от сегодня, иначе дни добавятся к текущему сроку.', okText: 'Продлить' })
  if (v === null) return
  const days = Number(v)
  if (!days) return fail('Укажите число дней')
  action('extend', `Продлён на ${days} дн.`, { days })
}
async function regen(a, title, text) {
  if (await confirm(title, text, { okText: 'Сгенерировать' })) action(a, 'Готово')
}
async function save() {
  busy.save = true
  try { await put('clients/' + c.value.id, form.value.body()); ok('Сохранено'); mode.value = 'view'; await load(); emit('changed') } catch (e) { fail(e) } finally { busy.save = false }
}
async function remove() {
  if (!await confirm(`Удалить ${c.value.name}?`, 'Клиент пропадёт со всех инбаундов и серверов связки, ссылки перестанут работать. Отменить нельзя.', { danger: true, okText: 'Удалить' })) return
  try { await del('clients/' + c.value.id); ok('Клиент удалён'); emit('changed'); emit('close') } catch (e) { fail(e) }
}
</script>
