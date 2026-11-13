<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowRight, CheckCircle2, Clock, ExternalLink, HelpCircle, RefreshCw, Sparkles, Star, ThumbsDown, ThumbsUp, ZoomIn } from 'lucide-vue-next'
import { api, jsonBody } from '../api/client'
import type { AnnotationStats, FeedbackResponse, Movie, NextAnnotationResponse, AnnotationCandidatesResponse } from '../types'
import ImageLightbox, { type LightboxItem } from '../components/ImageLightbox.vue'
import SmoothImage from '../components/SmoothImage.vue'
import MoviePlayer from '../components/MoviePlayer.vue'
import { MoviePreload } from '../components/moviePreload'

const { t } = useI18n()

const movie = ref<Movie | null>(null)
const preloaded = new MoviePreload()
let prefetchVersion = 0
let disposed = false

async function prefetchCandidates(currentId: string) {
  if (!currentId || preloaded.ids().length >= 2) return
  const version = ++prefetchVersion
  try {
    const result = await api<AnnotationCandidatesResponse>(`/annotations/candidates?exclude=${encodeURIComponent(currentId)}`)
    if (!disposed && version === prefetchVersion && movie.value?.normalized_id === currentId) preloaded.add(result.data.movies, currentId)
  } catch { /* 预取失败时保留原有获取流程 */ }
}

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
const feedbackSaved = ref(false)
const error = ref(false)
const descExpanded = ref(false)

// 记录最近一次的交互动作，用于触发对应的卡片滑动离开过渡动画
const lastAction = ref<'like' | 'dislike' | 'uncertain' | 'skip' | null>(null)

// 弹窗画廊状态
const lightboxOpen = ref(false)
const lightboxIndex = ref(0)

const progressPercent = computed(() => {
  if (!stats.value.min_required) return 100
  return Math.min(100, Math.round((stats.value.total / stats.value.min_required) * 100))
})

const remainingToReady = computed(() => {
  return Math.max(0, stats.value.min_required - stats.value.total)
})

const sampleImages = computed<string[]>(() => {
  const meta = movie.value?.preferred_item?.metadata
  if (meta?.sample_image_urls?.length) {
    return meta.sample_image_urls.filter(Boolean)
  }
  return []
})

const galleryImages = computed<LightboxItem[]>(() => {
  const list: LightboxItem[] = []
  const coverUrl = movie.value?.preferred_item?.cover_image_url || movie.value?.cover_image_url
  if (coverUrl) {
    list.push({ url: coverUrl, title: `${movie.value?.normalized_id || ''} - 封面` })
  }
  for (let i = 0; i < sampleImages.value.length; i++) {
    list.push({
      url: sampleImages.value[i],
      title: `${movie.value?.normalized_id || ''} - 预览图 ${i + 1}`,
    })
  }
  return list
})

// 根据用户的打分动作决定切卡时的过渡动画效果
const transitionName = computed(() => {
  if (lastAction.value === 'like') return 'card-swipe-like'
  if (lastAction.value === 'dislike') return 'card-swipe-dislike'
  if (lastAction.value === 'uncertain' || lastAction.value === 'skip') return 'card-swipe-up'
  return 'card-fade'
})

function onTransitionAfterEnter() {
  lastAction.value = null
}

function openCoverLightbox() {
  const coverUrl = movie.value?.preferred_item?.cover_image_url || movie.value?.cover_image_url
  if (!coverUrl) return
  lightboxIndex.value = 0
  lightboxOpen.value = true
}

function openSampleLightbox(index: number) {
  const coverUrl = movie.value?.preferred_item?.cover_image_url || movie.value?.cover_image_url
  const offset = coverUrl ? 1 : 0
  lightboxIndex.value = offset + index
  lightboxOpen.value = true
}

async function fetchNext(excludeId = '') {
  loading.value = true
  error.value = false
  descExpanded.value = false
  lightboxOpen.value = false
  try {
    const url = excludeId ? `/annotations/next?exclude=${encodeURIComponent(excludeId)}` : '/annotations/next'
    const res = await api<NextAnnotationResponse>(url)
    movie.value = res.data.movie
    stats.value = res.data.stats
    feedbackSaved.value = false
    void prefetchCandidates(movie.value?.normalized_id || '')
  } catch {
    error.value = true
  } finally {
    loading.value = false
  }
}

async function skipMovie() {
  if (submitting.value || loading.value || !movie.value) return
  submitting.value = true
  lastAction.value = 'skip'
  try {
    const ids = preloaded.ids()
    const valid = ids.length ? (await api<string[]>('/annotations/candidates/check', { method: 'POST', body: jsonBody({ ids }) })).data : []
    const next = preloaded.takeEligible(valid)
    if (next) {
      movie.value = next
      feedbackSaved.value = false
      descExpanded.value = false
      lightboxOpen.value = false
      void prefetchCandidates(next.normalized_id)
    } else await fetchNext(movie.value.normalized_id)
  } catch { error.value = true } finally { submitting.value = false }
}

async function submitAction(action: 'like' | 'dislike' | 'uncertain') {
  if (!movie.value || submitting.value || loading.value || feedbackSaved.value) return
  submitting.value = true
  lastAction.value = action
  error.value = false
  lightboxOpen.value = false
  const currentId = movie.value.normalized_id
  const candidateIds = preloaded.ids()
  try {
    const res = await api<FeedbackResponse>('/annotations/feedback', {
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
    descExpanded.value = false
    const next = preloaded.takeEligible(res.data.eligible_ids || [])
    if (next) {
      movie.value = next
      feedbackSaved.value = false
      void prefetchCandidates(next.normalized_id)
    } else {
      await fetchNext(currentId)
    }
  } catch {
    error.value = true
  } finally {
    submitting.value = false
  }
}

function handleKeyDown(event: KeyboardEvent) {
  // 弹窗开启时不触发快捷键，交由画廊处理
  if (lightboxOpen.value) return

  const target = event.target as HTMLElement
  if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) {
    return
  }

  if (event.code === 'Digit1' || event.key === '1') {
    event.preventDefault()
    void submitAction('like')
  } else if (event.code === 'Digit2' || event.key === '2') {
    event.preventDefault()
    void submitAction('uncertain')
  } else if (event.code === 'Digit3' || event.key === '3') {
    event.preventDefault()
    void submitAction('dislike')
  } else if (event.code === 'Space') {
    event.preventDefault()
    void skipMovie()
  }
}

onMounted(() => {
  void fetchNext()
  window.addEventListener('keydown', handleKeyDown)
})

onBeforeUnmount(() => {
  disposed = true
  prefetchVersion++
  preloaded.clear()
  window.removeEventListener('keydown', handleKeyDown)
})
</script>

<template>
  <div class="max-w-6xl mx-auto px-3 sm:px-4 py-4 sm:py-6">
    <!-- 顶部标题与说明 -->
    <div class="mb-4 sm:mb-6 flex flex-col sm:flex-row sm:items-end justify-between gap-3 sm:gap-4">
      <div>
        <h1 class="text-2xl sm:text-3xl font-bold flex items-center gap-2">
          <span>{{ t('annotations.title') }}</span>
        </h1>
        <p class="text-xs sm:text-sm text-base-content/70 mt-1">{{ t('annotations.subtitle') }}</p>
      </div>

      <RouterLink
        v-if="stats.ready"
        to="/recommendations"
        class="btn btn-primary btn-sm sm:btn-md gap-2 shadow-sm self-start sm:self-auto"
      >
        <Sparkles :size="16" class="sm:w-[18px] sm:h-[18px]" />
        <span>{{ t('annotations.goToRecommendations') }}</span>
        <ArrowRight :size="15" class="sm:w-[16px] sm:h-[16px]" />
      </RouterLink>
    </div>

    <!-- 统计卡片与进度条 -->
    <div class="card bg-base-200 shadow-sm p-3 sm:p-4 mb-4 sm:mb-6">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 sm:gap-4 mb-2.5">
        <div class="flex flex-wrap items-center gap-2 sm:gap-3 text-xs sm:text-sm font-medium">
          <span class="badge badge-sm sm:badge-md badge-neutral">
            {{ t('annotations.totalAnnotated', { count: stats.total }) }}
          </span>
          <span class="text-success flex items-center gap-1">
            <ThumbsUp :size="14" /> {{ t('annotations.likedCount', { count: stats.liked }) }}
          </span>
          <span class="text-error flex items-center gap-1">
            <ThumbsDown :size="14" /> {{ t('annotations.dislikedCount', { count: stats.disliked }) }}
          </span>
          <span class="text-base-content/60 flex items-center gap-1">
            <HelpCircle :size="14" /> {{ t('annotations.uncertainCount', { count: stats.uncertain }) }}
          </span>
        </div>

        <div v-if="stats.ready" class="text-success font-medium flex items-center gap-1 text-xs sm:text-sm">
          <CheckCircle2 :size="16" />
          <span>{{ t('annotations.readyTip') }}</span>
        </div>
        <div v-else class="text-warning font-medium text-xs sm:text-sm">
          {{ t('annotations.progressTip', { remaining: remainingToReady }) }}
        </div>
      </div>

      <!-- 进度条 -->
      <div class="w-full bg-base-300 rounded-full h-2 sm:h-2.5 overflow-hidden">
        <div
          class="h-full rounded-full transition-all duration-300"
          :class="stats.ready ? 'bg-success' : 'bg-primary'"
          :style="{ width: `${progressPercent}%` }"
        ></div>
      </div>
    </div>

    <!-- 错误提醒 -->
    <div v-if="error" class="alert alert-error mb-4 sm:mb-6 shadow-sm">
      <span>{{ t('common.error') }}</span>
      <button class="btn btn-sm btn-ghost" @click="fetchNext()">{{ t('common.retry') }}</button>
    </div>

    <!-- 主标注卡片 -->
    <div v-if="loading && !movie" class="flex justify-center items-center py-24">
      <span class="loading loading-spinner loading-lg text-primary"></span>
    </div>

    <div v-else-if="!movie" class="card bg-base-100 shadow-md p-8 sm:p-12 text-center">
      <p class="text-base sm:text-lg text-base-content/60">{{ t('annotations.noMovies') }}</p>
      <div class="mt-4">
        <button class="btn btn-outline gap-2" @click="fetchNext()">
          <RefreshCw :size="16" />
          <span>{{ t('common.retry') }}</span>
        </button>
      </div>
    </div>

    <!-- 卡片平滑切换过渡动效 -->
    <Transition
      v-else
      :name="transitionName"
      mode="out-in"
      @after-enter="onTransitionAfterEnter"
    >
      <div
        :key="movie.normalized_id"
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
                :src="movie.preferred_item?.cover_image_url || movie.cover_image_url"
                :alt="movie.preferred_item?.title || movie.normalized_id"
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
                    {{ movie.normalized_id }}
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
                  :to="`/movies/${movie.normalized_id}`"
                  target="_blank"
                  class="link link-hover text-lg sm:text-2xl font-bold line-clamp-2"
                  :title="movie.preferred_item?.title || movie.normalized_id"
                >
                  {{ movie.preferred_item?.title || movie.normalized_id }}
                </RouterLink>

                <RouterLink
                  :to="`/movies/${movie.normalized_id}`"
                  target="_blank"
                  class="btn btn-ghost btn-xs flex-shrink-0 gap-1 text-base-content/60"
                  :title="t('movies.visit')"
                >
                  <ExternalLink :size="14" />
                </RouterLink>
              </div>

              <MoviePlayer :id="movie.normalized_id" />

              <!-- 详细属性 -->
              <div class="space-y-2.5 sm:space-y-3 mt-3 sm:mt-4 text-xs sm:text-sm text-base-content/80">
                <!-- 发行日期与时长与评分 -->
                <div class="flex flex-wrap items-center gap-2.5 sm:gap-4 text-xs font-medium">
                  <span v-if="movie.release_date || movie.preferred_item?.release_date" class="text-base-content/70">
                    {{ t('common.date') }}: {{ (movie.release_date || movie.preferred_item?.release_date || '').slice(0, 10) }}
                  </span>
                  <span v-if="movie.preferred_item?.metadata?.volume_minutes" class="flex items-center gap-1 text-base-content/70">
                    <Clock :size="12" /> {{ movie.preferred_item.metadata.volume_minutes }} {{ t('movies.minutes') }}
                  </span>
                  <span v-if="movie.preferred_item?.metadata?.rating != null" class="flex items-center gap-1 text-amber-600 font-bold">
                    <Star :size="12" class="fill-amber-400 text-amber-500" /> {{ movie.preferred_item.metadata.rating }}
                  </span>
                </div>

                <!-- 演员列表 -->
                <div v-if="movie.preferred_item?.metadata?.cast?.length" class="flex gap-2 items-start">
                  <span class="text-base-content/50 w-12 sm:w-16 flex-shrink-0 pt-0.5 text-xs">{{ t('movies.cast') }}:</span>
                  <div class="flex flex-wrap gap-1 sm:gap-1.5">
                    <span
                      v-for="actress in movie.preferred_item.metadata.cast"
                      :key="actress.name"
                      class="badge badge-secondary badge-outline text-[11px] sm:text-xs py-0.5"
                    >
                      {{ actress.name }}
                    </span>
                  </div>
                </div>

                <!-- 类别标签 -->
                <div v-if="movie.preferred_item?.metadata?.genres?.length" class="flex gap-2 items-start">
                  <span class="text-base-content/50 w-12 sm:w-16 flex-shrink-0 pt-0.5 text-xs">{{ t('movies.genres') }}:</span>
                  <div class="flex flex-wrap gap-1 sm:gap-1.5">
                    <span
                      v-for="genre in movie.preferred_item.metadata.genres"
                      :key="genre"
                      class="badge badge-ghost text-[11px] sm:text-xs py-0.5"
                    >
                      {{ genre }}
                    </span>
                  </div>
                </div>

                <!-- 厂牌 / 制作商 -->
                <div v-if="movie.preferred_item?.metadata?.makers?.length || movie.preferred_item?.metadata?.labels?.length" class="flex gap-2 text-xs">
                  <span class="text-base-content/50 w-12 sm:w-16 flex-shrink-0">{{ t('movies.makers') }}:</span>
                  <span class="line-clamp-1">{{ [...(movie.preferred_item.metadata?.makers || []), ...(movie.preferred_item.metadata?.labels || [])].join(' / ') }}</span>
                </div>

                <!-- 导演 -->
                <div v-if="movie.preferred_item?.metadata?.directors?.length" class="flex gap-2 text-xs">
                  <span class="text-base-content/50 w-12 sm:w-16 flex-shrink-0">{{ t('movies.directors') }}:</span>
                  <span class="line-clamp-1">{{ movie.preferred_item.metadata.directors.join(', ') }}</span>
                </div>

                <!-- 简介（可展开） -->
                <div v-if="movie.preferred_item?.description" class="pt-1">
                  <p
                    class="text-xs text-base-content/70 leading-relaxed cursor-pointer"
                    :class="{ 'line-clamp-2': !descExpanded }"
                    @click="descExpanded = !descExpanded"
                  >
                    {{ movie.preferred_item.description }}
                  </p>
                  <button
                    v-if="movie.preferred_item.description.length > 80"
                    type="button"
                    class="text-[11px] text-primary mt-0.5"
                    @click="descExpanded = !descExpanded"
                  >
                    {{ descExpanded ? t('common.close') : t('common.view') }}
                  </button>
                </div>

                <!-- 预览截图网格展示（可点击弹窗放大并左右切换，使用平滑渐变加载） -->
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

            <!-- 3个选项操作栏 -->
            <div class="mt-4 sm:mt-6 pt-3 sm:pt-4 border-t border-base-200">
              <div class="grid grid-cols-3 gap-2 sm:gap-3">
                <!-- 喜欢 -->
                <button
                  class="btn btn-success text-white shadow font-bold text-sm sm:text-base h-12 sm:h-14 px-1 sm:px-4 active:scale-95 transition-transform"
                  :disabled="submitting || feedbackSaved"
                  @click="submitAction('like')"
                >
                  <ThumbsUp :size="18" class="sm:w-5 sm:h-5 flex-shrink-0" />
                  <span class="truncate">{{ t('annotations.like') }}</span>
                  <kbd class="hidden sm:inline-flex kbd kbd-sm bg-success-content/20 text-white ml-1">1</kbd>
                </button>

                <!-- 不确定 -->
                <button
                  class="btn btn-outline border-base-content/30 text-sm sm:text-base h-12 sm:h-14 px-1 sm:px-4 active:scale-95 transition-transform"
                  :disabled="submitting || feedbackSaved"
                  @click="submitAction('uncertain')"
                >
                  <HelpCircle :size="18" class="sm:w-5 sm:h-5 flex-shrink-0" />
                  <span class="truncate">{{ t('annotations.uncertain') }}</span>
                  <kbd class="hidden sm:inline-flex kbd kbd-sm bg-base-300 ml-1">2</kbd>
                </button>

                <!-- 不喜欢 -->
                <button
                  class="btn btn-error text-white shadow font-bold text-sm sm:text-base h-12 sm:h-14 px-1 sm:px-4 active:scale-95 transition-transform"
                  :disabled="submitting || feedbackSaved"
                  @click="submitAction('dislike')"
                >
                  <ThumbsDown :size="18" class="sm:w-5 sm:h-5 flex-shrink-0" />
                  <span class="truncate">{{ t('annotations.dislike') }}</span>
                  <kbd class="hidden sm:inline-flex kbd kbd-sm bg-error-content/20 text-white ml-1">3</kbd>
                </button>
              </div>

              <!-- 跳过换一个与快捷键提示 -->
              <div class="flex items-center justify-between mt-2.5 sm:mt-3 text-xs text-base-content/60">
                <span class="hidden sm:inline">{{ t('annotations.shortcutHint') }}</span>
                <span class="sm:hidden text-[11px] text-base-content/40">点击按键即可记录偏好</span>
                <button
                  class="btn btn-ghost btn-xs gap-1 ml-auto"
                  :disabled="submitting"
                  @click="skipMovie"
                >
                  <RefreshCw :size="12" :class="{ 'animate-spin': submitting }" />
                  <span>{{ t('annotations.skip') }}</span>
                  <span class="hidden sm:inline">(Space)</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>

    <!-- 高清大图弹窗画廊（支持封面与所有预览图左右切换） -->
    <ImageLightbox
      :open="lightboxOpen"
      :images="galleryImages"
      :initial-index="lightboxIndex"
      @close="lightboxOpen = false"
    />
  </div>
</template>

<style scoped>
/* 卡片切换过渡动效 */
.card-swipe-like-leave-active,
.card-swipe-dislike-leave-active,
.card-swipe-up-leave-active,
.card-fade-leave-active {
  transition: opacity 0.26s cubic-bezier(0.4, 0, 1, 1), transform 0.26s cubic-bezier(0.4, 0, 1, 1);
}

.card-swipe-like-enter-active,
.card-swipe-dislike-enter-active,
.card-swipe-up-enter-active,
.card-fade-enter-active {
  transition: opacity 0.3s cubic-bezier(0.16, 1, 0.3, 1), transform 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}

/* 喜欢操作：向右微倾斜滑出 */
.card-swipe-like-leave-to {
  opacity: 0;
  transform: translate3d(50px, -8px, 0) rotate(2.5deg) scale(0.96);
}

/* 不喜欢操作：向左微倾斜滑出 */
.card-swipe-dislike-leave-to {
  opacity: 0;
  transform: translate3d(-50px, -8px, 0) rotate(-2.5deg) scale(0.96);
}

/* 不确定或跳过：向上平滑漂移浮出 */
.card-swipe-up-leave-to {
  opacity: 0;
  transform: translate3d(0, -22px, 0) scale(0.97);
}

.card-fade-leave-to {
  opacity: 0;
  transform: scale(0.98);
}

/* 新卡片进入：从底部轻微浮入并恢复正常大小 */
.card-swipe-like-enter-from,
.card-swipe-dislike-enter-from,
.card-swipe-up-enter-from,
.card-fade-enter-from {
  opacity: 0;
  transform: translate3d(0, 20px, 0) scale(0.97);
}

.card-swipe-like-enter-to,
.card-swipe-dislike-enter-to,
.card-swipe-up-enter-to,
.card-fade-enter-to {
  opacity: 1;
  transform: translate3d(0, 0, 0) scale(1);
}
</style>
