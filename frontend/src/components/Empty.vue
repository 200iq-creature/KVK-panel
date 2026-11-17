<template>
  <div class="empty"><span class="empty-ic"><Icon :n="icon" /></span><b>{{ title }}</b><span v-if="text" class="hint">{{ text }}</span><slot /></div>
</template>
<script setup>
import Icon from './Icon.vue'
defineProps({ icon: { type: String, default: 'info' }, title: String, text: String })
</script>
