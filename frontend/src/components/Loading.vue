<template>
  <div class="pg-state">
    <template v-if="error"><b>Не удалось загрузить</b><span>{{ error }}</span><div class="pg-state-acts"><button class="btn" type="button" @click="$emit('retry')"><Icon n="refresh" />Повторить</button></div></template>
    <template v-else><span class="spin"></span><span>Загружаем…</span></template>
  </div>
</template>
<script setup>
import Icon from './Icon.vue'
defineProps({ error: String })
defineEmits(['retry'])
</script>
