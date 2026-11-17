<template><svg class="ic" viewBox="0 0 24 24" aria-hidden="true"><path :d="I[n] || n" /></svg></template>
<script setup>
import { I } from '../lib/icons.js'
defineProps({ n: { type: String, required: true } })
</script>
