import { createRouter, createWebHistory } from 'vue-router'
import { BASE, get, session } from './lib/api.js'

const routes = [
  { path: '/login', name: 'login', component: () => import('./pages/Login.vue'), meta: { title: 'Вход', public: true } },
  { path: '/', name: 'overview', component: () => import('./pages/Overview.vue'), meta: { title: 'Обзор', nav: 'overview' } },
  { path: '/clients', name: 'clients', component: () => import('./pages/Clients.vue'), meta: { title: 'Клиенты', nav: 'clients' } },
  { path: '/inbounds', name: 'inbounds', component: () => import('./pages/Inbounds.vue'), meta: { title: 'Инбаунды', nav: 'inb' } },
  { path: '/sub', name: 'sub', component: () => import('./pages/Sub.vue'), meta: { title: 'Подписка', nav: 'sub' } },
  { path: '/shop', name: 'shop', component: () => import('./pages/Shop.vue'), meta: { title: 'Бот-магазин', nav: 'shop' } },
  { path: '/tgproxy', name: 'tgproxy', component: () => import('./pages/TgProxy.vue'), meta: { title: 'Telegram-прокси', nav: 'tgp' } },
  { path: '/link', name: 'link', component: () => import('./pages/Link.vue'), meta: { title: 'Связка серверов', nav: 'link' } },
  { path: '/backups', name: 'backups', component: () => import('./pages/Backups.vue'), meta: { title: 'Бэкапы', nav: 'bak' } },
  { path: '/security', name: 'security', component: () => import('./pages/Security.vue'), meta: { title: 'Безопасность', nav: 'sec' } },
  { path: '/settings', name: 'settings', component: () => import('./pages/Settings.vue'), meta: { title: 'Настройки', nav: 'set' } },
  { path: '/filter', name: 'filter', component: () => import('./pages/Filter.vue'), meta: { title: 'Фильтрация', nav: 'flt' } },
  { path: '/warp', name: 'warp', component: () => import('./pages/Warp.vue'), meta: { title: 'WARP', nav: 'warp' } },
  { path: '/migrate', name: 'migrate', component: () => import('./pages/Migrate.vue'), meta: { title: 'Миграция', nav: 'mig' } },
  { path: '/domains', name: 'domains', component: () => import('./pages/Domains.vue'), meta: { title: 'Домены', nav: 'dom' } },
  { path: '/logs', name: 'logs', component: () => import('./pages/Logs.vue'), meta: { title: 'Логи', nav: 'logs' } },
  { path: '/:rest(.*)*', name: '404', component: () => import('./pages/NotFound.vue'), meta: { title: 'Такой страницы нет' } },
]

const router = createRouter({ history: createWebHistory(BASE), routes, scrollBehavior: () => ({ top: 0 }) })

router.beforeEach(async (to) => {
  if (session.authed === null) {
    try { Object.assign(session, await get('auth/me', { noRedirect: true })) } catch { session.authed = false }
    if (!session.authed) session.authed = false
  }
  if (!to.meta.public && !session.authed) return { name: 'login', query: to.fullPath !== '/' ? { next: to.fullPath } : {} }
  if (to.name === 'login' && session.authed) return { path: '/' }
})
router.afterEach((to) => { document.title = `KVK Panel — ${to.meta.title || ''}` })

export default router
