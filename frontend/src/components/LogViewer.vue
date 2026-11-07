<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { RefreshCw, Search } from 'lucide-vue-next'
import { api } from '../api/client'
import Pagination from './Pagination.vue'

interface LogEntry {
  id: number
  created_at: string
  level: string
  event: string
  source: string
  task_type: string
  task_id: string
  attempt: number
  reason: string
  http_status: number
  stage: string
  position_type: string
  position: number
}

const props = withDefaults(defineProps<{ taskId?: string }>(), { taskId: '' })
const filters = reactive({ level: '', source: '', task_id: props.taskId })
const entries = ref<LogEntry[]>([])
const page = ref(1)
const total = ref(0)
const busy = ref(false)
const error = ref(false)

async function load() {
  busy.value = true
  error.value = false
  const query = new URLSearchParams({ page: String(page.value) })
  for (const [key, value] of Object.entries(filters)) {
    if (value.trim()) query.set(key, value.trim())
  }
  try {
    const response = await api<LogEntry[]>(`/admin/logs?${query}`)
    entries.value = response.data || []
    total.value = response.meta?.total || 0
  } catch {
    error.value = true
  } finally {
    busy.value = false
  }
}

function search() { page.value = 1; void load() }
function changePage(value: number) { page.value = value; void load() }
function formatTime(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

onMounted(load)
watch(() => props.taskId, value => { filters.task_id = value; page.value = 1; void load() })
</script>

<template>
  <section class="log-viewer">
    <div class="admin-heading">
      <h2>{{ $t('logs.title') }}</h2>
      <button type="button" class="btn btn-ghost btn-square btn-sm" :disabled="busy" :title="$t('logs.refresh')" :aria-label="$t('logs.refresh')" @click="load"><RefreshCw :size="16" /></button>
    </div>
    <form class="log-filters" @submit.prevent="search">
      <label class="field"><span>{{ $t('logs.level') }}</span><select v-model="filters.level" name="level" class="select select-sm"><option value="">{{ $t('common.all') }}</option><option value="error">{{ $t('logs.levels.error') }}</option><option value="warn">{{ $t('logs.levels.warn') }}</option><option value="info">{{ $t('logs.levels.info') }}</option></select></label>
      <label class="field"><span>{{ $t('common.source') }}</span><select v-model="filters.source" name="source" class="select select-sm"><option value="">{{ $t('common.all') }}</option><option v-for="source in ['fanza', 'sod', 'mgstage', 'fc2', 'jable', 'avmoo', 'javlibrary', 'libredmm']" :key="source" :value="source">{{ source.toUpperCase() }}</option></select></label>
      <label class="field log-task-filter"><span>{{ $t('logs.taskID') }}</span><input v-model="filters.task_id" name="task_id" class="input input-sm" maxlength="36" :placeholder="$t('logs.taskID')"></label>
      <button class="btn btn-sm search-button" :disabled="busy"><Search :size="15" />{{ $t('common.filter') }}</button>
    </form>
    <div v-if="error" class="alert alert-error status-alert" role="alert">{{ $t('common.error') }}</div>
    <div v-if="busy && entries.length === 0" class="center-state">{{ $t('common.loading') }}</div>
    <div v-else-if="!error && entries.length === 0" class="center-state">{{ $t('common.noResults') }}</div>
    <div v-else class="log-list">
      <article v-for="entry in entries" :key="entry.id" class="log-row">
        <div class="log-time"><time :datetime="entry.created_at">{{ formatTime(entry.created_at) }}</time><span class="log-level" :class="`log-level-${entry.level}`">{{ $te(`logs.levels.${entry.level}`) ? $t(`logs.levels.${entry.level}`) : entry.level }}</span></div>
        <div class="log-detail">
          <div class="log-event">{{ $te(`logs.events.${entry.event}`) ? $t(`logs.events.${entry.event}`) : entry.event }}</div>
          <div v-if="entry.reason" class="log-reason">{{ $te(`logs.reasons.${entry.reason}`) ? $t(`logs.reasons.${entry.reason}`) : $t('logs.reasons.unknown') }}<span v-if="entry.http_status"> HTTP {{ entry.http_status }}</span></div>
          <div class="log-meta">
            <span v-if="entry.source">{{ entry.source.toUpperCase() }}</span>
            <span v-if="entry.task_type">{{ $te(`jobs.types.${entry.task_type}`) ? $t(`jobs.types.${entry.task_type}`) : entry.task_type }}</span>
            <span v-if="entry.attempt">{{ $t('logs.attempt', { number: entry.attempt }) }}</span>
            <span v-if="entry.stage && $te(`logs.stages.${entry.stage}`)">{{ $t(`logs.stages.${entry.stage}`) }}</span>
            <span v-if="entry.position_type === 'page'">{{ $t('logs.positionPage', { number: entry.position }) }}</span>
            <span v-if="entry.position_type === 'number'">{{ $t('logs.positionNumber', { number: entry.position }) }}</span>
          </div>
          <div v-if="entry.task_id" class="log-task-id">{{ entry.task_id }}</div>
        </div>
      </article>
    </div>
    <Pagination :page="page" :size="30" :total="total" @change="changePage" />
  </section>
</template>
