<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowUpDown, Search, UserRound } from 'lucide-vue-next'
import { api } from '../api/client'
import Pagination from '../components/Pagination.vue'
import type { Actress } from '../types'

const route = useRoute()
const router = useRouter()
const actresses = ref<Actress[]>([])
const query = ref('')
const order = ref('new')
const page = ref(1)
const total = ref(0)
const loading = ref(false)
const error = ref(false)
const broken = ref<Record<number, boolean>>({})

async function load() {
  query.value = String(route.query.q || '')
  order.value = String(route.query.order || 'new')
  page.value = Math.max(1, Number(route.query.page) || 1)
  loading.value = true
  error.value = false
  try {
    const params = new URLSearchParams({ q: query.value, order: order.value, page: String(page.value), page_size: '45' })
    const response = await api<Actress[]>(`/actresses?${params}`)
    actresses.value = response.data || []
    total.value = response.meta?.total || 0
  } catch { error.value = true } finally { loading.value = false }
}
function submit() { router.push({ path: '/actresses', query: { q: query.value || undefined, order: order.value } }) }
function changePage(value: number) { router.push({ path: '/actresses', query: { ...route.query, page: value } }) }
watch(() => route.fullPath, load, { immediate: true })
</script>

<template>
  <div class="content-wrap catalog-page">
    <div class="page-title-row">
      <div>
        <h1>{{ $t('actresses.title') }}</h1>
        <p>{{ $t('actresses.subtitle') }}</p>
      </div>
      <span class="count-label">{{ $t('common.total', { count: total.toLocaleString() }) }}</span>
    </div>
    <form class="filter-bar" @submit.prevent="submit">
      <label class="search-control">
        <Search :size="18" />
        <input v-model="query" class="input" :placeholder="$t('actresses.search')">
      </label>
      <label class="filter-select">
        <ArrowUpDown :size="15" />
        <select v-model="order" class="select order-select" :aria-label="$t('movies.order')">
          <option value="new">{{ $t('actresses.newest') }}</option>
          <option value="name">{{ $t('actresses.name') }}</option>
        </select>
      </label>
      <button class="btn search-button" type="submit">
        <Search :size="16" />
        {{ $t('common.search') }}
      </button>
    </form>
    <div class="section-heading">
      <div>
        <h2>{{ $t('actresses.title') }}</h2>
      </div>
      <span class="result-count">{{ $t('common.total', { count: total }) }}</span>
    </div>
    <div v-if="loading" class="center-state">
      <span class="loading loading-spinner loading-md text-primary"></span>
      <span class="text-xs text-base-content/65 font-medium">{{ $t('common.loading') }}</span>
    </div>
    <div v-else-if="error" class="center-state">
      <p class="error-text font-medium">{{ $t('common.error') }}</p>
      <button class="btn btn-outline btn-sm" @click="load">{{ $t('common.retry') }}</button>
    </div>
    <div v-else-if="actresses.length" class="actress-grid">
      <RouterLink v-for="actress in actresses" :key="actress.id" class="actress-card" :to="`/actresses/${actress.id}`">
        <div class="actress-photo">
          <img v-if="actress.image_url && !broken[actress.id]" :src="actress.image_url" :alt="actress.name" loading="lazy" referrerpolicy="no-referrer" @error="broken[actress.id] = true">
          <UserRound v-else :size="32" />
        </div>
        <strong>{{ actress.name }}</strong>
        <span v-if="actress.fanza_id">#{{ actress.fanza_id }}</span>
      </RouterLink>
    </div>
    <div v-else class="center-state">
      <UserRound :size="32" class="text-base-content/55" />
      <p class="font-medium text-base-content/75">{{ $t('common.noResults') }}</p>
    </div>
    <Pagination :page="page" :size="45" :total="total" @change="changePage" />
  </div>
</template>
