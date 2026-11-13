import { expect, test, type Page } from '@playwright/test'

async function expectAccount(page: Page, isMobile: boolean, email: string) {
  if (isMobile) {
    await page.getByRole('navigation', { name: '手机导航' }).getByRole('button', { name: '更多' }).click()
    await expect(page.getByRole('dialog', { name: '更多' }).getByText(email, { exact: true })).toBeVisible()
    await page.keyboard.press('Escape')
  } else {
    await expect(page.getByTitle(email)).toBeVisible()
  }
}

test('来源补全延期时显示待恢复的来源数量', async ({ page }) => {
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/setup')) return route.fulfill({ json: { data: { installed: true } } })
    if (path.endsWith('/metadata-job')) return route.fulfill({ json: { data: { id: 'job', status: 'completed', completed_sources: 2, failed_sources: 0, deferred_sources: 3, total_sources: 5, collection_status: 'deferred' } } })
    if (path.endsWith('/movies/ABC-123')) return route.fulfill({ json: { data: { id: 1, normalized_id: 'ABC-123', preferred_item: { id: 1, source: 'fanza', title: 'Example', metadata: {} }, versions: [] } } })
    return route.fulfill({ json: { data: [] } })
  })
  await page.goto('/movies/ABC-123')
  await expect(page.getByText('部分来源将在恢复后继续采集：3')).toBeVisible()
})

test('连续刷新保留令牌并汇总不同来源的详情与图片', async ({ page, isMobile }) => {
  await page.addInitScript(() => window.localStorage.setItem('librefanza-access-token', 'persistent-token'))
  const fanza = { id: 1, source: 'fanza', title: 'Fanza title', normalized_id: 'ABC-123', cover_image_url: '', metadata: { genres: ['剧情'], sample_image_urls: ['https://example.com/a.jpg'] } }
  const sod = { id: 2, source: 'sod', title: 'SOD title', normalized_id: 'ABC-123', cover_image_url: '', metadata: { genres: ['其他'], sample_image_urls: ['https://example.com/b.jpg'] } }
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/auth/me')) {
      expect(route.request().headers().authorization).toBe('Bearer persistent-token')
      return route.fulfill({ json: { data: { id: 1, email: 'admin@example.com', is_admin: true } } })
    }
    if (path.endsWith('/setup')) return route.fulfill({ json: { data: { installed: true } } })
    if (path.endsWith('/movies/ABC-123')) return route.fulfill({ json: { data: { id: 1, normalized_id: 'ABC-123', preferred_item: fanza, versions: [fanza, sod] } } })
    if (path.endsWith('/metadata-job')) return route.fulfill({ status: 404, json: { error: { code: 'not_found' } } })
    return route.fulfill({ json: { data: [] } })
  })
  await page.goto('/movies/ABC-123')
  for (let i = 0; i < 3; i++) {
    await page.reload()
    await expectAccount(page, isMobile, 'admin@example.com')
  }
  await expect(page.getByText('剧情', { exact: false }).first()).toBeVisible()
  await expect(page.getByText('其他', { exact: false }).first()).toBeVisible()
  await expect(page.locator('.sample-grid figure')).toHaveCount(2)
  await expect(page.locator('.sample-grid')).toContainText('FANZA')
  await expect(page.locator('.sample-grid')).toContainText('SOD')
  await page.getByRole('link', { name: 'SOD SOD title' }).click()
  await expect(page.locator('.sample-grid figure')).toHaveCount(1)
})

test('不同标签页同步登录退出且被动轮询不续期', async ({ context, page, isMobile }) => {
  let renewals = 0
  await context.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/setup')) return route.fulfill({ json: { data: { installed: true } } })
    if (path.endsWith('/auth/sessions') && route.request().method() === 'POST') return route.fulfill({ json: { data: { access_token: 'shared-token', user: { id: 1, email: 'user@example.com', is_admin: false } } } })
    if (path.endsWith('/auth/me')) { renewals++; return route.fulfill({ json: { data: { id: 1, email: 'user@example.com', is_admin: false } } }) }
    if (path.endsWith('/auth/sessions/current')) return route.fulfill({ status: 204 })
    return route.fulfill({ json: { data: [], meta: { page: 1, page_size: 30, total: 0 } } })
  })
  const second = await context.newPage()
  await second.goto('/movies')
  await page.goto('/login')
  await page.getByLabel('电子邮箱').fill('user@example.com')
  await page.getByLabel('密码').fill('password123')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expectAccount(second, isMobile, 'user@example.com')
  const beforeIdle = renewals
  await second.waitForTimeout(800)
  expect(renewals).toBe(beforeIdle)
  if (isMobile) await page.getByRole('navigation', { name: '手机导航' }).getByRole('button', { name: '更多' }).click()
  else await page.getByTitle('user@example.com').click()
  await page.getByRole('button', { name: '退出登录' }).click()
  if (isMobile) {
    await second.getByRole('navigation', { name: '手机导航' }).getByRole('button', { name: '更多' }).click()
    await expect(second.getByRole('dialog', { name: '更多' }).getByRole('link', { name: '登录' })).toBeVisible()
  } else {
    await expect(second.getByRole('link', { name: '登录' })).toBeVisible()
  }
  await second.close()
})

test('暂时无法验证时仍显示已缓存的登录用户', async ({ page, isMobile }) => {
  await page.addInitScript(() => {
    window.localStorage.setItem('librefanza-access-token', 'saved-token')
    window.localStorage.setItem('librefanza-user', JSON.stringify({ token: 'saved-token', user: { id: 1, email: 'saved@example.com', is_admin: false } }))
  })
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/setup')) return route.fulfill({ json: { data: { installed: true } } })
    if (path.endsWith('/auth/me')) return route.fulfill({ status: 503, json: { error: { code: 'database_unavailable' } } })
    return route.fulfill({ json: { data: [], meta: { page: 1, page_size: 30, total: 0 } } })
  })
  await page.goto('/movies')
  await expectAccount(page, isMobile, 'saved@example.com')
  expect(await page.evaluate(() => window.localStorage.getItem('librefanza-access-token'))).toBe('saved-token')
})
