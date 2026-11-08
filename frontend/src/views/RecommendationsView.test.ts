import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import RecommendationsView from './RecommendationsView.vue'
import { i18n } from '../i18n'

describe('RecommendationsView', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders prompt when annotations threshold is not met', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve({
        data: {
          ready: false,
          stats: {
            total: 2,
            liked: 1,
            disliked: 1,
            uncertain: 0,
            min_required: 5,
            ready: false,
          },
          movies: [],
        },
      }),
    })))

    const wrapper = mount(RecommendationsView, {
      global: {
        plugins: [i18n],
        stubs: { RouterLink: { template: '<a><slot /></a>' } },
      },
    })

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('推荐算法正在学习中')
      expect(wrapper.text()).toContain('前往人工标注')
    })
  })

  it('renders recommendation cards and allows rating when ready', async () => {
    vi.stubGlobal('fetch', vi.fn((url: string) => {
      if (url.includes('/recommendations/feedback')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({
            data: {
              success: true,
              stats: { total: 6, liked: 4, disliked: 1, uncertain: 1, min_required: 5, ready: true },
            },
          }),
        })
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({
          data: {
            ready: true,
            stats: {
              total: 5,
              liked: 3,
              disliked: 1,
              uncertain: 1,
              min_required: 5,
              ready: true,
            },
            movies: [
              {
                id: 1,
                normalized_id: 'REC-001',
                compressed_id: 'REC001',
                is_hidden: false,
                created_at: '',
                updated_at: '',
                preferred_item: {
                  id: 11,
                  source: 'fanza',
                  external_id: 'rec001',
                  normalized_id: 'REC-001',
                  priority: 0,
                  title: '推荐影片标题1',
                  cover_image_url: 'https://example.com/rec1.jpg',
                  thumbnail_image_url: '',
                  source_url: '',
                  description: '',
                  metadata: {
                    cast: [{ name: '推荐演员1' }],
                  },
                  created_at: '',
                  updated_at: '',
                },
              },
              {
                id: 2,
                normalized_id: 'REC-002',
                compressed_id: 'REC002',
                is_hidden: false,
                created_at: '',
                updated_at: '',
                preferred_item: {
                  id: 12,
                  source: 'fanza',
                  external_id: 'rec002',
                  normalized_id: 'REC-002',
                  priority: 0,
                  title: '推荐影片标题2',
                  cover_image_url: 'https://example.com/rec2.jpg',
                  thumbnail_image_url: '',
                  source_url: '',
                  description: '',
                  metadata: {
                    cast: [{ name: '推荐演员2' }],
                  },
                  created_at: '',
                  updated_at: '',
                },
              },
            ],
          },
        }),
      })
    }))

    const wrapper = mount(RecommendationsView, {
      global: {
        plugins: [i18n],
        stubs: { RouterLink: { template: '<a><slot /></a>' }, Teleport: true },
      },
    })

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('REC-001')
      expect(wrapper.text()).toContain('推荐影片标题1')
    })

    const refresh = wrapper.findAll('button').find(b => b.text().includes('换一批推荐'))
    expect(refresh).toBeDefined()
    await refresh!.trigger('click')
    await vi.waitFor(() => {
      expect((globalThis.fetch as any).mock.calls.some((c: any) => c[0].includes('page=2'))).toBe(true)
    })

    // 点击推荐卡片上的“喜欢”按钮
    const cardButtons = wrapper.findAll('.grid-cols-3 button')
    expect(cardButtons.length).toBeGreaterThanOrEqual(3)
    await cardButtons[0].trigger('click')

    await vi.waitFor(() => {
      const calls = (globalThis.fetch as any).mock.calls
      const feedbackCall = calls.find((c: any) => c[0].includes('/recommendations/feedback'))
      expect(feedbackCall).toBeDefined()
      const body = JSON.parse(feedbackCall[1].body)
      expect(body.normalized_id).toBe('REC-001')
      expect(body.action).toBe('like')
      // 评分后自动平滑推进至下一部 REC-002
      expect(wrapper.text()).toContain('REC-002')
    })

    // 找到上一部按钮并点击
    const prevMovieBtn = wrapper.findAll('button').find(b => b.text().includes('上一部') || b.attributes('title')?.includes('上一部'))
    expect(prevMovieBtn).toBeDefined()
    await prevMovieBtn!.trigger('click')

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('REC-001')
    })

    // 找到下一部按钮并点击
    const nextMovieBtn = wrapper.findAll('button').find(b => b.text().includes('下一部') || b.attributes('title')?.includes('下一部'))
    expect(nextMovieBtn).toBeDefined()
    await nextMovieBtn!.trigger('click')

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('REC-002')
    })
  })
})
