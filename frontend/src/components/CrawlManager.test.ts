import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import CrawlManager from './CrawlManager.vue'
import { i18n } from '../i18n'

const request = vi.hoisted(() => vi.fn())
vi.mock('../api/client', () => ({ api: request, jsonBody: JSON.stringify, ApiError: class extends Error {} }))

describe('采集管理', () => {
  beforeEach(() => {
    request.mockReset()
    request.mockImplementation(async (path: string) => {
      if (path === '/admin/settings/schedules') {
        const schedule = () => ({ enabled: false, cron: '*/30 * * * *' })
        return { data: { timezone: 'Asia/Shanghai', actress: schedule(), fanza: schedule(), mgstage: schedule(), jable: schedule(), avmoo: schedule(), javlibrary: schedule(), libredmm: schedule(), javdb: schedule(), javbus: schedule(), javbooks: schedule(), javquery: schedule() } }
      }
      if (path === '/admin/metadata-backfill') return { data: { progress: { phase: 'raw', processed: 0, failed: 0, completed: true }, failed_jobs: 0 } }
      return { data: path.startsWith('/admin/jobs') ? [] : {} }
    })
  })

  it('仅展示现役来源的采集入口', async () => {
    const wrapper = mount(CrawlManager, { global: { plugins: [i18n], stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await flushPromises()
    const names = wrapper.findAll('.crawl-row h3').map(row => row.text())
    expect(names).toEqual(expect.arrayContaining(['JavDB', 'JavBooks', 'JavQuery']))
    expect(names).not.toContain('JavBus')
    expect(names).not.toContain('AVMOO')
    expect(names).not.toContain('JavLibrary')
    wrapper.unmount()
  })

  it('显示来源采集跳过的无效详情数量', async () => {
    const previous = request.getMockImplementation()!
    request.mockImplementation(async (path: string, ...args: unknown[]) => {
      if (path.startsWith('/admin/jobs')) return { data: [{ id: 'job', type: 'source_crawl', status: 'completed', created_at: '2026-11-07T01:00:00Z', crawl: { source: 'javdb', mode: 'incremental' }, progress: { processed: 2, skipped: 3, undated: 0 } }] }
      return previous(path, ...args)
    })
    const wrapper = mount(CrawlManager, { global: { plugins: [i18n], stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await flushPromises()
    expect(wrapper.text()).toContain('已跳过 3')
    wrapper.unmount()
  })

  it('手动关闭新来源时保存人工选择标记', async () => {
    const previous = request.getMockImplementation()!
    let saved: Record<string, { enabled: boolean; auto?: boolean }> | undefined
    request.mockImplementation(async (path: string, options?: { method?: string; body?: string }) => {
      if (path === '/admin/settings/schedules' && options?.method === 'PUT') {
        saved = JSON.parse(options.body || '{}')
        return { data: {} }
      }
      if (path === '/admin/settings/schedules') {
        const response = await previous(path)
        response.data.javdb = { enabled: true, cron: '*/30 * * * *', auto: true }
        return response
      }
      return previous(path)
    })
    const wrapper = mount(CrawlManager, { global: { plugins: [i18n], stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await flushPromises()
    const row = wrapper.findAll('.crawl-row').find(value => value.text().includes('JavDB'))!
    await row.get('input[type=checkbox]').setValue(false)
    for (const name of ['JavBooks', 'JavQuery']) {
      const entry = wrapper.findAll('.crawl-row').find(value => value.text().includes(name))!
      await entry.get('input[type=checkbox]').setValue(true)
    }
    await wrapper.findAll('button').find(value => value.text().includes('保存'))!.trigger('click')
    await flushPromises()
    expect(saved?.javdb).toMatchObject({ enabled: false, auto: false })
    for (const source of ['javbooks', 'javquery']) expect(saved?.[source]).toMatchObject({ enabled: true, auto: false })
    expect(saved?.jable?.auto).toBeUndefined()
    wrapper.unmount()
  })
})
