<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { CalendarClock, FileCode2, FileText, Globe, HardDrive, LayoutDashboard, Layers, ListTodo, Mail, Play, RefreshCw, Save, ScrollText, ShieldCheck, Sparkles, Upload, X } from 'lucide-vue-next'
import { api, jsonBody } from '../api/client'
import Pagination from '../components/Pagination.vue'
import CrawlManager from '../components/CrawlManager.vue'
import LogViewer from '../components/LogViewer.vue'
import Dashboard from '../components/Dashboard.vue'
import SourceHealth, { type HealthState } from '../components/SourceHealth.vue'
import { useAuthStore } from '../stores/auth'
import type { GorseSettings, SourceItem } from '../types'

interface SourceView { fanza_configured: boolean; proxy_configured: boolean; fc2_url: string; sod_url: string; mgstage_url: string; mgstage_series_url: string; jable_url: string; libredmm_url: string; javdb_url: string; javbus_url: string; javbooks_url: string; javquery_url: string; jable_cookie_configured: boolean; javdb_cookie_configured: boolean; javbus_cookie_configured: boolean; javbooks_cookie_configured: boolean; javquery_cookie_configured: boolean; health: HealthState[] }
interface RedisView { enabled: boolean; address: string; username: string; password_configured: boolean; database: number; tls: boolean }
interface SMTP { enabled: boolean; host: string; port: number; username: string; password: string; password_configured?: boolean; from: string; base_url: string; tls_mode: string }
interface Page { id: number; source: string; url: string; raw_html?: string; created_at: string }
interface Job { id: string; type: string; status: string; attempt: number; created_at: string; metadata_movie_id?: string; metadata_failures?: { source: string; reason: string; http_status: number }[] }

const route = useRoute()
const auth = useAuthStore()
const section = computed(() => String(route.params.section || 'sources'))
const sourceInfo = ref<SourceView | null>(null)
const sourceForm = reactive({ fanza_api_id: '', fanza_affiliate_id: '', proxy_url: '', clear_proxy: false, fc2_url: '', sod_url: '', mgstage_url: '', mgstage_series_url: '', jable_url: '', libredmm_url: '', javdb_url: '', javbus_url: '', javbooks_url: '', javquery_url: '', jable_cookie: '', javdb_cookie: '', javbus_cookie: '', javbooks_cookie: '', javquery_cookie: '', clear_jable_cookie: false, clear_javdb_cookie: false, clear_javbus_cookie: false, clear_javbooks_cookie: false, clear_javquery_cookie: false })
const sourceAddresses = [
  ['jable_url', 'Jable URL', 'https://jable.tv/'], ['libredmm_url', 'LibreFanza URL', 'https://www.libredmm.com/'],
  ['javdb_url', 'JavDB URL', 'https://javdb.com/'],
  ['javbooks_url', 'JavBooks URL', 'https://javbooks.com/'], ['javquery_url', 'JavQuery URL', 'https://javquery.cc/'],
] as const
const sourceSessions = [
  ['Jable', 'jable_cookie', 'clear_jable_cookie', 'jable_cookie_configured'], ['JavDB', 'javdb_cookie', 'clear_javdb_cookie', 'javdb_cookie_configured'],
  ['JavBooks', 'javbooks_cookie', 'clear_javbooks_cookie', 'javbooks_cookie_configured'],
  ['JavQuery', 'javquery_cookie', 'clear_javquery_cookie', 'javquery_cookie_configured'],
] as const
const redis = reactive({ enabled: false, address: '', username: '', password: '', clear_password: false, database: 0, tls: false })
const smtp = reactive<SMTP>({ enabled: false, host: '', port: 587, username: '', password: '', from: '', base_url: '', tls_mode: 'starttls' })
const gorse = reactive({ enabled: false, entry_point: '', api_key: '', min_feedback_count: 5 })
const redisHasPassword = ref(false)
const gorseHasKey = ref(false)
const gorseManaged = ref(false)
const gorseConnected = ref(false)
const gorseSyncError = ref(false)
const gorseSync = ref<GorseSettings['sync']>()
const filter = ref('')
const items = ref<SourceItem[]>([])
const pages = ref<Page[]>([])
const jobs = ref<Job[]>([])
const total = ref(0)
const page = ref(1)
const selectedPage = ref<Page | null>(null)
const importing = ref(false)
const importForm = reactive({ source: 'sod', url: '', raw_html: '' })
const busy = ref(false)
const error = ref(false)
const success = ref(false)

async function load() {
  if (!auth.user?.is_admin) return
  error.value = false
  success.value = false
  busy.value = true
  try {
    if (section.value === 'sources') {
      sourceInfo.value = (await api<SourceView>('/admin/settings/sources')).data
      sourceForm.fc2_url = sourceInfo.value.fc2_url || ''
      sourceForm.sod_url = sourceInfo.value.sod_url || ''
      sourceForm.mgstage_url = sourceInfo.value.mgstage_url || ''
      sourceForm.mgstage_series_url = sourceInfo.value.mgstage_series_url || ''
      sourceForm.jable_url = sourceInfo.value.jable_url || ''
      sourceForm.libredmm_url = sourceInfo.value.libredmm_url || ''
      sourceForm.javdb_url = sourceInfo.value.javdb_url || ''
      sourceForm.javbus_url = sourceInfo.value.javbus_url || ''
      sourceForm.javbooks_url = sourceInfo.value.javbooks_url || ''
      sourceForm.javquery_url = sourceInfo.value.javquery_url || ''
    } else if (section.value === 'cache') {
      const value = (await api<RedisView>('/admin/settings/redis')).data
      Object.assign(redis, { enabled: value.enabled, address: value.address, username: value.username, database: value.database, tls: value.tls, password: '', clear_password: false })
      redisHasPassword.value = value.password_configured
    } else if (section.value === 'smtp') {
      const value = (await api<SMTP>('/admin/settings/smtp')).data
      Object.assign(smtp, value, { password: '', port: value.port || 587, tls_mode: value.tls_mode || 'starttls' })
    } else if (section.value === 'gorse') {
      const value = (await api<GorseSettings>('/admin/settings/gorse')).data
      Object.assign(gorse, { enabled: value.enabled, entry_point: value.entry_point || '', min_feedback_count: value.min_feedback_count || 5, api_key: '' })
      gorseHasKey.value = value.api_key_configured
      gorseManaged.value = !!value.managed
      gorseConnected.value = !!value.connected
      gorseSyncError.value = !!value.sync_error
      gorseSync.value = value.sync
    } else if (section.value === 'items') {
      const response = await api<SourceItem[]>(`/admin/source-items?source=${encodeURIComponent(filter.value)}&page=${page.value}`)
      items.value = response.data || []; total.value = response.meta?.total || 0
    } else if (section.value === 'pages') {
      const response = await api<Page[]>(`/admin/source-pages?source=${encodeURIComponent(filter.value)}&page=${page.value}`)
      pages.value = response.data || []; total.value = response.meta?.total || 0
    } else if (section.value === 'tasks') {
      const parameters = new URLSearchParams({ page: String(page.value) })
      if (route.query.kind === 'metadata') parameters.set('kind', 'metadata')
      if (route.query.status === 'failed') parameters.set('status', 'failed')
      const response = await api<Job[]>(`/admin/jobs?${parameters}`)
      jobs.value = response.data || []; total.value = response.meta?.total || 0
    }
  } catch { error.value = true } finally { busy.value = false }
}

async function saveSources() {
  busy.value = true; error.value = false; success.value = false
  try { await api('/admin/settings/sources', { method: 'PUT', body: jsonBody(sourceForm) }); sourceForm.fanza_api_id = ''; sourceForm.fanza_affiliate_id = ''; sourceForm.proxy_url = ''; sourceForm.clear_proxy = false; for (const [, cookie, clear] of sourceSessions) { sourceForm[cookie] = ''; sourceForm[clear] = false }; await load(); success.value = true } catch { error.value = true } finally { busy.value = false }
}
async function saveRedis() {
  busy.value = true; error.value = false; success.value = false
  try { await api('/admin/settings/redis', { method: 'PUT', body: jsonBody(redis) }); await load(); success.value = true } catch { error.value = true } finally { busy.value = false }
}
async function testRedis() {
  busy.value = true; error.value = false; success.value = false
  try { await api('/admin/settings/redis-checks', { method: 'POST', body: jsonBody(redis) }); success.value = true } catch { error.value = true } finally { busy.value = false }
}
async function saveSettings(kind: 'smtp') {
  busy.value = true; error.value = false; success.value = false
  try { await api(`/admin/settings/${kind}`, { method: 'PUT', body: jsonBody(smtp) }); await load(); success.value = true } catch { error.value = true } finally { busy.value = false }
}
async function saveGorse() {
  busy.value = true; error.value = false; success.value = false
  try { await api('/admin/settings/gorse', { method: 'PUT', body: jsonBody(gorse) }); await load(); success.value = true } catch { error.value = true } finally { busy.value = false }
}
async function testGorse() {
  busy.value = true; error.value = false; success.value = false
  try { await api('/admin/settings/gorse-checks', { method: 'POST', body: jsonBody({ entry_point: gorse.entry_point, api_key: gorse.api_key }) }); success.value = true } catch { error.value = true } finally { busy.value = false }
}
async function updatePriority(item: SourceItem, delta: number) {
  try { await api(`/admin/source-items/${item.id}`, { method: 'PATCH', body: jsonBody({ priority: item.priority + delta }) }); await load() } catch { error.value = true }
}
async function rederive(item: SourceItem) {
  try { await api(`/admin/source-items/${item.id}/rederivations`, { method: 'POST' }); await load() } catch { error.value = true }
}
async function retryJob(job: Job) {
  try { await api(`/admin/jobs/${job.id}/retries`, { method: 'POST' }); await load() } catch { error.value = true }
}
async function runTask(type: string) {
  busy.value = true; error.value = false; success.value = false
  try { await api('/admin/maintenance-jobs', { method: 'POST', body: jsonBody({ type }) }); await load(); success.value = true } catch { error.value = true } finally { busy.value = false }
}
async function viewPage(id: number) {
  try { selectedPage.value = (await api<Page>(`/admin/source-pages/${id}`)).data } catch { error.value = true }
}
async function importPage() {
  busy.value = true; error.value = false
  try { await api('/admin/source-pages', { method: 'POST', body: jsonBody(importForm) }); importing.value = false; importForm.url = ''; importForm.raw_html = ''; await load(); success.value = true } catch { error.value = true } finally { busy.value = false }
}
function changePage(value: number) { page.value = value; void load() }
watch([section, () => route.query.kind, () => route.query.status], () => { page.value = 1; filter.value = ''; void load() }, { immediate: true })
</script>

<template>
  <div class="content-wrap admin-wrap">
    <div class="page-title-row">
      <div>
        <h1>{{ $t('admin.title') }}</h1>
      </div>
    </div>
    <div v-if="!auth.user?.is_admin" class="center-state">{{ $t('common.error') }}</div>
    <div v-else class="admin-layout">
      <aside class="admin-sidebar">
        <RouterLink to="/admin/dashboard"><LayoutDashboard :size="16" class="inline-block mr-2 opacity-70" />{{ $t('dashboard.title') }}</RouterLink>
        <RouterLink to="/admin/sources"><Globe :size="16" class="inline-block mr-2 opacity-70" />{{ $t('admin.sources') }}</RouterLink>
        <RouterLink to="/admin/cache"><HardDrive :size="16" class="inline-block mr-2 opacity-70" />{{ $t('admin.cache') }}</RouterLink>
        <RouterLink to="/admin/schedule"><CalendarClock :size="16" class="inline-block mr-2 opacity-70" />{{ $t('admin.schedule') }}</RouterLink>
        <RouterLink to="/admin/smtp"><Mail :size="16" class="inline-block mr-2 opacity-70" />{{ $t('admin.smtp') }}</RouterLink>
        <RouterLink to="/admin/gorse"><Sparkles :size="16" class="inline-block mr-2 opacity-70" />{{ $t('admin.gorse') }}</RouterLink>
        <RouterLink to="/admin/items"><Layers :size="16" class="inline-block mr-2 opacity-70" />{{ $t('admin.items') }}</RouterLink>
        <RouterLink to="/admin/pages"><FileCode2 :size="16" class="inline-block mr-2 opacity-70" />{{ $t('admin.pages') }}</RouterLink>
        <RouterLink to="/admin/tasks"><ListTodo :size="16" class="inline-block mr-2 opacity-70" />{{ $t('admin.tasks') }}</RouterLink>
        <RouterLink to="/admin/logs"><ScrollText :size="16" class="inline-block mr-2 opacity-70" />{{ $t('logs.title') }}</RouterLink>
      </aside>
      <div class="admin-content">
        <div v-if="error" class="alert alert-error status-alert" role="alert">{{ $t('common.error') }}</div>
        <div v-if="success" class="alert alert-success status-alert" role="status">{{ $t('common.success') }}</div>

        <Dashboard v-if="section === 'dashboard'" />
        <form v-else-if="section === 'sources'" class="admin-form" @submit.prevent="saveSources">
          <h2>{{ $t('admin.sources') }}</h2>
          <label class="field"><span>{{ $t('admin.apiID') }}</span><input v-model="sourceForm.fanza_api_id" class="input" type="password" :placeholder="sourceInfo?.fanza_configured ? $t('admin.configured') : $t('admin.notConfigured')"></label>
          <label class="field"><span>{{ $t('admin.affiliateID') }}</span><input v-model="sourceForm.fanza_affiliate_id" class="input" type="password" :placeholder="sourceInfo?.fanza_configured ? $t('admin.configured') : $t('admin.notConfigured')"></label>
          <label class="field"><span>{{ $t('admin.proxy') }}</span><input v-model="sourceForm.proxy_url" class="input" :disabled="sourceForm.clear_proxy" :placeholder="sourceInfo?.proxy_configured ? $t('admin.configured') : $t('admin.notConfigured')"></label>
          <label v-if="sourceInfo?.proxy_configured" class="check-row"><input v-model="sourceForm.clear_proxy" class="checkbox checkbox-sm checkbox-success" type="checkbox">{{ $t('admin.clearProxy') }}</label>
          <label class="field"><span>FC2 URL</span><input v-model="sourceForm.fc2_url" class="input" type="url"></label>
          <label class="field"><span>SOD URL</span><input v-model="sourceForm.sod_url" class="input" type="url"></label>
          <label class="field"><span>MGStage URL</span><input v-model="sourceForm.mgstage_url" class="input" type="url"></label>
          <label class="field"><span>{{ $t('admin.seriesURL') }}</span><input v-model="sourceForm.mgstage_series_url" class="input" type="url"></label>
          <label v-for="entry in sourceAddresses" :key="entry[0]" class="field"><span>{{ entry[1] }}</span><input v-model="sourceForm[entry[0]]" class="input" type="url" :placeholder="entry[2]"></label>
          <template v-for="entry in sourceSessions" :key="entry[0]">
            <label class="field"><span>{{ entry[0] }} Cookie</span><input v-model="sourceForm[entry[1]]" class="input" type="password" autocomplete="off" :disabled="sourceForm[entry[2]]" :placeholder="sourceInfo?.[entry[3]] ? $t('admin.configured') : $t('admin.notConfigured')"></label>
            <label v-if="sourceInfo?.[entry[3]]" class="check-row"><input v-model="sourceForm[entry[2]]" class="checkbox checkbox-sm checkbox-success" type="checkbox">{{ $t('sourceSession.clear', { source: entry[0] }) }}</label>
          </template>
          <button class="btn search-button" :disabled="busy"><Save :size="16" />{{ $t('common.save') }}</button>
          <SourceHealth v-if="sourceInfo" :health="sourceInfo.health || []" />
        </form>

        <form v-else-if="section === 'cache'" class="admin-form" @submit.prevent="saveRedis">
          <h2>{{ $t('admin.cache') }}</h2>
          <label class="toggle-row"><span>{{ $t('setup.redisOptional') }}</span><input v-model="redis.enabled" type="checkbox" class="toggle toggle-success"></label>
          <label class="field"><span>{{ $t('admin.redisAddress') }}</span><input v-model="redis.address" class="input"></label>
          <div class="field-grid">
            <label class="field"><span>{{ $t('admin.username') }}</span><input v-model="redis.username" class="input"></label>
            <label class="field"><span>{{ $t('setup.password') }}</span><input v-model="redis.password" class="input" type="password" :disabled="redis.clear_password" :placeholder="redisHasPassword ? $t('admin.configured') : ''"></label>
          </div>
          <label v-if="redisHasPassword" class="check-row"><input v-model="redis.clear_password" class="checkbox checkbox-sm checkbox-success" type="checkbox">{{ $t('admin.clearPassword') }}</label>
          <label class="field"><span>{{ $t('admin.redisDatabase') }}</span><input v-model.number="redis.database" class="input" type="number" min="0"></label>
          <label class="check-row"><input v-model="redis.tls" type="checkbox" class="checkbox checkbox-sm checkbox-success">{{ $t('setup.tls') }}</label>
          <div class="form-actions">
            <button type="button" class="btn btn-outline" :disabled="busy" @click="testRedis">{{ $t('setup.test') }}</button>
            <button class="btn search-button" :disabled="busy"><Save :size="16" />{{ $t('common.save') }}</button>
          </div>
        </form>

        <CrawlManager v-else-if="section === 'schedule'" />
        <LogViewer v-else-if="section === 'logs'" :task-id="typeof route.query.task_id === 'string' ? route.query.task_id : ''" />

        <form v-else-if="section === 'smtp'" class="admin-form" @submit.prevent="saveSettings('smtp')">
          <h2>{{ $t('admin.smtp') }}</h2>
          <label class="toggle-row"><span>{{ $t('common.enabled') }}</span><input v-model="smtp.enabled" class="toggle toggle-success" type="checkbox"></label>
          <div class="field-grid">
            <label class="field"><span>{{ $t('setup.host') }}</span><input v-model="smtp.host" class="input" :required="smtp.enabled"></label>
            <label class="field"><span>{{ $t('setup.port') }}</span><input v-model.number="smtp.port" class="input" type="number" min="1" max="65535"></label>
          </div>
          <div class="field-grid">
            <label class="field"><span>{{ $t('admin.username') }}</span><input v-model="smtp.username" class="input"></label>
            <label class="field"><span>{{ $t('setup.password') }}</span><input v-model="smtp.password" class="input" type="password" :placeholder="smtp.password_configured ? $t('admin.configured') : ''"></label>
          </div>
          <label class="field"><span>{{ $t('admin.sender') }}</span><input v-model="smtp.from" class="input" type="email" :required="smtp.enabled"></label>
          <label class="field"><span>{{ $t('admin.baseURL') }}</span><input v-model="smtp.base_url" class="input" type="url" :required="smtp.enabled"></label>
          <label class="field"><span>{{ $t('admin.tlsMode') }}</span><select v-model="smtp.tls_mode" class="select"><option value="starttls">STARTTLS</option><option value="tls">TLS</option></select></label>
          <button class="btn search-button" :disabled="busy"><Save :size="16" />{{ $t('common.save') }}</button>
        </form>

        <form v-else-if="section === 'gorse'" class="admin-form" @submit.prevent="saveGorse">
          <h2>{{ $t('admin.gorse') }}</h2>
          <div v-if="gorseManaged" class="log-meta">
            <span>{{ $t('managedGorse.internal') }}</span>
            <span>{{ gorseConnected ? $t('managedGorse.connected') : $t('managedGorse.disconnected') }}</span>
            <span v-if="gorseSyncError">{{ $t('managedGorse.error') }}</span>
            <span>{{ gorseSync?.ready ? $t('managedGorse.ready') : $t('managedGorse.syncing') }}</span>
            <span v-if="gorseSync?.phase">{{ $t(`managedGorse.${gorseSync.phase}`) }}: {{ gorseSync.cursor }}</span>
            <button type="button" class="btn btn-ghost btn-square btn-sm" :title="$t('common.retry')" @click="load"><RefreshCw :size="16" /></button>
          </div>
          <label class="toggle-row"><span>{{ $t('admin.gorseEnabled') }}</span><input v-model="gorse.enabled" class="toggle toggle-success" type="checkbox"></label>
          <label v-if="!gorseManaged" class="field"><span>{{ $t('admin.gorseEntryPoint') }}</span><input v-model="gorse.entry_point" class="input" placeholder="http://127.0.0.1:8088"></label>
          <label v-if="!gorseManaged" class="field"><span>{{ $t('admin.gorseAPIKey') }}</span><input v-model="gorse.api_key" class="input" type="password" :placeholder="gorseHasKey ? $t('admin.configured') : ''"></label>
          <label class="field"><span>{{ $t('admin.gorseMinCount') }}</span><input v-model.number="gorse.min_feedback_count" class="input" type="number" min="1" max="100"></label>
          <div class="field-actions">
            <button v-if="!gorseManaged" type="button" class="btn btn-outline" :disabled="busy || !gorse.entry_point" @click="testGorse">{{ $t('admin.gorseTest') }}</button>
            <button class="btn search-button" :disabled="busy"><Save :size="16" />{{ $t('common.save') }}</button>
          </div>
        </form>

        <template v-else-if="section === 'tasks'">
          <div class="admin-heading">
            <h2>{{ $t('admin.tasks') }}</h2>
            <button class="btn btn-ghost btn-square btn-sm" :title="$t('common.retry')" @click="load"><RefreshCw :size="16" /></button>
          </div>
          <div class="maintenance-actions">
            <RouterLink v-if="route.query.kind" to="/admin/tasks" class="btn btn-outline btn-sm">{{ $t('common.all') }}</RouterLink>
            <button v-for="entry in ([['actress_crawl', 'admin.actresses'], ['fanza_crawl', 'admin.fanza'], ['mgstage_crawl', 'admin.mgstage'], ['rebuild_movies', 'admin.rebuild']] as const)" :key="entry[0]" class="btn btn-outline btn-sm shadow-xs" :disabled="busy" @click="runTask(entry[0])"><Play :size="14" />{{ $t(entry[1]) }}</button>
          </div>
          <div class="table-wrap">
            <table class="table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>{{ $t('common.type') }}</th>
                  <th>{{ $t('common.status') }}</th>
                  <th>{{ $t('common.created') }}</th>
                  <th>{{ $t('common.actions') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="job in jobs" :key="job.id">
                  <td class="font-mono text-xs" data-label="ID">{{ job.id.slice(0, 8) }}</td>
                  <td :data-label="$t('common.type')">
                    <span class="font-medium text-base-content">{{ $te(`jobs.types.${job.type}`) ? $t(`jobs.types.${job.type}`) : job.type }}</span>
                    <div v-for="failure in job.metadata_failures || []" :key="failure.source" class="table-subtitle text-error font-medium">
                      {{ failure.source.toUpperCase() }}: {{ $te(`logs.reasons.${failure.reason}`) ? $t(`logs.reasons.${failure.reason}`) : $t('logs.reasons.unknown') }}<template v-if="failure.http_status"> HTTP {{ failure.http_status }}</template>
                    </div>
                  </td>
                  <td :data-label="$t('common.status')">
                    <span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold" :class="job.status === 'completed' ? 'bg-success/15 text-success border border-success/30' : job.status === 'failed' ? 'bg-error/15 text-error border border-error/30' : 'bg-warning/15 text-warning border border-warning/30'">
                      {{ $te(`jobs.statuses.${job.status}`) ? $t(`jobs.statuses.${job.status}`) : job.status }}
                    </span>
                  </td>
                  <td class="font-mono text-xs text-base-content/70" :data-label="$t('common.created')">{{ job.created_at.slice(0, 19).replace('T', ' ') }}</td>
                  <td :data-label="$t('common.actions')">
                    <div class="table-actions">
                      <RouterLink class="btn btn-ghost btn-square btn-xs" :to="{ path: '/admin/logs', query: { task_id: job.id } }" :title="$t('logActions.viewJob')" :aria-label="$t('logActions.viewJob')"><FileText :size="15" /></RouterLink>
                      <button v-if="job.status === 'failed'" class="btn btn-ghost btn-square btn-xs text-warning" :title="$t('common.retry')" @click="retryJob(job)"><RefreshCw :size="15" /></button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="!busy && total === 0" class="center-state">{{ $t('common.noResults') }}</div>
          <Pagination :page="page" :size="30" :total="total" @change="changePage" />
        </template>

        <template v-else-if="section === 'items' || section === 'pages'">
          <div class="admin-heading">
            <h2>{{ $t(section === 'items' ? 'admin.items' : 'admin.pages') }}</h2>
            <button v-if="section === 'pages'" class="btn search-button btn-sm" @click="importing = true"><Upload :size="16" />{{ $t('admin.import') }}</button>
          </div>
          <div class="admin-toolbar">
            <select v-model="filter" class="select select-sm" :aria-label="$t('admin.sourceFilter')" @change="page = 1; load()">
              <option value="">{{ $t('common.all') }}</option>
              <option v-for="source in ['fanza', 'sod', 'mgstage', 'javlibrary', 'fc2', 'jable', 'avmoo', 'libredmm', 'javdb', 'javbus', 'javbooks', 'javquery']" :key="source" :value="source">{{ source.toUpperCase() }}</option>
            </select>
          </div>
          <div class="table-wrap">
            <table class="table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>{{ $t('common.source') }}</th>
                  <th>{{ section === 'items' ? $t('movies.title') : $t('admin.url') }}</th>
                  <th>{{ $t('common.actions') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in section === 'items' ? items : []" :key="`item-${item.id}`">
                  <td class="font-mono text-xs" data-label="ID">{{ item.id }}</td>
                  <td :data-label="$t('common.source')"><span class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-bold bg-base-200 text-base-content/85 tracking-wider font-mono">{{ item.source.toUpperCase() }}</span></td>
                  <td :data-label="$t('movies.title')">
                    <RouterLink :to="`/movies/${item.normalized_id}`" class="text-link font-mono font-bold">{{ item.normalized_id }}</RouterLink>
                    <div class="table-subtitle">{{ item.title }}</div>
                  </td>
                  <td :data-label="$t('common.actions')">
                    <div class="table-actions">
                      <template v-if="item.source === 'fanza'">
                        <button class="btn btn-ghost btn-xs font-bold" title="-1" @click="updatePriority(item, -1)">−</button>
                        <span class="font-mono text-xs font-semibold px-1">{{ item.priority }}</span>
                        <button class="btn btn-ghost btn-xs font-bold" title="+1" @click="updatePriority(item, 1)">+</button>
                      </template>
                      <button class="btn btn-ghost btn-square btn-xs" :title="$t('admin.rederive')" @click="rederive(item)"><RefreshCw :size="15" /></button>
                    </div>
                  </td>
                </tr>
                <tr v-for="entry in section === 'pages' ? pages : []" :key="`page-${entry.id}`">
                  <td class="font-mono text-xs" data-label="ID">{{ entry.id }}</td>
                  <td :data-label="$t('common.source')"><span class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-bold bg-base-200 text-base-content/85 tracking-wider font-mono">{{ entry.source.toUpperCase() }}</span></td>
                  <td class="break-text font-mono text-xs text-base-content/75" :data-label="$t('admin.url')">{{ entry.url }}</td>
                  <td :data-label="$t('common.actions')">
                    <button class="btn btn-ghost btn-square btn-xs text-base-content/75 hover:text-primary" :title="$t('common.view')" @click="viewPage(entry.id)"><FileCode2 :size="16" /></button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-if="!busy && total === 0" class="center-state">{{ $t('common.noResults') }}</div>
          <Pagination :page="page" :size="30" :total="total" @change="changePage" />
        </template>
      </div>
    </div>

    <div v-if="importing || selectedPage" class="modal modal-open" role="dialog" aria-modal="true">
      <div class="modal-box source-modal">
        <div class="admin-heading">
          <h2>{{ selectedPage ? $t('admin.pages') : $t('admin.import') }}</h2>
          <button class="btn btn-ghost btn-square btn-sm" :aria-label="$t('common.close')" @click="importing = false; selectedPage = null"><X :size="18" /></button>
        </div>
        <pre v-if="selectedPage" class="raw-html">{{ selectedPage.raw_html }}</pre>
        <form v-else class="admin-form" @submit.prevent="importPage">
          <label class="field"><span>{{ $t('common.source') }}</span><select v-model="importForm.source" class="select"><option v-for="source in ['sod', 'mgstage', 'fc2', 'javdb', 'javbus', 'javbooks', 'javquery']" :key="source" :value="source">{{ source.toUpperCase() }}</option></select></label>
          <label class="field"><span>{{ $t('admin.url') }}</span><input v-model="importForm.url" class="input" type="url" required></label>
          <label class="field"><span>{{ $t('admin.rawHTML') }}</span><textarea v-model="importForm.raw_html" class="textarea raw-input" required></textarea></label>
          <button class="btn search-button" :disabled="busy"><Upload :size="16" />{{ $t('admin.import') }}</button>
        </form>
      </div>
      <button class="modal-backdrop" :aria-label="$t('common.close')" @click="importing = false; selectedPage = null"></button>
    </div>
  </div>
</template>
