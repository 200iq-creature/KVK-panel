<template>
  <button class="i-tg" :class="{ on: modelValue }" type="button" role="switch" :aria-checked="!!modelValue" :disabled="disabled" @click="$emit('update:modelValue', !modelValue)"><span></span></button>
</template>
<script setup>
defineProps({ modelValue: Boolean, disabled: Boolean })
defineEmits(['update:modelValue'])
</script>
