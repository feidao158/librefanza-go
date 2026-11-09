<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { Heart } from 'lucide-vue-next'
import { api } from '../api/client'
import MovieCard from '../components/MovieCard.vue'
import Pagination from '../components/Pagination.vue'
import type { Movie } from '../types'

const route = useRoute()
const router = useRouter()
const movies = ref<Movie[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const error = ref(false)
const size = 30
const scrollKey = 'librefanza-likes-scroll'

onBeforeRouteLeave(to => {
  if (!to.path.startsWith('/movies/')) return
  try {
    sessionStorage.setItem(scrollKey, JSON.stringify({ page: page.value, top: window.scrollY }))
  } catch { /* 存储不可用时仍允许正常导航 */ }
})

async function restoreScroll() {
  try {
    const stored = sessionStorage.getItem(scrollKey)
    if (!stored) return
    sessionStorage.removeItem(scrollKey)
    const position = JSON.parse(stored) as { page?: number; top?: number }
    if (position.page !== page.value || typeof position.top !== 'number' || !Number.isFinite(position.top)) return
    await nextTick()
    window.scrollTo({ top: Math.max(0, position.top), behavior: 'instant' })
  } catch { /* 存储不可用或记录无效时保持默认位置 */ }
}

async function load() {
  page.value = Math.max(1, Number(route.query.page) || 1)
  loading.value = true
  error.value = false
  try {
    const response = await api<Movie[]>(`/annotations/likes?page=${page.value}&page_size=${size}`)
    movies.value = response.data || []
    total.value = response.meta?.total || 0
  } catch { error.value = true } finally { loading.value = false }
  if (!error.value) await restoreScroll()
}

watch(() => route.fullPath, load, { immediate: true })
</script>

<template>
  <div class="content-wrap">
    <div class="page-title-row">
      <div><h1><Heart :size="24" class="inline-block mr-2" />{{ $t('nav.likes') }}</h1></div>
      <span class="count-label">{{ $t('common.total', { count: total }) }}</span>
    </div>
    <div v-if="loading" class="center-state"><span class="loading loading-spinner loading-md"></span></div>
    <div v-else-if="error" class="center-state"><p class="error-text">{{ $t('common.error') }}</p><button class="btn btn-outline btn-sm" @click="load">{{ $t('common.retry') }}</button></div>
    <div v-else-if="movies.length" class="movie-grid"><MovieCard v-for="movie in movies" :key="movie.id" :movie="movie" :likes-page="page" /></div>
    <div v-else class="center-state">{{ $t('common.noResults') }}</div>
    <Pagination :page="page" :size="size" :total="total" @change="router.push({ path: '/likes', query: { page: $event } })" />
  </div>
</template>
