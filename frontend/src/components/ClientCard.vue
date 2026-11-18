<template>
  <div class="cc-exp" @click.stop>
    <div class="cc-col">
      <span class="cc-t">Данные</span>
      <div class="cc-kv">
        <div class="cc-r"><span>Осталось</span><b>{{ c.expiresAt ? `${left(c.expiresAt)} · до ${dt(c.expiresAt, true)}` : 'бессрочно' }}</b></div>
        <div class="cc-r"><span>Трафик</span><b>{{ bytes(c.up + c.down) }} {{ c.trafficLimit ? 'из ' + bytes(c.trafficLimit) : '· безлимит' }}</b></div>
        <div class="cc-r"><span>Отдано / получено</span><b>{{ bytes(c.up) }} / {{ bytes(c.down) }}</b></div>
        <div class="cc-r"><span>За всё время</span><b>{{ bytes(c.totalUp + c.totalDown) }}</b></div>
        <div class="cc-r"><span>Устройства</span><b>{{ c.devices }} / {{ c.deviceLimit || '∞' }}</b></div>
        <div v-if="c.ips?.length" class="cc-r"><span>IP онлайн</span><code>{{ c.ips.join(', ') }}</code></div>
        <div class="cc-r"><span>Сброс трафика</span><b>{{ resetLabel }}</b></div>
        <div class="cc-r"><span>Был онлайн</span><b>{{ ago(c.lastOnline) }}</b></div>
        <div class="cc-r"><span>Подписка</span><b>{{ c.lastSubAt ? `${ago(c.lastSubAt)}${c.app ? ' · ' + c.app : ''}` : 'ещё не обновлялась' }}</b></div>
        <div v-if="c.telegram" class="cc-r"><span>Telegram</span><b>{{ c.telegram }}</b></div>
        <div class="cc-r"><span>Создан</span><b>{{ dt(c.createdAt, true) }}</b></div>
        <div class="cc-r"><span>UUID</span><code class="cc-cp" title="Скопировать" @click="copy(c.uuid, 'UUID скопирован')">{{ c.uuid }}</code></div>
        <div v-if="c.note" class="cc-r"><span>Заметка</span><b class="cc-note">{{ c.note }}</b></div>
      </div>
      <span class="cc-t cc-t2">Ссылка подписки</span>
      <div class="cc-sub">
        <code>{{ c.subUrl }}</code>
        <button class="btn btn-sm btn-g btn-ic" type="button" title="Скопировать" @click="copy(c.subUrl, 'Ссылка подписки скопирована')"><Icon n="copy" /></button>
        <button class="btn btn-sm btn-g btn-ic" type="button" title="QR-код" @click="qr = !qr"><Icon n="qr" /></button>
        <a class="btn btn-sm btn-g btn-ic" :href="c.subUrl" target="_blank" rel="noopener" title="Открыть страницу подписки"><Icon n="ext" /></a>
      </div>
      <div v-if="qr" class="cc-qr"><Qr :text="c.subUrl" :size="200" /></div>
    </div>

    <div class="cc-col">
      <div class="cc-th">
        <span class="cc-t">Инбаунды · {{ given }} из {{ inbounds.length }}</span>
        <span class="cc-ga">
          <button type="button" :disabled="busy || given === inbounds.length" @click="setAll(true)">выдать все</button>
          <button type="button" :disabled="busy || !given" @click="setAll(false)">забрать все</button>
        </span>
      </div>
      <label v-if="inbounds.length > 10" class="i-ls cc-q"><Icon n="search" /><input v-model="q" placeholder="Название, протокол, порт"></label>
      <div class="cc-ib">
        <template v-for="g in groups" :key="g.id">
          <div class="cc-g">
            <span>{{ g.name }} <small>{{ g.on }} / {{ g.items.length }}</small></span>
            <span class="cc-ga">
              <button type="button" :disabled="busy || g.on === g.items.length" @click="setMany(g.items, true)">все</button>
              <button type="button" :disabled="busy || !g.on" @click="setMany(g.items, false)">никакие</button>
            </span>
          </div>
          <div v-for="i in g.shown" :key="i.id" class="cc-i" :class="{ off: !has(i.id) }" @click="setMany([i], !has(i.id))">
            <span class="cc-in"><b>{{ i.remark }}</b><small>{{ PROTO[i.protocol] || i.protocol }}<template v-if="netOf(i)"> · {{ netOf(i) }}</template> · {{ SEC[i.security] || i.security }} · {{ i.port }}<template v-if="!i.enabled"> · выключен</template></small></span>
            <button class="i-tg" :class="{ on: has(i.id) }" type="button" :title="has(i.id) ? 'Забрать' : 'Выдать'" :disabled="busy" @click.stop="setMany([i], !has(i.id))"><span></span></button>
          </div>
          <p v-if="!g.shown.length" class="cc-none">ничего не нашли</p>
        </template>
        <p v-if="!inbounds.length" class="cc-none">Инбаундов пока нет — создайте их в разделе «Инбаунды».</p>
      </div>
    </div>

    <div class="cc-col cc-act">
      <span class="cc-t">Действия</span>
      <button class="btn btn-sm btn-p" type="button" @click="$emit('edit', c.id)"><Icon n="edit" />Изменить</button>
      <button class="btn btn-sm" type="button" @click="$emit('view', c.id)"><Icon n="qr" />Конфиги и QR</button>
      <button class="btn btn-sm" type="button" @click="extend"><Icon n="clock" />Продлить</button>
      <button class="btn btn-sm" type="button" @click="action('reset-traffic', 'Трафик сброшен')"><Icon n="refresh" />Сбросить трафик</button>
      <button v-if="c.frozen" class="btn btn-sm" type="button" @click="action('unfreeze', 'Разморожен')"><Icon n="snow" />Разморозить</button>
      <button v-else class="btn btn-sm" type="button" @click="action('freeze', 'Заморожен — срок остановлен')"><Icon n="snow" />Заморозить</button>
      <button v-if="c.banned" class="btn btn-sm" type="button" @click="action('unban', 'Разбанен')"><Icon n="check" />Разбанить</button>
      <button v-else class="btn btn-sm" type="button" @click="action('ban', 'Забанен')"><Icon n="ban" />Забанить</button>
      <button class="btn btn-sm btn-g" type="button" @click="regen('regen-uuid', 'Новый UUID', 'Старые ссылки клиента перестанут работать. Подписка останется той же — клиенту достаточно обновить её.')">Новый UUID</button>
      <button class="btn btn-sm btn-g" type="button" @click="regen('regen-sub', 'Новая ссылка подписки', 'Старая ссылка подписки перестанет работать — клиенту нужно будет добавить новую.')">Новая ссылка подписки</button>
      <button class="btn btn-sm btn-g cc-bad" type="button" @click="remove"><Icon n="trash" />Удалить клиента</button>
    </div>
  </div>
</template>
<script setup>
import { ref, computed, watch } from 'vue'
import Icon from './Icon.vue'
import Qr from './Qr.vue'
import { post, del } from '../lib/api.js'
import { copy, ok, fail, confirm, prompt } from '../lib/ui.js'
import { bytes, left, dt, ago, PROTO, NET, SEC, flag } from '../lib/fmt.js'
import { RESET } from '../lib/client.js'
import { meta } from '../lib/meta.js'

const p = defineProps({ c: { type: Object, required: true }, inbounds: { type: Array, default: () => [] }, countries: { type: Object, default: () => ({}) } })
const emit = defineEmits(['changed', 'edit', 'view', 'deleted'])
const qr = ref(false)
const q = ref('')
const busy = ref(false)
// local copy of the access list: switches react at once, the list reload confirms
const mine = ref(new Set(p.c.inbounds || []))
watch(() => p.c.inbounds, (v) => { if (!busy.value) mine.value = new Set(v || []) })
const has = (id) => mine.value.has(id)
const given = computed(() => p.inbounds.filter(i => mine.value.has(i.id)).length)
const netOf = (i) => ({ hysteria2: 'QUIC', tuic: 'QUIC', amneziawg: 'UDP', masque: 'HTTP/3' })[i.protocol] || NET[i.network] || i.network
const groups = computed(() => {
  const s = q.value.trim().toLowerCase()
  const by = new Map()
  for (const i of p.inbounds) {
    const id = i.nodeId || ''
    if (!by.has(id)) by.set(id, { id, name: id ? `${flag(p.countries[id] || '')} ${i.node}`.trim() : `${flag(meta.country || '')} ${meta.node || 'Этот сервер'} · этот`.trim(), items: [] })
    by.get(id).items.push(i)
  }
  return [...by.values()].map(g => ({
    ...g,
    on: g.items.filter(i => mine.value.has(i.id)).length,
    shown: s ? g.items.filter(i => [i.remark, PROTO[i.protocol], i.protocol, NET[i.network], String(i.port)].some(x => (x || '').toLowerCase().includes(s))) : g.items,
  }))
})
const resetLabel = computed(() => {
  const r = RESET.find(x => x[0] === (p.c.resetPeriod || ''))
  let s = r ? r[1] : p.c.resetPeriod
  if (p.c.resetPeriod === 'days') s = `каждые ${p.c.resetDays} дн.`
  if (p.c.resetPeriod === 'mday') s = `каждый месяц ${p.c.resetDays}-го`
  return s
})

async function setMany(items, on) {
  const ids = items.map(i => i.id).filter(id => mine.value.has(id) !== on)
  if (!ids.length || busy.value) return
  busy.value = true
  const prev = new Set(mine.value)
  const next = new Set(mine.value)
  ids.forEach(id => on ? next.add(id) : next.delete(id))
  mine.value = next
  try {
    await post(`clients/${p.c.id}/action`, { action: on ? 'grant' : 'revoke', inbounds: ids })
    ok(ids.length === 1 ? `${on ? 'Выдан' : 'Забран'}: ${items.find(i => i.id === ids[0])?.remark}` : `${on ? 'Выдано' : 'Забрано'} инбаундов: ${ids.length}`)
    emit('changed')
  } catch (e) { mine.value = prev; fail(e) } finally { busy.value = false }
}
const setAll = (on) => setMany(p.inbounds, on)
async function action(a, msg, extra = {}) {
  try { await post(`clients/${p.c.id}/action`, { action: a, ...extra }); ok(msg); emit('changed') } catch (e) { fail(e) }
}
async function extend() {
  const v = await prompt('Продлить подписку', { label: 'На сколько дней', value: '30', type: 'number', text: 'Если срок уже истёк — отсчёт от сегодня, иначе дни добавятся к текущему сроку.', okText: 'Продлить' })
  if (v === null) return
  const days = Number(v)
  if (!days) return fail('Укажите число дней')
  action('extend', `Продлён на ${days} дн.`, { days })
}
async function regen(a, title, text) { if (await confirm(title, text, { okText: 'Сгенерировать' })) action(a, 'Готово') }
async function remove() {
  if (!await confirm(`Удалить ${p.c.name}?`, 'Клиент пропадёт со всех инбаундов и серверов связки, ссылки перестанут работать. Отменить нельзя.', { danger: true, okText: 'Удалить' })) return
  try { await del('clients/' + p.c.id); ok('Клиент удалён'); emit('deleted') } catch (e) { fail(e) }
}
</script>
<style>
.h1 .cc-exp{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1.15fr) 200px;gap:20px;padding:16px 0 6px 44px;align-items:start;cursor:default}
.h1 .cc-col{display:flex;flex-direction:column;min-width:0}
.h1 .cc-t{font-size:var(--fs-sm);font-weight:600;color:var(--muted);margin-bottom:4px}
.h1 .cc-t2{margin-top:14px}
.h1 .cc-r{display:grid;grid-template-columns:minmax(96px,auto) minmax(0,1fr);gap:12px;align-items:baseline;padding:7px 0;border-top:1px solid var(--line2);font-size:var(--fs-sm)}
.h1 .cc-r:first-child{border-top:0}
.h1 .cc-r>span{color:var(--muted)}
.h1 .cc-r>b,.h1 .cc-r>code{justify-self:end;text-align:right;min-width:0;overflow-wrap:anywhere}
.h1 .cc-r>b{color:var(--strong);font-weight:600}
.h1 .cc-r>code{font-family:var(--mono);font-size:var(--fs-xs);color:var(--text)}
.h1 .cc-cp{cursor:copy}
.h1 .cc-cp:hover{color:var(--accent)}
.h1 .cc-note{white-space:pre-wrap;font-weight:500!important}
.h1 .cc-sub{display:flex;align-items:center;gap:4px;min-width:0;padding:6px 6px 6px 10px;border:1px solid var(--line);border-radius:var(--r-md,10px);background:var(--surface)}
.h1 .cc-sub code{flex:1;min-width:0;font-family:var(--mono);font-size:var(--fs-xs);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.h1 .cc-qr{margin-top:10px;align-self:flex-start;padding:10px;border-radius:12px;background:#fff}
.h1 .cc-th{display:flex;align-items:center;justify-content:space-between;gap:8px;margin-bottom:6px}
.h1 .cc-th .cc-t{margin:0;white-space:nowrap}
.h1 .cc-ta{display:flex;gap:4px}
.h1 .cc-q{margin:0 0 8px;height:38px;width:100%;max-width:none}
.h1 .cc-ib{display:flex;flex-direction:column;max-height:440px;overflow-y:auto;overscroll-behavior:contain;border:1px solid var(--line);border-radius:var(--r-md,10px);background:var(--surface)}
.h1 .cc-g{position:sticky;top:0;z-index:1;display:flex;align-items:center;justify-content:space-between;gap:8px;padding:8px 12px;background:var(--surface2);border-top:1px solid var(--line2);font-size:var(--fs-sm);font-weight:600;color:var(--strong)}
.h1 .cc-g:first-child{border-top:0}
.h1 .cc-g small{color:var(--muted);font-weight:500;margin-left:4px}
.h1 .cc-ga{display:flex;gap:2px}
.h1 .cc-ga button{border:0;background:none;color:var(--accent);font:inherit;font-size:var(--fs-xs);font-weight:600;padding:2px 6px;border-radius:6px;cursor:pointer}
.h1 .cc-ga button:hover:not(:disabled){background:var(--surface)}
.h1 .cc-ga button:disabled{color:var(--muted);opacity:.5;cursor:default}
.h1 .cc-i{display:flex;align-items:center;justify-content:space-between;gap:10px;padding:8px 12px;border-top:1px solid var(--line2);cursor:pointer}
.h1 .cc-g+.cc-i{border-top:0}
.h1 .cc-i:hover{background:var(--surface2)}
.h1 .cc-i.off .cc-in{opacity:.6}
.h1 .cc-in{display:flex;flex-direction:column;min-width:0;line-height:1.3}
.h1 .cc-in b{color:var(--strong);font-weight:600;font-size:var(--fs-sm);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.h1 .cc-in small{color:var(--muted);font-size:var(--fs-xs)}
.h1 .cc-i .i-tg{flex:none}
.h1 .cc-none{margin:0;padding:10px 12px;color:var(--muted);font-size:var(--fs-sm)}
.h1 .cc-act{gap:6px}
.h1 .cc-act .cc-t{margin-bottom:0}
.h1 .cc-act .btn{width:100%;justify-content:flex-start}
.h1 .cc-bad{color:var(--bad)!important}
@media (max-width:1180px){
  .h1 .cc-exp{grid-template-columns:minmax(0,1fr) minmax(0,1fr);padding-left:0}
  .h1 .cc-act{grid-column:1/-1;flex-direction:row;flex-wrap:wrap;align-items:center}
  .h1 .cc-act .cc-t{width:100%}
  .h1 .cc-act .btn{width:auto}
}
@media (max-width:760px){
  .h1 .cc-exp{grid-template-columns:1fr;padding:14px 0 4px}
  .h1 .cc-th{flex-wrap:wrap}
}
</style>
