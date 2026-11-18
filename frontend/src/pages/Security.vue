<template>
  <div class="pg-security">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="w2-eb"><router-link to="/settings">Настройки</router-link> · доступ к панели</span><h1>Безопасность</h1><p>Двухфакторный вход, подтверждение в Telegram, HTTPS и журнал входов. Панель скрыта за секретным путём — без него сервер отвечает обычной страницей nginx 404.</p></div>
      </div>
      <Loading v-if="!d" :error="error" @retry="reload" />
      <template v-else>
        <div class="i-mt">
          <Metric label="2FA (TOTP)" :value="d.totp ? 'включена' : 'выключена'" :note="d.totp ? 'код из приложения при входе' : 'рекомендуем включить'" :note-tone="d.totp ? 'ok' : 'wa'" />
          <Metric label="Вход через Telegram" :value="d.telegram ? 'включён' : 'выключен'" :note="d.tgConfigured ? 'бот настроен' : 'бот не настроен'" />
          <Metric label="HTTPS панели" :value="d.https ? 'включён' : 'нет'" :note="d.https ? `${d.httpsDomain} · ${d.httpsExpires ? 'до ' + date(d.httpsExpires) : ''}` : 'пароль идёт открытым текстом'" :note-tone="d.https ? 'ok' : 'bad'" />
          <Metric label="Неудачных входов" :value="failed24" note="за последние 24 часа" :note-tone="failed24 > 5 ? 'bad' : ''" />
        </div>
        <div v-if="!d.https" class="i-alrt bad"><Icon n="lock" /><span><b>Панель открыта по HTTP.</b> <span class="muted">Добавьте домен или IP в «Доменах» и включите для него «Использовать для панели» — сертификат выпустится и подключится автоматически.</span></span><router-link class="btn btn-sm" to="/domains">Настроить HTTPS</router-link></div>
        <div class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>Двухфакторная аутентификация</h2><span class="i-stt" :class="d.totp ? 'ok' : 'mu'">{{ d.totp ? 'включена' : 'выключена' }}</span></div>
            <div class="i-pb">
              <template v-if="!d.totp && !totp.secret">
                <p class="hint">При входе, кроме пароля, понадобится 6-значный код из Google Authenticator, Aegis, 2FAS или любого TOTP-приложения.</p>
                <div class="ft"><span class="grow"></span><button class="btn btn-p" type="button" @click="setup"><Icon n="shield" />Включить 2FA</button></div>
              </template>
              <template v-else-if="!d.totp">
                <ol class="steps"><li>Отсканируйте QR-код в приложении или введите ключ вручную.</li><li>Введите 6-значный код, который покажет приложение.</li></ol>
                <div class="row" style="align-items:flex-start"><Qr :text="totp.uri" :size="170" /><div class="col grow"><Kr k="Ключ" :v="totp.secret" code copyable /><div class="fld"><label class="lbl">Код из приложения</label><input v-model="totp.code" class="inp mono" inputmode="numeric" maxlength="6" placeholder="000000" @keydown.enter="enable"></div></div></div>
                <div class="ft"><button class="btn btn-g" type="button" @click="totp.secret = ''">Отмена</button><span class="grow"></span><button class="btn btn-p" type="button" :disabled="totp.code.length !== 6" @click="enable">Подтвердить и включить</button></div>
              </template>
              <template v-else>
                <p class="hint">Вход защищён кодом из приложения. Потеряли телефон? На сервере выполните <code class="i-mono">kvk</code> → «Отключить 2FA» или создайте файл <code class="i-mono">{{ d.recoveryFile }}</code>.</p>
                <div class="fld"><label class="lbl">Чтобы выключить — введите текущий код</label><div class="row" style="flex-wrap:nowrap"><input v-model="totp.code" class="inp mono" inputmode="numeric" maxlength="6" placeholder="000000"><button class="btn btn-danger" type="button" :disabled="totp.code.length !== 6" @click="disable">Выключить</button></div></div>
              </template>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Telegram-бот уведомлений</h2><span class="i-stt" :class="d.tgConfigured ? 'ok' : 'mu'">{{ d.tgConfigured ? 'настроен' : 'не настроен' }}</span></div>
            <div class="i-pb">
              <div class="fld"><label class="lbl">Токен бота</label><input v-model="tg.token" class="inp mono" placeholder="123456:ABC… от @BotFather" autocomplete="off"><p class="hint">Можно тот же бот, что и в бот-магазине, или отдельный.</p></div>
              <div class="fld"><label class="lbl">Ваш Telegram ID</label><input v-model="tg.adminId" class="inp mono" placeholder="123456789"><p class="hint">Узнать: напишите <a href="https://t.me/userinfobot" target="_blank" rel="noopener">@userinfobot</a>. Сначала нажмите /start в своём боте.</p></div>
              <TgRow v-model="tg.notify" bold label="Уведомления" hint="входы в панель, падение Xray, истекающие сертификаты, оплаты" />
              <TgRow v-model="tg.enabled" bold label="Подтверждать вход в Telegram" hint="после пароля бот пришлёт код — альтернатива или дополнение к TOTP" />
              <div class="ft"><button class="btn btn-g" type="button" :disabled="!d.tgConfigured" @click="tgTest"><Icon n="send" />Тестовое сообщение</button><span class="grow"></span><button class="btn btn-p" type="button" @click="tgSave">Сохранить</button></div>
            </div>
          </section>
        </div>
        <div class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>Логин и пароль</h2><span>{{ session.user }}</span></div>
            <div class="i-pb">
              <div class="fld"><label class="lbl">Текущий пароль</label><input v-model="cred.current" class="inp" type="password" autocomplete="current-password"></div>
              <div class="two">
                <div class="fld"><label class="lbl">Новый логин</label><input v-model="cred.username" class="inp" autocomplete="username" :placeholder="session.user"></div>
                <div class="fld"><label class="lbl">Новый пароль</label><input v-model="cred.newPassword" class="inp" type="password" autocomplete="new-password" placeholder="минимум 8 символов"></div>
              </div>
              <p class="hint">Остальные сессии будут завершены. Пустое поле — не менять.</p>
              <div class="ft"><button class="btn btn-g" type="button" @click="genPass">Сгенерировать пароль</button><span class="grow"></span><button class="btn btn-p" type="button" :disabled="!cred.current || (!cred.username && !cred.newPassword)" @click="saveCred">Сменить</button></div>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Скрытие панели</h2></div>
            <div class="i-pb">
              <div class="i-kv">
                <Kr k="Секретный путь" :v="'/' + meta.basePath + '/'" code copyable />
                <Kr k="Ответ на другие пути" v="nginx 404" />
                <Kr k="Блок после неудачных входов" v="5 ошибок за 5 минут → пауза" />
                <Kr k="Соединение" :v="d.secure ? 'HTTPS' : 'HTTP'" />
              </div>
              <div class="ft"><span class="grow"></span><router-link class="btn btn-sm" to="/settings">Сменить порт и путь</router-link></div>
            </div>
          </section>
        </div>
        <section class="i-pn">
          <div class="i-ph"><h2>Журнал входов <span class="i-cnt">{{ d.logins.length }}</span></h2><Seg v-model="lf" :options="[['all', 'Все'], ['bad', 'Неудачные']]" /></div>
          <div class="tbl-w">
            <table class="tbl">
              <thead><tr><th>Время</th><th>Результат</th><th>Логин</th><th>Способ</th><th>IP</th><th>Браузер</th></tr></thead>
              <tbody>
                <tr v-for="(l, i) in logins" :key="i">
                  <td class="nw">{{ dt(l.t) }}</td>
                  <td><span class="i-stt" :class="l.ok ? 'ok' : 'bad'">{{ l.ok ? 'вход' : (l.reason || 'отказ') }}</span></td>
                  <td>{{ l.user }}</td>
                  <td>{{ l.method || "—" }}</td>
                  <td class="i-mono">{{ l.ip }}</td>
                  <td class="muted" style="font-size:12px;max-width:320px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" :title="l.ua">{{ l.ua }}</td>
                </tr>
              </tbody>
            </table>
            <Empty v-if="!logins.length" icon="lock" title="Записей нет" />
          </div>
        </section>
      </template>
    </div>
  </div>
</template>
<script setup>
import '../styles/Security.css'
import { ref, reactive, computed, watch } from 'vue'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import Metric from '../components/Metric.vue'
import Empty from '../components/Empty.vue'
import TgRow from '../components/TgRow.vue'
import Seg from '../components/Seg.vue'
import Kr from '../components/Kr.vue'
import Qr from '../components/Qr.vue'
import { get, post, put, session } from '../lib/api.js'
import { meta } from '../lib/meta.js'
import { useLoad } from '../lib/useLoad.js'
import { ok, fail, confirm, copy } from '../lib/ui.js'
import { dt, date } from '../lib/fmt.js'

const { data: d, error, reload } = useLoad(() => get('security'), { interval: 30000 })
const totp = reactive({ secret: '', uri: '', code: '' })
const tg = reactive({ token: '', adminId: '', notify: true, enabled: false })
const cred = reactive({ current: '', username: '', newPassword: '' })
const lf = ref('all')
let inited = false
watch(d, (v) => { if (v && !inited) { inited = true; Object.assign(tg, { token: v.tgBotToken, adminId: v.tgAdminId, notify: v.tgNotify, enabled: v.telegram }) } })
const failed24 = computed(() => (d.value?.logins || []).filter(l => !l.ok && l.t > Date.now() - 864e5).length)
const logins = computed(() => (d.value?.logins || []).filter(l => lf.value === 'all' || !l.ok))
async function setup() { try { Object.assign(totp, await post('security/totp/setup'), { code: '' }) } catch (e) { fail(e) } }
async function enable() { try { await post('security/totp/enable', { code: totp.code }); ok('2FA включена'); Object.assign(totp, { secret: '', uri: '', code: '' }); reload(true) } catch (e) { fail(e) } }
async function disable() { if (!await confirm('Выключить 2FA?', 'Для входа будет достаточно пароля.', { danger: true, okText: 'Выключить' })) return; try { await post('security/totp/disable', { code: totp.code }); ok('2FA выключена'); totp.code = ''; reload(true) } catch (e) { fail(e) } }
async function tgSave() { try { await put('security/telegram', { ...tg }); ok('Сохранено'); inited = false; reload(true) } catch (e) { fail(e) } }
async function tgTest() { try { await post('security/telegram/test'); ok('Сообщение отправлено — проверьте Telegram') } catch (e) { fail(e) } }
function genPass() { const a = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789'; const r = crypto.getRandomValues(new Uint32Array(16)); cred.newPassword = Array.from(r, x => a[x % a.length]).join(''); copy(cred.newPassword, 'Пароль сгенерирован и скопирован') }
async function saveCred() {
  try {
    await post('settings/credentials', { ...cred })
    if (cred.username) session.user = cred.username
    ok('Данные входа изменены'); Object.assign(cred, { current: '', username: '', newPassword: '' })
  } catch (e) { fail(e) }
}
</script>
