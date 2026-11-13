<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowRight, Clock, ExternalLink, HelpCircle, Layers, Sparkles, Star, ThumbsDown, ThumbsUp, ZoomIn } from 'lucide-vue-next'
import { ApiError, api, jsonBody } from '../api/client'
import type { AnnotationStats, FeedbackResponse, Movie, RecommendationResponse } from '../types'
import ImageLightbox, { type LightboxItem } from '../components/ImageLightbox.vue'
import SmoothImage from '../components/SmoothImage.vue'
import MoviePlayer from '../components/MoviePlayer.vue'
import { MoviePreload } from '../components/moviePreload'

const { t } = useI18n()

const currentMovie = ref<Movie | null>(null)
const preloaded = new MoviePreload()
let disposed = false
const stats = ref<AnnotationStats>({
  total: 0,
  liked: 0,
  disliked: 0,
  uncertain: 0,
  min_required: 5,
  ready: false,
})
const loading = ref(true)
const submitting = ref(false)
const error = ref(false)
const unavailable = ref(false)
const feedbackError = ref(false)
const feedbackSaved = ref(false)
const descExpanded = ref(false)

// 记录最近一次切换动作以触发卡片滑动离开过渡动效
const lastAction = ref<'like' | 'dislike' | 'uncertain' | null>(null)

// 弹窗画廊状态
const lightboxOpen = ref(false)
const lightboxIndex = ref(0)

const progressPercent = computed(() => {
  if (!stats.value.min_required) return 100
  return Math.min(100, Math.round((stats.value.total / stats.value.min_required) * 100))
})

const sampleImages = computed<string[]>(() => {
  const meta = currentMovie.value?.preferred_item?.metadata
  if (meta?.sample_image_urls?.length) {
    return meta.sample_image_urls.filter(Boolean)
  }
  return []
})

const galleryImages = computed<LightboxItem[]>(() => {
  if (!currentMovie.value) return []
  const list: LightboxItem[] = []
  const coverUrl = currentMovie.value.preferred_item?.cover_image_url || currentMovie.value.cover_image_url
  if (coverUrl) {
    list.push({ url: coverUrl, title: `${currentMovie.value.normalized_id} - 封面` })
  }
  for (let i = 0; i < sampleImages.value.length; i++) {
    list.push({
      url: sampleImages.value[i],
      title: `${currentMovie.value.normalized_id} - 预览图 ${i + 1}`,
    })
  }
  return list
})

function openCoverLightbox() {
  const coverUrl = currentMovie.value?.preferred_item?.cover_image_url || currentMovie.value?.cover_image_url
  if (!coverUrl) return
  lightboxIndex.value = 0
  lightboxOpen.value = true
}

function openSampleLightbox(index: number) {
  const coverUrl = currentMovie.value?.preferred_item?.cover_image_url || currentMovie.value?.cover_image_url
  const offset = coverUrl ? 1 : 0
  lightboxIndex.value = offset + index
  lightboxOpen.value = true
}

async function loadRecommendations() {
  loading.value = true
  error.value = false
  unavailable.value = false
  feedbackError.value = false
  descExpanded.value = false
  lightboxOpen.value = false
  try {
    const res = await api<RecommendationResponse>('/recommendations?page_size=5&page=1')
    stats.value = res.data.stats
    preloaded.clear()
    currentMovie.value = res.data.ready ? res.data.movies[0] || null : null
    preloaded.add(res.data.movies.slice(1), currentMovie.value?.normalized_id || '')
    feedbackSaved.value = false
  } catch (cause) {
    unavailable.value = cause instanceof ApiError && cause.code === 'recommendation_unavailable'
    error.value = true
  } finally {
    loading.value = false
  }
}

// 提交喜欢 / 不喜欢 / 不确定反馈，并丝滑切换至下一部
async function submitFeedback(action: 'like' | 'dislike' | 'uncertain') {
  if (!currentMovie.value || submitting.value || loading.value || feedbackSaved.value) return
  submitting.value = true
  feedbackError.value = false
  lastAction.value = action
  const currentId = currentMovie.value.normalized_id
  const candidateIds = preloaded.ids()

  try {
    const res = await api<FeedbackResponse>('/recommendations/feedback', {
      method: 'POST',
      body: jsonBody({
        normalized_id: currentId,
        action,
        prefetched: true,
        candidate_ids: candidateIds,
      }),
    })
    stats.value = res.data.stats

    feedbackSaved.value = true
    const next = preloaded.takeEligible(res.data.eligible_ids || [])
    if (next && !disposed) {
      currentMovie.value = next
      feedbackSaved.value = false
      descExpanded.value = false
      lightboxOpen.value = false
    } else {
      await loadRecommendations()
    }
  } catch {
    feedbackError.value = true
  } finally {
    submitting.value = false
  }
}

// 键盘快捷键监听
function handleKeyDown(event: KeyboardEvent) {
  if (lightboxOpen.value) return
  const target = event.target as HTMLElement
  if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) {
    return
  }

  if (event.code === 'Digit1' || event.key === '1') {
    event.preventDefault()
    void submitFeedback('like')
  } else if (event.code === 'Digit2' || event.key === '2') {
    event.preventDefault()
    void submitFeedback('uncertain')
  } else if (event.code === 'Digit3' || event.key === '3') {
    event.preventDefault()
    void submitFeedback('dislike')
  }
}

onMounted(() => {
  void loadRecommendations()
  window.addEventListener('keydown', handleKeyDown)
})

onBeforeUnmount(() => {
  disposed = true
  preloaded.clear()
  window.removeEventListener('keydown', handleKeyDown)
})
</script>

<template>
  <div class="max-w-6xl mx-auto px-3 sm:px-4 py-4 sm:py-6">
    <!-- 顶部标题与控制栏 -->
    <div class="mb-4 sm:mb-6 flex flex-col sm:flex-row sm:items-end justify-between gap-3 sm:gap-4">
      <div>
        <h1 class="text-2xl sm:text-3xl font-bold flex items-center gap-2">
          <Sparkles class="text-primary" :size="26" />
          <span>{{ t('recommendations.title') }}</span>
        </h1>
        <p class="text-xs sm:text-sm text-base-content/70 mt-1">{{ t('recommendations.subtitle') }}</p>
      </div>

      <div class="flex items-center gap-2 w-full sm:w-auto">
        <RouterLink
          to="/annotations"
          class="btn btn-outline btn-sm gap-1.5 flex-1 sm:flex-initial justify-center"
        >
          <Layers :size="15" />
          <span>{{ t('recommendations.continueAnnotating') }}</span>
        </RouterLink>

      </div>
    </div>

    <!-- 错误提示 -->
    <div v-if="error" class="alert alert-error mb-4 sm:mb-6 shadow-sm">
      <span>{{ unavailable ? t('recommendations.unavailable') : t('common.error') }}</span>
      <button class="btn btn-sm btn-ghost" @click="loadRecommendations()">{{ t('common.retry') }}</button>
    </div>
    <div v-if="feedbackError" class="alert alert-error mb-4 sm:mb-6 shadow-sm" role="alert">
      <span>{{ t('recommendations.feedbackFailed') }}</span>
    </div>

    <!-- 全局加载状态 -->
    <div v-if="loading && !currentMovie" class="flex justify-center items-center py-24">
      <span class="loading loading-spinner loading-lg text-primary"></span>
    </div>

    <!-- 情况 1：未达到标注门槛，展示学习引导卡片 -->
    <div v-else-if="!error && !stats.ready && !currentMovie" class="card bg-base-100 shadow-md border border-base-300 p-6 sm:p-8 md:p-12 text-center max-w-2xl mx-auto">
      <div class="w-14 h-14 sm:w-16 sm:h-16 rounded-full bg-primary/10 text-primary flex items-center justify-center mx-auto mb-4">
        <Sparkles :size="28" class="sm:w-8 sm:h-8" />
      </div>
      <h2 class="text-xl sm:text-2xl font-bold mb-2.5">{{ t('recommendations.notReadyTitle') }}</h2>
      <p class="text-xs sm:text-sm text-base-content/70 mb-5 leading-relaxed">
        {{ t('recommendations.notReadyDesc', { total: stats.total, min: stats.min_required }) }}
      </p>

      <!-- 进度条 -->
      <div class="w-full bg-base-200 rounded-full h-2.5 sm:h-3 mb-2 overflow-hidden">
        <div
          class="bg-primary h-full rounded-full transition-all duration-300"
          :style="{ width: `${progressPercent}%` }"
        ></div>
      </div>
      <div class="text-xs text-base-content/50 mb-5 font-medium">
        {{ t('recommendations.progress', { total: stats.total, min: stats.min_required }) }}
      </div>

      <div>
        <RouterLink to="/annotations" class="btn btn-primary btn-sm sm:btn-md gap-2 px-5 sm:px-6 shadow-md">
          <span>{{ t('recommendations.startAnnotating') }}</span>
          <ArrowRight :size="15" />
        </RouterLink>
      </div>
    </div>

    <!-- 情况 2：猜你喜欢已激活（大屏精选切卡视图） -->
    <div v-else-if="!error || currentMovie">
      <!-- 顶部推荐状态与快捷切换条 -->
      <div class="card bg-base-200 shadow-sm p-3 sm:p-4 mb-4 sm:mb-6 flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 sm:gap-4">
        <div class="flex flex-wrap items-center gap-2 sm:gap-3 text-xs sm:text-sm font-medium">
          <span class="text-success flex items-center gap-1">
            <ThumbsUp :size="14" /> {{ stats.liked }}
          </span>
          <span class="text-error flex items-center gap-1">
            <ThumbsDown :size="14" /> {{ stats.disliked }}
          </span>
          <span class="text-base-content/60 text-xs hidden sm:inline">
            {{ t('recommendations.trainingInfo', { total: stats.total, liked: stats.liked, disliked: stats.disliked }) }}
          </span>
        </div>

      </div>

      <!-- 空结果提示 -->
      <div v-if="!currentMovie" class="card bg-base-100 shadow-md p-8 sm:p-12 text-center border border-base-200">
        <p class="text-sm sm:text-base text-base-content/60">{{ t('recommendations.noRecommendations') }}</p>
        <div class="mt-4">
          <RouterLink to="/annotations" class="btn btn-outline btn-sm gap-1.5">
            <span>{{ t('recommendations.startAnnotating') }}</span>
          </RouterLink>
        </div>
      </div>

      <!-- 大屏主推荐卡片（带物理惯性滑动过渡动效） -->
      <div v-else class="recommendation-stage">
      <Transition name="recommendation-card">
        <div
          :key="currentMovie.normalized_id"
          class="card bg-base-100 shadow-lg border border-base-300 overflow-hidden relative"
        >
          <div class="grid grid-cols-1 md:grid-cols-12 gap-4 sm:gap-6 p-4 sm:p-6">
            <!-- 封面大图（支持完整原比例展示，使用平滑渐变加载与氛围背景，避免裁剪画面） -->
            <div class="md:col-span-5 flex flex-col items-center justify-center">
              <div
                class="w-full relative min-h-[260px] sm:min-h-[320px] md:min-h-[440px] max-h-[500px] bg-base-200/60 rounded-xl overflow-hidden shadow-inner flex items-center justify-center cursor-pointer group border border-base-300/40"
                :title="t('common.previewDetails')"
                @click="openCoverLightbox"
              >
                <SmoothImage
                  :src="currentMovie.preferred_item?.cover_image_url || currentMovie.cover_image_url"
                  :alt="currentMovie.preferred_item?.title || currentMovie.normalized_id"
                  aspect="w-full h-full min-h-[260px] sm:min-h-[320px] md:min-h-[440px] max-h-[500px]"
                  object-fit="contain"
                  blur-backdrop
                  :fallback-text="t('movies.noCover')"
                  loading="eager"
                  img-class="group-hover:scale-[1.02] transition-transform duration-300 drop-shadow-sm max-h-[490px]"
                >
                  <!-- 番号角标 -->
                  <div class="absolute top-2.5 left-2.5 z-10">
                    <span class="badge badge-neutral shadow font-mono text-xs sm:text-sm px-2 py-1.5 sm:px-2.5 sm:py-2">
                      {{ currentMovie.normalized_id }}
                    </span>
                  </div>

                  <!-- 为你推荐角标 -->
                  <div class="absolute top-2.5 right-2.5 z-10">
                    <span class="badge badge-primary gap-1 shadow text-xs font-semibold py-2 px-2.5">
                      <Sparkles :size="12" /> {{ t('recommendations.forYouBadge') }}
                    </span>
                  </div>

                  <!-- 点击看大图提示浮层 -->
                  <div class="absolute inset-0 bg-black/25 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center z-10">
                    <span class="badge badge-neutral bg-black/70 text-white gap-1 py-1.5 px-2.5 text-xs shadow">
                      <ZoomIn :size="14" /> 点击查看大图
                    </span>
                  </div>
                </SmoothImage>
              </div>

              <!-- 缩略图快速预览提示 -->
              <p v-if="sampleImages.length" class="text-[11px] sm:text-xs text-base-content/50 mt-1.5 text-center">
                共包含 {{ sampleImages.length }} 张预览截图，点击可全屏浏览
              </p>
            </div>

            <!-- 详细信息 -->
            <div class="md:col-span-7 flex flex-col justify-between">
              <div>
                <div class="flex items-start justify-between gap-2 mb-2">
                  <RouterLink
                    :to="`/movies/${currentMovie.normalized_id}`"
                    target="_blank"
                    class="link link-hover text-lg sm:text-2xl font-bold line-clamp-2"
                    :title="currentMovie.preferred_item?.title || currentMovie.normalized_id"
                  >
                    {{ currentMovie.preferred_item?.title || currentMovie.normalized_id }}
                  </RouterLink>

                  <RouterLink
                    :to="`/movies/${currentMovie.normalized_id}`"
                    target="_blank"
                    class="btn btn-ghost btn-xs flex-shrink-0 gap-1 text-base-content/60"
                    :title="t('movies.visit')"
                  >
                    <ExternalLink :size="14" />
                  </RouterLink>
                </div>

                <MoviePlayer :id="currentMovie.normalized_id" />

                <!-- 详细属性 -->
                <div class="space-y-2.5 sm:space-y-3 mt-3 sm:mt-4 text-xs sm:text-sm text-base-content/80">
                  <!-- 发行日期与时长与评分 -->
                  <div class="flex flex-wrap items-center gap-2.5 sm:gap-4 text-xs font-medium">
                    <span v-if="currentMovie.release_date || currentMovie.preferred_item?.release_date" class="text-base-content/70">
                      {{ t('common.date') }}: {{ (currentMovie.release_date || currentMovie.preferred_item?.release_date || '').slice(0, 10) }}
                    </span>
                    <span v-if="currentMovie.preferred_item?.metadata?.volume_minutes" class="flex items-center gap-1 text-base-content/70">
                      <Clock :size="12" /> {{ currentMovie.preferred_item.metadata.volume_minutes }} {{ t('movies.minutes') }}
                    </span>
                    <span v-if="currentMovie.preferred_item?.metadata?.rating != null" class="flex items-center gap-1 text-amber-600 font-bold">
                      <Star :size="12" class="fill-amber-400 text-amber-500" /> {{ currentMovie.preferred_item.metadata.rating }}
                    </span>
                  </div>

                  <!-- 演员列表 -->
                  <div v-if="currentMovie.preferred_item?.metadata?.cast?.length" class="flex gap-2 items-start">
                    <span class="text-base-content/50 w-12 sm:w-16 flex-shrink-0 pt-0.5 text-xs">{{ t('movies.cast') }}:</span>
                    <div class="flex flex-wrap gap-1 sm:gap-1.5">
                      <span
                        v-for="actress in currentMovie.preferred_item.metadata.cast"
                        :key="actress.name"
                        class="badge badge-secondary badge-outline text-[11px] sm:text-xs py-0.5"
                      >
                        {{ actress.name }}
                      </span>
                    </div>
                  </div>

                  <!-- 类别标签 -->
                  <div v-if="currentMovie.preferred_item?.metadata?.genres?.length" class="flex gap-2 items-start">
                    <span class="text-base-content/50 w-12 sm:w-16 flex-shrink-0 pt-0.5 text-xs">{{ t('movies.genres') }}:</span>
                    <div class="flex flex-wrap gap-1 sm:gap-1.5">
                      <span
                        v-for="genre in currentMovie.preferred_item.metadata.genres"
                        :key="genre"
                        class="badge badge-ghost text-[11px] sm:text-xs py-0.5"
                      >
                        {{ genre }}
                      </span>
                    </div>
                  </div>

                  <!-- 厂牌 / 制作商 -->
                  <div v-if="currentMovie.preferred_item?.metadata?.makers?.length || currentMovie.preferred_item?.metadata?.labels?.length" class="flex gap-2 text-xs">
                    <span class="text-base-content/50 w-12 sm:w-16 flex-shrink-0">{{ t('movies.makers') }}:</span>
                    <span class="line-clamp-1">{{ [...(currentMovie.preferred_item.metadata?.makers || []), ...(currentMovie.preferred_item.metadata?.labels || [])].join(' / ') }}</span>
                  </div>

                  <!-- 导演 -->
                  <div v-if="currentMovie.preferred_item?.metadata?.directors?.length" class="flex gap-2 text-xs">
                    <span class="text-base-content/50 w-12 sm:w-16 flex-shrink-0">{{ t('movies.directors') }}:</span>
                    <span class="line-clamp-1">{{ currentMovie.preferred_item.metadata.directors.join(', ') }}</span>
                  </div>

                  <!-- 简介（可展开） -->
                  <div v-if="currentMovie.preferred_item?.description" class="pt-1">
                    <p
                      class="text-xs text-base-content/70 leading-relaxed cursor-pointer"
                      :class="{ 'line-clamp-2': !descExpanded }"
                      @click="descExpanded = !descExpanded"
                    >
                      {{ currentMovie.preferred_item.description }}
                    </p>
                    <button
                      v-if="currentMovie.preferred_item.description.length > 80"
                      type="button"
                      class="text-[11px] text-primary mt-0.5"
                      @click="descExpanded = !descExpanded"
                    >
                      {{ descExpanded ? t('common.close') : t('common.view') }}
                    </button>
                  </div>

                  <!-- 预览截图网格展示 -->
                  <div v-if="sampleImages.length" class="pt-2">
                    <div class="text-xs text-base-content/60 font-medium mb-1.5 flex items-center justify-between">
                      <span>{{ t('movies.samples') }} ({{ sampleImages.length }})</span>
                      <span class="text-[11px] text-base-content/40 hidden sm:inline">点击放大可左右键切换</span>
                    </div>
                    <div class="grid grid-cols-4 sm:grid-cols-5 md:grid-cols-6 gap-1.5 sm:gap-2 max-h-24 sm:max-h-36 overflow-y-auto p-1 bg-base-200/50 rounded-lg">
                      <figure
                        v-for="(sampleUrl, idx) in sampleImages"
                        :key="sampleUrl"
                        class="relative aspect-[1.48] rounded overflow-hidden cursor-pointer group bg-base-300 hover:ring-2 hover:ring-primary transition-all"
                        :title="`预览图 ${idx + 1}`"
                        @click="openSampleLightbox(idx)"
                      >
                        <SmoothImage
                          :src="sampleUrl"
                          :alt="`Sample ${idx + 1}`"
                          aspect="w-full h-full"
                          object-fit="cover"
                          loading="lazy"
                          img-class="group-hover:scale-105 transition-transform"
                        >
                          <div class="absolute inset-0 bg-black/20 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center z-10">
                            <ZoomIn :size="14" class="text-white" />
                          </div>
                        </SmoothImage>
                      </figure>
                    </div>
                  </div>
                </div>
              </div>

              <!-- 3个选项操作栏与切换按键 -->
              <div class="mt-4 sm:mt-6 pt-3 sm:pt-4 border-t border-base-200">
                <div class="grid grid-cols-3 gap-2 sm:gap-3">
                  <!-- 喜欢 -->
                  <button
                    class="btn btn-success text-white shadow font-bold text-sm sm:text-base h-12 sm:h-14 px-1 sm:px-4 active:scale-95 transition-transform"
                    :disabled="submitting || loading || feedbackSaved"
                    @click="submitFeedback('like')"
                  >
                    <span v-if="submitting && lastAction === 'like'" class="loading loading-spinner loading-sm" aria-hidden="true"></span>
                    <ThumbsUp v-else :size="18" class="sm:w-5 sm:h-5 flex-shrink-0" />
                    <span class="truncate">{{ t('annotations.like') }}</span>
                    <kbd class="hidden sm:inline-flex kbd kbd-sm bg-success-content/20 text-white ml-1">1</kbd>
                  </button>

                  <!-- 不确定 -->
                  <button
                    class="btn btn-outline border-base-content/30 text-sm sm:text-base h-12 sm:h-14 px-1 sm:px-4 active:scale-95 transition-transform"
                    :disabled="submitting || loading || feedbackSaved"
                    @click="submitFeedback('uncertain')"
                  >
                    <span v-if="submitting && lastAction === 'uncertain'" class="loading loading-spinner loading-sm" aria-hidden="true"></span>
                    <HelpCircle v-else :size="18" class="sm:w-5 sm:h-5 flex-shrink-0" />
                    <span class="truncate">{{ t('annotations.uncertain') }}</span>
                    <kbd class="hidden sm:inline-flex kbd kbd-sm bg-base-300 ml-1">2</kbd>
                  </button>

                  <!-- 不喜欢 -->
                  <button
                    class="btn btn-error text-white shadow font-bold text-sm sm:text-base h-12 sm:h-14 px-1 sm:px-4 active:scale-95 transition-transform"
                    :disabled="submitting || loading || feedbackSaved"
                    @click="submitFeedback('dislike')"
                  >
                    <span v-if="submitting && lastAction === 'dislike'" class="loading loading-spinner loading-sm" aria-hidden="true"></span>
                    <ThumbsDown v-else :size="18" class="sm:w-5 sm:h-5 flex-shrink-0" />
                    <span class="truncate">{{ t('annotations.dislike') }}</span>
                    <kbd class="hidden sm:inline-flex kbd kbd-sm bg-error-content/20 text-white ml-1">3</kbd>
                  </button>
                </div>

              </div>
            </div>
          </div>
        </div>
      </Transition>
      </div>

    </div>

    <!-- 高清大图画廊（支持封面与所有预览图左右切换） -->
    <ImageLightbox
      :open="lightboxOpen"
      :images="galleryImages"
      :initial-index="lightboxIndex"
      @close="lightboxOpen = false"
    />
  </div>
</template>

<style scoped>
.recommendation-stage {
  display: grid;
}

.recommendation-stage > * {
  grid-area: 1 / 1;
}

.recommendation-card-enter-active,
.recommendation-card-leave-active {
  transition: opacity 0.22s ease, transform 0.22s ease;
}

.recommendation-card-enter-from,
.recommendation-card-leave-to {
  opacity: 0;
  transform: translateY(8px);
}

.recommendation-card-leave-active {
  pointer-events: none;
}

@media (prefers-reduced-motion: reduce) {
  .recommendation-card-enter-active,
  .recommendation-card-leave-active {
    transition: none;
  }
}
</style>
