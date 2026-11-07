export const accessTokenKey = 'librefanza-access-token'
let accessToken = window.localStorage.getItem(accessTokenKey) || ''
const tokenListeners = new Set<(value: string, external: boolean) => void>()

export class ApiError extends Error {
  constructor(public status: number, public code: string) {
    super(code)
  }
}

export interface ApiResponse<T> {
  data: T
  meta?: { page: number; page_size: number; total: number }
}

export function onAccessTokenChange(listener: (value: string, external: boolean) => void) {
  tokenListeners.add(listener)
  return () => tokenListeners.delete(listener)
}

export function setAccessToken(value: string) {
  accessToken = value
  if (value) window.localStorage.setItem(accessTokenKey, value)
  else window.localStorage.removeItem(accessTokenKey)
  tokenListeners.forEach(listener => listener(value, false))
}

export function currentAccessToken() {
  return window.localStorage.getItem(accessTokenKey) || ''
}

window.addEventListener('storage', event => {
  if (event.key !== accessTokenKey && event.key !== null) return
  accessToken = window.localStorage.getItem(accessTokenKey) || ''
  tokenListeners.forEach(listener => listener(accessToken, true))
})

export async function restoreSession(): Promise<boolean> {
  accessToken = window.localStorage.getItem(accessTokenKey) || ''
  return Boolean(accessToken)
}

export async function api<T = unknown>(path: string, options: RequestInit = {}): Promise<ApiResponse<T>> {
  const headers = new Headers(options.headers)
  const sentToken = accessToken
  if (sentToken) headers.set('Authorization', `Bearer ${sentToken}`)
  if (options.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const response = await fetch(`/api/v1${path}`, { ...options, headers, credentials: 'include' })
  const isAuthenticationRequest = path === '/auth/sessions' || path === '/users' || path.startsWith('/auth/password-')
  if (response.status === 401 && sentToken && sentToken === accessToken && sentToken === currentAccessToken() && !isAuthenticationRequest) setAccessToken('')
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
