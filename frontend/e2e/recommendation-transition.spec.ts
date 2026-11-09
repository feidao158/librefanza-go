import { expect, test } from '@playwright/test'

test('推荐标记后等待下一部期间保持卡片可见', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' })
  const stats = { total: 5, liked: 3, disliked: 1, uncertain: 1, min_required: 5, ready: true }
  let nextRequested = false
  let releaseNext!: () => void
  const waitForNext = new Promise<void>(resolve => { releaseNext = resolve })
  let requests = 0
  await page.addInitScript(() => {
    window.localStorage.setItem('librefanza-access-token', 'test-token')
    window.requestAnimationFrame = callback => window.setTimeout(() => callback(performance.now()), 16)
    window.cancelAnimationFrame = handle => window.clearTimeout(handle)
  })
  await page.route('**/api/v1/setup', route => route.fulfill({ json: { data: { installed: true } } }))
  await page.route('**/api/v1/auth/me', route => route.fulfill({ json: { data: { id: 1, email: 'test@example.com', is_admin: false } } }))
  await page.route('**/api/v1/recommendations/feedback', route => route.fulfill({ json: { data: { stats } } }))
  await page.route('**/api/v1/recommendations?*', async route => {
    requests++
    if (requests > 1) {
      nextRequested = true
      await waitForNext
    }
    await route.fulfill({ json: { data: { ready: true, stats, movies: [{ normalized_id: requests === 1 ? 'REC-001' : 'REC-002' }] } } })
  })
  await page.goto('/recommendations')
  await expect(page.getByText('REC-001').first()).toBeVisible()
  await page.getByRole('button', { name: /喜欢/ }).first().focus()
  await page.getByRole('button', { name: /喜欢/ }).first().press('Enter')
  await expect.poll(() => nextRequested).toBe(true)
  await expect(page.getByText('REC-001').first()).toBeVisible()
  await expect(page.getByRole('button', { name: /喜欢/ }).first()).toBeDisabled()
  releaseNext()
  await expect(page.getByText('REC-002').first()).toBeVisible()
  await expect(page.getByText('REC-001')).toHaveCount(0)
})
