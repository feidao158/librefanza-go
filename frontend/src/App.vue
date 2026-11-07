<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Globe2, LogIn, LogOut, Menu, Settings2, X } from 'lucide-vue-next'
import { api } from './api/client'
import { useAuthStore } from './stores/auth'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const { locale } = useI18n()
const ready = ref(false)
const failed = ref(false)
const menuOpen = ref(false)
const logoutFailed = ref(false)
let lastActivityRenewal = 0
const isSetup = computed(() => route.path === '/setup')

async function bootstrap() {
  failed.value = false
  try {
    const status = await api<{ installed: boolean }>('/setup')
    if (!status.data.installed && !isSetup.value) await router.replace('/setup')
    if (status.data.installed && isSetup.value) await router.replace('/')
    if (status.data.installed) await auth.restore()
    ready.value = true
  } catch {
    failed.value = true
  }
}

function setLanguage(value: string) {
  locale.value = value
  window.localStorage.setItem('librefanza-locale', value)
  document.documentElement.lang = value === 'zh' ? 'zh-CN' : 'en'
}

async function logout() {
  try {
    logoutFailed.value = false
    await auth.logout()
    await router.push('/')
  } catch { logoutFailed.value = true }
}

function renewOnActivity() {
  if (!auth.user || Date.now() - lastActivityRenewal < 5 * 60_000) return
  lastActivityRenewal = Date.now()
  void auth.restore().catch(() => {})
}

onMounted(() => {
  void bootstrap()
  window.addEventListener('pointerdown', renewOnActivity)
  window.addEventListener('keydown', renewOnActivity)
  window.addEventListener('scroll', renewOnActivity, { passive: true })
})
onBeforeUnmount(() => {
  window.removeEventListener('pointerdown', renewOnActivity)
  window.removeEventListener('keydown', renewOnActivity)
  window.removeEventListener('scroll', renewOnActivity)
})
</script>

<template>
  <div class="app-shell">
    <header v-if="!isSetup" class="site-header">
      <div class="site-header-inner">
        <RouterLink to="/" class="brand" @click="menuOpen = false"><span class="brand-icon">lf</span><span>LibreFanza</span></RouterLink>
        <nav class="desktop-nav" aria-label="Main navigation">
          <RouterLink to="/movies">{{ $t('nav.movies') }}</RouterLink>
          <RouterLink to="/actresses">{{ $t('nav.actresses') }}</RouterLink>
          <RouterLink to="/annotations">{{ $t('nav.annotations') }}</RouterLink>
          <RouterLink to="/recommendations">{{ $t('nav.recommendations') }}</RouterLink>
          <RouterLink v-if="auth.user?.is_admin" to="/admin/sources">{{ $t('nav.manage') }}</RouterLink>
        </nav>
        <div class="header-actions">
          <label class="language-select" :title="$t('nav.settings')"><Globe2 :size="16" /><select :value="locale" aria-label="Language" @change="setLanguage(($event.target as HTMLSelectElement).value)"><option value="zh">中文</option><option value="en">EN</option></select></label>
          <RouterLink v-if="!auth.user" to="/login" class="account-link" :aria-label="$t('nav.signIn')"><LogIn :size="16" /><span>{{ $t('nav.signIn') }}</span></RouterLink>
          <div v-else class="dropdown dropdown-end account-dropdown">
            <button tabindex="0" class="account-avatar" :title="auth.user.email">{{ auth.user.email.slice(0, 1).toUpperCase() }}</button>
            <ul tabindex="0" class="dropdown-content menu bg-base-100 shadow-sm account-menu">
              <li class="account-email">{{ auth.user.email }}</li>
              <li v-if="auth.user.is_admin"><RouterLink to="/admin/sources"><Settings2 :size="16" />{{ $t('nav.manage') }}</RouterLink></li>
              <li><button @click="logout"><LogOut :size="16" />{{ $t('nav.signOut') }}</button></li>
            </ul>
          </div>
          <button class="btn btn-ghost btn-square mobile-menu-button" :aria-label="menuOpen ? 'Close menu' : 'Open menu'" @click="menuOpen = !menuOpen"><X v-if="menuOpen" :size="21" /><Menu v-else :size="21" /></button>
        </div>
      </div>
      <nav v-if="menuOpen" class="mobile-nav"><RouterLink to="/movies" @click="menuOpen = false">{{ $t('nav.movies') }}</RouterLink><RouterLink to="/actresses" @click="menuOpen = false">{{ $t('nav.actresses') }}</RouterLink><RouterLink to="/annotations" @click="menuOpen = false">{{ $t('nav.annotations') }}</RouterLink><RouterLink to="/recommendations" @click="menuOpen = false">{{ $t('nav.recommendations') }}</RouterLink><RouterLink v-if="auth.user?.is_admin" to="/admin/sources" @click="menuOpen = false">{{ $t('nav.manage') }}</RouterLink></nav>
    </header>
    <main v-if="ready" class="main-content" :class="{ 'setup-main': isSetup }"><p v-if="logoutFailed" class="error-text">{{ $t('common.error') }}</p><RouterView /></main>
    <main v-else class="startup-state"><div v-if="failed" class="notice-error">{{ $t('common.error') }} <button class="btn btn-sm" @click="bootstrap">{{ $t('common.retry') }}</button></div><span v-else class="loading loading-spinner loading-md"></span></main>
    <footer v-if="!isSetup && ready" class="site-footer"><span>LibreFanza</span><a href="https://github.com/libredmm/librefanza" target="_blank" rel="noreferrer">GitHub ↗</a></footer>
  </div>
</template>
