<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { api } from '../api/client'

interface Summary { movies: number; source_items: number; errors_24h: number; queues: { queue: string; status: string; count: number }[] }
const summary = ref<Summary | null>(null)
const busy = ref(false)
const error = ref(false)
async function load() {
  busy.value = true
  error.value = false
  try { summary.value = (await api<Summary>('/admin/dashboard')).data } catch { error.value = true } finally { busy.value = false }
}
onMounted(load)
</script>

<template>
  <section>
    <div class="admin-heading"><h2>{{ $t('dashboard.title') }}</h2><button class="btn btn-ghost btn-square btn-sm" :disabled="busy" :title="$t('logs.refresh')" :aria-label="$t('logs.refresh')" @click="load"><RefreshCw :size="16" /></button></div>
    <p v-if="error" class="error-text" role="alert">{{ $t('common.error') }}</p>
    <div v-if="summary" class="dashboard-summary">
      <div class="dashboard-counts">
        <div><span>{{ $t('dashboard.movies') }}</span><strong>{{ summary.movies.toLocaleString() }}</strong></div>
        <div><span>{{ $t('dashboard.sources') }}</span><strong>{{ summary.source_items.toLocaleString() }}</strong></div>
        <RouterLink to="/admin/logs"><span>{{ $t('dashboard.errors') }}</span><strong>{{ summary.errors_24h.toLocaleString() }}</strong></RouterLink>
      </div>
      <h3>{{ $t('dashboard.queues') }}</h3>
      <table class="table"><thead><tr><th>{{ $t('dashboard.queue') }}</th><th>{{ $t('dashboard.status') }}</th><th>{{ $t('dashboard.count') }}</th></tr></thead><tbody><tr v-for="row in summary.queues || []" :key="`${row.queue}:${row.status}`"><td>{{ row.queue }}</td><td>{{ $te(`jobs.statuses.${row.status}`) ? $t(`jobs.statuses.${row.status}`) : row.status }}</td><td>{{ row.count }}</td></tr></tbody></table>
      <p v-if="!summary.queues?.length">{{ $t('common.noResults') }}</p>
      <RouterLink class="btn btn-outline btn-sm mt-4" to="/admin/tasks">{{ $t('admin.tasks') }}</RouterLink>
    </div>
  </section>
</template>

<style scoped>
.dashboard-counts { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 1rem; border-bottom: 1px solid #d6dce0; padding: 1rem 0; margin-bottom: 1.5rem; }
.dashboard-counts > * { display: flex; flex-direction: column; gap: .35rem; }
.dashboard-counts span { color: #53616a; font-size: .85rem; }
.dashboard-counts strong { font-size: 1.5rem; }
</style>
