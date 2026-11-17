<template>
  <div class="pagein i-wrap">
    <div class="pg-state">
      <span class="empty-ic"><Icon n="globe" /></span>
      <b>Такой страницы нет</b>
      <span>Адрес <span class="i-mono">{{ route.fullPath }}</span> не найден в панели.</span>
      <div class="pg-state-acts"><router-link class="btn btn-p" to="/">На обзор</router-link></div>
    </div>
  </div>
</template>
<script setup>
import { useRoute } from 'vue-router'
import Icon from '../components/Icon.vue'
const route = useRoute()
</script>
