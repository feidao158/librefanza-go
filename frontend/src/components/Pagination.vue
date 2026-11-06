<script setup lang="ts">
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'

defineProps<{ page: number; size: number; total: number }>()
defineEmits<{ change: [page: number] }>()
</script>

<template>
  <div v-if="total > 0" class="pagination-row">
    <span>{{ $t('common.total', { count: total }) }}</span>
    <div class="pagination-controls">
      <button class="btn btn-ghost btn-sm btn-square" :disabled="page <= 1" :aria-label="$t('common.previous')" :title="$t('common.previous')" @click="$emit('change', page - 1)"><ChevronLeft :size="17" /></button>
      <span>{{ $t('common.page', { page }) }}</span>
      <button class="btn btn-ghost btn-sm btn-square" :disabled="page * size >= total" :aria-label="$t('common.next')" :title="$t('common.next')" @click="$emit('change', page + 1)"><ChevronRight :size="17" /></button>
    </div>
  </div>
</template>
