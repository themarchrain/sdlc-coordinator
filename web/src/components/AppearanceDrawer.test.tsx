import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'

import { AppearanceDrawerButton } from './AppearanceDrawer'
import { ThemeProvider } from '../theme/ThemeProvider'
import { THEME_LIST, THEME_STORAGE_KEY } from '../themes/registry'

// 外观主题入口迁至全局外壳后的门面回归：这里承接原系统配置页上对面板本体的
// 交互覆盖（展开/明暗筛选/搜索/换肤）。抽屉走 antd portal 挂到 document.body，
// DOM 断言用 document 而不是 render 返回的 container。
describe('AppearanceDrawerButton', { timeout: 20_000 }, () => {
  beforeEach(() => {
    localStorage.clear()
    delete document.documentElement.dataset.theme
  })

  it('opens the drawer from the trigger button and renders the panel summary', () => {
    render(<ThemeProvider><AppearanceDrawerButton /></ThemeProvider>)

    const trigger = screen.getByRole('button', { name: '外观主题' })
    expect(document.querySelectorAll('.theme-card')).toHaveLength(0)
    fireEvent.click(trigger)

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('region', { name: '外观主题' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /外观主题：默认/ })).toBeInTheDocument()
  })

  it('renders all theme cards grouped by curated family with light/dark chips after expanding', () => {
    render(<ThemeProvider><AppearanceDrawerButton /></ThemeProvider>)
    fireEvent.click(screen.getByRole('button', { name: '外观主题' }))

    fireEvent.click(screen.getByRole('button', { name: /外观主题：默认/ }))
    expect(screen.getByText('本地浏览器偏好，不写入服务端配置。')).toBeInTheDocument()
    expect(screen.getByRole('group', { name: '明暗筛选' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^全部 \d+$/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^亮色 \d+$/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^暗色 \d+$/ })).toBeInTheDocument()
    expect(document.querySelectorAll('.theme-card')).toHaveLength(THEME_LIST.length)
    expect(document.querySelectorAll('.theme-family').length).toBeGreaterThan(1)
    expect(screen.getByRole('heading', { name: '暗夜' })).toBeInTheDocument()
    expect(document.querySelector('.theme-card--active')).toBeTruthy()
    expect(document.querySelector('.theme-card--active')?.textContent).toContain('默认')
    expect(document.querySelector('.theme-card--active')?.textContent).toContain('当前使用')
  })

  it('narrows the grid to dark themes when the dark chip is pressed', () => {
    render(<ThemeProvider><AppearanceDrawerButton /></ThemeProvider>)
    fireEvent.click(screen.getByRole('button', { name: '外观主题' }))

    fireEvent.click(screen.getByRole('button', { name: /外观主题：默认/ }))
    expect(document.querySelectorAll('.theme-card')).toHaveLength(THEME_LIST.length)

    fireEvent.click(screen.getByRole('button', { name: /^暗色 \d+$/ }))

    const darkCount = THEME_LIST.filter((theme) => theme.isDark).length
    expect(document.querySelectorAll('.theme-card')).toHaveLength(darkCount)
    expect(document.querySelectorAll('.theme-card__mode--dark')).toHaveLength(darkCount)
    expect(screen.getByRole('heading', { name: '暗夜' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '极简白' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: /^全部 \d+$/ }))
    expect(document.querySelectorAll('.theme-card')).toHaveLength(THEME_LIST.length)
  })

  it('filters theme cards by the search keyword and restores the full grid on clear', () => {
    render(<ThemeProvider><AppearanceDrawerButton /></ThemeProvider>)
    fireEvent.click(screen.getByRole('button', { name: '外观主题' }))

    fireEvent.click(screen.getByRole('button', { name: /外观主题：默认/ }))
    fireEvent.change(screen.getByRole('textbox', { name: '搜索主题' }), { target: { value: 'framer' } })
    expect(document.querySelectorAll('.theme-card')).toHaveLength(1)
    expect(document.querySelector('.theme-card')?.textContent).toContain('Framer')

    fireEvent.change(screen.getByRole('textbox', { name: '搜索主题' }), { target: { value: '不存在的主题' } })
    expect(document.querySelectorAll('.theme-card')).toHaveLength(0)
    expect(screen.getByText('没有匹配“不存在的主题”的主题。')).toBeInTheDocument()

    fireEvent.change(screen.getByRole('textbox', { name: '搜索主题' }), { target: { value: '' } })
    expect(document.querySelectorAll('.theme-card')).toHaveLength(THEME_LIST.length)
    expect(screen.queryByText(/没有匹配/)).toBeNull()
  })

  it('switches the live theme immediately when a card is clicked and stays expanded', () => {
    render(<ThemeProvider><AppearanceDrawerButton /></ThemeProvider>)
    fireEvent.click(screen.getByRole('button', { name: '外观主题' }))

    expect(document.documentElement.dataset.theme).toBe('default')

    fireEvent.click(screen.getByRole('button', { name: /外观主题：默认/ }))
    fireEvent.click(screen.getByRole('button', { name: /Supabase/ }))

    expect(document.documentElement.dataset.theme).toBe('supabase')
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('supabase')
    expect(document.querySelector('.theme-card--active')?.textContent).toContain('Supabase')
    expect(document.querySelectorAll('.theme-card')).toHaveLength(THEME_LIST.length)
    expect(screen.getByRole('button', { name: /外观主题：Supabase/ })).toBeInTheDocument()
  })
})
