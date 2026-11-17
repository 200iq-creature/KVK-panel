<template>
  <div class="i-opt">
    <button v-for="o in options" :key="String(o.v)" type="button" :class="{ on: o.v === modelValue }" :disabled="disabled" @click="$emit('update:modelValue', o.v)">
      <span class="i-rd"></span><b>{{ o.l }}</b><small>{{ o.s || '' }}</small>
    </button>
  </div>
</template>
<script setup>
defineProps({ modelValue: [String, Number, Boolean], options: Array, disabled: Boolean })
defineEmits(['update:modelValue'])
</script>
