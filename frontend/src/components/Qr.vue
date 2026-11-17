<template><canvas ref="cv" class="qr" :aria-label="'QR: ' + text"></canvas></template>
<script setup>
import { ref, watch, onMounted } from 'vue'
import QRCode from 'qrcode'
const p = defineProps({ text: String, size: { type: Number, default: 220 } })
const cv = ref(null)
const draw = () => { if (cv.value && p.text) QRCode.toCanvas(cv.value, p.text, { width: p.size, margin: 1, errorCorrectionLevel: 'M', color: { dark: '#0e0e10', light: '#ffffff' } }).catch(() => {}) }
onMounted(draw)
watch(() => [p.text, p.size], draw)
</script>
