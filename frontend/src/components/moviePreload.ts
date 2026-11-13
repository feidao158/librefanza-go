import type { Movie } from '../types'

function preloadImage(url: string) {
  const image = new Image()
  image.referrerPolicy = 'no-referrer'
  image.src = url
}

export class MoviePreload {
  private movies: Movie[] = []
  constructor(private readonly warm: (url: string) => void = preloadImage) {}

  add(movies: Movie[], current: string) {
    const known = new Set([current, ...this.movies.map(movie => movie.normalized_id)])
    for (const movie of movies) {
      if (!movie.normalized_id || known.has(movie.normalized_id) || this.movies.length >= 5) continue
      known.add(movie.normalized_id)
      this.movies.push(movie)
      const urls = [movie.preferred_item?.cover_image_url || movie.cover_image_url, movie.preferred_item?.metadata?.sample_image_urls?.[0]]
      for (const url of new Set(urls.filter((value): value is string => Boolean(value)))) this.warm(url)
    }
  }

  ids() { return this.movies.map(movie => movie.normalized_id) }

  takeEligible(ids: string[]): Movie | null {
    const allowed = new Set(ids)
    this.movies = this.movies.filter(movie => allowed.has(movie.normalized_id))
    const next = ids.find(id => this.movies.some(movie => movie.normalized_id === id))
    if (!next) return null
    const index = this.movies.findIndex(movie => movie.normalized_id === next)
    return this.movies.splice(index, 1)[0]
  }

  clear() { this.movies = [] }
}
