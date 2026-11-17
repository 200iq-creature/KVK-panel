<template>
  <router-view v-if="route.meta.public" />
  <Shell v-else-if="route.name" />
  <GlobalUi />
</template>
<script setup>
import { useRoute } from 'vue-router'
import Shell from './components/Shell.vue'
import GlobalUi from './components/GlobalUi.vue'
const route = useRoute()
</script>
