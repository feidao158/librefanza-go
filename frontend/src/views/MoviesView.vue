<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowUpDown, Film, Search, SlidersHorizontal } from 'lucide-vue-next'
import { api } from '../api/client'
import MovieCard from '../components/MovieCard.vue'
import Pagination from '../components/Pagination.vue'
import type { Movie } from '../types'

const route = useRoute()
const router = useRouter()
const movies = ref<Movie[]>([])
const total = ref(0)
const page = ref(1)
const size = 30
const query = ref('')
const match = ref('fuzzy')
const order = ref('release_date')
const loading = ref(false)
const error = ref(false)

async function load() {
  query.value = String(route.query.q || '')
  match.value = String(route.query.match || 'fuzzy')
  order.value = String(route.query.order || 'release_date')
  page.value = Math.max(1, Number(route.query.page) || 1)
  loading.value = true
  error.value = false
  try {
    const params = new URLSearchParams({ q: query.value, match: match.value, order: order.value, page: String(page.value), page_size: String(size) })
    const response = await api<Movie[]>(`/movies?${params}`)
    movies.value = response.data || []
    total.value = response.meta?.total || 0
  } catch { error.value = true } finally { loading.value = false }
}

async function submit() {
  if (query.value.trim() && match.value !== 'prefix') {
    try {
      const result = await api<{ valid: boolean; normalized: string }>(`/product-ids/normalize?q=${encodeURIComponent(query.value.trim())}`)
      if (result.data.valid) { await router.push(`/movies/${encodeURIComponent(result.data.normalized)}`); return }
    } catch { error.value = true; return }
  }
  router.push({ path: '/movies', query: { q: query.value || undefined, match: match.value, order: order.value } })
}

function changePage(value: number) {
  router.push({ path: '/movies', query: { ...route.query, page: value } })
}

watch(() => route.fullPath, load, { immediate: true })
</script>

<template>
  <div class="content-wrap">
    <div class="page-title-row">
      <div>
        <div class="section-kicker"><Film :size="12" /> LIBREFANZA / COLLECTION</div>
        <h1>{{ $t('movies.title') }}</h1>
        <p>{{ $t('movies.subtitle') }}</p>
      </div>
      <span class="count-label">{{ total.toLocaleString() }} <span>INDEXED</span></span>
    </div>
    <form class="filter-bar" @submit.prevent="submit">
      <label class="search-control">
        <Search :size="18" />
        <input v-model="query" class="input" :placeholder="$t('movies.searchPlaceholder')" autocomplete="off">
      </label>
      <label class="filter-select">
        <SlidersHorizontal :size="15" />
        <select v-model="match" class="select" :aria-label="$t('common.filter')">
          <option value="fuzzy">{{ $t('movies.contains') }}</option>
          <option value="prefix">{{ $t('movies.prefix') }}</option>
        </select>
      </label>
      <label class="filter-select">
        <ArrowUpDown :size="15" />
        <select v-model="order" class="select order-select" :aria-label="$t('movies.order')">
          <option value="release_date">{{ $t('movies.byDate') }}</option>
          <option value="title">{{ $t('movies.byID') }}</option>
          <option value="date_added">{{ $t('movies.byAdded') }}</option>
        </select>
      </label>
      <button class="btn search-button" type="submit">
        <Search :size="16" />
        {{ $t('common.search') }}
      </button>
    </form>
    <div class="section-heading">
      <div>
        <span class="section-number">01</span>
        <h2>{{ query ? $t('common.search') : $t('movies.latest') }}</h2>
        <span v-if="query" class="query-chip">{{ query }}</span>
      </div>
      <span class="result-count">{{ $t('common.total', { count: total }) }}</span>
    </div>
    <div v-if="loading" class="center-state">
      <span class="loading loading-spinner loading-md text-emerald-600"></span>
      <span class="text-xs text-slate-400 font-medium">{{ $t('common.loading') }}</span>
    </div>
    <div v-else-if="error" class="center-state">
      <p class="error-text font-medium">{{ $t('common.error') }}</p>
      <button class="btn btn-outline btn-sm" @click="load">{{ $t('common.retry') }}</button>
    </div>
    <div v-else-if="movies.length" class="movie-grid">
      <MovieCard v-for="movie in movies" :key="movie.id" :movie="movie" />
    </div>
    <div v-else class="center-state">
      <Search :size="32" class="text-slate-300" />
      <p class="font-medium text-slate-600">{{ $t('common.noResults') }}</p>
      <RouterLink v-if="query" class="btn btn-outline btn-sm" :to="`/movies/${encodeURIComponent(query)}`">
        {{ $t('movies.searchSources') }}
      </RouterLink>
    </div>
    <Pagination :page="page" :size="size" :total="total" @change="changePage" />
  </div>
</template>
