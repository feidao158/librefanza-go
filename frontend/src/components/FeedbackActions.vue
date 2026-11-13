<script setup lang="ts">
import { HelpCircle, ThumbsDown, ThumbsUp } from 'lucide-vue-next'

type Feedback = 'like' | 'uncertain' | 'dislike'
defineProps<{ disabled: boolean; pendingAction?: Feedback | 'skip' | null }>()
const emit = defineEmits<{ select: [action: Feedback] }>()

const actions = [
  { value: 'like', label: 'annotations.like', icon: ThumbsUp, key: '1', style: 'btn-success text-success-content', keyStyle: 'bg-success-content/20 text-success-content' },
  { value: 'uncertain', label: 'annotations.uncertain', icon: HelpCircle, key: '2', style: 'btn-outline border-base-content/30', keyStyle: 'bg-base-300' },
  { value: 'dislike', label: 'annotations.dislike', icon: ThumbsDown, key: '3', style: 'btn-error text-error-content', keyStyle: 'bg-error-content/20 text-error-content' },
] as const
</script>

<template>
  <div class="feedback-actions grid grid-cols-3 gap-2 sm:gap-3">
    <button v-for="action in actions" :key="action.value" type="button" class="btn shadow font-bold text-sm sm:text-base h-12 sm:h-14 px-1 sm:px-4 active:scale-95 transition-transform" :class="action.style" :disabled="disabled" @click="emit('select', action.value)">
      <span v-if="pendingAction === action.value" class="loading loading-spinner loading-sm" aria-hidden="true"></span>
      <component :is="action.icon" v-else :size="18" class="sm:w-5 sm:h-5 flex-shrink-0" />
      <span>{{ $t(action.label) }}</span>
      <kbd class="hidden sm:inline-flex kbd kbd-sm ml-1" :class="action.keyStyle">{{ action.key }}</kbd>
    </button>
  </div>
</template>
