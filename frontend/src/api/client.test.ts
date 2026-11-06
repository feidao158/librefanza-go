import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, setAccessToken } from './client'

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

  it('refreshes an expired token before logging out', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ error: { code: 'unauthorized' } }), { status: 401 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { access_token: 'new-token' } }), { status: 200 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetchMock)
    setAccessToken('expired-token')
    await expect(api('/auth/sessions/current', { method: 'DELETE' })).resolves.toEqual({ data: undefined })
    expect(fetchMock).toHaveBeenCalledTimes(3)
    expect(new Headers((fetchMock.mock.calls[2][1] as RequestInit).headers).get('Authorization')).toBe('Bearer new-token')
  })
})
