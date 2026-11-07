<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { ChevronLeft, ChevronRight, X } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'

export interface LightboxItem {
  url: string
  title?: string
}

const props = withDefaults(
  defineProps<{
    open: boolean
    images: (string | LightboxItem)[]
    initialIndex?: number
  }>(),
  {
    initialIndex: 0,
  }
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'update:index', index: number): void
}>()

const { t } = useI18n()

const currentIndex = ref(props.initialIndex)
const imgLoading = ref(true)
const imgError = ref(false)
const thumbnailContainer = ref<HTMLElement | null>(null)

const normalizedImages = computed<LightboxItem[]>(() => {
  return props.images.map((item, idx) => {
    if (typeof item === 'string') {
      return { url: item, title: '' }
    }
    return { url: item.url, title: item.title || '' }
  }).filter(img => Boolean(img.url))
})

const currentImage = computed<LightboxItem | null>(() => {
  if (normalizedImages.value.length === 0) return null
  const idx = Math.max(0, Math.min(currentIndex.value, normalizedImages.value.length - 1))
  return normalizedImages.value[idx] || null
})

function prev() {
  if (normalizedImages.value.length <= 1) return
  if (currentIndex.value > 0) {
    currentIndex.value--
  } else {
    currentIndex.value = normalizedImages.value.length - 1
  }
  emit('update:index', currentIndex.value)
  scrollThumbIntoView()
}

function next() {
  if (normalizedImages.value.length <= 1) return
  if (currentIndex.value < normalizedImages.value.length - 1) {
    currentIndex.value++
  } else {
    currentIndex.value = 0
  }
  emit('update:index', currentIndex.value)
  scrollThumbIntoView()
}

function goTo(index: number) {
  if (index >= 0 && index < normalizedImages.value.length) {
    currentIndex.value = index
    emit('update:index', index)
    scrollThumbIntoView()
  }
}

function close() {
  emit('close')
}

function scrollThumbIntoView() {
  void nextTick(() => {
    if (!thumbnailContainer.value) return
    const activeEl = thumbnailContainer.value.querySelector('[data-active="true"]') as HTMLElement | null
    if (activeEl && typeof activeEl.scrollIntoView === 'function') {
      activeEl.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'center' })
    }
  })
}

function handleKeyDown(event: KeyboardEvent) {
  if (!props.open) return
  if (event.key === 'Escape') {
    event.preventDefault()
    close()
  } else if (event.key === 'ArrowLeft') {
    event.preventDefault()
    prev()
  } else if (event.key === 'ArrowRight') {
    event.preventDefault()
    next()
  }
}

watch(
  () => props.initialIndex,
  newVal => {
    currentIndex.value = newVal
    scrollThumbIntoView()
  }
)

watch(
  () => currentIndex.value,
  () => {
    imgLoading.value = true
    imgError.value = false
  }
)

watch(
  () => props.open,
  newVal => {
    if (newVal) {
      currentIndex.value = props.initialIndex
      imgLoading.value = true
      imgError.value = false
      document.body.style.overflow = 'hidden'
      window.addEventListener('keydown', handleKeyDown)
      scrollThumbIntoView()
    } else {
      document.body.style.overflow = ''
      window.removeEventListener('keydown', handleKeyDown)
    }
  },
  { immediate: true }
)

onBeforeUnmount(() => {
  document.body.style.overflow = ''
  window.removeEventListener('keydown', handleKeyDown)
})
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="open && currentImage"
        class="fixed inset-0 z-[100] flex flex-col justify-between bg-black/90 text-white select-none backdrop-blur-md"
        @click.self="close"
      >
        <!-- 顶栏：序号与关闭按钮 -->
        <header class="flex items-center justify-between px-6 py-4 bg-gradient-to-b from-black/80 to-transparent z-10">
          <div class="flex items-center gap-3">
            <span class="badge badge-neutral bg-white/20 text-white border-none font-mono text-xs px-2.5 py-1.5">
              {{ currentIndex + 1 }} / {{ normalizedImages.length }}
            </span>
            <span v-if="currentImage.title" class="text-xs text-white/80 font-medium truncate max-w-md">
              {{ currentImage.title }}
            </span>
          </div>

          <div class="flex items-center gap-2">
            <button
              class="btn btn-ghost btn-circle btn-sm text-white hover:bg-white/20"
              :title="t('common.closePreview')"
              :aria-label="t('common.closePreview')"
              data-action="close"
              @click="close"
            >
              <X :size="20" />
            </button>
          </div>
        </header>

        <!-- 中间：大图展示与左右切换按钮 -->
        <main class="relative flex-1 flex items-center justify-center p-4 overflow-hidden" @click.self="close">
          <!-- 上一张按钮 -->
          <button
            v-if="normalizedImages.length > 1"
            class="absolute left-4 z-20 btn btn-circle btn-neutral bg-black/40 hover:bg-black/70 text-white border-white/20 shadow-xl transition-transform hover:scale-110"
            :title="t('common.prevImage')"
            :aria-label="t('common.prevImage')"
            data-action="prev"
            @click.stop="prev"
          >
            <ChevronLeft :size="26" />
          </button>

          <!-- 图片展示区域 -->
          <div class="relative max-h-[80vh] max-w-[88vw] flex items-center justify-center" @click.stop>
            <span v-if="imgLoading" class="loading loading-spinner loading-lg text-primary absolute"></span>

            <img
              :key="currentImage.url"
              :src="currentImage.url"
              :alt="currentImage.title || 'Image preview'"
              class="max-h-[80vh] max-w-[88vw] object-contain rounded-lg shadow-2xl transition-opacity duration-200"
              :class="{ 'opacity-0': imgLoading, 'opacity-100': !imgLoading }"
              referrerpolicy="no-referrer"
              @load="imgLoading = false"
              @error="imgLoading = false; imgError = true"
            />

            <div v-if="imgError" class="p-6 bg-base-300 text-base-content rounded-lg text-center">
              <p class="text-sm">{{ t('common.error') }}</p>
            </div>
          </div>

          <!-- 下一张按钮 -->
          <button
            v-if="normalizedImages.length > 1"
            class="absolute right-4 z-20 btn btn-circle btn-neutral bg-black/40 hover:bg-black/70 text-white border-white/20 shadow-xl transition-transform hover:scale-110"
            :title="t('common.nextImage')"
            :aria-label="t('common.nextImage')"
            data-action="next"
            @click.stop="next"
          >
            <ChevronRight :size="26" />
          </button>
        </main>

        <!-- 底栏：缩略图列表 -->
        <footer
          v-if="normalizedImages.length > 1"
          ref="thumbnailContainer"
          class="flex items-center justify-center gap-2 px-6 py-3 bg-gradient-to-t from-black/90 to-transparent overflow-x-auto max-w-full z-10 scrollbar-none"
        >
          <button
            v-for="(img, idx) in normalizedImages"
            :key="idx"
            type="button"
            class="relative flex-shrink-0 w-14 h-10 rounded overflow-hidden transition-all border-2"
            :class="currentIndex === idx ? 'border-primary ring-2 ring-primary/50 scale-105 opacity-100' : 'border-transparent opacity-50 hover:opacity-80'"
            :data-active="currentIndex === idx"
            @click.stop="goTo(idx)"
          >
            <img :src="img.url" class="w-full h-full object-cover" referrerpolicy="no-referrer" />
          </button>
        </footer>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
