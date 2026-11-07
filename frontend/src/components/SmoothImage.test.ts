import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import SmoothImage from './SmoothImage.vue'

describe('SmoothImage', () => {
  it('renders placeholder when loading and shows image on load', async () => {
    const wrapper = mount(SmoothImage, {
      props: {
        src: 'https://example.com/test.jpg',
        alt: 'Test Alt',
        fallbackText: '暂无封面',
      },
    })

    const img = wrapper.find('img')
    expect(img.exists()).toBe(true)
    expect(img.attributes('src')).toBe('https://example.com/test.jpg')
    expect(img.classes()).toContain('opacity-0')

    await img.trigger('load')
    expect(img.classes()).toContain('opacity-100')
    expect(wrapper.emitted('load')).toHaveLength(1)
  })

  it('renders fallback when src is missing or image errors', async () => {
    const wrapper = mount(SmoothImage, {
      props: {
        src: '',
        fallbackText: '暂无封面',
      },
    })

    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toContain('暂无封面')

    await wrapper.setProps({ src: 'https://example.com/broken.jpg' })
    const img = wrapper.find('img')
    expect(img.exists()).toBe(true)

    await img.trigger('error')
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toContain('暂无封面')
    expect(wrapper.emitted('error')).toHaveLength(1)
  })
})
