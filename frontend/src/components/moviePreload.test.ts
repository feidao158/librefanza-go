import { describe, expect, it, vi } from 'vitest'
import { MoviePreload } from './moviePreload'
import type { Movie } from '../types'

const movie = (id: string, cover = '') => ({ normalized_id: id, cover_image_url: cover } as Movie)

describe('后台影片预加载', () => {
  it('去重并按服务端确认的顺序消费候选', () => {
    const queue = new MoviePreload()
    queue.add([movie('A'), movie('B'), movie('A')], 'A')
    expect(queue.ids()).toEqual(['B'])
    queue.add([movie('C'), movie('B')], 'A')
    expect(queue.takeEligible(['C', 'B'])?.normalized_id).toBe('C')
    expect(queue.ids()).toEqual(['B'])
    expect(queue.takeEligible([])).toBeNull()
    expect(queue.ids()).toEqual([])
  })
  it('只预热有限数量的封面和预览图', () => {
    const created: string[] = []
    const warm = vi.fn((url: string) => created.push(url))
    const queue = new MoviePreload(warm)
    const films = Array.from({ length: 8 }, (_, i) => ({ ...movie(String(i), `cover${i}`), preferred_item: { metadata: { sample_image_urls: [`sample${i}`, `more${i}`] } } } as Movie))
    queue.add(films, '')
    expect(queue.ids()).toHaveLength(5)
    expect(created).toHaveLength(10)
    expect(created).not.toContain('more0')
  })
})
