import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import MovieCard from './MovieCard.vue'
import { i18n } from '../i18n'

describe('MovieCard', () => {
  it('shows the product ID and source title', () => {
    const wrapper = mount(MovieCard, {
      props: { movie: { id: 1, normalized_id: 'ABC-123', compressed_id: 'ABC-123', is_hidden: false, created_at: '', updated_at: '', preferred_item: { id: 2, source: 'fanza', external_id: 'abc123', normalized_id: 'ABC-123', priority: 0, title: 'Example', cover_image_url: '', thumbnail_image_url: '', source_url: '', description: '', metadata: {}, created_at: '', updated_at: '' } } },
      global: { plugins: [i18n], stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    expect(wrapper.text()).toContain('ABC-123')
    expect(wrapper.text()).toContain('Example')
  })

  it('prefers the cover and falls back to the thumbnail when it fails', async () => {
    const wrapper = mount(MovieCard, {
      props: { movie: { id: 1, normalized_id: 'ABC-123', compressed_id: 'ABC-123', is_hidden: false, created_at: '', updated_at: '', preferred_item: { id: 2, source: 'fanza', external_id: 'abc123', normalized_id: 'ABC-123', priority: 0, title: 'Example', cover_image_url: 'https://example.com/large.jpg', thumbnail_image_url: 'https://example.com/small.jpg', source_url: '', description: '', metadata: {}, created_at: '', updated_at: '' } } },
      global: { plugins: [i18n], stubs: { RouterLink: { template: '<a><slot /></a>' } } },
    })
    expect(wrapper.get('img').attributes('src')).toBe('https://example.com/large.jpg')
    await wrapper.get('img').trigger('error')
    expect(wrapper.get('img').attributes('src')).toBe('https://example.com/small.jpg')
    await wrapper.get('img').trigger('error')
    expect(wrapper.find('img').exists()).toBe(false)
  })
})
