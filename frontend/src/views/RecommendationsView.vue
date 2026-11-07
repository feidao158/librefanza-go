<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowRight, Check, CheckCircle2, Clock, ExternalLink, Eye, HelpCircle, Layers, RefreshCw, Sparkles, Star, ThumbsDown, ThumbsUp, X, ZoomIn } from 'lucide-vue-next'
import { api, jsonBody } from '../api/client'
import type { AnnotationStats, FeedbackResponse, Movie, RecommendationResponse } from '../types'
import ImageLightbox, { type LightboxItem } from '../components/ImageLightbox.vue'

const { t } = useI18n()

const movies = ref<Movie[]>([])
const stats = ref<AnnotationStats>({
  total: 0,
  liked: 0,
  disliked: 0,
  uncertain: 0,
  min_required: 5,
  ready: false,
})
const loading = ref(true)
const refreshing = ref(false)
const error = ref(false)

// 详情弹窗所选中的影片
const selectedMovie = ref<Movie | null>(null)

// 记录卡片上的交互状态 (normalized_id -> 'like' | 'dislike' | 'uncertain')
const cardFeedback = ref<Record<string, 'like' | 'dislike' | 'uncertain'>>({})

// 高清大图画廊状态
const lightboxOpen = ref(false)
const lightboxIndex = ref(0)

const progressPercent = computed(() => {
  if (!stats.value.min_required) return 100
  return Math.min(100, Math.round((stats.value.total / stats.value.min_required) * 100))
})

const selectedSampleImages = computed<string[]>(() => {
  const meta = selectedMovie.value?.preferred_item?.metadata
  if (meta?.sample_image_urls?.length) {
    return meta.sample_image_urls.filter(Boolean)
  }
  return []
})

const selectedGalleryImages = computed<LightboxItem[]>(() => {
  if (!selectedMovie.value) return []
  const list: LightboxItem[] = []
  const coverUrl = selectedMovie.value.preferred_item?.cover_image_url || selectedMovie.value.cover_image_url
  if (coverUrl) {
    list.push({ url: coverUrl, title: `${selectedMovie.value.normalized_id} - 封面` })
  }
  for (let i = 0; i < selectedSampleImages.value.length; i++) {
    list.push({
      url: selectedSampleImages.value[i],
      title: `${selectedMovie.value.normalized_id} - 预览图 ${i + 1}`,
    })
  }
  return list
})

function openLightbox(index: number) {
  lightboxIndex.value = index
  lightboxOpen.value = true
}

function openCoverLightbox() {
  const coverUrl = selectedMovie.value?.preferred_item?.cover_image_url || selectedMovie.value?.cover_image_url
  if (!coverUrl) return
  openLightbox(0)
}

function openSampleLightbox(index: number) {
  const coverUrl = selectedMovie.value?.preferred_item?.cover_image_url || selectedMovie.value?.cover_image_url
  const offset = coverUrl ? 1 : 0
  openLightbox(offset + index)
}

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

async function rateMovie(id: string, action: 'like' | 'dislike' | 'uncertain') {
  try {
    cardFeedback.value[id] = action
    const res = await api<FeedbackResponse>('/recommendations/feedback', {
      method: 'POST',
      body: jsonBody({
        normalized_id: id,
        action,
      }),
    })
    stats.value = res.data.stats
  } catch {
    // 忽略反馈失败
  }
}

function openDetailModal(item: Movie) {
  selectedMovie.value = item
}

function closeDetailModal() {
  selectedMovie.value = null
  lightboxOpen.value = false
}

onMounted(() => {
  void loadRecommendations()
})
</script>

<template>
  <div class="max-w-7xl mx-auto px-4 py-6">
    <!-- 顶部标题与控制栏 -->
    <div class="mb-6 flex flex-col md:flex-row md:items-end justify-between gap-4">
      <div>
        <h1 class="text-3xl font-bold flex items-center gap-2">
          <Sparkles class="text-primary" :size="28" />
          <span>{{ t('recommendations.title') }}</span>
        </h1>
        <p class="text-base-content/70 mt-1">{{ t('recommendations.subtitle') }}</p>
      </div>

      <div class="flex items-center gap-2">
        <RouterLink
          to="/annotations"
          class="btn btn-outline btn-sm gap-1.5"
        >
          <Layers :size="16" />
          <span>{{ t('recommendations.continueAnnotating') }}</span>
        </RouterLink>

        <button
          v-if="stats.ready"
          class="btn btn-primary btn-sm gap-1.5 shadow-sm"
          :disabled="refreshing"
          @click="loadRecommendations(true)"
        >
          <RefreshCw :size="16" :class="{ 'animate-spin': refreshing }" />
          <span>{{ t('recommendations.refresh') }}</span>
        </button>
      </div>
    </div>

    <!-- 错误提示 -->
    <div v-if="error" class="alert alert-error mb-6 shadow-sm">
      <span>{{ t('common.error') }}</span>
      <button class="btn btn-sm btn-ghost" @click="loadRecommendations()">{{ t('common.retry') }}</button>
    </div>

    <!-- 加载中 -->
    <div v-if="loading" class="flex justify-center items-center py-24">
      <span class="loading loading-spinner loading-lg text-primary"></span>
    </div>

    <!-- 情况 1：未达到标注门槛，展示学习引导卡片 -->
    <div v-else-if="!stats.ready" class="card bg-base-100 shadow-md border border-base-300 p-8 md:p-12 text-center max-w-2xl mx-auto">
      <div class="w-16 h-16 rounded-full bg-primary/10 text-primary flex items-center justify-center mx-auto mb-4">
        <Sparkles :size="32" />
      </div>
      <h2 class="text-2xl font-bold mb-3">{{ t('recommendations.notReadyTitle') }}</h2>
      <p class="text-base-content/70 mb-6 leading-relaxed">
        {{ t('recommendations.notReadyDesc', { total: stats.total, min: stats.min_required }) }}
      </p>

      <!-- 进度条 -->
      <div class="w-full bg-base-200 rounded-full h-3 mb-2 overflow-hidden">
        <div
          class="bg-primary h-3 rounded-full transition-all duration-300"
          :style="{ width: `${progressPercent}%` }"
        ></div>
      </div>
      <div class="text-xs text-base-content/50 mb-6 font-medium">
        {{ t('recommendations.progress', { total: stats.total, min: stats.min_required }) }}
      </div>

      <div>
        <RouterLink to="/annotations" class="btn btn-primary gap-2 px-6 shadow-md">
          <span>{{ t('recommendations.startAnnotating') }}</span>
          <ArrowRight :size="16" />
        </RouterLink>
      </div>
    </div>

    <!-- 情况 2：猜你喜欢已生效 -->
    <div v-else>
      <!-- 模型训练信息条 -->
      <div class="card bg-base-200 shadow-sm p-4 mb-6 flex flex-wrap items-center justify-between gap-4">
        <div class="text-sm font-medium flex items-center gap-2">
          <CheckCircle2 class="text-success" :size="18" />
          <span>{{ t('recommendations.trainingInfo', { total: stats.total, liked: stats.liked, disliked: stats.disliked }) }}</span>
        </div>
        <div class="text-xs text-base-content/60">
          {{ t('annotations.totalAnnotated', { count: stats.total }) }}
        </div>
      </div>

      <!-- 空结果提示 -->
      <div v-if="movies.length === 0" class="card bg-base-100 shadow-sm p-12 text-center border border-base-200">
        <p class="text-base-content/60">{{ t('recommendations.noRecommendations') }}</p>
        <div class="mt-4">
          <RouterLink to="/annotations" class="btn btn-outline gap-1.5">
            <span>{{ t('recommendations.startAnnotating') }}</span>
          </RouterLink>
        </div>
      </div>

      <!-- 推荐影片网格 -->
      <div v-else class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
        <div
          v-for="item in movies"
          :key="item.normalized_id"
          class="card bg-base-100 shadow-sm border border-base-200 hover:shadow-md transition-shadow overflow-hidden flex flex-col justify-between"
        >
          <div>
            <!-- 封面图（支持悬浮查看详情与点击预览） -->
            <div
              class="relative aspect-[2/3] bg-base-200 overflow-hidden group cursor-pointer"
              @click="openDetailModal(item)"
            >
              <img
                v-if="item.preferred_item?.cover_image_url || item.cover_image_url"
                :src="item.preferred_item?.cover_image_url || item.cover_image_url"
                :alt="item.preferred_item?.title || item.normalized_id"
                class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
                loading="lazy"
              />
              <div v-else class="w-full h-full flex items-center justify-center text-xs text-base-content/40">
                {{ t('movies.noCover') }}
              </div>

              <!-- 悬浮详情提示按钮 -->
              <div class="absolute inset-0 bg-black/30 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                <span class="badge badge-neutral bg-black/70 text-white gap-1 py-2 px-2.5 text-xs shadow">
                  <Eye :size="13" /> {{ t('common.previewDetails') }}
                </span>
              </div>

              <!-- 番号角标 -->
              <span class="absolute top-2 left-2 badge badge-neutral badge-sm shadow font-mono">
                {{ item.normalized_id }}
              </span>

              <!-- 已反馈角标 -->
              <span
                v-if="cardFeedback[item.normalized_id]"
                class="absolute bottom-2 right-2 badge badge-sm shadow font-semibold"
                :class="{
                  'badge-success text-white': cardFeedback[item.normalized_id] === 'like',
                  'badge-error text-white': cardFeedback[item.normalized_id] === 'dislike',
                  'badge-neutral': cardFeedback[item.normalized_id] === 'uncertain'
                }"
              >
                <Check :size="12" class="mr-0.5" />
                {{
                  cardFeedback[item.normalized_id] === 'like' ? t('annotations.like') :
                  cardFeedback[item.normalized_id] === 'dislike' ? t('annotations.dislike') : t('annotations.uncertain')
                }}
              </span>
            </div>

            <!-- 标题、演员与分类 -->
            <div class="p-3">
              <button
                type="button"
                class="text-left text-xs font-semibold line-clamp-2 link link-hover mb-1 w-full"
                :title="item.preferred_item?.title || item.normalized_id"
                @click="openDetailModal(item)"
              >
                {{ item.preferred_item?.title || item.normalized_id }}
              </button>

              <div v-if="item.preferred_item?.metadata?.cast?.length" class="text-xs text-base-content/60 line-clamp-1 mb-1">
                {{ item.preferred_item.metadata.cast.map(c => c.name).join(', ') }}
              </div>

              <!-- 类别标签小徽章 -->
              <div v-if="item.preferred_item?.metadata?.genres?.length" class="flex flex-wrap gap-1 mb-1 max-h-5 overflow-hidden">
                <span
                  v-for="genre in item.preferred_item.metadata.genres.slice(0, 2)"
                  :key="genre"
                  class="badge badge-ghost text-[10px] px-1 py-0 h-4"
                >
                  {{ genre }}
                </span>
              </div>

              <div v-if="item.release_date || item.preferred_item?.release_date" class="text-[10px] text-base-content/40">
                {{ (item.release_date || item.preferred_item?.release_date || '').slice(0, 10) }}
              </div>
            </div>
          </div>

          <!-- 推荐卡片交互按钮：喜欢 / 不喜欢 / 不确定 (持续强化训练数据) -->
          <div class="p-2 pt-0 border-t border-base-200/60 mt-1">
            <div class="grid grid-cols-3 gap-1">
              <button
                class="btn btn-ghost btn-xs text-success hover:bg-success/10 px-1"
                :class="{ 'btn-active bg-success/20': cardFeedback[item.normalized_id] === 'like' }"
                :title="t('annotations.like')"
                @click="rateMovie(item.normalized_id, 'like')"
              >
                <ThumbsUp :size="13" />
              </button>

              <button
                class="btn btn-ghost btn-xs text-base-content/50 hover:bg-base-200 px-1"
                :class="{ 'btn-active bg-base-300': cardFeedback[item.normalized_id] === 'uncertain' }"
                :title="t('annotations.uncertain')"
                @click="rateMovie(item.normalized_id, 'uncertain')"
              >
                <HelpCircle :size="13" />
              </button>

              <button
                class="btn btn-ghost btn-xs text-error hover:bg-error/10 px-1"
                :class="{ 'btn-active bg-error/20': cardFeedback[item.normalized_id] === 'dislike' }"
                :title="t('annotations.dislike')"
                @click="rateMovie(item.normalized_id, 'dislike')"
              >
                <ThumbsDown :size="13" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 猜你喜欢：影片高清详情与预览弹窗 -->
    <Teleport to="body">
      <div
        v-if="selectedMovie"
        class="fixed inset-0 z-40 bg-black/60 flex items-center justify-center p-4 backdrop-blur-xs"
        @click.self="closeDetailModal"
      >
        <div class="card bg-base-100 shadow-2xl border border-base-300 w-full max-w-4xl max-h-[90vh] overflow-y-auto relative animate-scale-in">
          <!-- 弹窗右上角关闭按钮 -->
          <button
            class="btn btn-sm btn-circle btn-ghost absolute right-3 top-3 z-10"
            @click="closeDetailModal"
          >
            <X :size="18" />
          </button>

          <div class="grid grid-cols-1 md:grid-cols-12 gap-6 p-6">
            <!-- 弹窗封面 -->
            <div class="md:col-span-5 flex flex-col items-center">
              <div
                class="w-full relative aspect-[2/3] max-w-[320px] bg-base-200 rounded-lg overflow-hidden shadow cursor-pointer group"
                :title="t('common.previewDetails')"
                @click="openCoverLightbox"
              >
                <img
                  v-if="selectedMovie.preferred_item?.cover_image_url || selectedMovie.cover_image_url"
                  :src="selectedMovie.preferred_item?.cover_image_url || selectedMovie.cover_image_url"
                  :alt="selectedMovie.preferred_item?.title || selectedMovie.normalized_id"
                  class="w-full h-full object-cover group-hover:scale-105 transition-transform"
                />
                <div v-else class="w-full h-full flex items-center justify-center text-sm text-base-content/40">
                  {{ t('movies.noCover') }}
                </div>

                <div class="absolute inset-0 bg-black/30 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                  <span class="badge badge-neutral bg-black/70 text-white gap-1 py-1.5 px-2.5 text-xs shadow">
                    <ZoomIn :size="14" /> 点击看大图
                  </span>
                </div>

                <span class="absolute top-2 left-2 badge badge-neutral shadow font-mono text-xs">
                  {{ selectedMovie.normalized_id }}
                </span>
              </div>
            </div>

            <!-- 弹窗详细信息 -->
            <div class="md:col-span-7 flex flex-col justify-between">
              <div>
                <div class="flex items-start justify-between gap-2 mb-2 pr-8">
                  <h3 class="text-xl font-bold line-clamp-2">
                    {{ selectedMovie.preferred_item?.title || selectedMovie.normalized_id }}
                  </h3>
                </div>

                <div class="space-y-2.5 text-xs text-base-content/80 mt-2">
                  <div class="flex flex-wrap items-center gap-3">
                    <span v-if="selectedMovie.release_date || selectedMovie.preferred_item?.release_date">
                      {{ t('common.date') }}: {{ (selectedMovie.release_date || selectedMovie.preferred_item?.release_date || '').slice(0, 10) }}
                    </span>
                    <span v-if="selectedMovie.preferred_item?.metadata?.volume_minutes" class="flex items-center gap-1">
                      <Clock :size="12" /> {{ selectedMovie.preferred_item.metadata.volume_minutes }} {{ t('movies.minutes') }}
                    </span>
                    <span v-if="selectedMovie.preferred_item?.metadata?.rating != null" class="flex items-center gap-1 text-amber-600 font-bold">
                      <Star :size="12" class="fill-amber-400 text-amber-500" /> {{ selectedMovie.preferred_item.metadata.rating }}
                    </span>
                  </div>

                  <!-- 演员 -->
                  <div v-if="selectedMovie.preferred_item?.metadata?.cast?.length" class="flex gap-2 items-start">
                    <span class="text-base-content/50 w-12 flex-shrink-0">{{ t('movies.cast') }}:</span>
                    <div class="flex flex-wrap gap-1">
                      <span
                        v-for="c in selectedMovie.preferred_item.metadata.cast"
                        :key="c.name"
                        class="badge badge-secondary badge-outline text-[11px]"
                      >
                        {{ c.name }}
                      </span>
                    </div>
                  </div>

                  <!-- 分类 -->
                  <div v-if="selectedMovie.preferred_item?.metadata?.genres?.length" class="flex gap-2 items-start">
                    <span class="text-base-content/50 w-12 flex-shrink-0">{{ t('movies.genres') }}:</span>
                    <div class="flex flex-wrap gap-1">
                      <span
                        v-for="g in selectedMovie.preferred_item.metadata.genres"
                        :key="g"
                        class="badge badge-ghost text-[11px]"
                      >
                        {{ g }}
                      </span>
                    </div>
                  </div>

                  <!-- 制作商 / 厂牌 -->
                  <div v-if="selectedMovie.preferred_item?.metadata?.makers?.length || selectedMovie.preferred_item?.metadata?.labels?.length" class="flex gap-2">
                    <span class="text-base-content/50 w-12 flex-shrink-0">{{ t('movies.makers') }}:</span>
                    <span>{{ [...(selectedMovie.preferred_item.metadata?.makers || []), ...(selectedMovie.preferred_item.metadata?.labels || [])].join(' / ') }}</span>
                  </div>

                  <!-- 简介 -->
                  <div v-if="selectedMovie.preferred_item?.description" class="pt-1">
                    <p class="text-[11px] text-base-content/70 line-clamp-3 leading-relaxed">
                      {{ selectedMovie.preferred_item.description }}
                    </p>
                  </div>

                  <!-- 预览截图网格（点击弹窗放大并可左右切换下一张） -->
                  <div v-if="selectedSampleImages.length" class="pt-2">
                    <div class="text-[11px] text-base-content/60 font-medium mb-1 flex items-center justify-between">
                      <span>{{ t('movies.samples') }} ({{ selectedSampleImages.length }})</span>
                      <span class="text-[10px] text-base-content/40">点击放大支持键盘左右切换</span>
                    </div>
                    <div class="grid grid-cols-4 gap-1.5 max-h-28 overflow-y-auto p-1 bg-base-200/50 rounded">
                      <figure
                        v-for="(sampleUrl, idx) in selectedSampleImages"
                        :key="sampleUrl"
                        class="relative aspect-[1.48] rounded overflow-hidden cursor-pointer group bg-base-300 hover:ring-2 hover:ring-primary transition-all"
                        @click="openSampleLightbox(idx)"
                      >
                        <img
                          :src="sampleUrl"
                          class="w-full h-full object-cover group-hover:scale-105 transition-transform"
                          loading="lazy"
                          referrerpolicy="no-referrer"
                        />
                        <div class="absolute inset-0 bg-black/20 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                          <ZoomIn :size="12" class="text-white" />
                        </div>
                      </figure>
                    </div>
                  </div>
                </div>
              </div>

              <!-- 弹窗底部操作按钮 -->
              <div class="mt-4 pt-3 border-t border-base-200 flex flex-wrap items-center justify-between gap-3">
                <div class="flex items-center gap-2">
                  <button
                    class="btn btn-sm btn-success text-white gap-1"
                    :class="{ 'ring-2 ring-success ring-offset-2': cardFeedback[selectedMovie.normalized_id] === 'like' }"
                    @click="rateMovie(selectedMovie.normalized_id, 'like')"
                  >
                    <ThumbsUp :size="14" />
                    <span>{{ t('annotations.like') }}</span>
                  </button>

                  <button
                    class="btn btn-sm btn-outline gap-1"
                    :class="{ 'bg-base-300': cardFeedback[selectedMovie.normalized_id] === 'uncertain' }"
                    @click="rateMovie(selectedMovie.normalized_id, 'uncertain')"
                  >
                    <HelpCircle :size="14" />
                    <span>{{ t('annotations.uncertain') }}</span>
                  </button>

                  <button
                    class="btn btn-sm btn-error text-white gap-1"
                    :class="{ 'ring-2 ring-error ring-offset-2': cardFeedback[selectedMovie.normalized_id] === 'dislike' }"
                    @click="rateMovie(selectedMovie.normalized_id, 'dislike')"
                  >
                    <ThumbsDown :size="14" />
                    <span>{{ t('annotations.dislike') }}</span>
                  </button>
                </div>

                <RouterLink
                  :to="`/movies/${selectedMovie.normalized_id}`"
                  target="_blank"
                  class="btn btn-sm btn-ghost gap-1 text-xs"
                >
                  <span>{{ t('movies.details') }}</span>
                  <ExternalLink :size="13" />
                </RouterLink>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- 高清大图画廊（支持封面与所有预览图左右切换） -->
    <ImageLightbox
      :open="lightboxOpen"
      :images="selectedGalleryImages"
      :initial-index="lightboxIndex"
      @close="lightboxOpen = false"
    />
  </div>
</template>
