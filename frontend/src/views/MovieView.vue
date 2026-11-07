<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft, Calendar, Check, Clock, Copy, ExternalLink, Film, Globe, Image as ImageIcon, Layers, RefreshCw, Search, Sparkles, Star, Tag, Users, ZoomIn } from 'lucide-vue-next'
import { api, ApiError, jsonBody } from '../api/client'
import { useAuthStore } from '../stores/auth'
import type { Movie, MovieMetadata } from '../types'
import MoviePlayer from '../components/MoviePlayer.vue'
import ImageLightbox, { type LightboxItem } from '../components/ImageLightbox.vue'
import { collectMovieDetails } from '../components/movieDetails'

interface Job { id: string; status: string; result?: { found: boolean; cached: boolean } }
interface MetadataJob { id: string; status: string; completed_sources: number; failed_sources: number; deferred_sources?: number; total_sources: number; collection_status?: string }

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
const metadataJob = ref<MetadataJob | null>(null)
let timer: ReturnType<typeof setTimeout> | undefined
let metadataTimer: ReturnType<typeof setTimeout> | undefined
let attempted = false
const id = computed(() => String(route.params.id || '').toUpperCase())
const activeItem = computed(() => movie.value?.versions?.find(item => String(item.id) === String(route.query.source_item || '')) || movie.value?.preferred_item)
const metadata = computed<MovieMetadata>(() => activeItem.value?.metadata || {})
const items = computed(() => movie.value?.versions?.length ? movie.value.versions : movie.value?.preferred_item ? [movie.value.preferred_item] : [])
const details = computed(() => collectMovieDetails(route.query.source_item && activeItem.value ? [activeItem.value] : items.value))
const summaryItems = computed(() => route.query.source_item && activeItem.value ? [activeItem.value] : items.value)
const description = computed(() => activeItem.value?.description || summaryItems.value.find(item => item.description)?.description || '')
const displayCover = computed(() => movie.value?.cover_image_url || activeItem.value?.cover_image_url || summaryItems.value.find(item => item.cover_image_url)?.cover_image_url || '')
const displayDate = computed(() => activeItem.value?.release_date || summaryItems.value.find(item => item.release_date)?.release_date)
const displayVolume = computed(() => metadata.value.volume_minutes || summaryItems.value.find(item => item.metadata?.volume_minutes)?.metadata.volume_minutes)
const displayRating = computed(() => metadata.value.rating ?? summaryItems.value.find(item => item.metadata?.rating != null)?.metadata.rating)
const sourceURL = computed(() => {
  try {
    const url = new URL(activeItem.value?.source_url || '')
    return ['http:', 'https:'].includes(url.protocol) ? url.href : ''
  } catch { return '' }
})

const lightboxOpen = ref(false)
const lightboxIndex = ref(0)

const galleryImages = computed<LightboxItem[]>(() => {
  const list: LightboxItem[] = []
  if (displayCover.value) {
    list.push({ url: displayCover.value, title: `${movie.value?.normalized_id || ''} - 封面` })
  }
  for (let i = 0; i < details.value.images.length; i++) {
    const img = details.value.images[i]
    list.push({
      url: img.url,
      title: `${movie.value?.normalized_id || ''} - 预览图 ${i + 1} (${img.sources.join('/').toUpperCase()})`,
    })
  }
  return list
})

function openCoverLightbox() {
  if (!displayCover.value) return
  lightboxIndex.value = 0
  lightboxOpen.value = true
}

function openSampleLightbox(index: number) {
  const offset = displayCover.value ? 1 : 0
  lightboxIndex.value = offset + index
  lightboxOpen.value = true
}

async function load() {
  if (timer) clearTimeout(timer)
  if (metadataTimer) clearTimeout(metadataTimer)
  loading.value = true
  notFound.value = false
  error.value = false
  imageFailed.value = false
  imageLoaded.value = false
  try {
    movie.value = (await api<Movie>(`/movies/${encodeURIComponent(id.value)}`)).data
    cover.value = movie.value.cover_image_url || ''
    void pollMetadata(id.value)
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

async function pollMetadata(movieID: string) {
  try {
    const previous = metadataJob.value
    const job = (await api<MetadataJob>(`/movies/${encodeURIComponent(movieID)}/metadata-job`)).data
    if (movieID !== id.value) return
    metadataJob.value = job
    if (['pending', 'running', 'retrying', 'waiting'].includes(job.status)) {
      metadataTimer = setTimeout(() => { void pollMetadata(movieID) }, 5000)
    } else if (job.collection_status === 'deferred' || job.collection_status === 'collecting') {
      metadataTimer = setTimeout(() => { void pollMetadata(movieID) }, 30000)
    } else if (previous && ['pending', 'running', 'retrying', 'waiting'].includes(previous.status)) {
      movie.value = (await api<Movie>(`/movies/${encodeURIComponent(movieID)}`)).data
    } else if (['deferred', 'collecting'].includes(previous?.collection_status || '') && job.collection_status === 'complete') {
      movie.value = (await api<Movie>(`/movies/${encodeURIComponent(movieID)}`)).data
    }
  } catch { metadataJob.value = null }
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

watch(() => route.params.id, () => { attempted = false; metadataJob.value = null; void load() }, { immediate: true })
watch(() => route.query.source_item, () => { imageFailed.value = false; imageLoaded.value = false })
watch(displayCover, () => { imageFailed.value = false; imageLoaded.value = false })
onBeforeUnmount(() => { if (timer) clearTimeout(timer); if (metadataTimer) clearTimeout(metadataTimer) })
</script>

<template>
  <div class="content-wrap detail-wrap">
    <RouterLink to="/movies" class="back-link"><ArrowLeft :size="16" />{{ $t('nav.movies') }}</RouterLink>
    <div v-if="loading" class="center-state">
      <span class="loading loading-spinner loading-md text-emerald-600"></span>
      <span class="text-xs text-slate-400 font-medium">{{ $t('common.loading') }}</span>
    </div>
    <div v-else-if="notFound" class="empty-detail">
      <span class="section-kicker">{{ id }}</span>
      <h1>{{ $t('movies.notFound') }}</h1>
      <button class="btn search-button" :disabled="searching" @click="search(false)">
        <span v-if="searching" class="loading loading-spinner loading-xs"></span>
        <Search v-else :size="17" />
        {{ searching ? $t('movies.searching') : $t('movies.searchSources') }}
      </button>
      <p v-if="error" class="error-text">{{ $t('movies.searchFailed') }}</p>
    </div>
    <div v-else-if="error && !movie" class="center-state">
      <p class="error-text font-medium">{{ $t('common.error') }}</p>
    </div>
    <template v-else-if="movie && activeItem">
      <div class="detail-heading">
        <div>
          <div class="section-kicker"><Film :size="12" /> {{ activeItem.source.toUpperCase() }} / {{ $t('movies.details') }}</div>
          <h1>{{ movie.normalized_id }}</h1>
        </div>
        <button class="btn btn-outline btn-sm shadow-xs" :title="$t('movies.copy')" @click="copyID">
          <Check v-if="copied" :size="15" class="text-emerald-600" />
          <Copy v-else :size="15" />
          {{ copied ? $t('movies.copied') : $t('movies.copy') }}
        </button>
      </div>
      <h2 class="detail-title">{{ activeItem.title }}</h2>
      <div class="detail-grid">
        <div class="detail-image cursor-pointer group relative" :title="displayCover ? '点击查看高清大图' : ''" @click="openCoverLightbox">
          <span v-if="!imageLoaded" class="text-slate-400 text-sm font-medium">{{ $t(!imageFailed && displayCover ? 'movies.loadingCover' : 'movies.noCover') }}</span>
          <img v-if="!imageFailed && displayCover" :class="{ 'image-pending': !imageLoaded }" :src="displayCover" :alt="movie.normalized_id" referrerpolicy="no-referrer" @load="imageLoaded = true" @error="imageFailed = true">
          <div v-if="!imageFailed && displayCover && imageLoaded" class="absolute inset-0 bg-black/20 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center rounded-lg">
            <span class="badge badge-neutral bg-black/60 text-white gap-1 py-3 px-3 shadow"><ZoomIn :size="16" /> 点击看大图</span>
          </div>
        </div>
        <aside class="facts">
          <h3>{{ $t('movies.details') }}</h3>
          <dl>
            <dt>{{ $t('common.date') }}</dt>
            <dd>{{ displayDate?.slice(0, 10) || $t('common.unknown') }}</dd>
            <dt>{{ $t('common.source') }}</dt>
            <dd><span class="inline-flex items-center px-2 py-0.5 rounded-md text-xs font-bold bg-slate-100 text-slate-800 tracking-wider font-mono">{{ activeItem.source.toUpperCase() }}</span></dd>
            <template v-if="displayVolume">
              <dt>{{ $t('movies.duration') }}</dt>
              <dd>{{ displayVolume }} {{ $t('movies.minutes') }}</dd>
            </template>
            <template v-if="metadata.genres?.length">
              <dt>{{ $t('movies.genres') }}</dt>
              <dd>{{ metadata.genres.join(' / ') }}</dd>
            </template>
            <template v-if="metadata.labels?.length">
              <dt>{{ $t('movies.labels') }}</dt>
              <dd>{{ metadata.labels.join(' / ') }}</dd>
            </template>
            <template v-if="displayRating != null">
              <dt>{{ $t('movies.rating') }}</dt>
              <dd><span class="inline-flex items-center gap-1 text-amber-600 font-bold"><Star :size="13" class="fill-amber-400 text-amber-500" /> {{ displayRating }}</span></dd>
            </template>
            <template v-if="metadata.makers?.length">
              <dt>{{ $t('movies.makers') }}</dt>
              <dd>{{ metadata.makers.join(' / ') }}</dd>
            </template>
            <template v-if="metadata.directors?.length">
              <dt>{{ $t('movies.directors') }}</dt>
              <dd>{{ metadata.directors.join(' / ') }}</dd>
            </template>
          </dl>
          <a v-if="sourceURL" :href="sourceURL" target="_blank" rel="noopener noreferrer" class="btn btn-outline btn-sm source-button shadow-xs">
            {{ $t('movies.visit') }} <ExternalLink :size="15" />
          </a>
        </aside>
      </div>
      <div v-if="auth.user?.is_admin" class="admin-inline">
        <button class="btn btn-outline btn-sm shadow-xs" :disabled="searching" @click="search(true)">
          <RefreshCw :size="15" />{{ $t('movies.reSearch') }}
        </button>
        <label class="cover-editor">
          <span>{{ $t('movies.cover') }}</span>
          <input v-model="cover" class="input input-sm" type="url">
          <button class="btn btn-sm btn-primary" @click="saveCover">{{ $t('common.save') }}</button>
        </label>
      </div>
      <p v-if="error" class="error-text font-medium">{{ $t('common.error') }}</p>
      <p v-if="metadataJob && (['pending', 'running', 'retrying', 'waiting'].includes(metadataJob.status) || metadataJob.collection_status === 'collecting')" class="metadata-progress">
        <Sparkles :size="14" /> {{ $t('metadata.collecting', { completed: metadataJob.completed_sources, total: metadataJob.total_sources }) }}
      </p>
      <p v-else-if="metadataJob?.collection_status === 'deferred'" class="metadata-progress">
        <Clock :size="14" /> {{ $t('sourceHealth.deferred', { count: metadataJob.deferred_sources || 0 }) }}
      </p>
      <p v-else-if="metadataJob?.status === 'failed' && metadataJob.collection_status !== 'complete'" class="error-text font-medium">
        {{ $t('metadata.failed', { count: metadataJob.failed_sources }) }}
      </p>
      <MoviePlayer :id="movie.normalized_id" />
      <section v-if="details.cast.length" class="detail-section">
        <h3>{{ $t('movies.cast') }}</h3>
        <div class="cast-list">
          <span v-for="person in details.cast" :key="person.value">
            <RouterLink :to="{ name: 'actress-by-name', params: { name: person.value } }">{{ person.value }}</RouterLink>
            <small>{{ person.sources.join(' / ').toUpperCase() }}</small>
          </span>
        </div>
      </section>
      <section v-if="details.genres.length || details.labels.length || details.makers.length || details.directors.length || details.series.length" class="detail-section">
        <h3>{{ $t('metadata.allDetails') }}</h3>
        <dl class="metadata-fields">
          <template v-for="field in (['genres', 'labels', 'makers', 'directors', 'series'] as const)" :key="field">
            <template v-if="details[field].length">
              <dt>{{ $t(`metadata.${field}`) }}</dt>
              <dd>
                <span v-for="entry in details[field]" :key="entry.value" class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-slate-100 text-slate-800 text-xs font-medium">
                  {{ entry.value }} <small>{{ entry.sources.join(' / ').toUpperCase() }}</small>
                </span>
              </dd>
            </template>
          </template>
        </dl>
      </section>
      <section v-if="description" class="detail-section">
        <h3>{{ $t('movies.description') }}</h3>
        <p class="description">{{ description }}</p>
      </section>
      <section v-if="details.images.length" class="detail-section">
        <h3>{{ $t('movies.samples') }}</h3>
        <div class="sample-grid">
          <figure v-for="(image, index) in details.images" :key="image.url">
            <button
              type="button"
              class="w-full text-left block cursor-pointer transition-transform hover:scale-[1.02] focus:outline-none"
              :title="$t('common.previewDetails')"
              @click="openSampleLightbox(index)"
            >
              <img :src="image.url" :alt="`${movie.normalized_id} ${index + 1}`" loading="lazy" referrerpolicy="no-referrer">
            </button>
            <figcaption>{{ image.sources.join(' / ').toUpperCase() }}</figcaption>
          </figure>
        </div>
      </section>
      <section v-if="movie.versions && movie.versions.length > 1" class="detail-section">
        <h3>{{ $t('movies.versions') }}</h3>
        <div class="version-list">
          <RouterLink :to="{ path: `/movies/${encodeURIComponent(movie.normalized_id)}` }" :aria-current="!route.query.source_item ? 'true' : undefined">
            <strong>{{ $t('common.all') }}</strong>
          </RouterLink>
          <RouterLink v-for="item in movie.versions" :key="item.id" :to="{ path: `/movies/${encodeURIComponent(movie.normalized_id)}`, query: { source_item: item.id } }" :aria-current="route.query.source_item && item.id === activeItem.id ? 'true' : undefined">
            <strong>{{ item.source.toUpperCase() }}</strong>
            <span>{{ item.title }}</span>
          </RouterLink>
        </div>
      </section>
      <ImageLightbox
        :open="lightboxOpen"
        :images="galleryImages"
        :initial-index="lightboxIndex"
        @close="lightboxOpen = false"
      />
    </template>
  </div>
</template>
