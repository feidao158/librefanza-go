<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Clapperboard, Globe2, Heart, LayoutGrid, LogIn, LogOut, MessageSquareHeart, Settings2, Sparkles, UsersRound, X } from 'lucide-vue-next'
import type { User } from '../types'

defineProps<{ user: User | null; locale: string }>()
const emit = defineEmits<{ language: [value: string]; logout: [] }>()
const route = useRoute()
const menuOpen = ref(false)
const moreButton = ref<HTMLButtonElement | null>(null)
const closeButton = ref<HTMLButtonElement | null>(null)
const morePanel = ref<HTMLElement | null>(null)

const links = [
  { to: '/movies', label: 'nav.movies', icon: Clapperboard },
  { to: '/actresses', label: 'nav.actresses', icon: UsersRound },
  { to: '/recommendations', label: 'nav.recommendations', icon: Sparkles },
  { to: '/likes', label: 'nav.likes', icon: Heart },
]

function isActive(path: string) {
  if (path === '/movies' && route.path === '/') return true
  return route.path === path || route.path.startsWith(`${path}/`)
}

function onKeydown(event: KeyboardEvent) {
  if (!menuOpen.value) return
  if (event.key === 'Escape') {
    menuOpen.value = false
    return
  }
  if (event.key !== 'Tab' || !morePanel.value) return
  const focusable = [...morePanel.value.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), select:not([disabled])')]
  const first = focusable[0]
  const last = focusable[focusable.length - 1]
  if (!first || !last) return
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
}

watch(() => route.fullPath, () => { menuOpen.value = false })
watch(menuOpen, async (open, wasOpen) => {
  await nextTick()
  if (open) closeButton.value?.focus()
  else if (wasOpen) moreButton.value?.focus()
})
onMounted(() => window.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <header class="site-header">
    <div class="site-header-inner">
      <RouterLink to="/" class="brand"><span class="brand-icon">lf</span><span>LibreFanza</span></RouterLink>
      <nav class="desktop-nav" :aria-label="$t('nav.mainNavigation')">
        <RouterLink v-for="link in links" :key="link.to" :to="link.to" :class="{ 'router-link-active': isActive(link.to) }" :aria-current="isActive(link.to) ? 'page' : undefined"><component :is="link.icon" :size="19" /><span>{{ $t(link.label) }}</span></RouterLink>
        <RouterLink to="/annotations"><MessageSquareHeart :size="19" /><span>{{ $t('nav.annotations') }}</span></RouterLink>
        <RouterLink v-if="user?.is_admin" to="/admin/sources"><Settings2 :size="19" /><span>{{ $t('nav.manage') }}</span></RouterLink>
      </nav>
      <div class="header-actions">
        <label class="language-select"><Globe2 :size="16" /><select :value="locale" :aria-label="$t('nav.language')" @change="emit('language', ($event.target as HTMLSelectElement).value)"><option value="zh">中文</option><option value="en">English</option></select></label>
        <RouterLink v-if="!user" to="/login" class="account-link"><LogIn :size="17" /><span>{{ $t('nav.signIn') }}</span></RouterLink>
        <div v-else class="dropdown dropdown-top account-dropdown">
          <button tabindex="0" class="account-avatar" :title="user.email" :aria-label="user.email">{{ user.email.slice(0, 1).toUpperCase() }}</button>
          <ul tabindex="0" class="dropdown-content menu bg-base-100 account-menu">
            <li class="account-email">{{ user.email }}</li>
            <li v-if="user.is_admin"><RouterLink to="/admin/sources"><Settings2 :size="16" />{{ $t('nav.manage') }}</RouterLink></li>
            <li><button @click="emit('logout')"><LogOut :size="16" />{{ $t('nav.signOut') }}</button></li>
          </ul>
        </div>
      </div>
    </div>
  </header>

  <template v-if="menuOpen">
    <button class="mobile-menu-backdrop" :aria-label="$t('common.close')" @click="menuOpen = false"></button>
    <section id="mobile-more" ref="morePanel" class="mobile-more" role="dialog" aria-modal="true" :aria-label="$t('nav.more')">
      <div class="mobile-more-heading"><strong>{{ $t('nav.more') }}</strong><button ref="closeButton" type="button" class="mobile-more-close" :aria-label="$t('common.close')" @click="menuOpen = false"><X :size="20" /></button></div>
      <p v-if="user" class="mobile-account-email">{{ user.email }}</p>
      <RouterLink to="/annotations"><MessageSquareHeart :size="19" />{{ $t('nav.annotations') }}</RouterLink>
      <RouterLink v-if="user?.is_admin" to="/admin/sources"><Settings2 :size="19" />{{ $t('nav.manage') }}</RouterLink>
      <RouterLink v-if="!user" to="/login"><LogIn :size="19" />{{ $t('nav.signIn') }}</RouterLink>
      <button v-else type="button" @click="menuOpen = false; emit('logout')"><LogOut :size="19" />{{ $t('nav.signOut') }}</button>
      <label class="mobile-language"><Globe2 :size="19" /><span>{{ $t('nav.language') }}</span><select :value="locale" :aria-label="$t('nav.language')" @change="emit('language', ($event.target as HTMLSelectElement).value)"><option value="zh">中文</option><option value="en">English</option></select></label>
    </section>
  </template>
  <nav class="bottom-nav" :aria-label="$t('nav.mobileNavigation')">
    <RouterLink v-for="link in links" :key="link.to" :to="link.to" :class="{ 'router-link-active': isActive(link.to) }" :aria-current="isActive(link.to) ? 'page' : undefined"><component :is="link.icon" :size="20" /><span>{{ $t(link.label) }}</span></RouterLink>
    <button ref="moreButton" type="button" :aria-expanded="menuOpen" aria-controls="mobile-more" @click="menuOpen = !menuOpen"><LayoutGrid :size="20" /><span>{{ $t('nav.more') }}</span></button>
  </nav>
</template>
