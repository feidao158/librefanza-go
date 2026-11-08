export interface MovieMetadata {
  cast?: { name: string; fanza_id?: number }[]
  genres?: string[]
  labels?: string[]
  makers?: string[]
  directors?: string[]
  series?: string[]
  sample_image_urls?: string[]
  sample_image_original_urls?: string[]
  volume_minutes?: number
  rating?: number
}

export interface SourceItem {
  id: number
  source: string
  external_id: string
  normalized_id: string
  priority: number
  title: string
  release_date?: string | null
  description: string
  source_url: string
  cover_image_url: string
  cover_image_original_url?: string
  thumbnail_image_url: string
  metadata: MovieMetadata
  created_at: string
  updated_at: string
}

export interface Movie {
  id: number
  normalized_id: string
  compressed_id: string
  release_date?: string | null
  cover_image_url?: string
  is_hidden: boolean
  created_at: string
  updated_at: string
  preferred_item?: SourceItem
  versions?: SourceItem[]
}

export interface Actress {
  id: number
  fanza_id?: number
  name: string
  image_url: string
}

export interface User {
  id: number
  email: string
  is_admin: boolean
}

export interface AnnotationStats {
  total: number
  liked: number
  disliked: number
  uncertain: number
  min_required: number
  ready: boolean
}

export interface NextAnnotationResponse {
  movie: Movie | null
  stats: AnnotationStats
}

export interface FeedbackResponse {
  success: boolean
  stats: AnnotationStats
  next_movie?: Movie | null
}

export interface RecommendationResponse {
  ready: boolean
  stats: AnnotationStats
  movies: Movie[]
}

export interface GorseSettings {
  entry_point: string
  api_key?: string
  api_key_configured: boolean
  enabled: boolean
  min_feedback_count: number
  managed?: boolean
  connected?: boolean
  sync_error?: boolean
  sync?: { phase: string; cursor: number; ready: boolean; next_cycle: string }
}
