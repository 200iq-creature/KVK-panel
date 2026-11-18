<template>
  <div class="app-root">
    <input id="navtg" v-model="navOpen" class="navtg" type="checkbox" aria-hidden="true" tabindex="-1">
    <header class="tx-top">
      <label class="tx-ib tx-burger" for="navtg" aria-label="Разделы"><Icon n="menu" /></label>
      <router-link class="tx-logo" to="/" :aria-label="'KVK Panel — ' + meta.node">
        <img :src="LOGO" alt="">
        <span><b>KVK Panel</b><em>{{ meta.node || 'сервер' }} · {{ meta.host }} · v{{ meta.version || session.version }}<template v-if="meta.branch && meta.branch !== 'main'"> · {{ meta.branch }}</template></em></span>
      </router-link>
      <span class="tx-sp"></span>
      <button v-if="meta.loaded" class="tx-xr" type="button" :title="'Ядро Xray — сменить версию, обновить или откатить'" @click="openXray"><i :class="meta.xray?.running ? 'ok' : 'bad'"></i><span>Xray</span><b>{{ meta.xray?.version ? 'v' + meta.xray.version : 'нет' }}</b></button>
      <button class="kbar" type="button" aria-haspopup="dialog" @click="kOpen = true"><Icon n="search" /><span>Клиент, раздел или действие…</span><span class="kbd">Ctrl K</span></button>
      <button class="tx-ib tx-th" type="button" :title="'Тема: ' + themeLabel" :aria-label="'Тема: ' + themeLabel" @click="cycleTheme"><Icon :n="theme.mode === 'light' ? 'sun' : theme.mode === 'system' ? 'auto' : 'moon'" /></button>
      <span class="tx-mew">
        <button class="tx-me" type="button" :aria-expanded="menuOpen" aria-label="Меню профиля" @click="menuOpen = !menuOpen">{{ (session.user || 'A').slice(0, 1).toUpperCase() }}</button>
        <template v-if="menuOpen">
          <button class="tx-veil" type="button" aria-label="Закрыть" @click="menuOpen = false"></button>
          <div class="card tx-pop" role="menu">
            <div class="mi-h"><b>{{ session.user }}</b><small>администратор · {{ meta.node }}</small></div>
            <hr>
            <p class="eyebrow">Тема</p>
            <button v-for="t in THEMES" :key="t[0]" type="button" :style="theme.mode === t[0] ? 'color:var(--accentText);font-weight:600' : ''" @click="applyTheme(t[0])"><Icon :n="t[2]" />{{ t[1] }}</button>
            <hr>
            <router-link to="/settings" @click="menuOpen = false"><Icon n="set" />Настройки</router-link>
            <router-link to="/security" @click="menuOpen = false"><Icon n="sec" />Безопасность</router-link>
            <button type="button" @click="menuOpen = false; openXray()"><Icon n="cpu" />Версия Xray</button>
            <button type="button" @click="restartXray"><Icon n="refresh" />Перезапустить Xray</button>
            <hr>
            <button type="button" @click="logout"><Icon n="logout" />Выйти</button>
          </div>
        </template>
      </span>
    </header>
    <label class="navveil" for="navtg" aria-hidden="true"></label>
    <div class="tx-body">
      <aside class="tx-rail" aria-label="Разделы">
        <div class="tx-rh-m"><b>KVK Panel</b><label class="tx-ib" for="navtg" aria-label="Закрыть меню" role="button"><Icon n="x" /></label></div>
        <nav aria-label="Разделы" style="display:flex;flex-direction:column;gap:10px">
          <div v-for="g in NAV_SIDE" :key="g.label" style="display:flex;flex-direction:column;gap:1px">
            <p class="eyebrow tx-gl" style="padding:0 11px 3px">{{ g.label }}</p>
            <router-link v-for="it in g.items" :key="it.id" class="nav" :class="{ on: navOf(route.meta.nav) === it.id }" :to="it.to" :aria-current="navOf(route.meta.nav) === it.id ? 'page' : undefined" @click="navOpen = false">
              <Icon :n="it.icon" /><span>{{ it.label }}</span><span v-if="it.count && meta.clients" class="cnt">{{ meta.clients }}</span>
            </router-link>
          </div>
        </nav>
      </aside>
      <main class="tx-main">
        <div v-if="meta.xray && meta.loaded && !meta.xray.running" class="i-alrt" style="margin:0 0 14px">
          <Icon n="warn" /><span><b>Xray не запущен.</b> <span class="muted">{{ meta.xray.error || (meta.xray.installed ? 'Клиенты не могут подключиться.' : 'Ядро Xray не установлено — нажмите «Установить Xray».') }}</span></span>
          <button class="btn btn-sm" type="button" @click="openXray">{{ meta.xray.installed ? 'Ядро Xray' : 'Установить Xray' }}</button>
        </div>
        <router-view v-slot="{ Component }"><component :is="Component" :key="route.path" /></router-view>
      </main>
    </div>
    <Kbar v-model:open="kOpen" />
    <XrayModal />
  </div>
</template>
<script setup>
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Icon from './Icon.vue'
import Kbar from './Kbar.vue'
import XrayModal from './XrayModal.vue'
import { openXray } from '../lib/xrayui.js'
import { NAV_SIDE, navOf } from '../lib/nav.js'
import { meta, loadMeta } from '../lib/meta.js'
import { session, post, LOGO } from '../lib/api.js'
import { theme, applyTheme, ok, fail } from '../lib/ui.js'

const route = useRoute()
const router = useRouter()
const navOpen = ref(false)
const menuOpen = ref(false)
const kOpen = ref(false)
const THEMES = [['dark', 'Новая (тёмная)', 'moon'], ['light', 'Белая', 'sun'], ['hacker', 'Хакерская', 'cpu'], ['system', 'Как в системе', 'auto']]
const themeLabel = computed(() => (THEMES.find(t => t[0] === theme.mode) || THEMES[0])[1])
function cycleTheme() { const order = ['system', 'light', 'dark', 'hacker']; applyTheme(order[(order.indexOf(theme.mode) + 1) % order.length]) }

async function logout() {
  menuOpen.value = false
  try { await post('auth/logout') } catch {}
  session.authed = false
  router.replace('/login')
}
async function restartXray() {
  menuOpen.value = false
  try { await post('xray/restart'); ok('Xray перезапущен'); loadMeta() } catch (e) { fail(e) }
}
const onKey = (e) => {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); kOpen.value = !kOpen.value }
  if (e.key === '/' && !/INPUT|TEXTAREA|SELECT/.test(document.activeElement?.tagName || '')) { e.preventDefault(); kOpen.value = true }
}
let timer
onMounted(() => { loadMeta(); window.addEventListener('keydown', onKey); timer = setInterval(loadMeta, 30000) })
onBeforeUnmount(() => { window.removeEventListener('keydown', onKey); clearInterval(timer) })
watch(() => route.path, () => { menuOpen.value = false })
</script>
