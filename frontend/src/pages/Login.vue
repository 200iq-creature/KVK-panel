<template>
  <div class="ui-login">
    <div class="ap-au">
      <section class="ap-br lg-brand">
        <div class="ap-brt">
          <span class="ap-brl"><img :src="LOGO" alt=""><b>KVK Panel</b></span>
          <button class="ap-thm" type="button" @click="toggleTheme"><Icon :n="dark ? 'sun' : 'moon'" />{{ dark ? 'Светлая' : 'Тёмная' }}</button>
        </div>
        <div class="ap-brm">
          <span class="i-tagw">Xray-core · VLESS · Reality · XHTTP · Hysteria2</span>
          <h1>Своя VPN-панель <em>на Xray</em></h1>
          <p>Клиенты, подписки, бот-магазин и связка серверов — в одном окне. Вход только для администратора.</p>
          <div class="ap-pts">
            <div class="ap-pt"><b>7</b><span>протоколов и транспортов</span></div>
            <div class="ap-pt"><b>2FA</b><span>TOTP и Telegram</span></div>
            <div class="ap-pt"><b>1</b><span>подписка на все серверы</span></div>
          </div>
        </div>
        <div class="ap-brf"><span>KVK Panel</span><a href="https://github.com/XTLS/Xray-core" target="_blank" rel="noopener">Xray-core</a></div>
      </section>
      <div class="ap-side">
        <form class="card ap-card" @submit.prevent="submit">
          <div class="ap-head">
            <span class="ap-ic"><Icon :n="step === 'pass' ? 'lock' : 'shield'" /></span>
            <h2 class="ap-t">{{ step === 'pass' ? 'Вход в панель' : 'Подтверждение входа' }}</h2>
            <p v-if="step === 'pass'">Логин и пароль администратора. Забыли — в консоли сервера выполните <span class="i-mono">kvk</span> → «Сбросить логин и пароль».</p>
            <p v-else>Второй шаг: {{ methods.includes('totp') ? 'код из приложения-аутентификатора' : '' }}{{ methods.length > 1 ? ' или ' : '' }}{{ methods.includes('telegram') ? 'код из Telegram' : '' }}.</p>
          </div>
          <div v-if="expired && step === 'pass' && !error" class="i-hint wa"><Icon n="clock" /><span>Сессия истекла — войдите в панель заново.</span></div>
          <template v-if="step === 'pass'">
            <div class="ap-f"><label class="ap-l" for="lg-u">Логин</label><input id="lg-u" ref="userInp" v-model="username" class="inp" :class="{ err: !!error }" autocomplete="username" autocapitalize="off" spellcheck="false" required></div>
            <div class="ap-f">
              <div class="ap-lr"><label class="ap-l" for="lg-p">Пароль</label><button class="ap-lnk" type="button" @click="showPass = !showPass">{{ showPass ? 'Скрыть' : 'Показать' }}</button></div>
              <input id="lg-p" v-model="password" class="inp" :class="{ err: !!error }" :type="showPass ? 'text' : 'password'" autocomplete="current-password" required>
            </div>
          </template>
          <template v-else>
            <div v-if="methods.length > 1" class="i-seg ap-seg"><button type="button" :class="{ on: method === 'totp' }" @click="method = 'totp'">Приложение</button><button type="button" :class="{ on: method === 'telegram' }" @click="method = 'telegram'">Telegram</button></div>
            <div v-if="method === 'telegram'" class="ap-tg">
              <div class="ap-tgs"><span class="ap-n">1</span><span class="ap-lt"><b>Отправьте код в Telegram</b><small>Бот пришлёт 6 цифр администратору панели.</small><button class="btn btn-sm" type="button" :disabled="busy.tg" @click="sendTg"><span v-if="busy.tg" class="spin"></span><Icon v-else n="send" />{{ tgSent ? 'Отправить ещё раз' : 'Отправить код' }}</button></span></div>
              <div class="ap-tgs"><span class="ap-n">2</span><span class="ap-lt"><b>Введите код ниже</b><small>Код действует 5 минут.</small></span></div>
            </div>
            <div class="ap-f"><label class="ap-l" for="lg-c">Код</label><input id="lg-c" ref="codeInp" v-model="code" class="inp ap-otp" :class="{ err: !!error }" inputmode="numeric" autocomplete="one-time-code" maxlength="6" placeholder="6 цифр" required></div>
          </template>
          <div v-if="error" class="i-hint bad ap-err"><Icon n="warn" /><span>{{ error }}</span></div>
          <button class="btn btn-p" type="submit" :disabled="busy.go"><span v-if="busy.go" class="spin"></span>{{ step === 'pass' ? 'Войти' : 'Подтвердить' }}</button>
          <button v-if="step !== 'pass'" class="btn btn-g" type="button" @click="reset">Назад к паролю</button>
          <p class="ap-cap">5 неверных попыток за 5 минут — вход с этого IP блокируется до конца окна.</p>
        </form>
      </div>
    </div>
  </div>
</template>
<script setup>
import { ref, reactive, onMounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Icon from '../components/Icon.vue'
import { post, get, session, LOGO } from '../lib/api.js'
import { theme, applyTheme } from '../lib/ui.js'

const route = useRoute()
const router = useRouter()
const username = ref('')
const password = ref('')
const code = ref('')
const showPass = ref(false)
const error = ref('')
const step = ref('pass')
const ticket = ref('')
const methods = ref([])
const method = ref('totp')
const tgSent = ref(false)
const busy = reactive({ go: false, tg: false })
const expired = !!route.query.expired
const userInp = ref(null)
const codeInp = ref(null)
const dark = ref(document.documentElement.classList.contains('dark'))
function toggleTheme() { applyTheme(dark.value ? 'light' : 'dark'); dark.value = !dark.value }
void theme

async function finish() {
  Object.assign(session, await get('auth/me', { noRedirect: true }))
  session.authed = true
  const next = typeof route.query.next === 'string' && route.query.next.startsWith('/') ? route.query.next : '/'
  router.replace(next)
}
async function submit() {
  if (busy.go) return
  busy.go = true; error.value = ''
  try {
    if (step.value === 'pass') {
      const r = await post('auth/login', { username: username.value.trim(), password: password.value }, { noRedirect: true })
      if (r && r.need2fa) {
        ticket.value = r.ticket; methods.value = r.methods || ['totp']; method.value = methods.value[0]; step.value = 'code'
        await nextTick(); codeInp.value && codeInp.value.focus()
        if (method.value === 'telegram' && methods.value.length === 1) sendTg()
        return
      }
    } else {
      await post('auth/2fa', { ticket: ticket.value, code: code.value.trim(), method: method.value }, { noRedirect: true })
    }
    await finish()
  } catch (e) {
    error.value = e.message
    if (step.value !== 'pass' && /истекла/.test(e.message)) reset(e.message)
  } finally { busy.go = false }
}
async function sendTg() {
  busy.tg = true; error.value = ''
  try { await post('auth/2fa/telegram', { ticket: ticket.value }, { noRedirect: true }); tgSent.value = true } catch (e) { error.value = e.message } finally { busy.tg = false }
}
function reset(msg) { step.value = 'pass'; code.value = ''; ticket.value = ''; tgSent.value = false; error.value = typeof msg === 'string' ? msg : ''; password.value = '' }
onMounted(() => userInp.value && userInp.value.focus())
</script>
<style>
.h1 .lg-brand{display:flex;flex-direction:column;border-radius:var(--r-lg);background:radial-gradient(120% 90% at 0% 0%,#7a73ff 0%,#5b54ee 38%,#2c2799 100%);color:#fff;position:relative;overflow:hidden}
.h1 .lg-brand{--lgInk:#fff;--acSub:rgba(255,255,255,.78);--lgRule:rgba(255,255,255,.2);--lgEdge:rgba(255,255,255,.3);--lgHover:rgba(255,255,255,.14)}
.h1.hack .lg-brand{background:radial-gradient(120% 90% at 0% 0%,#1fd17f 0%,#0d6b40 45%,#03130a 100%)}
.h1 .lg-brand:after{content:"";position:absolute;right:-120px;bottom:-120px;width:420px;height:420px;border-radius:50%;border:60px solid rgba(255,255,255,.06)}
.h1 .ui-login .ap-thm{display:inline-flex;align-items:center;gap:8px;cursor:pointer}
.h1 .ui-login .ap-thm .ic{width:18px;height:18px}
.h1 .ui-login .ap-card>.btn{width:100%}
</style>
