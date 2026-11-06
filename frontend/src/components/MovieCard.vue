<script setup lang="ts">
import { ref } from 'vue'
import type { Movie } from '../types'

defineProps<{ movie: Movie }>()
const imageFailed = ref(false)
const imageLoaded = ref(false)
</script>

<template>
  <RouterLink class="movie-card" :to="`/movies/${encodeURIComponent(movie.normalized_id)}`">
    <div class="movie-cover">
      <span v-if="!imageLoaded" class="cover-placeholder">{{ $t(!imageFailed && (movie.preferred_item?.thumbnail_image_url || movie.preferred_item?.cover_image_url) ? 'movies.loadingCover' : 'movies.noCover') }}</span>
      <img v-if="!imageFailed && (movie.preferred_item?.thumbnail_image_url || movie.preferred_item?.cover_image_url)" :class="{ 'image-pending': !imageLoaded }" :src="movie.preferred_item?.thumbnail_image_url || movie.preferred_item?.cover_image_url" :alt="movie.normalized_id" loading="lazy" referrerpolicy="no-referrer" @load="imageLoaded = true" @error="imageFailed = true">
      <span v-if="movie.preferred_item?.source" class="source-label">{{ movie.preferred_item.source.toUpperCase() }}</span>
    </div>
    <div class="movie-card-body">
      <div class="movie-id">{{ movie.normalized_id }}</div>
      <div class="movie-title">{{ movie.preferred_item?.title || movie.normalized_id }}</div>
      <div class="movie-date">{{ movie.release_date?.slice(0, 10) || $t('common.unknown') }}</div>
    </div>
  </RouterLink>
</template>
