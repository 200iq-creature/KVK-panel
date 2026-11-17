<template>
  <div class="i-kr">
    <span>{{ k }}</span>
    <code v-if="code">{{ shown }}</code><b v-else><slot>{{ v }}</slot></b>
    <span v-if="copyable || secret" class="kr-a">
      <button v-if="secret" class="btn btn-sm btn-g" type="button" @click="reveal = !reveal">{{ reveal ? 'Скрыть' : 'Показать' }}</button>
      <button v-if="copyable" class="i-cp" type="button" title="Скопировать" @click="copy(String(v))"><Icon n="copy" /></button>
    </span>
    <span v-else><slot name="a"></slot></span>
  </div>
</template>
<script setup>
import { ref, computed } from 'vue'
import Icon from './Icon.vue'
import { copy } from '../lib/ui.js'
const p = defineProps({ k: String, v: [String, Number], code: Boolean, copyable: Boolean, secret: Boolean })
const reveal = ref(false)
const shown = computed(() => {
  const s = String(p.v ?? '—')
  if (!p.secret || reveal.value || s.length < 12) return s || '—'
  return s.slice(0, 4) + '•'.repeat(Math.min(28, s.length - 12)) + s.slice(-8)
})
</script>
