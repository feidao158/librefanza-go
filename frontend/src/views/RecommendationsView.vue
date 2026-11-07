<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowRight, Check, CheckCircle2, ChevronLeft, ChevronRight, Clock, ExternalLink, HelpCircle, Layers, RefreshCw, Sparkles, Star, ThumbsDown, ThumbsUp, ZoomIn } from 'lucide-vue-next'
import { api, jsonBody } from '../api/client'
import type { AnnotationStats, FeedbackResponse, Movie, RecommendationResponse } from '../types'
import ImageLightbox, { type LightboxItem } from '../components/ImageLightbox.vue'
import SmoothImage from '../components/SmoothImage.vue'

const { t } = useI18n()

const movies = ref<Movie[]>([])
const currentIndex = ref(0)
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
const refreshing = ref(false)
const error = ref(false)
const descExpanded = ref(false)

// 记录卡片上的交互状态 (normalized_id -> 'like' | 'dislike' | 'uncertain')
const cardFeedback = ref<Record<string, 'like' | 'dislike' | 'uncertain'>>({})

// 记录最近一次切换动作以触发卡片滑动离开过渡动效
const lastAction = ref<'like' | 'dislike' | 'uncertain' | 'skip' | 'prev' | null>(null)

// 弹窗画廊状态
const lightboxOpen = ref(false)
const lightboxIndex = ref(0)

// 底部推荐队列横向滚动容器
const queueContainer = ref<HTMLElement | null>(null)

const progressPercent = computed(() => {
  if (!stats.value.min_required) return 100
  return Math.min(100, Math.round((stats.value.total / stats.value.min_required) * 100))
})

// 当前展示的精选大屏推荐影片
const currentMovie = computed<Movie | null>(() => {
  if (movies.value.length === 0) return null
  return movies.value[currentIndex.value] || movies.value[0] || null
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

// 根据用户的切卡或评分动作触发对应过渡动画
const transitionName = computed(() => {
  if (lastAction.value === 'like') return 'card-swipe-like'
  if (lastAction.value === 'dislike') return 'card-swipe-dislike'
  if (lastAction.value === 'prev') return 'card-swipe-prev'
  if (lastAction.value === 'uncertain' || lastAction.value === 'skip') return 'card-swipe-up'
  return 'card-fade'
})

function onTransitionAfterEnter() {
  lastAction.value = null
}

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

// 自动滚动底部队列将当前选中的影片居中
function scrollQueueActiveIntoView() {
  void nextTick(() => {
    if (!queueContainer.value) return
    const activeEl = queueContainer.value.querySelector('[data-active="true"]') as HTMLElement | null
    if (activeEl && typeof activeEl.scrollIntoView === 'function') {
      activeEl.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'center' })
    }
  })
}

watch(currentIndex, () => {
  descExpanded.value = false
  scrollQueueActiveIntoView()
})

async function loadRecommendations(isRefresh = false) {
  if (isRefresh) {
    refreshing.value = true
  } else {
    loading.value = true
  }
  error.value = false
  try {
    const res = await api<RecommendationResponse>('/recommendations?page_size=24')
    stats.value = res.data.stats
    if (res.data.ready) {
      movies.value = res.data.movies
      currentIndex.value = 0
    } else {
      movies.value = []
    }
  } catch {
    error.value = true
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

// 切换到下一部推荐影片
function nextMovie() {
  if (movies.value.length === 0 || submitting.value) return
  lastAction.value = 'skip'
  if (currentIndex.value < movies.value.length - 1) {
    currentIndex.value++
  } else {
    currentIndex.value = 0
  }
}

// 切换到上一部推荐影片
function prevMovie() {
  if (movies.value.length === 0 || submitting.value) return
  lastAction.value = 'prev'
  if (currentIndex.value > 0) {
    currentIndex.value--
  } else {
    currentIndex.value = movies.value.length - 1
  }
}

// 跳转到指定推荐条目
function jumpTo(index: number) {
  if (index === currentIndex.value || submitting.value) return
  lastAction.value = index > currentIndex.value ? 'skip' : 'prev'
  currentIndex.value = index
}

// 提交喜欢 / 不喜欢 / 不确定反馈，并丝滑切换至下一部
async function submitFeedback(action: 'like' | 'dislike' | 'uncertain') {
  if (!currentMovie.value || submitting.value) return
  submitting.value = true
  lastAction.value = action
  const currentId = currentMovie.value.normalized_id
  cardFeedback.value[currentId] = action

  try {
    const res = await api<FeedbackResponse>('/recommendations/feedback', {
      method: 'POST',
      body: jsonBody({
        normalized_id: currentId,
        action,
      }),
    })
    stats.value = res.data.stats

    // 评分后自动平滑推进至下一部
    if (currentIndex.value < movies.value.length - 1) {
      currentIndex.value++
    } else if (res.data.next_movie) {
      movies.value.push(res.data.next_movie)
      currentIndex.value++
    } else {
      currentIndex.value = 0
    }
  } catch {
    // 忽略反馈失败，继续保持交互流畅
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
  } else if (event.code === 'Space' || event.key === 'ArrowRight') {
    event.preventDefault()
    nextMovie()
  } else if (event.key === 'ArrowLeft') {
    event.preventDefault()
    prevMovie()
  }
}

onMounted(() => {
  void loadRecommendations()
  window.addEventListener('keydown', handleKeyDown)
})

onBeforeUnmount(() => {
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

        <button
          v-if="stats.ready"
          class="btn btn-primary btn-sm gap-1.5 shadow-sm flex-1 sm:flex-initial justify-center"
          :disabled="refreshing"
          @click="loadRecommendations(true)"
        >
          <RefreshCw :size="15" :class="{ 'animate-spin': refreshing }" />
          <span>{{ t('recommendations.refresh') }}</span>
        </button>
      </div>
    </div>

    <!-- 错误提示 -->
    <div v-if="error" class="alert alert-error mb-4 sm:mb-6 shadow-sm">
      <span>{{ t('common.error') }}</span>
      <button class="btn btn-sm btn-ghost" @click="loadRecommendations()">{{ t('common.retry') }}</button>
    </div>

    <!-- 全局加载状态 -->
    <div v-if="loading" class="flex justify-center items-center py-24">
      <span class="loading loading-spinner loading-lg text-primary"></span>
    </div>

    <!-- 情况 1：未达到标注门槛，展示学习引导卡片 -->
    <div v-else-if="!stats.ready" class="card bg-base-100 shadow-md border border-base-300 p-6 sm:p-8 md:p-12 text-center max-w-2xl mx-auto">
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
    <div v-else>
      <!-- 顶部推荐状态与快捷切换条 -->
      <div class="card bg-base-200 shadow-sm p-3 sm:p-4 mb-4 sm:mb-6 flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 sm:gap-4">
        <div class="flex flex-wrap items-center gap-2 sm:gap-3 text-xs sm:text-sm font-medium">
          <span v-if="movies.length" class="badge badge-sm sm:badge-md badge-neutral font-mono">
            {{ t('recommendations.currentIndex', { current: currentIndex + 1, total: movies.length }) }}
          </span>
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

        <div class="flex items-center gap-1.5 self-end sm:self-auto">
          <button
            class="btn btn-ghost btn-xs sm:btn-sm gap-1"
            :disabled="movies.length <= 1 || submitting"
            :title="t('common.prevMovie')"
            @click="prevMovie"
          >
            <ChevronLeft :size="16" />
            <span class="hidden sm:inline">{{ t('common.prevMovie') }}</span>
          </button>
          <button
            class="btn btn-ghost btn-xs sm:btn-sm gap-1"
            :disabled="movies.length <= 1 || submitting"
            :title="t('common.nextMovie')"
            @click="nextMovie"
          >
            <span class="hidden sm:inline">{{ t('common.nextMovie') }}</span>
            <ChevronRight :size="16" />
          </button>
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
      <Transition
        v-else
        :name="transitionName"
        mode="out-in"
        @after-enter="onTransitionAfterEnter"
      >
        <div
          :key="currentMovie.normalized_id"
          class="card bg-base-100 shadow-lg border border-base-300 overflow-hidden relative"
        >
          <div class="grid grid-cols-1 md:grid-cols-12 gap-4 sm:gap-6 p-4 sm:p-6">
            <!-- 封面大图（可点击弹窗查看高清大图，使用平滑渐显加载） -->
            <div class="md:col-span-5 flex flex-col items-center">
              <div
                class="w-full relative aspect-[2/3] max-w-[200px] sm:max-w-[260px] md:max-w-[340px] bg-base-200 rounded-lg overflow-hidden shadow-inner flex items-center justify-center cursor-pointer group"
                :title="t('common.previewDetails')"
                @click="openCoverLightbox"
              >
                <SmoothImage
                  :src="currentMovie.preferred_item?.cover_image_url || currentMovie.cover_image_url"
                  :alt="currentMovie.preferred_item?.title || currentMovie.normalized_id"
                  aspect="w-full h-full"
                  :fallback-text="t('movies.noCover')"
                  loading="eager"
                  img-class="group-hover:scale-105 transition-transform duration-300"
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

                  <!-- 已打分标记角标 -->
                  <div v-if="cardFeedback[currentMovie.normalized_id]" class="absolute bottom-2.5 right-2.5 z-10">
                    <span
                      class="badge badge-sm sm:badge-md shadow font-semibold"
                      :class="{
                        'badge-success text-white': cardFeedback[currentMovie.normalized_id] === 'like',
                        'badge-error text-white': cardFeedback[currentMovie.normalized_id] === 'dislike',
                        'badge-neutral': cardFeedback[currentMovie.normalized_id] === 'uncertain'
                      }"
                    >
                      <Check :size="12" class="mr-0.5" />
                      {{
                        cardFeedback[currentMovie.normalized_id] === 'like' ? t('annotations.like') :
                        cardFeedback[currentMovie.normalized_id] === 'dislike' ? t('annotations.dislike') : t('annotations.uncertain')
                      }}
                    </span>
                  </div>

                  <!-- 点击看大图提示浮层 -->
                  <div class="absolute inset-0 bg-black/30 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center z-10">
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
                    :class="{ 'ring-2 ring-success ring-offset-2': cardFeedback[currentMovie.normalized_id] === 'like' }"
                    :disabled="submitting"
                    @click="submitFeedback('like')"
                  >
                    <ThumbsUp :size="18" class="sm:w-5 sm:h-5 flex-shrink-0" />
                    <span class="truncate">{{ t('annotations.like') }}</span>
                    <kbd class="hidden sm:inline-flex kbd kbd-sm bg-success-content/20 text-white ml-1">1</kbd>
                  </button>

                  <!-- 不确定 -->
                  <button
                    class="btn btn-outline border-base-content/30 text-sm sm:text-base h-12 sm:h-14 px-1 sm:px-4 active:scale-95 transition-transform"
                    :class="{ 'bg-base-300': cardFeedback[currentMovie.normalized_id] === 'uncertain' }"
                    :disabled="submitting"
                    @click="submitFeedback('uncertain')"
                  >
                    <HelpCircle :size="18" class="sm:w-5 sm:h-5 flex-shrink-0" />
                    <span class="truncate">{{ t('annotations.uncertain') }}</span>
                    <kbd class="hidden sm:inline-flex kbd kbd-sm bg-base-300 ml-1">2</kbd>
                  </button>

                  <!-- 不喜欢 -->
                  <button
                    class="btn btn-error text-white shadow font-bold text-sm sm:text-base h-12 sm:h-14 px-1 sm:px-4 active:scale-95 transition-transform"
                    :class="{ 'ring-2 ring-error ring-offset-2': cardFeedback[currentMovie.normalized_id] === 'dislike' }"
                    :disabled="submitting"
                    @click="submitFeedback('dislike')"
                  >
                    <ThumbsDown :size="18" class="sm:w-5 sm:h-5 flex-shrink-0" />
                    <span class="truncate">{{ t('annotations.dislike') }}</span>
                    <kbd class="hidden sm:inline-flex kbd kbd-sm bg-error-content/20 text-white ml-1">3</kbd>
                  </button>
                </div>

                <!-- 切换下一个 / 快捷键提示 -->
                <div class="flex items-center justify-between mt-2.5 sm:mt-3 text-xs text-base-content/60">
                  <span class="hidden sm:inline">{{ t('recommendations.shortcutHint') }}</span>
                  <span class="sm:hidden text-[11px] text-base-content/40">点击按键或左右滑动</span>

                  <div class="flex items-center gap-1 ml-auto">
                    <button
                      class="btn btn-ghost btn-xs gap-1"
                      :disabled="movies.length <= 1 || submitting"
                      @click="prevMovie"
                    >
                      <ChevronLeft :size="12" />
                      <span>{{ t('common.prevMovie') }}</span>
                    </button>
                    <button
                      class="btn btn-ghost btn-xs gap-1"
                      :disabled="movies.length <= 1 || submitting"
                      @click="nextMovie"
                    >
                      <span>{{ t('common.nextMovie') }}</span>
                      <ChevronRight :size="12" />
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </Transition>

      <!-- 底部推荐影片缩略胶卷队列（可横向自由浏览点击任意一部无缝切换） -->
      <div v-if="movies.length > 1" class="mt-6">
        <div class="flex items-center justify-between text-xs text-base-content/60 mb-2 px-1">
          <span class="font-medium flex items-center gap-1.5">
            <Sparkles :size="13" class="text-primary" />
            <span>{{ t('recommendations.queueTitle') }} ({{ movies.length }})</span>
          </span>
          <span>点击缩略图直接跳转</span>
        </div>

        <div
          ref="queueContainer"
          class="flex items-center gap-2.5 overflow-x-auto py-2 px-1 scrollbar-none"
        >
          <button
            v-for="(item, idx) in movies"
            :key="item.normalized_id"
            type="button"
            class="relative flex-shrink-0 w-20 sm:w-24 rounded-lg overflow-hidden border-2 transition-all duration-200 cursor-pointer text-left bg-base-100 group shadow-xs"
            :class="currentIndex === idx
              ? 'border-primary ring-2 ring-primary/40 scale-105 shadow-md opacity-100'
              : 'border-base-300 opacity-60 hover:opacity-100 hover:scale-102'"
            :data-active="currentIndex === idx"
            @click="jumpTo(idx)"
          >
            <div class="aspect-[2/3] w-full relative bg-base-200">
              <SmoothImage
                :src="item.preferred_item?.cover_image_url || item.cover_image_url"
                :alt="item.normalized_id"
                aspect="w-full h-full"
                loading="lazy"
                img-class="w-full h-full object-cover"
              >
                <!-- 序号角标 -->
                <span class="absolute top-1 left-1 badge badge-neutral badge-xs font-mono scale-90 origin-top-left z-10">
                  {{ idx + 1 }}
                </span>

                <!-- 已反馈状态角标 -->
                <span
                  v-if="cardFeedback[item.normalized_id]"
                  class="absolute bottom-1 right-1 badge badge-xs z-10 scale-90 origin-bottom-right"
                  :class="{
                    'badge-success text-white': cardFeedback[item.normalized_id] === 'like',
                    'badge-error text-white': cardFeedback[item.normalized_id] === 'dislike',
                    'badge-neutral': cardFeedback[item.normalized_id] === 'uncertain'
                  }"
                >
                  <Check :size="10" />
                </span>
              </SmoothImage>
            </div>
            <div class="p-1">
              <div class="text-[10px] font-mono font-semibold truncate">{{ item.normalized_id }}</div>
            </div>
          </button>
        </div>
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
/* 卡片切换过渡动效 */
.card-swipe-like-leave-active,
.card-swipe-dislike-leave-active,
.card-swipe-up-leave-active,
.card-swipe-prev-leave-active,
.card-fade-leave-active {
  transition: opacity 0.26s cubic-bezier(0.4, 0, 1, 1), transform 0.26s cubic-bezier(0.4, 0, 1, 1);
}

.card-swipe-like-enter-active,
.card-swipe-dislike-enter-active,
.card-swipe-up-enter-active,
.card-swipe-prev-enter-active,
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

/* 切换上一部：向右滑出 */
.card-swipe-prev-leave-to {
  opacity: 0;
  transform: translate3d(40px, 0, 0) scale(0.97);
}

/* 不确定或跳过下一部：向上平滑漂移浮出 */
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
.card-swipe-prev-enter-from,
.card-fade-enter-from {
  opacity: 0;
  transform: translate3d(0, 20px, 0) scale(0.97);
}

.card-swipe-like-enter-to,
.card-swipe-dislike-enter-to,
.card-swipe-up-enter-to,
.card-swipe-prev-enter-to,
.card-fade-enter-to {
  opacity: 1;
  transform: translate3d(0, 0, 0) scale(1);
}
</style>
