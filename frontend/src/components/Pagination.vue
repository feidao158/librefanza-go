<script setup lang="ts">
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'

defineProps<{ page: number; size: number; total: number }>()
defineEmits<{ change: [page: number] }>()
</script>

<template>
  <div v-if="total > 0" class="pagination-row">
    <span class="text-xs font-medium text-slate-500 bg-slate-100 px-3 py-1 rounded-lg border border-slate-200/60">{{ $t('common.total', { count: total }) }}</span>
    <div class="pagination-controls">
      <button class="btn btn-ghost btn-sm btn-square" :disabled="page <= 1" :aria-label="$t('common.previous')" :title="$t('common.previous')" @click="$emit('change', page - 1)"><ChevronLeft :size="17" /></button>
      <span class="px-3 py-1 bg-white border border-slate-200 rounded-lg shadow-xs text-xs font-semibold text-slate-700">{{ $t('common.page', { page }) }}</span>
      <button class="btn btn-ghost btn-sm btn-square" :disabled="page * size >= total" :aria-label="$t('common.next')" :title="$t('common.next')" @click="$emit('change', page + 1)"><ChevronRight :size="17" /></button>
    </div>
  </div>
</template>
