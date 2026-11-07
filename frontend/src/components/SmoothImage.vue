<script setup lang="ts">
import { ref, watch } from 'vue'
import { ImageOff } from 'lucide-vue-next'

// 平滑渐变加载图片组件属性定义
const props = withDefaults(
  defineProps<{
    src?: string | null
    alt?: string
    aspect?: string
    objectFit?: 'cover' | 'contain'
    fallbackText?: string
    referrerpolicy?: 'no-referrer' | 'origin' | 'no-referrer-when-downgrade' | 'unsafe-url'
    loading?: 'lazy' | 'eager'
    wrapperClass?: string
    imgClass?: string
  }>(),
  {
    src: '',
    alt: '',
    aspect: 'aspect-[2/3]',
    objectFit: 'cover',
    fallbackText: '',
    referrerpolicy: 'no-referrer',
    loading: 'lazy',
    wrapperClass: '',
    imgClass: '',
  }
)

const emit = defineEmits<{
  (e: 'load'): void
  (e: 'error'): void
}>()

// 图片加载与错误状态
const isLoaded = ref(false)
const hasError = ref(false)

// 当图片地址切换时，重置状态以激活过渡动画
watch(
  () => props.src,
  (newSrc) => {
    isLoaded.value = false
    hasError.value = !newSrc
  },
  { immediate: true }
)

function handleLoad() {
  isLoaded.value = true
  hasError.value = false
  emit('load')
}

function handleError() {
  isLoaded.value = false
  hasError.value = true
  emit('error')
}
</script>

<template>
  <div class="relative overflow-hidden bg-base-200 select-none" :class="[aspect, wrapperClass]">
    <!-- 骨架屏占位与呼吸脉冲动画 -->
    <div
      v-if="!isLoaded && !hasError && src"
      class="absolute inset-0 bg-base-300 animate-pulse flex items-center justify-center text-base-content/20 pointer-events-none z-0"
    >
      <div class="w-7 h-7 rounded-full border-2 border-base-content/15 border-t-primary/70 animate-spin" />
    </div>

    <!-- 无图或加载失败占位提示 -->
    <div
      v-if="hasError || !src"
      class="absolute inset-0 flex flex-col items-center justify-center p-3 text-center text-base-content/40 bg-base-200 pointer-events-none z-0"
    >
      <ImageOff :size="20" class="opacity-40 mb-1" />
      <span v-if="fallbackText" class="text-xs font-medium">{{ fallbackText }}</span>
    </div>

    <!-- 真实图片（平滑渐显与轻微缩放过渡） -->
    <img
      v-if="src && !hasError"
      :src="src"
      :alt="alt || ''"
      :loading="loading"
      :referrerpolicy="referrerpolicy"
      class="w-full h-full transition-all duration-350 ease-out z-1"
      :class="[
        objectFit === 'contain' ? 'object-contain' : 'object-cover',
        isLoaded ? 'opacity-100 scale-100 blur-0' : 'opacity-0 scale-[1.03] blur-xs',
        imgClass,
      ]"
      @load="handleLoad"
      @error="handleError"
    />

    <!-- 插槽：用于放置号码角标、悬浮按钮、阴影蒙版等覆盖物 -->
    <slot :loaded="isLoaded" :error="hasError" />
  </div>
</template>
