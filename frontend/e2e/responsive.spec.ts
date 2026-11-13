import { expect, test, type Page } from '@playwright/test'

async function mockApp(page: Page) {
  await page.addInitScript(() => {
    window.localStorage.setItem('librefanza-access-token', 'test-token')
    window.requestAnimationFrame = callback => window.setTimeout(() => callback(performance.now()), 16)
    window.cancelAnimationFrame = handle => window.clearTimeout(handle)
  })
  const item = { id: 1, source: 'fanza', title: '测试影片', normalized_id: 'TEST-001', cover_image_url: '', thumbnail_image_url: '', metadata: {}, source_url: 'https://example.test/' }
  const movie = { id: 1, normalized_id: 'TEST-001', preferred_item: item, versions: [item] }
  const stats = { total: 5, liked: 3, disliked: 1, uncertain: 1, min_required: 5, ready: true }
  let retried = false
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname
    let data: unknown = {}
    let meta: unknown
    if (path === '/api/v1/setup') data = { installed: true }
    else if (path === '/api/v1/auth/me') data = { id: 1, email: 'admin@example.test', is_admin: true }
    else if (path === '/api/v1/movies') { data = [movie]; meta = { total: 1 } }
    else if (path === '/api/v1/movies/TEST-001') data = movie
    else if (path === '/api/v1/movies/TEST-001/playbacks') data = []
    else if (path === '/api/v1/movies/TEST-001/metadata-job') return route.fulfill({ status: 404, json: { error: { code: 'not_found' } } })
    else if (path === '/api/v1/actresses') { data = [{ id: 1, name: '测试演员', image_url: '' }]; meta = { total: 1 } }
    else if (path === '/api/v1/recommendations') data = { ready: true, stats, movies: [movie] }
    else if (path === '/api/v1/annotations/next') data = { stats, movie }
    else if (path === '/api/v1/annotations/candidates') data = { stats, movies: [] }
    else if (path === '/api/v1/annotations/likes') { data = [movie]; meta = { total: 1 } }
    else if (path === '/api/v1/admin/jobs') { data = [{ id: '12345678-test', type: 'search', status: retried ? 'pending' : 'failed', attempt: 1, created_at: '2026-11-13T12:00:00Z' }]; meta = { total: 1 } }
    else if (path === '/api/v1/admin/jobs/12345678-test/retries') { retried = true; data = {} }
    else return route.fulfill({ status: 404, json: { error: { code: 'unmocked_request' } } })
    return route.fulfill({ json: { data, meta } })
  })
}

test('手机底部导航和更多菜单可操作且没有横向溢出', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 320, height: 640 })
  await mockApp(page)
  await page.goto('/movies')
  await expect(page.getByText('TEST-001').first()).toBeVisible()
  const navigation = page.getByRole('navigation', { name: '手机导航' })
  await expect(navigation).toBeVisible()
  await navigation.getByRole('button', { name: '更多' }).click()
  const more = page.getByRole('dialog', { name: '更多' })
  await expect(more.getByRole('button', { name: '关闭' })).toBeFocused()
  await expect(more.getByRole('link', { name: '人工标注' })).toBeVisible()
  await expect(more.getByRole('link', { name: '管理' })).toBeVisible()
  await expect(more.getByText('admin@example.test')).toBeVisible()
  await more.getByLabel('语言').selectOption('en')
  await expect(page.getByRole('heading', { name: 'Movies' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(more).not.toBeVisible()
  await expect(page.getByRole('button', { name: 'More' })).toBeFocused()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: testInfo.outputPath('movies-320.png') })
})

test('手机导航可浏览演员、推荐、标注和喜欢列表', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 640 })
  await mockApp(page)
  await page.goto('/movies')
  const navigation = page.getByRole('navigation', { name: '手机导航' })
  await navigation.getByRole('link', { name: '演员' }).click()
  await expect(page.getByRole('link', { name: '测试演员' })).toBeVisible()
  await navigation.getByRole('link', { name: '猜你喜欢' }).click()
  await expect(page.getByText('TEST-001').first()).toBeVisible()
  await page.getByRole('link', { name: '继续标注训练' }).click()
  await expect(page.getByRole('heading', { name: '人工标注' })).toBeVisible()
  await expect(page.getByText('TEST-001').first()).toBeVisible()
  await navigation.getByRole('link', { name: '喜欢过' }).click()
  await expect(page.getByRole('heading', { name: '喜欢过' })).toBeVisible()
  await expect(page.getByText('TEST-001').first()).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})

test('桌面侧栏和影片列表显示完整', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await mockApp(page)
  await page.goto('/movies')
  await expect(page.getByRole('navigation', { name: '主导航' })).toBeVisible()
  await expect(page.getByRole('heading', { name: '影片' })).toBeVisible()
  await expect(page.getByText('TEST-001').first()).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  await page.screenshot({ path: testInfo.outputPath('movies-desktop.png') })
})

test('手机任务卡片完整显示信息并可重试', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 640 })
  await mockApp(page)
  await page.goto('/admin/tasks')
  const row = page.getByRole('row', { name: /12345678/ })
  await expect(row).toContainText('失败')
  await row.getByTitle('重试').click()
  await expect(row).toContainText('等待中')
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})

test('手机详情中的播放器可打开并关闭', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 640 })
  await mockApp(page)
  await page.goto('/movies/TEST-001')
  await page.getByRole('button', { name: '查找并播放' }).click()
  await expect(page.getByText('未找到可播放的来源')).toBeVisible()
  const frame = page.locator('.video-frame')
  await expect(frame).toBeVisible()
  expect(await frame.evaluate(element => element.getBoundingClientRect().width)).toBeLessThanOrEqual(320)
  await page.locator('.playback-section').getByRole('button', { name: '关闭' }).click()
  await expect(frame).not.toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})
