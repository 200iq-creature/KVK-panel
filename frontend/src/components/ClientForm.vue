<template>
  <div class="col">
    <div class="fld"><label class="lbl">Имя</label><input v-model="f.name" class="inp" maxlength="64" placeholder="пусто — сгенерируем" autocomplete="off"><p class="hint">Латиница, кириллица, цифры. Видно в приложении в названии конфига.</p></div>
    <div class="two">
      <div class="fld">
        <label class="lbl">Срок</label>
        <select v-model="f.expMode" class="inp"><option value="days">на N дней</option><option value="date">до даты</option><option value="never">бессрочно</option></select>
      </div>
      <div v-if="f.expMode === 'days'" class="fld"><label class="lbl">Дней</label><input v-model.number="f.days" class="inp" type="number" min="0.04" step="1"></div>
      <div v-else-if="f.expMode === 'date'" class="fld"><label class="lbl">До</label><input v-model="f.date" class="inp" type="datetime-local"></div>
    </div>
    <div class="two">
      <div class="fld"><label class="lbl">Лимит трафика, ГБ</label><input v-model.number="f.trafficGb" class="inp" type="number" min="0" step="1" placeholder="0 — безлимит"></div>
      <div class="fld"><label class="lbl">Устройств одновременно</label><input v-model.number="f.deviceLimit" class="inp" type="number" min="0" step="1" placeholder="0 — без ограничений"></div>
    </div>
    <div class="two">
      <div class="fld"><label class="lbl">Авто-сброс трафика</label><select v-model="f.resetPeriod" class="inp"><option v-for="r in RESET" :key="r[0]" :value="r[0]">{{ r[1] }}</option></select></div>
      <div v-if="f.resetPeriod === 'days' || f.resetPeriod === 'mday'" class="fld"><label class="lbl">{{ f.resetPeriod === 'mday' ? 'Число месяца' : 'Каждые N дней' }}</label><input v-model.number="f.resetDays" class="inp" type="number" min="1" :max="f.resetPeriod === 'mday' ? 28 : 365"></div>
    </div>
    <div class="two">
      <div class="fld"><label class="lbl">Telegram</label><input v-model="f.telegram" class="inp" placeholder="@username или ID"></div>
      <div class="fld"><label class="lbl">Теги</label><input v-model="f.tagsText" class="inp" placeholder="через запятую"><div v-if="allTags.length" class="tags"><button v-for="t in allTags" :key="t" class="chip" :class="{ on: tagList.includes(t) }" type="button" style="height:28px;padding:0 10px" @click="toggleTag(t)">{{ t }}</button></div></div>
    </div>
    <div class="fld"><label class="lbl">Заметка</label><textarea v-model="f.note" class="inp" rows="2" placeholder="видна только в панели"></textarea></div>
    <div class="fld">
      <label class="lbl">Инбаунды · {{ f.inbounds.length }} из {{ inbounds.length }}</label>
      <div class="list-rows">
        <template v-for="g in groups" :key="g.key">
          <div v-if="groups.length > 1" class="inb-g">
            <span><Icon :n="g.node ? 'link' : 'inb'" />{{ g.node ? 'Сервер «' + g.node + '»' : 'Этот сервер' }}<small v-if="g.node && !g.enabled"> · связка выключена</small></span>
            <button class="btn btn-sm btn-g" type="button" @click="toggleGroup(g)">{{ g.items.every(i => f.inbounds.includes(i.id)) ? 'Снять' : 'Все' }}</button>
          </div>
          <label v-for="i in g.items" :key="i.id" class="lr" style="cursor:pointer">
            <button class="chk" :class="{ on: f.inbounds.includes(i.id) }" type="button" @click.prevent="toggleInb(i.id)"><Icon n="check" /></button>
            <span class="lr-t"><b>{{ i.remark }}</b><small>{{ PROTO[i.protocol] || i.protocol }} · {{ transport(i) }} · порт {{ i.port }}{{ i.enabled ? '' : (i.node ? '' : ' · выключен') }}</small></span>
          </label>
        </template>
        <p v-if="!inbounds.length" class="hint">Инбаундов нет — создайте их в разделе «Инбаунды».</p>
      </div>
      <div class="row"><button class="btn btn-sm btn-g" type="button" @click="f.inbounds = inbounds.map(i => i.id)">Все</button><button class="btn btn-sm btn-g" type="button" @click="f.inbounds = []">Ни одного</button></div>
    </div>
    <details v-if="edit" class="det">
      <summary><span><b>Ключи</b><small>UUID и ID подписки — менять только при переносе</small></span><Icon n="chevD" /></summary>
      <div class="det-b">
        <div class="fld"><label class="lbl">UUID</label><input v-model="f.uuid" class="inp mono"></div>
        <div class="fld"><label class="lbl">ID подписки</label><input v-model="f.subId" class="inp mono"></div>
      </div>
    </details>
  </div>
</template>
<script setup>
import { reactive, computed, watch } from 'vue'
import Icon from './Icon.vue'
import { RESET } from '../lib/client.js'
import { PROTO, NET, SEC, toLocalInput, fromLocalInput } from '../lib/fmt.js'

const p = defineProps({ client: Object, inbounds: { type: Array, default: () => [] }, allTags: { type: Array, default: () => [] }, edit: Boolean })
const c = p.client || {}
const f = reactive({
  name: c.name || '', expMode: c.id ? (c.expiresAt ? 'date' : 'never') : 'days', days: 30, date: toLocalInput(c.expiresAt || Date.now() + 30 * 864e5),
  trafficGb: c.trafficLimit ? Math.round(c.trafficLimit / 1073741824 * 100) / 100 : 0, deviceLimit: c.deviceLimit || 0,
  resetPeriod: c.resetPeriod || '', resetDays: c.resetDays || 30, telegram: c.telegram || '', tagsText: (c.tags || []).join(', '), note: c.note || '',
  inbounds: c.id ? [...(c.inbounds || [])] : p.inbounds.filter(i => i.enabled).map(i => i.id), uuid: c.uuid || '', subId: c.subId || '',
})
watch(() => p.inbounds, (v) => { if (!c.id && !f.inbounds.length) f.inbounds = v.filter(i => i.enabled).map(i => i.id) })
const initReset = f.resetPeriod
const tagList = computed(() => f.tagsText.split(',').map(s => s.trim()).filter(Boolean))
function toggleTag(t) { const l = tagList.value; f.tagsText = (l.includes(t) ? l.filter(x => x !== t) : [...l, t]).join(', ') }
// UDP-based protocols carry a placeholder network/security — show what they really use
const OWN = { tuic: 'QUIC · TLS', hysteria2: 'QUIC · TLS', amneziawg: 'UDP', masque: 'HTTP/3 · TLS' }
const transport = (i) => OWN[i.protocol] || `${NET[i.network] || i.network} · ${SEC[i.security] || i.security}`
const groups = computed(() => {
  const out = [{ key: 'local', node: '', enabled: true, items: p.inbounds.filter(i => !i.nodeId) }]
  for (const i of p.inbounds) {
    if (!i.nodeId) continue
    let g = out.find(x => x.key === i.nodeId)
    if (!g) { g = { key: i.nodeId, node: i.node, enabled: i.enabled, items: [] }; out.push(g) }
    g.items.push(i)
  }
  return out.filter(g => g.items.length || g.key === 'local')
})
function toggleGroup(g) {
  const ids = g.items.map(i => i.id)
  f.inbounds = ids.every(id => f.inbounds.includes(id)) ? f.inbounds.filter(x => !ids.includes(x)) : [...new Set([...f.inbounds, ...ids])]
}
function toggleInb(id) { f.inbounds = f.inbounds.includes(id) ? f.inbounds.filter(x => x !== id) : [...f.inbounds, id] }

function body() {
  const b = { name: f.name.trim(), trafficGb: Number(f.trafficGb) || 0, deviceLimit: Number(f.deviceLimit) || 0, telegram: f.telegram.trim(), note: f.note, tags: tagList.value, inbounds: f.inbounds }
  if (p.edit || f.resetPeriod !== initReset) { b.resetPeriod = f.resetPeriod; b.resetDays = Number(f.resetDays) || 0 }
  if (f.expMode === 'days') b.days = Number(f.days) || 0
  else if (f.expMode === 'date') b.expiresAt = fromLocalInput(f.date)
  else b.expiresAt = 0
  if (p.edit) { b.uuid = f.uuid.trim(); b.subId = f.subId.trim() }
  return b
}
defineExpose({ body })
</script>
