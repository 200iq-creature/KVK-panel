<template>
  <div class="i-tgr">
    <Tg :model-value="modelValue" :disabled="disabled" @update:model-value="$emit('update:modelValue', $event)" />
    <span><b v-if="bold">{{ label }}</b><template v-else>{{ label }}</template><small v-if="hint || $slots.default"><slot>{{ hint }}</slot></small></span>
  </div>
</template>
<script setup>
import Tg from './Tg.vue'
defineProps({ modelValue: Boolean, label: String, hint: String, disabled: Boolean, bold: Boolean })
defineEmits(['update:modelValue'])
</script>
