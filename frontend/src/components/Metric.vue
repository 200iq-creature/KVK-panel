<template>
  <div class="i-pn i-m">
    <span>{{ label }}</span>
    <b><slot>{{ value }}</slot></b>
    <div v-if="pct !== undefined && pct !== null" class="i-mpr" :class="tone"><i :style="{ width: Math.max(0, Math.min(100, pct)) + '%' }"></i></div>
    <span v-if="note" class="m-nt" :class="noteTone">{{ note }}</span>
  </div>
</template>
<script setup>
defineProps({ label: String, value: [String, Number], pct: Number, tone: String, note: String, noteTone: String })
</script>
