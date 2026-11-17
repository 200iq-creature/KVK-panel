<template>
  <div class="pg-filter">
    <div class="pagein i-wrap">
      <div class="i-hd">
        <div><span class="w2-eb">для всех клиентов сервера</span><h1>Фильтрация</h1><p>Блокировка рекламы, трекеров, вредоносных и взрослых сайтов прямо на сервере — работает в любом приложении без настроек у клиента.</p></div>
        <div class="i-hd-a">
          <button v-if="f" class="btn" type="button" :disabled="busy.check" @click="check"><span v-if="busy.check" class="spin"></span><Icon v-else n="check" />Проверить</button>
          <button v-if="f" class="btn btn-p" type="button" :disabled="busy.save || !dirty" @click="save"><span v-if="busy.save" class="spin"></span>Сохранить</button>
        </div>
      </div>
      <Loading v-if="!f" :error="error" @retry="load" />
      <template v-else>
        <div class="i-mt">
          <Metric label="Фильтрация" :value="f.enabled ? 'включена' : 'выключена'" :note="f.enabled ? MODE[f.mode] : 'весь трафик без фильтра'" :note-tone="f.enabled ? 'ok' : ''" />
          <Metric label="Категорий" :value="[f.ads, f.security, f.adult].filter(Boolean).length" :note="[f.ads && 'реклама', f.security && 'угрозы', f.adult && '18+'].filter(Boolean).join(', ') || 'не выбраны'" />
          <Metric label="Свой чёрный список" :value="block.split('\n').filter(x => x.trim()).length" note="доменов и IP" />
          <Metric label="Белый список" :value="allow.split('\n').filter(x => x.trim()).length" note="никогда не блокируются" />
        </div>
        <div class="i-c2">
          <section class="i-pn">
            <div class="i-ph"><h2>Что блокировать</h2><span class="i-stt" :class="f.enabled ? 'ok' : 'mu'">{{ f.enabled ? 'включено' : 'выключено' }}</span></div>
            <div class="i-pb">
              <TgRow v-model="f.enabled" bold label="Включить фильтрацию" hint="применяется сразу ко всем подключениям" />
              <TgRow v-model="f.ads" bold label="Реклама и трекеры" hint="баннеры, счётчики, рекламные сети" />
              <TgRow v-model="f.security" bold label="Вредоносные и фишинговые сайты" hint="вирусы, фишинг, криптомайнеры" />
              <TgRow v-model="f.adult" bold label="Сайты для взрослых" hint="18+" />
              <div class="fld"><label class="lbl">Способ</label>
                <Opt v-model="f.mode" :options="[{ v: 'lists', l: 'Списки geosite', s: 'Xray блокирует по базе доменов — быстро, без внешних сервисов' }, { v: 'dns', l: 'DNS-фильтр AdGuard', s: 'большие актуальные списки, запросы идут к AdGuard DNS' }, { v: 'custom', l: 'Свой DoH', s: 'NextDNS, Control D, свой AdGuard Home…' }]" />
              </div>
              <div v-if="f.mode === 'custom'" class="fld"><label class="lbl">Адрес DoH</label><input v-model="f.customDoh" class="inp mono" placeholder="https://dns.nextdns.io/abc123"></div>
              <div v-if="f.mode === 'lists' && missing.length" class="i-hint wa"><Icon n="warn" /><span>В текущей базе geosite нет: {{ missing.join(', ') }}. Смените источник geo-файлов на Loyalsoldier в <router-link to="/settings">«Настройках»</router-link>.</span></div>
              <p v-if="f.mode !== 'lists'" class="hint">DNS сервера: <span class="i-mono">{{ doh }}</span></p>
            </div>
          </section>
          <section class="i-pn">
            <div class="i-ph"><h2>Свои списки</h2></div>
            <div class="i-pb">
              <div class="fld"><label class="lbl">Блокировать всегда</label><textarea v-model="block" class="inp mono" rows="6" placeholder="tiktok.com&#10;1.2.3.4&#10;10.0.0.0/8"></textarea><p class="hint">домен (с поддоменами), IP или подсеть — по одному в строке. Работает при любом способе.</p></div>
              <div class="fld"><label class="lbl">Никогда не блокировать</label><textarea v-model="allow" class="inp mono" rows="4" placeholder="ads.example-partner.com"></textarea><p class="hint">исключения из категорий выше</p></div>
            </div>
          </section>
        </div>
        <section v-if="checks" class="i-pn">
          <div class="i-ph"><h2>Проверка</h2><span>{{ checks.filter(c => c.ok).length }} из {{ checks.length }} в порядке</span></div>
          <div class="i-pb">
            <div class="list-rows">
              <div v-for="(c, i) in checks" :key="i" class="lr"><span class="lr-t"><b>{{ c.name }}</b><small>{{ c.detail }}</small></span><span class="i-stt" :class="c.ok ? 'ok' : 'bad'">{{ c.ok ? 'ок' : 'проблема' }}</span></div>
            </div>
            <p class="hint">Проверка не сохраняет настройки — она проверяет то, что сейчас на экране.</p>
          </div>
        </section>
      </template>
    </div>
  </div>
</template>
<script setup>
import '../styles/Filter.css'
import { ref, reactive, computed, onMounted } from 'vue'
import Icon from '../components/Icon.vue'
import Loading from '../components/Loading.vue'
import Metric from '../components/Metric.vue'
import TgRow from '../components/TgRow.vue'
import Opt from '../components/Opt.vue'
import { get, put, post } from '../lib/api.js'
import { ok, fail } from '../lib/ui.js'

const MODE = { lists: 'по спискам geosite', dns: 'через DNS AdGuard', custom: 'через свой DoH' }
const f = ref(null)
const block = ref('')
const allow = ref('')
const lists = ref({})
const doh = ref('')
const saved = ref('')
const error = ref('')
const checks = ref(null)
const busy = reactive({ save: false, check: false })
const body = () => ({ ...f.value, block: block.value.split('\n').map(x => x.trim()).filter(Boolean), allow: allow.value.split('\n').map(x => x.trim()).filter(Boolean) })
const dirty = computed(() => f.value && JSON.stringify(body()) !== saved.value)
const missing = computed(() => [f.value.ads && !lists.value['category-ads-all'] && 'category-ads-all', f.value.adult && !lists.value['category-porn'] && 'category-porn', f.value.security && !lists.value.malware && 'malware'].filter(Boolean))
async function load() {
  error.value = ''
  try {
    const r = await get('filter')
    f.value = { ...r.filter, mode: r.filter.mode || 'lists' }; block.value = r.filter.block.join('\n'); allow.value = r.filter.allow.join('\n'); lists.value = r.lists; doh.value = r.doh
    saved.value = JSON.stringify(body())
  } catch (e) { error.value = e.message }
}
async function save() { busy.save = true; try { await put('filter', body()); ok('Фильтрация применена'); await load() } catch (e) { fail(e) } finally { busy.save = false } }
async function check() { busy.check = true; try { const r = await post('filter/check', body()); checks.value = r.checks } catch (e) { fail(e) } finally { busy.check = false } }
onMounted(load)
</script>
