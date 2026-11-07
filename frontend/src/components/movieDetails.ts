import type { MovieMetadata, SourceItem } from '../types'

export interface SourcedValue { value: string; sources: string[] }
export interface SourcedImage { url: string; sources: string[] }

export function collectMovieDetails(items: SourceItem[]) {
  const fields: Record<'cast' | 'genres' | 'labels' | 'makers' | 'directors' | 'series', SourcedValue[]> = {
    cast: [], genres: [], labels: [], makers: [], directors: [], series: [],
  }
  const images: SourcedImage[] = []
  const imageKeys = new Map<string, SourcedImage>()
  const addValue = (values: SourcedValue[], value: string, source: string) => {
    const clean = value.trim()
    if (!clean) return
    const existing = values.find(entry => entry.value === clean)
    if (!existing) values.push({ value: clean, sources: [source] })
    else if (!existing.sources.includes(source)) existing.sources.push(source)
  }
  const addImage = (value: string, source: string, original = value) => {
    if (!value) return
    try {
      const address = new URL(value, window.location.origin)
      if (!['http:', 'https:'].includes(address.protocol)) return
      const key = new URL(original, window.location.origin).href
      const existing = imageKeys.get(key)
      if (!existing) { const entry = { url: value, sources: [source] }; images.push(entry); imageKeys.set(key, entry) }
      else if (!existing.sources.includes(source)) existing.sources.push(source)
    } catch { /* 忽略无效图片地址 */ }
  }
  for (const item of items) {
    const metadata: MovieMetadata = item.metadata || {}
    for (const member of metadata.cast || []) addValue(fields.cast, member.name, item.source)
    for (const key of ['genres', 'labels', 'makers', 'directors', 'series'] as const) {
      for (const value of metadata[key] || []) addValue(fields[key], value, item.source)
    }
    addImage(item.cover_image_url, item.source, item.cover_image_original_url || item.cover_image_url)
    for (const [index, image] of (metadata.sample_image_urls || []).entries()) addImage(image, item.source, metadata.sample_image_original_urls?.[index] || image)
  }
  return { ...fields, images }
}
