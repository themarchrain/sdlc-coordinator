import '@testing-library/jest-dom/vitest'
import '../styles/index.css'
import { act, cleanup } from '@testing-library/react'
import { message, notification } from 'antd'
import { afterEach } from 'vitest'

afterEach(async () => {
  await act(async () => {
    message.destroy()
    notification.destroy()
  })
  cleanup()
})

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => undefined,
    removeListener: () => undefined,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    dispatchEvent: () => false,
  }),
})

class ResizeObserverMock {
  observe() {}
  unobserve() {}
  disconnect() {}
}

globalThis.ResizeObserver = ResizeObserverMock

const nativeGetComputedStyle = window.getComputedStyle.bind(window)
window.getComputedStyle = (element: Element) => nativeGetComputedStyle(element)
