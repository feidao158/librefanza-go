<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { CalendarClock, FileText, Play, RefreshCw, Save, X } from 'lucide-vue-next'
import { api, ApiError, jsonBody } from '../api/client'
import { decodeSchedule, encodeSchedule, type ScheduleEditor } from './crawlSchedule'

type Source = 'actress' | 'fanza' | 'mgstage' | 'jable' | 'avmoo' | 'javlibrary' | 'libredmm'
interface Schedule { enabled: boolean; cron: string }
interface Settings { timezone: string; actress: Schedule; fanza: Schedule; mgstage: Schedule; jable: Schedule; avmoo: Schedule; javlibrary: Schedule; libredmm: Schedule }
interface Job { id: string; type: string; status: string; created_at: string; error_code?: string; crawl?: { source: string; mode: string }; progress?: { next_page: number; next_number: number; processed: number; undated: number } }

const entries: { key: Source; label: string; modern: boolean }[] = [
  { key: 'actress', label: 'admin.actresses', modern: false }, { key: 'fanza', label: 'admin.fanza', modern: false },
  { key: 'mgstage', label: 'admin.mgstage', modern: false }, { key: 'jable', label: 'Jable', modern: true },
  { key: 'avmoo', label: 'AVMOO', modern: true }, { key: 'javlibrary', label: 'JavLibrary', modern: true }, { key: 'libredmm', label: 'LibreFanza', modern: true },
]
const settings = ref<Settings | null>(null)
const editors = reactive<Partial<Record<Source, ScheduleEditor>>>({})
const jobs = ref<Job[]>([])
const backfill = ref<{ progress: { phase: string; processed: number; failed: number; completed: boolean }; failed_jobs: number } | null>(null)
const nextRuns = ref<Partial<Record<Source, string>>>({})
const busy = ref(false)
const error = ref('')
const success = ref(false)
const selected = ref<Source | null>(null)
const form = reactive({ mode: 'incremental', start_page: 1, end_page: 3, start_date: '', end_date: '', prefix: '', start_number: 1, end_number: 10, number_width: 3 })
let timer: ReturnType<typeof setInterval> | undefined
const hasActive = computed(() => jobs.value.some(job => ['pending', 'running', 'retrying', 'waiting'].includes(job.status)))

async function loadJobs(passive = false) {
  const headers = passive ? { 'X-Session-Activity': 'passive' } : undefined
  jobs.value = (await api<Job[]>('/admin/jobs?kind=collection&page=1&page_size=100', { headers })).data || []
  backfill.value = (await api<NonNullable<typeof backfill.value>>('/admin/metadata-backfill', { headers })).data
}
async function load() {
  busy.value = true; error.value = ''
  try {
    settings.value = (await api<Settings>('/admin/settings/schedules')).data
    nextRuns.value = (await api<Partial<Record<Source, string>>>('/admin/schedule-previews')).data
    for (const entry of entries) editors[entry.key] = decodeSchedule(settings.value[entry.key]?.cron || '0 0 * * *')
    await loadJobs()
  } catch { error.value = 'common.error' } finally { busy.value = false }
}
function latest(source: Source): Job | undefined {
  return jobs.value.find(job => job.crawl?.source === source || job.type === `${source}_crawl`)
}
function formatRun(value: string): string {
  try { return new Intl.DateTimeFormat('sv-SE', { timeZone: settings.value?.timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(new Date(value)) }
  catch { return value.slice(0, 16).replace('T', ' ') }
}
function label(value: string): string { return value.startsWith('admin.') ? value : value }
async function save() {
  if (!settings.value) return
  busy.value = true; error.value = ''; success.value = false
  try {
    for (const entry of entries) {
      const editor = editors[entry.key]
      if (editor && editor.mode !== 'custom') settings.value[entry.key].cron = encodeSchedule(editor)
    }
    await api('/admin/settings/schedules', { method: 'PUT', body: jsonBody(settings.value) })
    nextRuns.value = (await api<Partial<Record<Source, string>>>('/admin/schedule-previews')).data
    success.value = true
  } catch { error.value = 'common.error' } finally { busy.value = false }
}
async function run(source: Source, modern: boolean) {
  if (modern) { selected.value = source; form.mode = 'incremental'; return }
  busy.value = true; error.value = ''; success.value = false
  try {
    await api('/admin/maintenance-jobs', { method: 'POST', body: jsonBody({ type: `${source}_crawl` }) })
    await loadJobs(); success.value = true
  } catch { error.value = 'common.error' } finally { busy.value = false }
}
async function submit() {
  if (!selected.value) return
  busy.value = true; error.value = ''; success.value = false
  try {
    await api('/admin/crawl-jobs', { method: 'POST', body: jsonBody({ source: selected.value, ...form }) })
    selected.value = null; await loadJobs(); success.value = true
  } catch (e) { error.value = e instanceof ApiError && e.code === 'crawl_in_progress' ? 'crawl.inProgress' : 'common.error' } finally { busy.value = false }
}
async function retry(job: Job) {
  busy.value = true; error.value = ''
  try { await api(`/admin/jobs/${encodeURIComponent(job.id)}/retries`, { method: 'POST' }); await loadJobs() } catch { error.value = 'common.error' } finally { busy.value = false }
}
onMounted(() => { void load(); timer = setInterval(() => { if ((hasActive.value || !backfill.value?.progress.completed) && !busy.value) void loadJobs(true).catch(() => { error.value = 'common.error' }) }, 5000) })
onBeforeUnmount(() => { if (timer) clearInterval(timer) })
</script>

<template>
  <section class="crawl-manager">
    <div class="admin-heading"><div><h2>{{ $t('crawl.title') }}</h2><p class="crawl-subtitle">{{ $t('crawl.subtitle') }}</p></div><button class="btn btn-ghost btn-square btn-sm" :title="$t('common.retry')" :disabled="busy" @click="load"><RefreshCw :size="16" /></button></div>
    <div v-if="error" class="alert alert-error status-alert" role="alert">{{ $t(error) }}</div>
    <div v-if="success" class="alert alert-success status-alert" role="status">{{ $t('common.success') }}</div>
    <template v-if="settings">
      <div class="crawl-toolbar"><label class="field"><span>{{ $t('admin.timezone') }}</span><input v-model="settings.timezone" class="input input-sm" required></label><button class="btn search-button btn-sm" :disabled="busy" @click="save"><Save :size="15" />{{ $t('common.save') }}</button></div>
      <div v-if="backfill" class="crawl-progress"><strong>{{ $t('metadata.backfill') }}</strong> · {{ $t(backfill.progress.completed ? 'metadata.done' : backfill.progress.phase === 'raw' ? 'metadata.raw' : 'metadata.movies') }} · {{ $t('metadata.processed') }} {{ backfill.progress.processed }} · {{ $t('metadata.failedJobs') }} {{ backfill.failed_jobs + backfill.progress.failed }} <RouterLink v-if="backfill.failed_jobs" :to="{ path: '/admin/tasks', query: { kind: 'metadata', status: 'failed' } }">{{ $t('common.retry') }}</RouterLink></div>
      <div class="crawl-list">
        <article v-for="entry in entries" :key="entry.key" class="crawl-row">
          <div class="crawl-row-heading"><div><h3>{{ entry.label.startsWith('admin.') ? $t(label(entry.label)) : entry.label }}</h3><div class="crawl-meta"><span :class="settings[entry.key].enabled ? 'crawl-on' : 'crawl-off'">{{ $t(settings[entry.key].enabled ? 'common.enabled' : 'crawl.disabled') }}</span><template v-if="latest(entry.key)"><span>{{ $t(`jobs.statuses.${latest(entry.key)?.status}`) }}</span><time>{{ formatRun(latest(entry.key)!.created_at) }}</time><RouterLink :to="{ path: '/admin/logs', query: { task_id: latest(entry.key)!.id } }" :title="$t('logActions.viewJob')" :aria-label="$t('logActions.viewJob')"><FileText :size="15" /></RouterLink></template><span v-else>{{ $t('crawl.never') }}</span></div></div><div class="crawl-actions"><label class="crawl-toggle"><span class="sr-only">{{ $t('common.enabled') }}</span><input v-model="settings[entry.key].enabled" class="toggle toggle-success toggle-sm" type="checkbox"></label><button class="btn btn-outline btn-sm" type="button" :disabled="busy" @click="run(entry.key, entry.modern)"><Play :size="14" />{{ $t('admin.run') }}</button></div></div>
          <div v-if="editors[entry.key]" class="crawl-controls">
            <CalendarClock :size="16" />
            <select v-model="editors[entry.key]!.mode" class="select select-sm" :aria-label="$t('crawl.frequency')">
              <option value="interval">{{ $t('intervalSchedule.mode') }}</option><option value="hourly">{{ $t('crawl.hourly') }}</option><option value="daily">{{ $t('crawl.daily') }}</option><option value="weekly">{{ $t('crawl.weekly') }}</option><option value="custom">{{ $t('crawl.custom') }}</option>
            </select>
            <select v-if="editors[entry.key]!.mode === 'interval'" :value="editors[entry.key]!.interval || 30" class="select select-sm" :aria-label="$t('crawl.frequency')" @change="editors[entry.key]!.interval = Number(($event.target as HTMLSelectElement).value)"><option v-for="interval in [5, 10, 15, 20, 30]" :key="interval" :value="interval">{{ $t('intervalSchedule.every', { count: interval }) }}</option></select>
            <template v-else-if="editors[entry.key]!.mode !== 'custom'"><select v-if="editors[entry.key]!.mode === 'weekly'" v-model.number="editors[entry.key]!.weekday" class="select select-sm" :aria-label="$t('crawl.weekday')"><option v-for="day in 7" :key="day" :value="day - 1">{{ $t(`crawl.days.${day - 1}`) }}</option></select><input v-if="editors[entry.key]!.mode !== 'hourly'" v-model.number="editors[entry.key]!.hour" class="input input-sm crawl-number" type="number" min="0" max="23" :aria-label="$t('crawl.hour')"><span v-if="editors[entry.key]!.mode !== 'hourly'">:</span><input v-model.number="editors[entry.key]!.minute" class="input input-sm crawl-number" type="number" min="0" max="59" :aria-label="$t('crawl.minute')"></template>
            <input v-else v-model="settings[entry.key].cron" class="input input-sm crawl-cron" :aria-label="$t('admin.expression')">
          </div>
          <div v-if="nextRuns[entry.key]" class="crawl-progress">{{ $t('schedulePreview.next') }} {{ nextRuns[entry.key]?.slice(0, 16).replace('T', ' ') }} <span>{{ $t('schedulePreview.saved') }}</span></div>
          <div v-if="latest(entry.key)?.progress" class="crawl-progress">{{ $t('crawl.processed') }} {{ latest(entry.key)?.progress?.processed }} <span v-if="latest(entry.key)?.progress?.next_page">· {{ $t('crawl.nextPage') }} {{ latest(entry.key)?.progress?.next_page }}</span><span v-if="latest(entry.key)?.progress?.undated"> · {{ $t('crawl.undated') }} {{ latest(entry.key)?.progress?.undated }}</span></div>
          <div v-if="latest(entry.key)?.error_code" class="crawl-error">{{ $t(`crawl.errors.${latest(entry.key)?.error_code}`) }} <button v-if="latest(entry.key)?.status === 'failed'" class="btn btn-ghost btn-xs" @click="retry(latest(entry.key)!)">{{ $t('common.retry') }}</button></div>
        </article>
      </div>
    </template>
    <div v-if="selected" class="modal modal-open" role="dialog" aria-modal="true"><div class="modal-box source-modal"><div class="admin-heading"><h2>{{ $t('crawl.runTitle') }} · {{ selected.toUpperCase() }}</h2><button class="btn btn-ghost btn-square btn-sm" :aria-label="$t('common.close')" @click="selected = null"><X :size="18" /></button></div><form class="admin-form" @submit.prevent="submit"><label class="field"><span>{{ $t('crawl.range') }}</span><select v-model="form.mode" class="select"><option value="incremental">{{ $t('crawl.incremental') }}</option><option value="full">{{ $t('crawl.full') }}</option><option value="pages">{{ $t('crawl.pages') }}</option><option v-if="selected !== 'jable'" value="dates">{{ $t('crawl.dates') }}</option><option value="ids">{{ $t('crawl.ids') }}</option></select></label><div v-if="form.mode === 'pages'" class="field-grid"><label class="field"><span>{{ $t('crawl.startPage') }}</span><input v-model.number="form.start_page" class="input" type="number" min="1" required></label><label class="field"><span>{{ $t('crawl.endPage') }}</span><input v-model.number="form.end_page" class="input" type="number" :min="form.start_page" required></label></div><div v-if="form.mode === 'dates'" class="field-grid"><label class="field"><span>{{ $t('crawl.startDate') }}</span><input v-model="form.start_date" class="input" type="date" required></label><label class="field"><span>{{ $t('crawl.endDate') }}</span><input v-model="form.end_date" class="input" type="date" :min="form.start_date" required></label></div><template v-if="form.mode === 'ids'"><label class="field"><span>{{ $t('crawl.prefix') }}</span><input v-model="form.prefix" class="input" pattern="[A-Za-z0-9]{1,24}" required></label><div class="field-grid"><label class="field"><span>{{ $t('crawl.startNumber') }}</span><input v-model.number="form.start_number" class="input" type="number" min="0" required></label><label class="field"><span>{{ $t('crawl.endNumber') }}</span><input v-model.number="form.end_number" class="input" type="number" :min="form.start_number" required></label></div><label class="field"><span>{{ $t('crawl.width') }}</span><input v-model.number="form.number_width" class="input" type="number" min="0" max="10"></label></template><button class="btn search-button" :disabled="busy"><Play :size="15" />{{ $t('admin.run') }}</button></form></div><button class="modal-backdrop" :aria-label="$t('common.close')" @click="selected = null"></button></div>
  </section>
</template>
