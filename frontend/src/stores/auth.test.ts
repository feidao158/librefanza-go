import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { setAccessToken } from '../api/client'
import { useAuthStore } from './auth'

describe('登录状态恢复', () => {
  beforeEach(() => { window.localStorage.clear(); setAccessToken(''); setActivePinia(createPinia()) })

  it('服务暂时不可用时保留令牌和用户显示', async () => {
    setAccessToken('saved-token')
    window.localStorage.setItem('librefanza-user', JSON.stringify({ token: 'saved-token', user: { id: 1, email: 'user@example.com', is_admin: true } }))
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))
    const auth = useAuthStore()
    await auth.restore()
    expect(auth.user?.email).toBe('user@example.com')
    expect(window.localStorage.getItem('librefanza-access-token')).toBe('saved-token')
    vi.unstubAllGlobals()
  })
})
