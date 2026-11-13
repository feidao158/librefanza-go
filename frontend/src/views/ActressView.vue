<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, UserRound, Users } from 'lucide-vue-next'
import { api } from '../api/client'
import MovieCard from '../components/MovieCard.vue'
import Pagination from '../components/Pagination.vue'
import type { Actress, Movie } from '../types'

const route = useRoute()
const router = useRouter()
const actress = ref<Actress | null>(null)
const movies = ref<Movie[]>([])
const total = ref(0)
const page = ref(1)
const solo = ref(false)
const loading = ref(true)
const error = ref(false)
const broken = ref(false)

async function load() {
  loading.value = true
  error.value = false
  broken.value = false
  page.value = Math.max(1, Number(route.query.page) || 1)
  solo.value = route.query.solo === 'true'
  try {
    if (route.name === 'actress-by-name') {
      const params = new URLSearchParams({ name: String(route.params.name), page: String(page.value), page_size: '30', solo: String(solo.value) })
      const works = await api<{ actress: Actress; movies: Movie[] }>(`/actress-filmographies?${params}`)
      actress.value = works.data.actress
      movies.value = works.data.movies || []
      total.value = works.meta?.total || 0
    } else {
      const id = encodeURIComponent(String(route.params.id))
      const [person, works] = await Promise.all([api<Actress>(`/actresses/${id}`), api<Movie[]>(`/actresses/${id}/movies?page=${page.value}&page_size=30&solo=${solo.value}`)])
      actress.value = person.data
      movies.value = works.data || []
      total.value = works.meta?.total || 0
    }
  } catch { error.value = true } finally { loading.value = false }
}
function setSolo(value: boolean) { router.push({ query: { solo: value ? 'true' : undefined } }) }
function changePage(value: number) { router.push({ query: { ...route.query, page: value } }) }
watch(() => route.fullPath, load, { immediate: true })
</script>

<template>
  <div class="content-wrap detail-wrap">
    <RouterLink to="/actresses" class="back-link"><ArrowLeft :size="16" />{{ $t('nav.actresses') }}</RouterLink>
    <div v-if="loading" class="center-state">
      <span class="loading loading-spinner loading-md text-primary"></span>
      <span class="text-xs text-base-content/65 font-medium">{{ $t('common.loading') }}</span>
    </div>
    <div v-else-if="error" class="center-state">
      <p class="error-text font-medium">{{ $t('common.error') }}</p>
      <button class="btn btn-outline btn-sm" @click="load">{{ $t('common.retry') }}</button>
    </div>
    <template v-else-if="actress">
      <div class="actress-profile">
        <div class="profile-photo">
          <img v-if="actress.image_url && !broken" :src="actress.image_url" :alt="actress.name" referrerpolicy="no-referrer" @error="broken = true">
          <UserRound v-else :size="38" />
        </div>
        <div>
          <h1>{{ actress.name }}</h1>
          <span v-if="actress.fanza_id" class="muted font-mono">FANZA #{{ actress.fanza_id }}</span>
        </div>
      </div>
      <div class="section-heading">
        <div>
          <h2>{{ $t('actresses.works') }}</h2>
        </div>
        <div class="segmented">
          <button class="btn btn-sm" :class="solo ? 'btn-ghost' : 'btn-active'" @click="setSolo(false)">{{ $t('actresses.allWorks') }}</button>
          <button class="btn btn-sm" :class="solo ? 'btn-active' : 'btn-ghost'" @click="setSolo(true)">{{ $t('actresses.solo') }}</button>
        </div>
      </div>
      <div v-if="movies.length" class="movie-grid">
        <MovieCard v-for="movie in movies" :key="movie.id" :movie="movie" />
      </div>
      <div v-else class="center-state">
        <p class="text-base-content/70 font-medium">{{ $t('common.noResults') }}</p>
      </div>
      <Pagination :page="page" :size="30" :total="total" @change="changePage" />
    </template>
  </div>
</template>
