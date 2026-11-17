<template>
  <div class="pg-tgproxy">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="vw-eb">Telegram без VPN · <span class="i-mono">{{ d?.host }}</span></span><h1>Telegram-прокси</h1><p>MTProto (Fake-TLS через mtg) — одна общая ссылка, открывается в Telegram в один клик. SOCKS5 — личный логин и пароль на каждого, со сроком действия.</p></div>
        <div class="i-hd-a"><button v-if="s" class="btn btn-p" type="button" :disabled="busy.save || !dirty" @click="save"><span v-if="busy.save" class="spin"></span><Icon v-else n="check" />Сохранить</button></div>
      </div>
      <Loading v-if="!s" :error="error" @retry="load" />
      <template v-else>
        <div class="i-mt">
          <Metric label="MTProto" :value="!s.mtEnabled ? 'выключен' : d.mtg.running ? 'работает' : 'не запущен'" :note="s.mtEnabled ? (d.mtg.error || `порт ${s.mtPort} · аптайм ${uptime(d.mtg.uptime)}`) : 'включите ниже'" :note-tone="d.mtg.error ? 'bad' : d.mtg.running ? 'ok' : ''" />
          <Metric label="SOCKS5" :value="s.socksEnabled ? 'включён' : 'выключен'" :note="s.socksEnabled ? `порт ${s.socksPort} · через Xray` : 'включите ниже'" />
          <Metric label="Доступы SOCKS5" :value="d.users.length" :note="`${d.active} действующих`" />
          <Metric label="mtg" :value="d.mtgInstalled ? (d.mtgVersion || 'установлен') : 'не установлен'" :note="d.mtgInstalled ? 'бинарник MTProto-сервера' : 'нужен для MTProto'" />
        </div>
        <div class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>MTProto · Fake-TLS</h2><span class="i-stt" :class="!s.mtEnabled ? 'mu' : d.mtg.running ? 'ok' : 'bad'">{{ !s.mtEnabled ? 'выключен' : d.mtg.running ? 'работает' : 'ошибка' }}</span></div>
            <div class="i-pb">
              <TgRow v-model="s.mtEnabled" bold label="Включить MTProto-прокси" hint="Трафик выглядит как HTTPS к сайту маскировки" />
              <div v-if="!d.mtgInstalled" class="i-hint wa"><Icon n="warn" /><span>mtg ещё не установлен — скачайте его с GitHub (9seconds/mtg) одной кнопкой.</span><button class="btn btn-sm" type="button" :disabled="busy.install" @click="install"><span v-if="busy.install" class="spin"></span>Установить mtg</button></div>
              <div class="two">
                <div class="fld"><label class="lbl">Порт</label><input v-model.number="s.mtPort" class="inp" type="number" min="1" max="65535"><p class="hint">443 маскируется лучше всего, если он свободен</p></div>
                <div class="fld"><label class="lbl">Сайт маскировки</label><input v-model="s.mtDomain" class="inp mono" placeholder="www.google.com"><p class="hint">при смене секрет перевыпустится</p></div>
              </div>
              <div v-if="d.links.mtproto" class="fld"><label class="lbl">Ссылка для Telegram</label>
                <div class="vw-link"><code>{{ d.links.mtproto }}</code><button class="i-cp" type="button" @click="copy(d.links.mtproto)"><Icon n="copy" /></button></div>
                <div class="row" style="margin-top:10px"><Qr :text="d.links.mtproto" :size="148" /><div class="col" style="gap:8px"><a class="btn btn-sm" :href="d.links.mtproto" target="_blank" rel="noopener"><Icon n="send" />Открыть в Telegram</a><button class="btn btn-sm btn-g" type="button" @click="regen"><Icon n="refresh" />Перевыпустить секрет</button></div></div>
              </div>
              <div v-if="d.mtg.error" class="err-box">{{ d.mtg.error }}</div>
              <details v-if="d.mtg.log?.length" class="det"><summary><span><b>Журнал mtg</b><small>последние строки</small></span><Icon n="chevD" /></summary><div class="det-b"><pre class="log-box">{{ d.mtg.log.join('\n') }}</pre></div></details>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>SOCKS5</h2><span class="i-stt" :class="s.socksEnabled ? 'ok' : 'mu'">{{ s.socksEnabled ? 'включён' : 'выключен' }}</span></div>
            <div class="i-pb">
              <TgRow v-model="s.socksEnabled" bold label="Включить SOCKS5-прокси" hint="Работает внутри Xray: у каждого доступа свой логин и срок" />
              <div class="two">
                <div class="fld"><label class="lbl">Порт</label><input v-model.number="s.socksPort" class="inp" type="number" min="1" max="65535"></div>
                <div class="fld" style="justify-content:flex-end"><TgRow v-model="s.socksUdp" label="UDP" hint="для звонков в Telegram" /></div>
              </div>
              <div class="i-hint"><Icon n="info" /><span>SOCKS5 не шифрует трафик и легко блокируется DPI. Для России надёжнее MTProto Fake-TLS или полноценный VPN.</span></div>
            </div>
          </section>
        </div>
        <section class="i-pn">
          <div class="i-ph"><h2>Доступы SOCKS5 <span class="i-cnt">{{ d.users.length }}</span></h2><button class="btn btn-sm" type="button" @click="openNew"><Icon n="plus" />Доступ</button></div>
          <div v-if="!s.socksEnabled && d.users.length" class="i-pb" style="padding-bottom:0"><div class="i-hint wa"><Icon n="warn" /><span>SOCKS5 выключен — доступы не работают, пока его не включить.</span></div></div>
          <div class="tbl-w">
            <table class="tbl">
              <thead><tr><th>Имя</th><th>Логин</th><th>Срок</th><th>Статус</th><th></th></tr></thead>
              <tbody>
                <tr v-for="u in d.users" :key="u.user.id">
                  <td><b>{{ u.user.name }}</b><div v-if="u.user.note" class="muted" style="font-size:12px">{{ u.user.note }}</div></td>
                  <td class="i-mono">{{ u.user.username }}</td>
                  <td class="nw">{{ u.user.expiresAt ? left(u.user.expiresAt) : 'бессрочно' }}</td>
                  <td><span class="i-stt" :class="u.active ? 'ok' : u.user.enabled ? 'bad' : 'mu'">{{ u.active ? 'активен' : u.user.enabled ? 'истёк' : 'выключен' }}</span></td>
                  <td class="r"><span class="acts">
                    <button v-if="u.links.socks" class="btn btn-sm" type="button" @click="copy(u.links.socks, 'Ссылка для Telegram скопирована')"><Icon n="copy" />Ссылка</button>
                    <button class="btn btn-sm btn-g" type="button" @click="openEdit(u.user)">Изменить</button>
                    <button class="btn btn-sm btn-g btn-ic" type="button" title="Удалить" @click="remove(u.user)"><Icon n="trash" /></button>
                  </span></td>
                </tr>
              </tbody>
            </table>
            <Empty v-if="!d.users.length" icon="tgp" title="Доступов пока нет" text="Создайте доступ — человек получит ссылку, которая открывает настройки прокси прямо в Telegram" />
          </div>
        </section>
      </template>
    </div>
    <Drawer :open="uf.open" :title="uf.id ? 'Изменить доступ' : 'Новый доступ SOCKS5'" eyebrow="telegram-прокси" @close="uf.open = false">
      <div class="col">
        <div class="fld"><label class="lbl">Имя</label><input v-model="uf.name" class="inp" placeholder="напр. Мама"></div>
        <div class="fld"><label class="lbl">{{ uf.id ? 'Продлить на, дней' : 'Срок, дней' }}</label><input v-model.number="uf.days" class="inp" type="number" min="0"><p class="hint">{{ uf.id ? '0 — не менять срок' : '0 — бессрочно' }}</p></div>
        <div v-if="uf.id" class="row"><button class="btn btn-sm btn-g" type="button" @click="uf.expiresAt = 0; uf.setExp = true">Сделать бессрочным</button><span v-if="uf.setExp" class="hint">будет бессрочным после сохранения</span></div>
        <div class="fld"><label class="lbl">Заметка</label><input v-model="uf.note" class="inp"></div>
        <TgRow v-if="uf.id" v-model="uf.enabled" label="Включён" />
        <TgRow v-if="uf.id" v-model="uf.regen" label="Сменить пароль" hint="старая ссылка перестанет работать" />
      </div>
      <template #foot><button class="btn" type="button" @click="uf.open = false">Отмена</button><button class="btn btn-p" type="button" @click="saveUser">{{ uf.id ? 'Сохранить' : 'Создать' }}</button></template>
    </Drawer>
  </div>
</template>
<script setup>
import '../styles/Proxy.css'
import { ref, reactive, computed, onMounted, onBeforeUnmount } from 'vue'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import Metric from '../components/Metric.vue'
import Empty from '../components/Empty.vue'
import TgRow from '../components/TgRow.vue'
import Qr from '../components/Qr.vue'
import Drawer from '../components/Drawer.vue'
import { get, put, post, del } from '../lib/api.js'
import { ok, fail, confirm, copy } from '../lib/ui.js'
import { left, uptime } from '../lib/fmt.js'

const d = ref(null)
const s = ref(null)
const saved = ref('')
const error = ref('')
const busy = reactive({ save: false, install: false })
const uf = reactive({ open: false, id: '', name: '', days: 30, note: '', enabled: true, regen: false, expiresAt: 0, setExp: false })
const dirty = computed(() => s.value && JSON.stringify(s.value) !== saved.value)
async function load() {
  error.value = ''
  try { const r = await get('tgproxy'); d.value = r; if (!dirty.value) { s.value = { ...r.settings }; saved.value = JSON.stringify(s.value) } } catch (e) { error.value = e.message }
}
async function save() {
  busy.save = true
  try { await put('tgproxy', s.value); saved.value = ''; ok('Сохранено'); s.value = null; await load(); setTimeout(load, 2000) } catch (e) { fail(e) } finally { busy.save = false }
}
async function install() { busy.install = true; try { const r = await post('tgproxy/install'); ok('mtg установлен ' + (r.version || '')); load() } catch (e) { fail(e) } finally { busy.install = false } }
async function regen() { if (await confirm('Перевыпустить секрет?', 'Старая ссылка MTProto перестанет работать у всех.', { danger: true, okText: 'Перевыпустить' })) { try { await post('tgproxy/secret'); ok('Новый секрет'); saved.value = ''; s.value = null; load() } catch (e) { fail(e) } } }
function openNew() { Object.assign(uf, { open: true, id: '', name: '', days: 30, note: '', enabled: true, regen: false, setExp: false }) }
function openEdit(u) { Object.assign(uf, { open: true, id: u.id, name: u.name, days: 0, note: u.note, enabled: u.enabled, regen: false, expiresAt: u.expiresAt, setExp: false }) }
async function saveUser() {
  try {
    if (uf.id) {
      const b = { name: uf.name, note: uf.note, enabled: uf.enabled, regen: uf.regen, days: Number(uf.days) || 0 }
      if (uf.setExp) b.expiresAt = 0
      await put('tgproxy/users/' + uf.id, b)
    } else await post('tgproxy/users', { name: uf.name, days: Number(uf.days) || 0, note: uf.note })
    ok('Сохранено'); uf.open = false; load()
  } catch (e) { fail(e) }
}
async function remove(u) { if (await confirm(`Удалить доступ «${u.name}»?`, 'Прокси перестанет работать у этого человека.', { danger: true, okText: 'Удалить' })) { try { await del('tgproxy/users/' + u.id); load() } catch (e) { fail(e) } } }
let t
onMounted(() => { load(); t = setInterval(load, 8000) })
onBeforeUnmount(() => clearInterval(t))
</script>
