import { createApp } from 'vue'
import '@fontsource/jost/400.css'
import '@fontsource/jost/500.css'
import '@fontsource/jost/600.css'
import '@fontsource/fira-code/400.css'
import '@fontsource/fira-code/500.css'
import './styles/base.css'
import './styles/extras.css'
import App from './App.vue'
import router from './router.js'
import { setUnauthorizedHandler, session } from './lib/api.js'

setUnauthorizedHandler(() => {
  const cur = router.currentRoute.value
  if (cur.name !== 'login') router.replace({ name: 'login', query: { next: cur.fullPath, expired: '1' } })
})
session.authed = null
createApp(App).use(router).mount('#app')
