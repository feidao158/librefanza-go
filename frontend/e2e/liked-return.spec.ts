import { expect, test } from '@playwright/test'

for (const returnMethod of ['页面返回', '浏览器后退'] as const) {
  test(`喜欢列表进入详情后通过${returnMethod}恢复页码和位置`, async ({ page }) => {
    const items = Array.from({ length: 30 }, (_, index) => ({
      id: index + 1,
      normalized_id: `LIKE-${String(index + 1).padStart(3, '0')}`,
      preferred_item: { id: 1, source: 'fanza', title: `作品 ${index + 1}`, metadata: {} },
    }))
    await page.addInitScript(() => window.localStorage.setItem('librefanza-access-token', 'test-token'))
    await page.route('**/api/v1/setup', route => route.fulfill({ json: { data: { installed: true } } }))
    await page.route('**/api/v1/auth/me', route => route.fulfill({ json: { data: { id: 1, email: 'test@example.com', is_admin: false } } }))
    await page.route('**/api/v1/annotations/likes?*', async route => {
      await new Promise(resolve => setTimeout(resolve, 250))
      await route.fulfill({ json: { data: items, meta: { total: 60 } } })
    })
    await page.route('**/api/v1/movies/LIKE-020', route => route.fulfill({ json: { data: { ...items[19], versions: [items[19].preferred_item, { id: 2, source: 'sod', title: '另一个来源', metadata: {} }] } } }))
    await page.route('**/api/v1/movies/LIKE-020/metadata-job', route => route.fulfill({ status: 404, json: { error: { code: 'not_found' } } }))
    await page.goto('/likes?page=2')
    const card = page.getByRole('link', { name: /LIKE-020/ })
    await card.evaluate(element => element.scrollIntoView({ block: 'center' }))
    const savedY = await page.evaluate(() => window.scrollY)
    expect(savedY).toBeGreaterThan(300)
    await card.focus()
    await card.press('Enter')
    await expect(page.getByRole('heading', { name: 'LIKE-020' })).toBeVisible()
    if (returnMethod === '页面返回') {
      const otherSource = page.getByRole('link', { name: /另一个来源/ })
      await otherSource.focus()
      await otherSource.press('Enter')
      const back = page.locator('.back-link')
      await expect(back).toHaveText('喜欢过')
      await back.focus()
      await back.press('Enter')
    } else {
      await page.goBack()
    }
    await expect(page).toHaveURL(/\/likes\?page=2/)
    await expect(page.getByRole('link', { name: /LIKE-020/ })).toBeVisible()
    await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(savedY - 30)
    expect(await page.evaluate(() => window.scrollY)).toBeLessThan(savedY + 30)
  })
}

test('喜欢列表首页加载失败后重试仍恢复浏览位置', async ({ page }) => {
  const items = Array.from({ length: 30 }, (_, index) => ({
    id: index + 1,
    normalized_id: `LIKE-${String(index + 1).padStart(3, '0')}`,
    preferred_item: { id: 1, source: 'fanza', title: `作品 ${index + 1}`, metadata: {} },
  }))
  let listRequests = 0
  await page.addInitScript(() => window.localStorage.setItem('librefanza-access-token', 'test-token'))
  await page.route('**/api/v1/setup', route => route.fulfill({ json: { data: { installed: true } } }))
  await page.route('**/api/v1/auth/me', route => route.fulfill({ json: { data: { id: 1, email: 'test@example.com', is_admin: false } } }))
  await page.route('**/api/v1/annotations/likes?*', route => {
    listRequests++
    if (listRequests === 2) return route.fulfill({ status: 500, json: { error: { code: 'server_error' } } })
    return route.fulfill({ json: { data: items, meta: { total: 30 } } })
  })
  await page.route('**/api/v1/movies/LIKE-020', route => route.fulfill({ json: { data: items[19] } }))
  await page.route('**/api/v1/movies/LIKE-020/metadata-job', route => route.fulfill({ status: 404, json: { error: { code: 'not_found' } } }))
  await page.goto('/likes')
  const card = page.getByRole('link', { name: /LIKE-020/ })
  await card.evaluate(element => element.scrollIntoView({ block: 'center' }))
  const savedY = await page.evaluate(() => window.scrollY)
  await card.focus()
  await card.press('Enter')
  await expect(page.getByRole('heading', { name: 'LIKE-020' })).toBeVisible()
  await page.locator('.back-link').focus()
  await page.locator('.back-link').press('Enter')
  await expect(page.getByRole('button', { name: '重试' })).toBeVisible()
  await page.getByRole('button', { name: '重试' }).focus()
  await page.getByRole('button', { name: '重试' }).press('Enter')
  await expect(page.getByRole('link', { name: /LIKE-020/ })).toBeVisible()
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(savedY - 30)
  expect(await page.evaluate(() => window.scrollY)).toBeLessThan(savedY + 30)
})
