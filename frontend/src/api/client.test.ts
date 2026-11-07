import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, restoreSession, setAccessToken } from './client'

describe('API client', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    setAccessToken('')
  })

  it('sends the access token and unwraps JSON responses', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ data: { id: 1 } }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    setAccessToken('token')
    await expect(api('/movies')).resolves.toEqual({ data: { id: 1 } })
    const request = fetchMock.mock.calls[0][1] as RequestInit
    expect(new Headers(request.headers).get('Authorization')).toBe('Bearer token')
    expect(request.credentials).toBe('include')
  })

  it('preserves the server error code', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code: 'not_found' } }), { status: 404 })))
    await expect(api('/movies/unknown')).rejects.toMatchObject({ status: 404, code: 'not_found' } satisfies Partial<ApiError>)
  })

  it('persists the same token for session restoration', async () => {
    setAccessToken('persistent-token')
    expect(window.localStorage.getItem('librefanza-access-token')).toBe('persistent-token')
    await expect(restoreSession()).resolves.toBe(true)
  })

  it('retains the token during a temporary service failure', async () => {
    setAccessToken('persistent-token')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{}', { status: 503 })))
    await expect(api('/auth/me')).rejects.toMatchObject({ status: 503 })
    expect(window.localStorage.getItem('librefanza-access-token')).toBe('persistent-token')
  })

  it('clears an invalid token without calling a refresh endpoint', async () => {
    setAccessToken('expired-token')
    const fetchMock = vi.fn().mockResolvedValue(new Response('{}', { status: 401 }))
    vi.stubGlobal('fetch', fetchMock)
    await expect(api('/auth/me')).rejects.toMatchObject({ status: 401 })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(window.localStorage.getItem('librefanza-access-token')).toBeNull()
  })

  it('does not erase a newer token when an old request fails', async () => {
    setAccessToken('old-token')
    let finish!: (response: Response) => void
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => new Promise<Response>(resolve => { finish = resolve })))
    const request = api('/auth/me')
    window.localStorage.setItem('librefanza-access-token', 'new-token')
    finish(new Response('{}', { status: 401 }))
    await expect(request).rejects.toMatchObject({ status: 401 })
    expect(window.localStorage.getItem('librefanza-access-token')).toBe('new-token')
  })
})
