import { describe, expect, it } from 'vitest'
import { collectMovieDetails } from './movieDetails'
import type { SourceItem } from '../types'

describe('影片来源信息汇总', () => {
  it('保留来源并去重图片和字段', () => {
    const items = [
      { id: 1, source: 'fanza', cover_image_url: 'https://example.com/cover.jpg', metadata: { genres: ['剧情'], sample_image_urls: ['https://example.com/1.jpg'] } },
      { id: 2, source: 'sod', cover_image_url: 'https://example.com/cover.jpg', metadata: { genres: ['剧情', '其他'], sample_image_urls: ['https://example.com/1.jpg', 'https://example.com/2.jpg'] } },
    ] as SourceItem[]
    const details = collectMovieDetails(items)
    expect(details.genres.map(item => item.value)).toEqual(['剧情', '其他'])
    expect(details.genres[0].sources).toEqual(['fanza', 'sod'])
    expect(details.images.map(item => item.url)).toEqual(['https://example.com/cover.jpg', 'https://example.com/1.jpg', 'https://example.com/2.jpg'])
    expect(details.images[1].sources).toEqual(['fanza', 'sod'])
  })

  it('将代理图片与相同的源站地址识别为同一图片', () => {
    const items = [
      { id: 1, source: 'fanza', metadata: { sample_image_urls: ['https://example.com/1.jpg'] } },
      { id: 2, source: 'sod', metadata: { sample_image_urls: ['/api/v1/source-items/2/images/samples/0'], sample_image_original_urls: ['https://example.com/1.jpg'] } },
    ] as SourceItem[]
    const details = collectMovieDetails(items)
    expect(details.images).toHaveLength(1)
    expect(details.images[0].sources).toEqual(['fanza', 'sod'])
  })
})
