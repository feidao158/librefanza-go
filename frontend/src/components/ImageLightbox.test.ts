import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ImageLightbox from './ImageLightbox.vue'
import { i18n } from '../i18n'

describe('ImageLightbox', () => {
  const images = [
    'https://example.com/img1.jpg',
    'https://example.com/img2.jpg',
    'https://example.com/img3.jpg',
  ]

  it('does not render overlay when open is false', () => {
    mount(ImageLightbox, {
      props: {
        open: false,
        images,
      },
      global: {
        plugins: [i18n],
      },
      attachTo: document.body,
    })
    expect(document.body.querySelector('.fixed.inset-0')).toBeNull()
  })

  it('renders correctly and switches images on next/prev', async () => {
    const wrapper = mount(ImageLightbox, {
      props: {
        open: true,
        images,
        initialIndex: 0,
      },
      global: {
        plugins: [i18n],
      },
      attachTo: document.body,
    })

    const overlay = document.body.querySelector('.fixed.inset-0')
    expect(overlay).not.toBeNull()
    expect(document.body.textContent).toContain('1 / 3')

    // Find next button
    const nextBtn = document.body.querySelector('button[data-action="next"]') as HTMLButtonElement
    expect(nextBtn).not.toBeNull()
    nextBtn.click()
    await wrapper.vm.$nextTick()

    expect(document.body.textContent).toContain('2 / 3')
    expect(wrapper.emitted('update:index')?.[0]).toEqual([1])

    // Find prev button
    const prevBtn = document.body.querySelector('button[data-action="prev"]') as HTMLButtonElement
    expect(prevBtn).not.toBeNull()
    prevBtn.click()
    await wrapper.vm.$nextTick()

    expect(document.body.textContent).toContain('1 / 3')

    // Close button
    const closeBtn = document.body.querySelector('button[data-action="close"]') as HTMLButtonElement
    expect(closeBtn).not.toBeNull()
    closeBtn.click()
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('close')).toBeTruthy()

    wrapper.unmount()
  })

  it('handles keyboard navigation', async () => {
    const wrapper = mount(ImageLightbox, {
      props: {
        open: true,
        images,
        initialIndex: 0,
      },
      global: {
        plugins: [i18n],
      },
      attachTo: document.body,
    })

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight' }))
    await wrapper.vm.$nextTick()
    expect(document.body.textContent).toContain('2 / 3')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft' }))
    await wrapper.vm.$nextTick()
    expect(document.body.textContent).toContain('1 / 3')

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toBeTruthy()

    wrapper.unmount()
  })
})
