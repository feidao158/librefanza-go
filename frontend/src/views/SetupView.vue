<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Check, Database, LockKeyhole, Server, ShieldCheck } from 'lucide-vue-next'
import { api, jsonBody } from '../api/client'

const step = ref(1)
const busy = ref(false)
const error = ref(false)
const checked = ref('')
const managedAvailable = ref(false)
const managed = ref(false)
const checkingSetup = ref(true)
const database = reactive({ driver: 'postgres', host: '127.0.0.1', port: 5432, name: 'librefanza', user: '', password: '', tls: false })
const redis = reactive({ enabled: false, address: '127.0.0.1:6379', username: '', password: '', database: 0, tls: false })
const admin = reactive({ email: '', password: '' })

onMounted(async () => {
  try {
    const status = await api<{ managed_database_available: boolean }>('/setup')
    managedAvailable.value = !!status.data.managed_database_available
    managed.value = managedAvailable.value
    if (managed.value) step.value = 3
  } catch { error.value = true } finally { checkingSetup.value = false }
})

async function testConnection(kind: 'database' | 'redis') {
  busy.value = true
  error.value = false
  checked.value = ''
  try {
    await api(`/setup/${kind}-checks`, { method: 'POST', body: jsonBody(kind === 'database' ? database : redis) })
    checked.value = kind
  } catch { error.value = true } finally { busy.value = false }
}

async function finish() {
  busy.value = true
  error.value = false
  try {
    await api('/setup/installations', { method: 'POST', body: jsonBody({ database, managed_database: managed.value, redis, email: admin.email, password: admin.password }) })
    window.location.assign('/')
  } catch { error.value = true; busy.value = false }
}
</script>

<template>
  <div class="setup-shell">
    <div class="setup-brand"><span class="brand-icon">lf</span><span>LibreFanza</span></div>
    <div class="setup-heading"><h1>{{ $t('setup.title') }}</h1></div>
    <div v-if="!managed" class="setup-steps"><div :class="{ active: step === 1, done: step > 1 }"><Database :size="17" /><span>{{ $t('setup.database') }}</span></div><div :class="{ active: step === 2, done: step > 2 }"><Server :size="17" /><span>{{ $t('setup.redis') }}</span></div><div :class="{ active: step === 3 }"><ShieldCheck :size="17" /><span>{{ $t('setup.admin') }}</span></div></div>
    <div v-if="checkingSetup" class="center-state">{{ $t('common.loading') }}</div>
    <form v-else-if="step === 1" class="setup-form" @submit.prevent="step = 2; error = false; checked = ''">
      <h2>{{ $t('setup.database') }}</h2>
      <label class="field"><span>{{ $t('setup.driver') }}</span><input class="input" value="PostgreSQL" readonly></label>
      <div class="field-grid"><label class="field"><span>{{ $t('setup.host') }}</span><input v-model="database.host" class="input" required></label><label class="field"><span>{{ $t('setup.port') }}</span><input v-model.number="database.port" class="input" type="number" min="1" max="65535" required></label></div>
      <label class="field"><span>{{ $t('setup.name') }}</span><input v-model="database.name" class="input" required></label>
      <div class="field-grid"><label class="field"><span>{{ $t('setup.user') }}</span><input v-model="database.user" class="input" autocomplete="username" required></label><label class="field"><span>{{ $t('setup.password') }}</span><input v-model="database.password" class="input" type="password" autocomplete="new-password"></label></div>
      <label class="check-row"><input v-model="database.tls" type="checkbox" class="checkbox checkbox-sm checkbox-success">{{ $t('setup.tls') }}</label>
      <div v-if="checked === 'database'" class="success-text"><Check :size="15" />{{ $t('setup.connected') }}</div><p v-if="error" class="error-text font-medium">{{ $t('setup.failed') }}</p>
      <div class="setup-actions"><button v-if="managedAvailable" type="button" class="btn btn-ghost" @click="managed = true; step = 3; error = false">{{ $t('managedSetup.internal') }}</button><button type="button" class="btn btn-outline" :disabled="busy" @click="testConnection('database')">{{ $t('setup.test') }}</button><button class="btn search-button" :disabled="busy">{{ $t('common.next') }}</button></div>
    </form>
    <form v-else-if="step === 2" class="setup-form" @submit.prevent="step = 3; error = false; checked = ''">
      <h2>{{ $t('setup.redis') }}</h2><label class="toggle-row"><span>{{ $t('setup.redisOptional') }}</span><input v-model="redis.enabled" type="checkbox" class="toggle toggle-success"></label>
      <template v-if="redis.enabled"><label class="field"><span>{{ $t('setup.redisAddress') }}</span><input v-model="redis.address" class="input" required></label><div class="field-grid"><label class="field"><span>{{ $t('setup.user') }}</span><input v-model="redis.username" class="input"></label><label class="field"><span>{{ $t('setup.password') }}</span><input v-model="redis.password" class="input" type="password"></label></div><label class="field"><span>{{ $t('setup.redisDB') }}</span><input v-model.number="redis.database" class="input" type="number" min="0"></label><label class="check-row"><input v-model="redis.tls" type="checkbox" class="checkbox checkbox-sm checkbox-success">{{ $t('setup.tls') }}</label><button type="button" class="btn btn-outline btn-sm" :disabled="busy" @click="testConnection('redis')">{{ $t('setup.test') }}</button></template>
      <div v-if="checked === 'redis'" class="success-text"><Check :size="15" />{{ $t('setup.connected') }}</div><p v-if="error" class="error-text font-medium">{{ $t('setup.failed') }}</p><div class="setup-actions"><button type="button" class="btn btn-ghost" @click="step = 1">{{ $t('common.previous') }}</button><button class="btn search-button">{{ $t('common.next') }}</button></div>
    </form>
    <form v-else class="setup-form" @submit.prevent="finish"><h2>{{ managed ? $t('managedSetup.admin') : $t('setup.admin') }}</h2><label class="field"><span>{{ $t('auth.email') }}</span><input v-model="admin.email" class="input" type="email" autocomplete="username" required></label><label class="field"><span>{{ $t('auth.password') }}</span><input v-model="admin.password" class="input" type="password" autocomplete="new-password" minlength="8" maxlength="72" required></label><p v-if="error" class="error-text font-medium">{{ $t('setup.failed') }}</p><div class="setup-actions"><button v-if="managedAvailable && managed" type="button" class="btn btn-ghost" @click="managed = false; step = 1">{{ $t('managedSetup.external') }}</button><button v-else type="button" class="btn btn-ghost" @click="step = 2">{{ $t('common.previous') }}</button><button class="btn search-button" :disabled="busy"><span v-if="busy" class="loading loading-spinner loading-xs"></span><LockKeyhole v-else :size="16" />{{ busy ? $t('setup.installing') : $t('setup.finish') }}</button></div></form>
  </div>
</template>
