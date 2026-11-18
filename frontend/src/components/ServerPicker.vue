<template>
  <div v-if="srv.nodes.length" class="srvp" role="group" aria-label="Сервер">
    <span class="srvp-l">Сервер</span>
    <button type="button" :class="{ on: !srv.id }" @click="pick('')"><Icon n="cpu" />{{ srv.self || 'Этот сервер' }}<small>этот</small></button>
    <button v-for="n in srv.nodes" :key="n.id" type="button" :class="{ on: srv.id === n.id, bad: n.lastError, wa: n.drift || n.replaced }" :title="n.lastError || (n.replaced ? 'новый сервер — восстановите конфигурацию' : n.drift ? 'конфигурация отличается от сохранённой' : n.url)" @click="pick(n.id)">
      <i></i>{{ flag(n.country) }} {{ n.name }}
    </button>
  </div>
  <div v-if="curNode && (curNode.drift || curNode.replaced)" class="i-alrt wa srvp-w">
    <Icon n="warn" /><span><b>{{ curNode.replaced ? `«${curNode.name}» — новый сервер, конфигурация ещё не восстановлена.` : `Конфигурация «${curNode.name}» отличается от сохранённой на основном.` }}</b> <span class="muted">Изменения здесь не сохранятся в копию, пока вы не выберете, какую конфигурацию оставить.</span></span>
    <router-link class="btn btn-sm" to="/link">Связка серверов</router-link>
  </div>
</template>
<script setup>
import { onMounted } from 'vue'
import Icon from './Icon.vue'
import { srv, curNode, loadServers } from '../lib/server.js'
import { flag } from '../lib/fmt.js'
const emit = defineEmits(['change'])
function pick(id) { if (srv.id !== id) { srv.id = id; emit('change', id) } }
onMounted(() => loadServers(true))
</script>
<style>
.h1 .srvp{display:flex;flex-wrap:wrap;align-items:center;gap:6px;margin:-6px 0 16px}
.h1 .srvp-l{font-size:var(--fs-sm);color:var(--muted);margin-right:4px}
.h1 .srvp button{display:inline-flex;align-items:center;gap:7px;height:var(--ctlSm);padding:0 12px;border-radius:99px;border:1px solid var(--line);background:var(--surface);color:var(--text);font:inherit;font-size:var(--fs-sm);cursor:pointer}
.h1 .srvp button .ic{width:15px;height:15px}
.h1 .srvp button small{font-size:var(--fs-xs);color:var(--muted)}
.h1 .srvp button i{width:7px;height:7px;border-radius:50%;background:var(--okDot)}
.h1 .srvp button.wa i{background:var(--warnDot)}
.h1 .srvp button.bad i{background:var(--badDot)}
.h1 .srvp button.on{border-color:var(--accent);background:var(--accentSoft);color:var(--accentText);font-weight:500}
@media (hover:hover){.h1 .srvp button:not(.on):hover{background:var(--surface2)}}
.h1 .srvp-w{margin:-4px 0 16px}
</style>
