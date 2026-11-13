<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ExternalLink } from 'lucide-vue-next'
import { api } from './api/client'
import AppNavigation from './components/AppNavigation.vue'
import { useAuthStore } from './stores/auth'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const { locale } = useI18n()
const ready = ref(false)
const failed = ref(false)
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
  <div class="app-shell" :class="{ 'is-setup': isSetup }">
    <AppNavigation v-if="!isSetup" :user="auth.user" :locale="locale" @language="setLanguage" @logout="logout" />
    <main v-if="ready" class="main-content" :class="{ 'setup-main': isSetup }"><p v-if="logoutFailed" class="error-text">{{ $t('common.error') }}</p><RouterView /></main>
    <main v-else class="startup-state"><div v-if="failed" class="notice-error">{{ $t('common.error') }} <button class="btn btn-sm" @click="bootstrap">{{ $t('common.retry') }}</button></div><span v-else class="loading loading-spinner loading-md"></span></main>
    <footer v-if="!isSetup && ready" class="site-footer"><span>LibreFanza</span><a href="https://github.com/libredmm/librefanza" target="_blank" rel="noreferrer">GitHub <ExternalLink :size="14" class="inline-block" /></a></footer>
  </div>
</template>
