import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AnnotationsView from './AnnotationsView.vue'
import { i18n } from '../i18n'

describe('AnnotationsView', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn((url: string, options?: RequestInit) => {
      if (url.includes('/api/v1/annotations/next')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({
            data: {
              movie: {
                id: 1,
                normalized_id: 'TEST-101',
                compressed_id: 'TEST101',
                is_hidden: false,
                created_at: '',
                updated_at: '',
                preferred_item: {
                  id: 10,
                  source: 'fanza',
                  external_id: 'test101',
                  normalized_id: 'TEST-101',
                  priority: 0,
                  title: '测试番号标题',
                  cover_image_url: 'https://example.com/cover.jpg',
                  thumbnail_image_url: '',
                  source_url: '',
                  description: '测试简介',
                  metadata: {
                    cast: [{ name: '测试演员' }],
                    genres: ['测试分类'],
                  },
                  created_at: '',
                  updated_at: '',
                },
              },
              stats: {
                total: 2,
                liked: 1,
                disliked: 1,
                uncertain: 0,
                min_required: 5,
                ready: false,
              },
            },
          }),
        })
      }
      if (url.includes('/api/v1/annotations/feedback')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({
            data: {
              success: true,
              stats: {
                total: 3,
                liked: 2,
                disliked: 1,
                uncertain: 0,
                min_required: 5,
                ready: false,
              },
              next_movie: null,
            },
          }),
        })
      }
      return Promise.reject(new Error('Unknown url: ' + url))
    }))
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders the movie details and allows rating', async () => {
    const wrapper = mount(AnnotationsView, {
      global: {
        plugins: [i18n],
        stubs: { RouterLink: { template: '<a><slot /></a>' } },
      },
    })

    // 等待数据加载
    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('TEST-101')
      expect(wrapper.text()).toContain('测试番号标题')
      expect(wrapper.text()).toContain('测试演员')
      expect(wrapper.text()).toContain('测试分类')
      expect(wrapper.find('a[href="https://jable.tv/videos/test-101/"]').exists()).toBe(true)
    })

    // 找到喜欢按钮并点击
    const likeBtn = wrapper.findAll('button').find(b => b.text().includes('喜欢') || b.text().includes('Like'))
    expect(likeBtn).toBeDefined()
    await likeBtn!.trigger('click')

    // 验证调用了 feedback 接口
    await vi.waitFor(() => {
      const calls = (globalThis.fetch as any).mock.calls
      const feedbackCall = calls.find((c: any) => c[0].includes('/annotations/feedback'))
      expect(feedbackCall).toBeDefined()
      const body = JSON.parse(feedbackCall[1].body)
      expect(body.normalized_id).toBe('TEST-101')
      expect(body.action).toBe('like')
    })
    wrapper.unmount()
  })

  it('allows skipping to the next movie', async () => {
    const wrapper = mount(AnnotationsView, {
      global: {
        plugins: [i18n],
        stubs: { RouterLink: { template: '<a><slot /></a>' } },
      },
    })

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('TEST-101')
    })

    const skipBtn = wrapper.findAll('button').find(b => b.text().includes('换一个') || b.text().includes('Next'))
    expect(skipBtn).toBeDefined()
    await skipBtn!.trigger('click')

    await vi.waitFor(() => {
      const calls = (globalThis.fetch as any).mock.calls
      const nextCall = calls.find((c: any) => c[0].includes('/annotations/next?exclude=TEST-101'))
      expect(nextCall).toBeDefined()
    })
    wrapper.unmount()
  })

  it('操作按钮时空格不会跳过影片，页面空白处仍可使用快捷键', async () => {
    const wrapper = mount(AnnotationsView, { global: { plugins: [i18n], stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await vi.waitFor(() => expect(wrapper.text()).toContain('TEST-101'))
    const button = document.createElement('button')
    document.body.appendChild(button)
    button.dispatchEvent(new KeyboardEvent('keydown', { key: ' ', code: 'Space', bubbles: true, cancelable: true }))
    const calls = (globalThis.fetch as any).mock.calls
    expect(calls.filter((call: any) => call[0].includes('/annotations/next'))).toHaveLength(1)
    document.body.dispatchEvent(new KeyboardEvent('keydown', { key: ' ', code: 'Space', bubbles: true, cancelable: true }))
    await vi.waitFor(() => expect(calls.filter((call: any) => call[0].includes('/annotations/next'))).toHaveLength(2))
    button.remove()
    wrapper.unmount()
  })

  it('使用后台候选切换且不展示服务端判定失效的影片', async () => {
    const stats = { total: 2, liked: 1, disliked: 1, uncertain: 0, min_required: 5, ready: false }
    const result = (data: unknown) => ({ ok: true, status: 200, json: async () => ({ data }) })
    const mock = vi.fn(async (url: string) => {
      if (url.includes('/annotations/next')) return result({ movie: { normalized_id: 'TEST-101' }, stats })
      if (url.includes('/annotations/candidates')) return result({ movies: [{ normalized_id: 'TEST-102' }, { normalized_id: 'TEST-103' }], stats })
      if (url.includes('/annotations/feedback')) return result({ stats, eligible_ids: ['TEST-103'] })
      throw new Error('未知请求')
    })
    vi.stubGlobal('fetch', mock)
    const wrapper = mount(AnnotationsView, { global: { plugins: [i18n], stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await vi.waitFor(() => expect(mock.mock.calls.some(([url]) => url.includes('/annotations/candidates'))).toBe(true))
    await vi.waitFor(() => expect(wrapper.text()).toContain('TEST-101'))
    await wrapper.findAll('.grid-cols-3 button')[0].trigger('click')
    await vi.waitFor(() => expect(wrapper.text()).toContain('TEST-103'))
    expect(wrapper.text()).not.toContain('TEST-102')
    expect(mock.mock.calls.filter(([url]) => url.includes('/annotations/next'))).toHaveLength(1)
    wrapper.unmount()
  })
})
