<template>
  <div class="pg-overview">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="dz-eb">панель · {{ d?.node || meta.node }} · <span class="i-mono">{{ d?.host || meta.host }}</span></span><h1>Обзор</h1></div>
        <div class="i-hd-a row"><router-link v-if="!meta.inbounds" class="btn" to="/site"><Icon n="bolt" />Сайт и VLESS</router-link><router-link class="btn btn-p" to="/clients?new=1"><Icon n="plus" />Создать клиента</router-link></div>
      </div>
      <Loading v-if="!d" :error="error" @retry="reload" />
      <template v-else>
        <div class="i-mt dz-mt5">
          <div class="i-pn i-m">
            <span>Клиенты</span><b>{{ d.clients.total }}</b>
            <div class="i-mpr ok" :title="`${d.clients.active} из ${d.clients.total} активны`"><i :style="{ width: pct(d.clients.active, d.clients.total) + '%' }"></i></div>
            <span class="dz-nt">{{ pn(d.clients.active, 'активный', 'активных', 'активных') }}</span>
          </div>
          <div class="i-pn i-m"><span>Онлайн сейчас</span><b>{{ d.clients.online }}</b><span class="dz-nt">{{ pn(d.clients.devices, 'устройство', 'устройства', 'устройств') }}</span></div>
          <div class="i-pn i-m"><span>Истекают за 3&nbsp;дня</span><b>{{ d.clients.soon }}</b><router-link v-if="d.clients.soon" class="dz-nt dz-lk" to="/clients?f=soon">показать</router-link></div>
          <div class="i-pn i-m"><span>Истекли · забанены</span><b>{{ d.clients.expired }} · {{ d.clients.banned }}</b><span class="dz-nt">{{ d.autoClean ? `автоочистка через ${d.autoClean} дн.` : 'автоочистка выключена' }}</span></div>
          <div class="i-pn i-m">
            <span>Скорость сейчас, Мбит/с</span>
            <b class="dz-rt"><span class="rx">↓ {{ fmtNum(mbit(d.sys.rxSpeed), 1) }}</span> <span class="tx">↑ {{ fmtNum(mbit(d.sys.txSpeed), 1) }}</span></b>
            <span class="dz-nt">{{ d.sys.iface || 'сеть' }} · всего ↓ {{ bytes(d.sys.rxTotal) }} ↑ {{ bytes(d.sys.txTotal) }}</span>
          </div>
        </div>
        <div class="i-g2">
          <div class="dz-col">
            <section class="i-pn" aria-label="Трафик клиентов">
              <div class="i-ph"><h2>Трафик клиентов</h2><Seg v-model="period" :options="[[14, '14\u00a0дней'], [30, '30\u00a0дней']]" /></div>
              <div class="i-pb">
                <span class="dz-sm">всего {{ bytes(total) }} за {{ period }} {{ plural(period, 'день', 'дня', 'дней') }} · в среднем {{ bytes(total / period) }} в день</span>
                <div class="kchart">
                  <div class="cols" role="img" aria-label="Трафик клиентов по дням">
                    <div v-for="x in days" :key="x.day" :class="{ best: x.bytes === maxDay && x.bytes > 0 }" :title="`${dayLabel(x.day)}: ${bytes(x.bytes)}`"><i :style="{ height: (maxDay ? Math.max(1, x.bytes / maxDay * 100) : 1) + '%' }"></i></div>
                  </div>
                  <div class="x"><span>{{ dayLabel(days[0].day) }}</span><span>{{ dayLabel(days[Math.floor(days.length / 2)].day) }}</span><span>сегодня</span></div>
                </div>
              </div>
            </section>
            <section class="i-pn" aria-label="Связка серверов">
              <div class="i-ph"><h2>Связка серверов</h2><router-link class="btn btn-sm" to="/link">Управлять</router-link></div>
              <div class="i-pb">
                <div class="i-tblw">
                  <table class="tbl dz-tbl">
                    <thead><tr><th>Сервер</th><th class="dz-r">Клиентов</th><th>CPU</th><th>Статус</th></tr></thead>
                    <tbody>
                      <tr>
                        <td><span class="dz-node"><span v-if="d.country" class="dz-cc">{{ d.country }}</span><b>{{ d.node }}</b><span class="dz-sm">{{ meta.main ? 'подключённый' : 'этот сервер' }}</span></span></td>
                        <td class="dz-r"><b>{{ d.localClients }}</b></td>
                        <td><span class="dz-bar"><span class="i-mpr" :class="d.sys.cpu > 85 ? 'bad' : d.sys.cpu > 60 ? 'wa' : ''"><i :style="{ width: Math.round(d.sys.cpu) + '%' }"></i></span><b>{{ Math.round(d.sys.cpu) }}%</b></span></td>
                        <td><span class="i-stt" :class="d.xray.running ? 'ok' : 'bad'">{{ d.xray.running ? 'в сети' : 'Xray остановлен' }}</span></td>
                      </tr>
                      <tr v-for="n in d.nodes" :key="n.name">
                        <td><span class="dz-node"><span v-if="n.country" class="dz-cc">{{ n.country }}</span><b>{{ n.name }}</b><span class="dz-sm">в связке</span></span></td>
                        <td class="dz-r"><b>{{ n.clients }}</b></td>
                        <td><span class="dz-bar"><span class="i-mpr"><i :style="{ width: Math.round(n.cpu || 0) + '%' }"></i></span><b>{{ Math.round(n.cpu || 0) }}%</b></span></td>
                        <td><span class="i-stt" :class="n.ok ? 'ok' : 'bad'">{{ n.status }}</span></td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            </section>
            <section class="i-pn" aria-label="Сервер">
              <div class="i-ph"><h2>Сервер</h2><span>{{ d.sys.os }} · {{ d.sys.arch }} · аптайм {{ uptime(d.sys.uptime) }}</span></div>
              <div class="i-pb">
                <div class="i-meter"><div><span class="dz-mn">CPU · {{ d.sys.cores }} {{ plural(d.sys.cores, 'ядро', 'ядра', 'ядер') }} · load {{ d.sys.load }}</span><b>{{ Math.round(d.sys.cpu) }}%</b></div><div class="i-mpr" :class="tone(d.sys.cpu)"><i :style="{ width: d.sys.cpu + '%' }"></i></div></div>
                <div class="i-meter"><div><span class="dz-mn">Память · {{ bytes(d.sys.memUsed) }} из {{ bytes(d.sys.memTotal) }}</span><b>{{ pct(d.sys.memUsed, d.sys.memTotal) }}%</b></div><div class="i-mpr" :class="tone(pct(d.sys.memUsed, d.sys.memTotal))"><i :style="{ width: pct(d.sys.memUsed, d.sys.memTotal) + '%' }"></i></div></div>
                <div class="i-meter"><div><span class="dz-mn">Диск · {{ bytes(d.sys.diskUsed) }} из {{ bytes(d.sys.diskTotal) }}</span><b>{{ pct(d.sys.diskUsed, d.sys.diskTotal) }}%</b></div><div class="i-mpr" :class="tone(pct(d.sys.diskUsed, d.sys.diskTotal))"><i :style="{ width: pct(d.sys.diskUsed, d.sys.diskTotal) + '%' }"></i></div></div>
                <div class="i-kv">
                  <div class="i-kr"><span>Xray</span><b><span class="i-stt" :class="d.xray.running ? 'ok' : 'bad'">{{ d.xray.running ? `работает · v${d.xray.version} · ${uptime(d.xray.uptime)}` : (d.xray.error || 'остановлен') }}</span></b><span class="row"><button class="btn btn-sm btn-g" type="button" title="Сменить версию, обновить или откатить" @click="openXray"><Icon n="cpu" />Версия</button><button class="btn btn-sm" type="button" :disabled="busy.x" @click="xray('restart')"><Icon n="refresh" />Перезапустить</button><button v-if="d.xray.running" class="btn btn-sm btn-g" type="button" :disabled="busy.x" @click="xray('stop')">Остановить</button><button v-else class="btn btn-sm btn-p" type="button" :disabled="busy.x" @click="xray('start')">Запустить</button></span></div>
                </div>
              </div>
            </section>
          </div>
          <div class="dz-col">
            <section class="i-pn" aria-label="Приложения клиентов">
              <div class="i-ph"><h2>Приложения клиентов</h2></div>
              <div class="i-pb dz-apps">
                <div v-for="a in apps" :key="a.name" class="i-meter"><div><span class="dz-mn">{{ a.name }}</span><b>{{ a.pct }}%</b></div><div class="i-mpr"><i :style="{ width: a.pct + '%' }"></i></div></div>
                <span v-if="!apps.length" class="dz-sm">Клиенты ещё не обновляли подписку.</span>
                <span v-else class="dz-sm">по последнему обновлению подписки</span>
              </div>
            </section>
            <section class="i-pn" aria-label="События">
              <div class="i-ph"><h2>События</h2><router-link class="btn btn-sm btn-g" to="/logs">Логи<Icon n="chevR" /></router-link></div>
              <div class="dz-feed">
                <div v-for="(e, i) in d.events.slice(0, 14)" :key="i" class="dz-fi">
                  <i :style="{ background: e.kind === 'bad' ? 'var(--badDot)' : e.kind === 'warn' ? 'var(--warnDot)' : e.kind === 'ok' ? 'var(--okDot)' : 'var(--accent)' }"></i>
                  <div><p>{{ e.text }}</p><small>{{ dt(e.t) }}</small></div>
                </div>
                <div v-if="!d.events.length" class="dz-fi"><div><p class="muted">Событий пока нет</p></div></div>
              </div>
            </section>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
<script setup>
import { openXray } from '../lib/xrayui.js'
import '../styles/Overview.css'
import { ref, reactive, computed } from 'vue'
import Icon from '../components/Icon.vue'
import Seg from '../components/Seg.vue'
import Loading from '../components/Loading.vue'
import { get, post } from '../lib/api.js'
import { meta, loadMeta } from '../lib/meta.js'
import { useLoad } from '../lib/useLoad.js'
import { bytes, fmtNum, mbit, pn, plural, dt, dayLabel, uptime } from '../lib/fmt.js'
import { ok, fail } from '../lib/ui.js'

const { data: d, error, reload } = useLoad(() => get('overview'), { interval: 5000 })
const period = ref(14)
const busy = reactive({ x: false })
const days = computed(() => (d.value?.daily || []).slice(-period.value))
const total = computed(() => days.value.reduce((s, x) => s + x.bytes, 0))
const maxDay = computed(() => Math.max(0, ...days.value.map(x => x.bytes)))
const apps = computed(() => {
  const a = d.value?.apps || {}
  const sum = Object.values(a).reduce((s, x) => s + x, 0)
  return Object.entries(a).sort((x, y) => y[1] - x[1]).map(([name, n]) => ({ name, pct: Math.round(n / sum * 100) }))
})
const pct = (a, b) => b ? Math.round(a / b * 100) : 0
const tone = (p) => p > 90 ? 'bad' : p > 70 ? 'wa' : ''
async function xray(action) {
  busy.x = true
  try { await post('xray/' + action); ok(action === 'stop' ? 'Xray остановлен' : 'Xray запущен'); await reload(true); loadMeta() } catch (e) { fail(e) } finally { busy.x = false }
}
</script>
