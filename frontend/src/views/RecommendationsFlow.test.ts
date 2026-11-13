import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import RecommendationsView from './RecommendationsView.vue'
import { i18n } from '../i18n'

const stats = { total: 5, liked: 3, disliked: 1, uncertain: 1, min_required: 5, ready: true }
const response = (data: unknown) => ({ ok: true, status: 200, json: async () => ({ data }) })
const mountView = () => mount(RecommendationsView, { global: { plugins: [i18n], stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true, Transition: false } } })

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

describe('推荐连续标记', () => {
  it('等待下一部期间保留当前卡片并阻止重复标记', async () => {
    let resolveNext!: (value: ReturnType<typeof response>) => void
    let requests = 0
    const fetchMock = vi.fn((url: string) => {
      if (url.includes('/feedback')) return Promise.resolve(response({ stats }))
      requests++
      if (requests === 1) return Promise.resolve(response({ ready: true, stats, movies: [{ normalized_id: 'REC-001' }] }))
      return new Promise<ReturnType<typeof response>>(resolve => { resolveNext = resolve })
    })
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mountView()
    await vi.waitFor(() => expect(wrapper.text()).toContain('REC-001'))
    expect(wrapper.find('a[href="https://jable.tv/videos/rec-001/"]').exists()).toBe(true)
    await wrapper.findAll('.grid-cols-3 button')[0].trigger('click')
    await vi.waitFor(() => expect(requests).toBe(2))
    expect(wrapper.text()).toContain('REC-001')
    expect(wrapper.findAll('.grid-cols-3 button').every(button => button.attributes('disabled') !== undefined)).toBe(true)
    window.dispatchEvent(new KeyboardEvent('keydown', { key: '1' }))
    expect(fetchMock.mock.calls.filter(([url]) => url.includes('/feedback'))).toHaveLength(1)
    resolveNext(response({ ready: true, stats, movies: [{ normalized_id: 'REC-002' }] }))
    await vi.waitFor(() => expect(wrapper.text()).toContain('REC-002'))
    expect(wrapper.find('a[href="https://jable.tv/videos/rec-002/"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('每次标记后重新请求一条推荐，不显示队列或循环旧作品', async () => {
    let count = 0
    const fetchMock = vi.fn(async (url: string) => url.includes('/feedback')
      ? response({ stats })
      : response({ ready: true, stats, movies: count++ < 2 ? [{ normalized_id: `REC-00${count}` }] : [] }))
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mountView()
    await vi.waitFor(() => expect(wrapper.text()).toContain('REC-001'))
    expect(wrapper.text()).not.toContain('推荐队列')
    expect(wrapper.text()).not.toContain('换一批推荐')
    await wrapper.findAll('.grid-cols-3 button')[0].trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('REC-002'))
    await vi.waitFor(() => expect(wrapper.findAll('.grid-cols-3 button')[1].attributes('disabled')).toBeUndefined())
    await wrapper.findAll('.grid-cols-3 button')[1].trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('暂无推荐结果'))
    expect(fetchMock.mock.calls.filter(([url]) => !url.includes('/feedback')).map(([url]) => url)).toEqual([
      expect.stringContaining('page_size=5'), expect.stringContaining('page_size=5'), expect.stringContaining('page_size=5'),
    ])
    wrapper.unmount()
  })

  it('后台候选通过校验后直接切换，失效候选不会展示', async () => {
    const fetchMock = vi.fn(async (url: string) => url.includes('/feedback')
      ? response({ stats, eligible_ids: ['REC-003'] })
      : response({ ready: true, stats, movies: ['REC-001', 'REC-002', 'REC-003'].map(normalized_id => ({ normalized_id })) }))
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mountView()
    await vi.waitFor(() => expect(wrapper.text()).toContain('REC-001'))
    await wrapper.findAll('.grid-cols-3 button')[0].trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('REC-003'))
    expect(wrapper.text()).not.toContain('REC-002')
    expect(fetchMock.mock.calls.filter(([url]) => !url.includes('/feedback'))).toHaveLength(1)
    wrapper.unmount()
  })

  it('反馈失败时保留当前作品并提示错误', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url: string) => url.includes('/feedback')
      ? { ok: false, status: 500, json: async () => ({}) }
      : response({ ready: true, stats, movies: [{ normalized_id: 'REC-001' }] })))
    const wrapper = mountView()
    await vi.waitFor(() => expect(wrapper.text()).toContain('REC-001'))
    await wrapper.findAll('.grid-cols-3 button')[0].trigger('click')
    await vi.waitFor(() => expect(wrapper.find('.alert-error').exists()).toBe(true))
    expect(wrapper.text()).toContain('REC-001')
    wrapper.unmount()
  })

  it('反馈成功但下一条获取失败时，重试只获取推荐，不重复提交反馈', async () => {
    let count = 0
    const fetchMock = vi.fn(async (url: string) => {
      if (url.includes('/feedback')) return response({ stats })
      count++
      if (count === 2) return { ok: false, status: 503, json: async () => ({ error: { code: 'recommendation_unavailable' } }) }
      return response({ ready: true, stats, movies: [{ normalized_id: count === 1 ? 'REC-001' : 'REC-002' }] })
    })
    vi.stubGlobal('fetch', fetchMock)
    const wrapper = mountView()
    await vi.waitFor(() => expect(wrapper.text()).toContain('REC-001'))
    await wrapper.findAll('.grid-cols-3 button')[0].trigger('click')
    await vi.waitFor(() => expect(wrapper.find('.alert-error').exists()).toBe(true))
    expect(wrapper.text()).toContain('REC-001')
    expect(wrapper.findAll('.grid-cols-3 button').every(button => button.attributes('disabled') !== undefined)).toBe(true)
    await wrapper.find('.alert-error button').trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('REC-002'))
    expect(fetchMock.mock.calls.filter(([url]) => url.includes('/feedback'))).toHaveLength(1)
    wrapper.unmount()
  })
})
