<template>
  <div class="pg-backups">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="w2-eb"><router-link to="/settings">Настройки</router-link> · клиенты, инбаунды, настройки, сертификаты</span><h1>Бэкапы</h1><p>Бэкап — zip с базой панели, сертификатами и ключами. Восстановление заменяет всё текущее; перед ним автоматически делается страховочная копия.</p></div>
        <div class="i-hd-a">
          <button class="btn" type="button" :disabled="busy.up" @click="fileIn.click()"><span v-if="busy.up" class="spin"></span><Icon v-else n="upload" />Загрузить бэкап</button>
          <button class="btn btn-p" type="button" :disabled="busy.create" @click="create"><span v-if="busy.create" class="spin"></span><Icon v-else n="plus" />Создать бэкап</button>
          <input ref="fileIn" type="file" accept=".zip,application/zip" hidden @change="upload">
        </div>
      </div>
      <Loading v-if="!data" :error="error" @retry="reload" />
      <template v-else>
        <div class="i-mt">
          <Metric label="Бэкапов" :value="data.backups.length" :note="`занимают ${bytes(data.total)}`" />
          <Metric label="Последний" :value="data.backups[0] ? ago(data.backups[0].created) : '—'" :note="data.backups[0] ? dt(data.backups[0].created) : 'ещё не создавали'" />
          <Metric label="Автобэкап" :value="data.autoBackup ? 'ежедневно' : 'выключен'" :note="data.autoBackup ? `в 03:00 МСК · храним ${data.backupKeep || 7}` : 'включите ниже'" :note-tone="data.autoBackup ? 'ok' : 'wa'" />
          <Metric label="В Telegram" :value="data.backupTg ? 'отправляем' : 'нет'" :note="data.tgReady ? 'бот уведомлений настроен' : 'настройте бота в «Безопасности»'" />
        </div>
        <div class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>Все бэкапы <span class="i-cnt">{{ data.backups.length }}</span></h2><span>новые сверху</span></div>
            <div class="tbl-w">
              <table class="tbl">
                <thead><tr><th>Файл</th><th>Тип</th><th>Размер</th><th>Создан</th><th></th></tr></thead>
                <tbody>
                  <tr v-for="b in data.backups" :key="b.name">
                    <td class="i-mono" style="font-size:12px">{{ b.name }}</td>
                    <td><span class="badge" :class="KIND[b.kind]?.[1] || 'b-mute'">{{ KIND[b.kind]?.[0] || b.kind }}</span></td>
                    <td class="nw">{{ bytes(b.size) }}</td>
                    <td class="nw">{{ dt(b.created) }}</td>
                    <td class="r"><span class="acts">
                      <button class="btn btn-sm btn-g btn-ic" type="button" title="Скачать" @click="download('backups/' + encodeURIComponent(b.name), b.name)"><Icon n="download" /></button>
                      <button class="btn btn-sm" type="button" @click="restore(b)">Восстановить</button>
                      <button class="btn btn-sm btn-g btn-ic" type="button" title="Удалить" @click="remove(b)"><Icon n="trash" /></button>
                    </span></td>
                  </tr>
                </tbody>
              </table>
              <Empty v-if="!data.backups.length" icon="bak" title="Бэкапов пока нет" text="Создайте первый — это пара секунд" />
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Автоматически</h2></div>
            <div class="i-pb">
              <TgRow v-model="st.autoBackup" bold label="Ежедневный бэкап" hint="каждый день в 03:00 по Москве" />
              <div class="fld"><label class="lbl">Хранить автобэкапов</label><input v-model.number="st.backupKeep" class="inp" type="number" min="1" max="90"><p class="hint">старые удаляются; ручные и загруженные не трогаем</p></div>
              <TgRow v-model="st.backupTg" bold label="Отправлять в Telegram" :disabled="!data.tgReady" :hint="data.tgReady ? 'файл придёт администратору в бот уведомлений' : 'сначала настройте бота в «Безопасности» → Telegram'" />
              <div class="ft"><span class="grow"></span><button class="btn btn-p" type="button" :disabled="busy.save" @click="saveSettings">Сохранить</button></div>
              <div class="i-hint"><Icon n="info" /><span>Из консоли: <code class="i-mono">kvk</code> → «Бэкап», или <code class="i-mono">kvk-panel backup</code> / <code class="i-mono">kvk-panel restore файл.zip</code>.</span></div>
            </div>
          </section>
        </div>
      </template>
    </div>
  </div>
</template>
<script setup>
import '../styles/Backups.css'
import { ref, reactive, watch } from 'vue'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import Metric from '../components/Metric.vue'
import Empty from '../components/Empty.vue'
import TgRow from '../components/TgRow.vue'
import { get, post, put, del, download } from '../lib/api.js'
import { useLoad } from '../lib/useLoad.js'
import { ok, fail, confirm } from '../lib/ui.js'
import { bytes, ago, dt } from '../lib/fmt.js'

const KIND = { manual: ['вручную', 'b-info'], auto: ['авто', 'b-ok'], upload: ['загружен', 'b-warn'], 'pre-restore': ['страховочный', 'b-mute'] }
const { data, error, reload } = useLoad(() => get('backups'), { interval: 30000 })
const st = reactive({ autoBackup: false, backupKeep: 7, backupTg: false })
const busy = reactive({ create: false, up: false, save: false })
const fileIn = ref(null)
let inited = false
watch(data, (d) => { if (d && !inited) { inited = true; Object.assign(st, { autoBackup: d.autoBackup, backupKeep: d.backupKeep || 7, backupTg: d.backupTg }) } })
async function create() { busy.create = true; try { const r = await post('backups'); ok('Бэкап создан: ' + r.name); reload(true) } catch (e) { fail(e) } finally { busy.create = false } }
async function upload(e) {
  const f = e.target.files[0]; e.target.value = ''
  if (!f) return
  busy.up = true
  try { const fd = new FormData(); fd.append('file', f); const r = await post('backups/upload', fd); ok('Загружен: ' + r.name); await reload(true); const b = data.value.backups.find(x => x.name === r.name); if (b) restore(b) } catch (e) { fail(e) } finally { busy.up = false }
}
async function restore(b) {
  if (!await confirm('Восстановить бэкап?', `${b.name} от ${dt(b.created)}. Все текущие клиенты, инбаунды и настройки будут заменены. Панель перезапустится; если в бэкапе другие порт, путь или логин — войдите с ними.`, { danger: true, okText: 'Восстановить' })) return
  try {
    const r = await post(`backups/${encodeURIComponent(b.name)}/restore`)
    ok('Восстановлено, панель перезапускается…')
    setTimeout(() => { location.href = r.url || location.href }, 3500)
  } catch (e) { fail(e) }
}
async function remove(b) { if (await confirm('Удалить бэкап?', b.name, { danger: true, okText: 'Удалить' })) { try { await del('backups/' + encodeURIComponent(b.name)); reload(true) } catch (e) { fail(e) } } }
async function saveSettings() { busy.save = true; try { await put('backups-settings', { ...st, backupKeep: Number(st.backupKeep) || 7 }); ok('Сохранено'); inited = false; reload(true) } catch (e) { fail(e) } finally { busy.save = false } }
</script>
