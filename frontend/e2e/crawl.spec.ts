import { expect, test } from '@playwright/test'

test('管理员可以按范围启动来源采集', async ({ page }) => {
  const schedule = { timezone: 'Asia/Shanghai', actress: { enabled: true, cron: '0 0 * * *' }, fanza: { enabled: true, cron: '0 1 * * *' }, mgstage: { enabled: true, cron: '0 2 * * *' }, jable: { enabled: false, cron: '0 3 * * *' }, avmoo: { enabled: false, cron: '0 4 * * *' }, javlibrary: { enabled: false, cron: '0 5 * * *' }, libredmm: { enabled: false, cron: '0 6 * * *' } }
  let submitted: Record<string, unknown> | undefined
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname
    let data: unknown = {}
    if (path.endsWith('/setup')) data = { installed: true }
    else if (path.endsWith('/auth/refresh')) data = { access_token: 'test' }
    else if (path.endsWith('/auth/me')) data = { id: 1, email: 'admin@example.com', is_admin: true }
    else if (path.endsWith('/settings/schedules')) data = schedule
    else if (path.endsWith('/schedule-previews')) data = { fanza: '2026-11-07T01:00:00+08:00' }
    else if (path.endsWith('/admin/jobs')) data = []
    else if (path.endsWith('/admin/crawl-jobs')) { submitted = route.request().postDataJSON(); data = { id: 'test-job' } }
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data }) })
  })
  await page.goto('/admin/schedule')
  await expect(page.getByRole('heading', { name: '采集管理' })).toBeVisible()
  await expect(page.locator('.crawl-row')).toHaveCount(7)
  await page.locator('.crawl-row').filter({ hasText: 'AVMOO' }).getByRole('button', { name: '立即运行' }).click()
  await page.getByLabel('采集范围').selectOption('ids')
  await page.getByLabel('番号前缀').fill('ABC')
  await page.getByLabel('起始编号').fill('1')
  await page.getByLabel('结束编号').fill('20')
  await page.getByRole('dialog').getByRole('button', { name: '立即运行' }).click()
  await expect.poll(() => submitted).toMatchObject({ source: 'avmoo', mode: 'ids', prefix: 'ABC', start_number: 1, end_number: 20 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
})

test('影片播放按需检索并呈现来源不可用状态', async ({ page }) => {
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/auth/refresh')) return route.fulfill({ status: 401, contentType: 'application/json', body: '{}' })
    let data: unknown = {}
    if (path.endsWith('/setup')) data = { installed: true }
    else if (path.endsWith('/movies/ABC-123/playbacks')) data = []
    else if (path.endsWith('/movies/ABC-123')) data = { normalized_id: 'ABC-123', is_hidden: false, preferred_item: { id: 1, source: 'fanza', title: 'Example', metadata: {}, source_url: 'https://example.com/' }, versions: [] }
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data }) })
  })
  await page.goto('/movies/ABC-123')
  await page.getByRole('button', { name: '查找并播放' }).click()
  await expect(page.getByText('未找到可播放的来源')).toBeVisible()
})

test('直连失败后尝试服务器转发', async ({ page }) => {
  await page.route('https://cdn.invalid/**', route => route.abort())
  await page.route('**/api/v1/**', async route => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/auth/refresh')) return route.fulfill({ status: 401, contentType: 'application/json', body: '{}' })
    if (path.includes('/playback/relay/')) return route.fulfill({ contentType: 'application/vnd.apple.mpegurl', body: '#EXTM3U\n#EXT-X-TARGETDURATION:10\n#EXTINF:8,\nsegment.ts\n#EXT-X-ENDLIST\n' })
    let data: unknown = {}
    if (path.endsWith('/setup')) data = { installed: true }
    else if (path.endsWith('/movies/ABC-123/playbacks')) data = [{ source: 'jable', direct_url: 'https://cdn.invalid/a.m3u8', relay_url: '/api/v1/playback/relay/test', source_url: 'https://jable.tv/videos/abc-123/' }]
    else if (path.endsWith('/movies/ABC-123')) data = { normalized_id: 'ABC-123', is_hidden: false, preferred_item: { id: 1, source: 'fanza', title: 'Example', metadata: {}, source_url: 'https://example.com/' }, versions: [] }
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data }) })
  })
  await page.goto('/movies/ABC-123')
  await page.getByRole('button', { name: '查找并播放' }).click()
  await expect(page.getByText('服务器转发')).toBeVisible({ timeout: 15_000 })
})
