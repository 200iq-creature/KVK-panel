import { reactive } from 'vue'

// global switch for the Xray core window (version picker, update, rollback)
export const xrayUi = reactive({ open: false })
export const openXray = () => { xrayUi.open = true }
