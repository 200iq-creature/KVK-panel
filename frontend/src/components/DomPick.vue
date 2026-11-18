<template>
  <div class="dpk">
    <Seg :model-value="modelValue.mode" :options="opts" @update:model-value="setMode" />
    <div v-if="modelValue.mode === 'free'" class="dpk-b">
      <div class="dpk-row">
        <input :value="modelValue.label" class="inp mono dpk-lbl" maxlength="30" :size="Math.max(5, (modelValue.label || '').length + 1)" placeholder="слово" @input="set({ label: clean($event.target.value) })">
        <span class="dpk-suf mono" @click="$event.currentTarget.previousElementSibling.focus()">-{{ dashed || '…' }}.sslip.io</span>
      </div>
      <p class="hint">Бесплатное имя без регистрации — сразу ведёт на {{ ip || 'IP сервера' }}. Слово можно поменять.</p>
    </div>
    <div v-else-if="modelValue.mode === 'exist'" class="dpk-b">
      <select :value="modelValue.id" class="inp" @change="set({ id: $event.target.value })">
        <option value="" disabled>выберите домен</option>
        <option v-for="d in list" :key="d.id" :value="d.id">{{ d.name }} · {{ d.status === 'active' ? 'сертификат есть' : d.issuing ? 'выпускается' : 'без сертификата' }}{{ d.panel ? ' · панель' : d.site ? ' · сайт' : '' }}</option>
      </select>
      <p v-if="!list.length" class="hint">На этом сервере пока нет доменов — выберите «Бесплатный» или «Свой».</p>
    </div>
    <div v-else class="dpk-b">
      <input :value="modelValue.name" class="inp mono" placeholder="vpn.example.com" @input="set({ name: $event.target.value.trim().toLowerCase() })">
      <p class="hint">Создайте у регистратора A-запись <b>{{ modelValue.name || 'домен' }} → {{ ip || 'IP сервера' }}</b>{{ direct ? ' без облака Cloudflare (серое облако)' : '' }}. Сертификат Let's Encrypt панель выпустит сама.</p>
    </div>
  </div>
</template>
<script setup>
import { computed } from 'vue'
import Seg from './Seg.vue'

const p = defineProps({ modelValue: Object, domains: Array, ip: String, direct: Boolean })
const emit = defineEmits(['update:modelValue'])
const list = computed(() => (p.domains || []).filter(d => d.kind === 'domain'))
const opts = computed(() => [{ v: 'free', l: 'Бесплатный' }, { v: 'exist', l: 'Из панели', c: list.value.length || undefined }, { v: 'own', l: 'Свой домен' }])
const dashed = computed(() => /^\d+\.\d+\.\d+\.\d+$/.test(p.ip || '') ? p.ip.replaceAll('.', '-') : '')
const clean = (s) => s.toLowerCase().replace(/[^a-z0-9-]/g, '').replace(/^-+/, '').slice(0, 30)
const set = (x) => emit('update:modelValue', { ...p.modelValue, ...x })
function setMode(m) {
  const x = { mode: m }
  if (m === 'exist' && !p.modelValue.id && list.value.length) x.id = (list.value.find(d => d.status === 'active') || list.value[0]).id
  set(x)
}
</script>
