<script setup lang="ts">
import { computed } from 'vue'

export interface HealthState {
  source: string
  status: string
  failures: number
  reason: string
  http_status: number
  last_checked: string | null
  next_retry: string | null
}

const props = defineProps<{ health: HealthState[] }>()
const sourceNames = ['fanza', 'sod', 'mgstage', 'fc2', 'jable', 'libredmm', 'javdb', 'javbooks', 'javquery']
const states = computed(() => new Map(props.health.map(value => [value.source, value])))

function formatTime(value: string | null | undefined) {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString()
}
</script>

<template>
  <section class="source-health" :aria-label="$t('sourceHealth.title')">
    <h3>{{ $t('sourceHealth.title') }}</h3>
    <div class="source-health-table-wrap">
      <table class="source-health-table">
        <thead><tr><th>{{ $t('common.source') }}</th><th>{{ $t('sourceHealth.status') }}</th><th>{{ $t('sourceHealth.reason') }}</th><th>{{ $t('sourceHealth.lastChecked') }}</th><th>{{ $t('sourceHealth.nextRetry') }}</th></tr></thead>
        <tbody>
          <tr v-for="name in sourceNames" :key="name" :data-source="name">
            <td>{{ name.toUpperCase() }}</td>
            <td>{{ $t(`sourceHealth.states.${states.get(name)?.status || 'unknown'}`) }}</td>
            <td><template v-if="states.get(name)?.reason">{{ $t(`logs.reasons.${states.get(name)?.reason}`) }}<span v-if="states.get(name)?.http_status"> (HTTP {{ states.get(name)?.http_status }})</span><span v-if="states.get(name)?.failures"> · {{ states.get(name)?.failures }}</span></template><template v-else>-</template></td>
            <td>{{ formatTime(states.get(name)?.last_checked) }}</td>
            <td>{{ formatTime(states.get(name)?.next_retry) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>

<style scoped>
.source-health { margin-top: 1.5rem; }
.source-health h3 { margin-bottom: .75rem; font-size: 1rem; font-weight: 600; }
.source-health-table-wrap { overflow-x: auto; }
.source-health-table { width: 100%; border-collapse: collapse; font-size: .875rem; text-align: left; }
.source-health-table th, .source-health-table td { padding: .5rem; border-bottom: 1px solid #e5e7eb; vertical-align: top; }
.source-health-table th { color: #475569; font-weight: 600; white-space: nowrap; }
</style>
