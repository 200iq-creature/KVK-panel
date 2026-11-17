import { createApp } from 'vue'
import '@fontsource/jost/400.css'
import '@fontsource/jost/500.css'
import '@fontsource/jost/600.css'
import '@fontsource/fira-code/400.css'
import './styles/base.css'
import './styles/extras.css'
import SubApp from './components/SubApp.vue'

createApp(SubApp).mount('#app')
