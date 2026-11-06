<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { LogIn } from 'lucide-vue-next'
import { ApiError } from '../api/client'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const register = computed(() => route.path === '/register')
const email = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  loading.value = true
  error.value = ''
  try {
    if (register.value) await auth.register(email.value, password.value)
    else await auth.login(email.value, password.value)
    await router.push('/')
  } catch (err) {
    error.value = err instanceof ApiError && err.code === 'account_exists' ? 'auth.accountExists' : 'auth.invalid'
  } finally { loading.value = false }
}
</script>

<template>
  <div class="auth-wrap"><div class="section-kicker">LIBREFANZA / ACCOUNT</div><h1>{{ $t(register ? 'auth.register' : 'auth.login') }}</h1><form class="auth-form" @submit.prevent="submit"><label class="field"><span>{{ $t('auth.email') }}</span><input v-model="email" class="input" type="email" autocomplete="email" required></label><label class="field"><span>{{ $t('auth.password') }}</span><input v-model="password" class="input" type="password" :autocomplete="register ? 'new-password' : 'current-password'" minlength="8" required></label><p v-if="error" class="error-text" role="alert">{{ $t(error) }}</p><button class="btn search-button auth-submit" :disabled="loading"><span v-if="loading" class="loading loading-spinner loading-xs"></span><LogIn v-else :size="17" />{{ $t(register ? 'auth.register' : 'auth.login') }}</button></form><div class="auth-links"><RouterLink class="text-link" :to="register ? '/login' : '/register'">{{ $t(register ? 'auth.hasAccount' : 'auth.noAccount') }}</RouterLink><RouterLink v-if="!register" class="text-link" to="/forgot-password">{{ $t('auth.forgot') }}</RouterLink></div></div>
</template>
