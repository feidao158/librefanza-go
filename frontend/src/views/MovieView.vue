<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft, Check, Copy, ExternalLink, RefreshCw, Search } from 'lucide-vue-next'
import { api, ApiError, jsonBody } from '../api/client'
import { useAuthStore } from '../stores/auth'
import type { Movie, MovieMetadata } from '../types'

interface Job { id: string; status: string; result?: { found: boolean; cached: boolean } }

const route = useRoute()
const auth = useAuthStore()
const movie = ref<Movie | null>(null)
const loading = ref(true)
const searching = ref(false)
const notFound = ref(false)
const error = ref(false)
const copied = ref(false)
const imageFailed = ref(false)
const imageLoaded = ref(false)
const cover = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
let attempted = false
const id = computed(() => String(route.params.id || '').toUpperCase())
const activeItem = computed(() => movie.value?.versions?.find(item => String(item.id) === String(route.query.source_item || '')) || movie.value?.preferred_item)
const metadata = computed<MovieMetadata>(() => activeItem.value?.metadata || {})
const sourceURL = computed(() => {
  try {
    const url = new URL(activeItem.value?.source_url || '')
    return ['http:', 'https:'].includes(url.protocol) ? url.href : ''
  } catch { return '' }
})

async function load() {
  if (timer) clearTimeout(timer)
  loading.value = true
  notFound.value = false
  error.value = false
  imageFailed.value = false
  imageLoaded.value = false
  try {
    movie.value = (await api<Movie>(`/movies/${encodeURIComponent(id.value)}`)).data
    cover.value = movie.value.cover_image_url || ''
  } catch (err) {
    movie.value = null
    if (err instanceof ApiError && err.status === 404) notFound.value = true
    else error.value = true
  } finally {
    loading.value = false
    if (notFound.value && !attempted) {
      attempted = true
      void search(false)
    }
  }
}

async function search(force = false) {
  searching.value = true
  error.value = false
  try {
    const response = await api<{ id: string }>('/search-jobs', { method: 'POST', body: jsonBody({ id: id.value, force }) })
    await poll(response.data.id, Date.now() + 120_000)
  } catch { searching.value = false; error.value = true }
}

async function poll(jobID: string, deadline: number) {
  try {
    const result = await api<Job>(`/search-jobs/${encodeURIComponent(jobID)}`)
    if (result.data.status === 'completed') {
      searching.value = false
      await load()
    } else if (['failed', 'cancelled'].includes(result.data.status) || Date.now() > deadline) {
      searching.value = false
      error.value = true
    } else {
      timer = setTimeout(() => { void poll(jobID, deadline) }, 1500)
    }
  } catch { searching.value = false; error.value = true }
}

async function saveCover() {
  try {
    await api(`/admin/movies/${encodeURIComponent(id.value)}`, { method: 'PATCH', body: jsonBody({ cover_image_url: cover.value }) })
    await load()
  } catch { error.value = true }
}

async function copyID() {
  try { await navigator.clipboard.writeText(id.value); copied.value = true; setTimeout(() => { copied.value = false }, 2000) } catch { error.value = true }
}

watch(() => route.params.id, () => { attempted = false; void load() }, { immediate: true })
watch(() => route.query.source_item, () => { imageFailed.value = false; imageLoaded.value = false })
onBeforeUnmount(() => { if (timer) clearTimeout(timer) })
</script>

<template>
  <div class="content-wrap detail-wrap">
    <RouterLink to="/movies" class="back-link"><ArrowLeft :size="16" />{{ $t('nav.movies') }}</RouterLink>
    <div v-if="loading" class="center-state"><span class="loading loading-spinner"></span></div>
    <div v-else-if="notFound" class="empty-detail"><span class="section-kicker">{{ id }}</span><h1>{{ $t('movies.notFound') }}</h1><button class="btn search-button" :disabled="searching" @click="search(false)"><span v-if="searching" class="loading loading-spinner loading-xs"></span><Search v-else :size="17" />{{ searching ? $t('movies.searching') : $t('movies.searchSources') }}</button><p v-if="error" class="error-text">{{ $t('movies.searchFailed') }}</p></div>
    <div v-else-if="error && !movie" class="center-state">{{ $t('common.error') }}</div>
    <template v-else-if="movie && activeItem">
      <div class="detail-heading"><div><div class="section-kicker">{{ activeItem.source.toUpperCase() }} / {{ $t('movies.details') }}</div><h1>{{ movie.normalized_id }}</h1></div><button class="btn btn-outline btn-sm" :title="$t('movies.copy')" @click="copyID"><Check v-if="copied" :size="16" /><Copy v-else :size="16" />{{ copied ? $t('movies.copied') : $t('movies.copy') }}</button></div>
      <h2 class="detail-title">{{ activeItem.title }}</h2>
      <div class="detail-grid">
        <div class="detail-image"><span v-if="!imageLoaded">{{ $t(!imageFailed && (movie.cover_image_url || activeItem.cover_image_url) ? 'movies.loadingCover' : 'movies.noCover') }}</span><img v-if="!imageFailed && (movie.cover_image_url || activeItem.cover_image_url)" :class="{ 'image-pending': !imageLoaded }" :src="movie.cover_image_url || activeItem.cover_image_url" :alt="movie.normalized_id" referrerpolicy="no-referrer" @load="imageLoaded = true" @error="imageFailed = true"></div>
        <aside class="facts"><h3>{{ $t('movies.details') }}</h3><dl><dt>{{ $t('common.date') }}</dt><dd>{{ activeItem.release_date?.slice(0, 10) || $t('common.unknown') }}</dd><dt>{{ $t('common.source') }}</dt><dd>{{ activeItem.source.toUpperCase() }}</dd><template v-if="metadata.volume_minutes"><dt>{{ $t('movies.duration') }}</dt><dd>{{ metadata.volume_minutes }} {{ $t('movies.minutes') }}</dd></template><template v-if="metadata.genres?.length"><dt>{{ $t('movies.genres') }}</dt><dd>{{ metadata.genres.join(' / ') }}</dd></template><template v-if="metadata.labels?.length"><dt>{{ $t('movies.labels') }}</dt><dd>{{ metadata.labels.join(' / ') }}</dd></template><template v-if="metadata.rating != null"><dt>{{ $t('movies.rating') }}</dt><dd>{{ metadata.rating }}</dd></template><template v-if="metadata.makers?.length"><dt>{{ $t('movies.makers') }}</dt><dd>{{ metadata.makers.join(' / ') }}</dd></template><template v-if="metadata.directors?.length"><dt>{{ $t('movies.directors') }}</dt><dd>{{ metadata.directors.join(' / ') }}</dd></template></dl><a v-if="sourceURL" :href="sourceURL" target="_blank" rel="noopener noreferrer" class="btn btn-outline btn-sm source-button">{{ $t('movies.visit') }} <ExternalLink :size="15" /></a></aside>
      </div>
      <div v-if="auth.user?.is_admin" class="admin-inline"><button class="btn btn-outline btn-sm" :disabled="searching" @click="search(true)"><RefreshCw :size="15" />{{ $t('movies.reSearch') }}</button><label class="cover-editor"><span>{{ $t('movies.cover') }}</span><input v-model="cover" class="input input-sm" type="url"><button class="btn btn-sm" @click="saveCover">{{ $t('common.save') }}</button></label></div>
      <p v-if="error" class="error-text">{{ $t('common.error') }}</p>
      <section v-if="metadata.cast?.length" class="detail-section"><h3>{{ $t('movies.cast') }}</h3><div class="cast-list"><RouterLink v-for="person in metadata.cast" :key="person.name" :to="{ name: 'actress-by-name', params: { name: person.name } }">{{ person.name }}</RouterLink></div></section>
      <section v-if="activeItem.description" class="detail-section"><h3>{{ $t('movies.description') }}</h3><p class="description">{{ activeItem.description }}</p></section>
      <section v-if="metadata.sample_image_urls?.length" class="detail-section"><h3>{{ $t('movies.samples') }}</h3><div class="sample-grid"><a v-for="(image, index) in metadata.sample_image_urls" :key="image" :href="image" target="_blank" rel="noopener noreferrer"><img :src="image" :alt="`${movie.normalized_id} ${index + 1}`" loading="lazy" referrerpolicy="no-referrer"></a></div></section>
      <section v-if="movie.versions && movie.versions.length > 1" class="detail-section"><h3>{{ $t('movies.versions') }}</h3><div class="version-list"><RouterLink v-for="item in movie.versions" :key="item.id" :to="{ path: `/movies/${encodeURIComponent(movie.normalized_id)}`, query: { source_item: item.id } }" :aria-current="item.id === activeItem.id ? 'true' : undefined"><strong>{{ item.source.toUpperCase() }}</strong><span>{{ item.title }}</span></RouterLink></div></section>
    </template>
  </div>
</template>
