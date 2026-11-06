import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, jsonBody, restoreSession, setAccessToken } from '../api/client'
import type { User } from '../types'

interface Tokens { access_token: string; user: User }

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)

  async function restore() {
    if (await restoreSession()) {
      try { user.value = (await api<User>('/auth/me')).data } catch { user.value = null }
    }
  }

  async function login(email: string, password: string) {
    const response = await api<Tokens>('/auth/sessions', { method: 'POST', body: jsonBody({ email, password, client: 'web' }) })
    setAccessToken(response.data.access_token)
    user.value = response.data.user
  }

  async function register(email: string, password: string) {
    await api('/users', { method: 'POST', body: jsonBody({ email, password }) })
    await login(email, password)
  }

  async function logout() {
    try { await api('/auth/sessions/current', { method: 'DELETE' }) } finally {
      setAccessToken('')
      user.value = null
    }
  }

  return { user, restore, login, register, logout }
})
