import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, ApiError, currentAccessToken, jsonBody, onAccessTokenChange, restoreSession, setAccessToken } from '../api/client'
import type { User } from '../types'

interface Tokens { access_token: string; user: User }

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)

  async function restore() {
    if (await restoreSession()) {
      const token = currentAccessToken()
      try {
        const currentUser = (await api<User>('/auth/me')).data
        if (currentAccessToken() === token) {
          user.value = currentUser
          window.localStorage.setItem('librefanza-user', JSON.stringify({ token, user: currentUser }))
        }
      } catch (error) {
        if (error instanceof ApiError && error.status === 401) { if (!currentAccessToken()) user.value = null }
        else {
          try {
            const cached = JSON.parse(window.localStorage.getItem('librefanza-user') || 'null') as { token: string; user: User } | null
            user.value = cached?.token === token ? cached.user : null
          } catch { user.value = null }
        }
      }
    } else user.value = null
  }

  onAccessTokenChange((token, external) => {
    if (!token) { user.value = null; window.localStorage.removeItem('librefanza-user') }
    else if (external) {
      user.value = null
      void restore().catch(() => {})
    }
  })

  async function login(email: string, password: string) {
    const response = await api<Tokens>('/auth/sessions', { method: 'POST', body: jsonBody({ email, password }) })
    setAccessToken(response.data.access_token)
    user.value = response.data.user
    window.localStorage.setItem('librefanza-user', JSON.stringify({ token: response.data.access_token, user: response.data.user }))
  }

  async function register(email: string, password: string) {
    await api('/users', { method: 'POST', body: jsonBody({ email, password }) })
    await login(email, password)
  }

  async function logout() {
    const token = currentAccessToken()
    try { await api('/auth/sessions/current', { method: 'DELETE' }) } catch (error) {
      if (!(error instanceof ApiError && error.status === 401)) throw error
    }
    if (!currentAccessToken() || currentAccessToken() === token) {
      setAccessToken('')
      user.value = null
    }
  }

  return { user, restore, login, register, logout }
})
