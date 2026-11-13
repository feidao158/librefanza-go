<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Calendar } from 'lucide-vue-next'
import type { Movie } from '../types'
import { jableURL } from './jable'
import MoviePlayer from './MoviePlayer.vue'

const props = defineProps<{ movie: Movie; likesPage?: number }>()
const imageFailed = ref(false)
const playerOpen = ref(false)
const imageLoaded = ref(false)
const imageIndex = ref(0)
const images = computed(() => [...new Set([
  props.movie.cover_image_url,
  props.movie.preferred_item?.cover_image_url,
  props.movie.preferred_item?.thumbnail_image_url,
].filter((value): value is string => Boolean(value)))])
const imageURL = computed(() => images.value[imageIndex.value] || '')

watch(images, () => {
  imageIndex.value = 0
  imageFailed.value = false
  imageLoaded.value = false
})

function onImageError() {
  imageLoaded.value = false
  if (imageIndex.value + 1 < images.value.length) imageIndex.value++
  else imageFailed.value = true
}
</script>

<template>
  <div class="movie-card">
  <RouterLink class="movie-card-link" :to="{ path: `/movies/${encodeURIComponent(movie.normalized_id)}`, query: likesPage ? { from: 'likes', likes_page: likesPage } : {} }">
    <div class="movie-cover">
      <span v-if="!imageLoaded" class="cover-placeholder">{{ $t(!imageFailed && imageURL ? 'movies.loadingCover' : 'movies.noCover') }}</span>
      <img
        v-if="!imageFailed && imageURL"
        :class="{ 'image-pending': !imageLoaded }"
        :src="imageURL"
        :alt="movie.normalized_id"
        loading="lazy"
        referrerpolicy="no-referrer"
        @load="imageLoaded = true"
        @error="onImageError"
      >
      <span v-if="movie.preferred_item?.source" class="source-label">{{ movie.preferred_item.source.toUpperCase() }}</span>
    </div>
    <div class="movie-card-body">
      <div class="movie-id">{{ movie.normalized_id }}</div>
      <div class="movie-title" :title="movie.preferred_item?.title || movie.normalized_id">{{ movie.preferred_item?.title || movie.normalized_id }}</div>
      <div class="movie-date">
        <Calendar :size="13" class="opacity-70 flex-shrink-0" />
        <span>{{ movie.release_date?.slice(0, 10) || $t('common.unknown') }}</span>
      </div>
    </div>
  </RouterLink>
  <button data-testid="card-play" class="btn btn-outline btn-sm mx-2 mb-1" @click="playerOpen = true">{{ $t('player.title') }}</button>
  <a v-if="jableURL(movie.normalized_id)" class="btn btn-ghost btn-sm mx-2 mb-2" :href="jableURL(movie.normalized_id)" target="_blank" rel="noopener noreferrer">{{ $t('player.openJable') }}</a>
  <Teleport to="body"><div v-if="playerOpen" class="modal modal-open" role="dialog" aria-modal="true">
    <div class="modal-box max-w-4xl"><div class="admin-heading"><h2>{{ movie.normalized_id }}</h2><button class="btn btn-ghost btn-sm" :aria-label="$t('common.close')" @click="playerOpen = false">{{ $t('common.close') }}</button></div><MoviePlayer :id="movie.normalized_id" /></div>
    <button class="modal-backdrop" :aria-label="$t('common.close')" @click="playerOpen = false"></button>
  </div></Teleport>
  </div>
</template>
