let accessToken = ''
let refreshPromise: Promise<boolean> | undefined

export class ApiError extends Error {
  constructor(public status: number, public code: string) {
    super(code)
  }
}

export interface ApiResponse<T> {
  data: T
  meta?: { page: number; page_size: number; total: number }
}

export function setAccessToken(value: string) {
  accessToken = value
}

async function refreshAccessToken(): Promise<boolean> {
  if (!refreshPromise) {
    refreshPromise = (async () => {
      try {
        const response = await fetch('/api/v1/auth/refresh', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'include',
          body: '{}',
        })
        if (!response.ok) return false
        const payload = await response.json() as ApiResponse<{ access_token: string }>
        accessToken = payload.data.access_token
        return true
      } catch {
        return false
      }
    })().finally(() => { refreshPromise = undefined })
  }
  return refreshPromise
}

export async function restoreSession(): Promise<boolean> {
  return refreshAccessToken()
}

export async function api<T = unknown>(path: string, options: RequestInit = {}, retry = true): Promise<ApiResponse<T>> {
  const headers = new Headers(options.headers)
  if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`)
  if (options.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const response = await fetch(`/api/v1${path}`, { ...options, headers, credentials: 'include' })
  const canRefresh = path === '/auth/me' || path === '/auth/sessions/current' || !path.startsWith('/auth/')
  if (response.status === 401 && accessToken && retry && canRefresh) {
    if (await refreshAccessToken()) return api<T>(path, options, false)
    accessToken = ''
  }
  if (!response.ok) {
    const payload = await response.json().catch(() => ({})) as { error?: { code?: string } }
    throw new ApiError(response.status, payload.error?.code || 'request_failed')
  }
  if (response.status === 204) return { data: undefined as T }
  return response.json() as Promise<ApiResponse<T>>
}

export function jsonBody(value: unknown): string {
  return JSON.stringify(value)
}
