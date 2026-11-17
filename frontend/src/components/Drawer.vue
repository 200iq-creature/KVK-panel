<template>
  <Teleport to="body">
    <template v-if="open">
      <button class="kveil" type="button" aria-label="Закрыть" @click="close"></button>
      <aside class="card drw kdrw" :class="cls" role="dialog" aria-modal="true" :style="{ width: width }">
        <div class="drw-h">
          <div class="drw-t"><span v-if="eyebrow" class="eyebrow">{{ eyebrow }}</span><h2>{{ title }}</h2><p v-if="sub" class="hint">{{ sub }}</p></div>
          <button class="btn btn-g btn-ic" type="button" aria-label="Закрыть" @click="close"><Icon n="x" /></button>
        </div>
        <div class="drw-b"><slot /></div>
        <div v-if="$slots.foot" class="drw-f"><slot name="foot" /></div>
      </aside>
    </template>
  </Teleport>
</template>
<script setup>
import { watch, onBeforeUnmount } from 'vue'
import Icon from './Icon.vue'
const p = defineProps({ open: Boolean, title: String, sub: String, eyebrow: String, width: String, cls: String })
const emit = defineEmits(['close'])
const close = () => emit('close')
const onKey = (e) => { if (e.key === 'Escape' && p.open) close() }
watch(() => p.open, (v) => { v ? window.addEventListener('keydown', onKey) : window.removeEventListener('keydown', onKey) }, { immediate: true })
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>
