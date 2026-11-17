<template>
  <Teleport to="body">
    <template v-if="dialog.open">
      <button class="kveil" type="button" aria-label="Закрыть" @click="done(false)"></button>
      <div class="card ui-dlg" role="alertdialog" aria-modal="true" style="display:flex;flex-direction:column;gap:14px">
        <div class="ui-dlg-h">
          <span class="ui-dlg-i" :class="{ bad: dialog.danger }"><Icon :n="dialog.danger ? 'warn' : dialog.input !== null ? 'edit' : 'info'" /></span>
          <b style="font-size:var(--fs-lg);color:var(--strong)">{{ dialog.title }}</b>
        </div>
        <p v-if="dialog.text" class="ui-dlg-t">{{ dialog.text }}</p>
        <div v-if="dialog.input !== null" class="ui-dlg-fl">
          <span v-if="dialog.inputLabel" class="ui-dlg-l">{{ dialog.inputLabel }}</span>
          <textarea v-if="dialog.inputType === 'textarea'" ref="inp" v-model="dialog.input" class="inp ui-dlg-ta" :placeholder="dialog.placeholder"></textarea>
          <input v-else ref="inp" v-model="dialog.input" class="inp" :type="dialog.inputType" :placeholder="dialog.placeholder" @keydown.enter="done(true)">
        </div>
        <div class="ui-dlg-f">
          <button class="btn" type="button" @click="done(false)">{{ dialog.cancelText }}</button>
          <button class="btn" :class="dialog.danger ? 'btn-danger' : 'btn-p'" type="button" @click="done(true)">{{ dialog.okText }}</button>
        </div>
      </div>
    </template>
    <div v-for="t in toasts" :key="t.id" class="ui-toast" role="status" :style="{ bottom: (24 + 58 * toasts.indexOf(t)) + 'px' }">
      <span class="ui-ti" :class="t.kind === 'ok' ? '' : t.kind"><Icon :n="t.kind === 'ok' ? 'check' : t.kind === 'bad' ? 'x' : 'info'" /></span>
      <span>{{ t.text }}</span>
    </div>
  </Teleport>
</template>
<script setup>
import { ref, watch, nextTick } from 'vue'
import Icon from './Icon.vue'
import { dialog, toasts } from '../lib/ui.js'
const inp = ref(null)
watch(() => dialog.open, async (v) => { if (v) { await nextTick(); inp.value && inp.value.focus() } })
function done(okd) {
  const r = dialog.resolve
  const val = dialog.input
  dialog.open = false
  if (r) r(dialog.input !== null ? (okd ? val : null) : okd)
}
window.addEventListener('keydown', (e) => { if (e.key === 'Escape' && dialog.open) done(false) })
</script>
