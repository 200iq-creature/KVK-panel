<template>
  <div class="i-seg" role="group">
    <button v-for="o in norm" :key="String(o.v)" type="button" :class="{ on: o.v === modelValue }" :disabled="disabled" @click="$emit('update:modelValue', o.v)">{{ o.l }}<i v-if="o.c !== undefined">{{ o.c }}</i></button>
  </div>
</template>
<script setup>
import { computed } from 'vue'
const p = defineProps({ modelValue: [String, Number, Boolean], options: Array, disabled: Boolean })
defineEmits(['update:modelValue'])
const norm = computed(() => p.options.map(o => Array.isArray(o) ? { v: o[0], l: o[1], c: o[2] } : (typeof o === 'object' ? o : { v: o, l: String(o) })))
</script>
