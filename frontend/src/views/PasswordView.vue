<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api, jsonBody } from '../api/client'

const route = useRoute()
const resetting = computed(() => route.path === '/reset-password')
const email = ref('')
const password = ref('')
const done = ref(false)
const error = ref(false)
const busy = ref(false)
async function submit() {
  busy.value = true; error.value = false
  try {
    if (resetting.value) await api('/auth/password-resets', { method: 'POST', body: jsonBody({ token: String(route.query.token || ''), password: password.value }) })
    else await api('/auth/password-reset-requests', { method: 'POST', body: jsonBody({ email: email.value }) })
    done.value = true
  } catch { error.value = true } finally { busy.value = false }
}
</script>

<template>
  <div class="auth-wrap"><div class="section-kicker">LIBREFANZA / ACCOUNT</div><h1>{{ $t(resetting ? 'auth.reset' : 'auth.forgot') }}</h1><p v-if="done" class="success-text">{{ $t(resetting ? 'auth.resetDone' : 'auth.resetSent') }}</p><form v-else class="auth-form" @submit.prevent="submit"><label v-if="resetting" class="field"><span>{{ $t('auth.password') }}</span><input v-model="password" class="input" type="password" minlength="8" maxlength="72" required></label><label v-else class="field"><span>{{ $t('auth.email') }}</span><input v-model="email" class="input" type="email" required></label><p v-if="error" class="error-text">{{ $t('common.error') }}</p><button class="btn search-button" :disabled="busy">{{ $t(resetting ? 'auth.reset' : 'auth.requestReset') }}</button></form><div class="auth-links"><RouterLink class="text-link" to="/login">{{ $t('auth.login') }}</RouterLink></div></div>
</template>
