import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import type { ConfigurationView } from '../api/types'
import { APP_VERSION } from '../app/version'
import { ThemeProvider } from '../theme/ThemeProvider'
import { THEME_LIST, THEME_STORAGE_KEY } from '../themes/registry'
import { ConfigurationPage } from './ConfigurationPage'

afterEach(() => vi.restoreAllMocks())

const configuration: ConfigurationView = {
  config: {
    agent_runner: { service_url: 'http://agent-runner:8081', workspace_root: '/app/workspaces', provider: 'codex-cli', provider_base_url: 'https://gateway.example', model: 'deepseek-v4-flash-0731', effort: 'high', task_timeout: '30m' },
    scheduler: {
      enabled: true, interval: '30s',
      callback_base_url: 'http://127.0.0.1:8080', workspace_root: '/app/workspaces',
      dispatch_limit: 1, poll_limit: 5, reconciliation_limit: 5, finalization_limit: 5, recovery_limit: 5, effect_limit: 5,
    },
  },
  secrets: {
    agent_runner_api_key_configured: true,
    agent_runner_custom_headers_configured: true,
  },
}

beforeEach(() => {
  vi.spyOn(adminApi, 'listPeople').mockResolvedValue({
    items: [],
    page: 1, page_size: 100, total: 0,
  })
})

describe('ConfigurationPage', () => {
  it('renders the four-step system wizard without org-scoped steps', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)

    expect(await screen.findByText('运行配置')).toBeInTheDocument()
    expect(screen.getByText('运行边界')).toBeInTheDocument()
    expect(screen.getAllByText('Agent Runner').length).toBeGreaterThan(0)
    expect(screen.getByText('调度')).toBeInTheDocument()
    expect(screen.getByText('保存启用')).toBeInTheDocument()
    // 组织段两步已移交组织管理员,系统向导不再出现。
    expect(screen.queryByText('代码仓库')).not.toBeInTheDocument()
    expect(screen.queryByText('通知')).not.toBeInTheDocument()
  })

  it('does not load people for the system wizard', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)
    await screen.findByText('运行配置')

    expect(adminApi.listPeople).not.toHaveBeenCalled()
  })

  it('shows masked secret state without rendering secret values', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)

    expect(await screen.findByText('运行配置')).toBeInTheDocument()
    expect(screen.getByText('2 / 2 已配置')).toBeInTheDocument()
    expect(screen.getByText('系统版本')).toBeInTheDocument()
    expect(screen.getByText(APP_VERSION)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    expect(screen.getAllByText('已配置').length).toBeGreaterThan(0)
    expect(screen.queryByDisplayValue('session-secret')).not.toBeInTheDocument()
  })

  it('configures the reviewer execution service with masked provider secrets', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)
    await screen.findByText('运行配置')
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))

    expect(screen.getByLabelText('Runner 服务地址')).toHaveValue('http://agent-runner:8081')
    expect(screen.getByLabelText('共享工作区')).toHaveValue('/app/workspaces')
    expect(screen.getByText('Codex CLI')).toBeInTheDocument()
    expect(screen.getByLabelText('执行模型')).toHaveValue('deepseek-v4-flash-0731')
    expect(screen.getByLabelText('单任务超时（分钟）')).toHaveValue('30')
    expect(screen.getByText('裸 Agent，不加载个人 Codex 配置、MCP、skills 或历史会话。协议要求：接入地址必须兼容 OpenAI Responses API。')).toBeInTheDocument()
    expect(screen.getAllByText('已配置').length).toBeGreaterThanOrEqual(2)
    expect(screen.queryByDisplayValue('runner-api-key')).not.toBeInTheDocument()
  })

  it('states the bare-agent guarantee and protocol requirement for the claude provider', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue({ ...configuration, config: { ...configuration.config, agent_runner: { ...configuration.config.agent_runner, provider: 'claude-cli' } } })

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)
    await screen.findByText('运行配置')
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))

    expect(screen.getByText('裸 Agent，不加载个人 Claude 配置、MCP、skills 或历史会话。协议要求：接入地址必须兼容 Anthropic Messages API。')).toBeInTheDocument()
  })

  it('validates, saves, and requests restart from the final step with a system-only payload', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'setup_pending', revision: 0 })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)
    vi.spyOn(adminApi, 'validateConfiguration').mockResolvedValue({ valid: true, field_errors: {} })
    const update = vi.spyOn(adminApi, 'updateConfiguration').mockResolvedValue({
      mode: 'restart_required', revision: 1, source: 'ui', configuration,
    })
    const restart = vi.spyOn(adminApi, 'restartConfiguration').mockResolvedValue({ status: 'shutdown_scheduled' })

    render(<MemoryRouter initialEntries={['/setup']}><ConfigurationPage /></MemoryRouter>)
    await screen.findByText('首次启动')
    for (let index = 0; index < 3; index += 1) fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: /保存并启用/ }))

    await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
    expect(update).toHaveBeenCalledWith(expect.objectContaining({
      config: expect.objectContaining({
        agent_runner: expect.objectContaining({
          service_url: 'http://agent-runner:8081',
          provider: 'codex-cli',
          model: 'deepseek-v4-flash-0731',
          task_timeout: '30m',
        }),
        scheduler: expect.objectContaining({ callback_base_url: 'http://127.0.0.1:8080', workspace_root: '/app/workspaces' }),
      }),
    }))
    const payload = update.mock.calls[0][0].config as unknown as Record<string, unknown>
    expect('codehost' in payload).toBe(false)
    expect('im' in payload).toBe(false)
    expect(restart).toHaveBeenCalledTimes(1)
  })

  it('can save from the configuration-error state through the four steps', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({
      mode: 'configuration_error',
      revision: 4,
      configuration_error: 'runtime configuration validation failed: runtime configuration has 4 validation error(s)',
    })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)
    vi.spyOn(adminApi, 'validateConfiguration').mockResolvedValue({ valid: true, field_errors: {} })
    const update = vi.spyOn(adminApi, 'updateConfiguration').mockResolvedValue({
      mode: 'restart_required', revision: 5, source: 'ui', configuration,
    })
    vi.spyOn(adminApi, 'restartConfiguration').mockResolvedValue({ status: 'shutdown_scheduled' })

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)
    expect(await screen.findByText('已保存配置无法装配')).toBeInTheDocument()
    for (let index = 0; index < 3; index += 1) fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: /保存并启用/ }))

    await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
  })
})

describe('ConfigurationPage appearance panel', { timeout: 20_000 }, () => {
  beforeEach(() => {
    localStorage.clear()
    delete document.documentElement.dataset.theme
  })

  it('renders all theme cards grouped by curated family with light/dark chips', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)

    const { container } = render(
      <ThemeProvider>
        <MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>
      </ThemeProvider>,
    )

    expect(await screen.findByText('运行配置')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /外观主题：默认/ })).toBeInTheDocument()
    expect(container.querySelectorAll('.theme-card')).toHaveLength(0)

    fireEvent.click(screen.getByRole('button', { name: /外观主题：默认/ }))
    expect(screen.getByText('本地浏览器偏好，不写入服务端配置。')).toBeInTheDocument()
    expect(screen.getByRole('group', { name: '明暗筛选' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^全部 \d+$/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^亮色 \d+$/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^暗色 \d+$/ })).toBeInTheDocument()
    expect(container.querySelectorAll('.theme-card')).toHaveLength(THEME_LIST.length)
    expect(container.querySelectorAll('.theme-family').length).toBeGreaterThan(1)
    expect(screen.getByRole('heading', { name: '暗夜' })).toBeInTheDocument()
    expect(container.querySelector('.theme-card--active')).toBeTruthy()
    expect(container.querySelector('.theme-card--active')?.textContent).toContain('默认')
    expect(container.querySelector('.theme-card--active')?.textContent).toContain('当前使用')
  })

  it('narrows the grid to dark themes when the dark chip is pressed', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)

    const { container } = render(
      <ThemeProvider>
        <MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>
      </ThemeProvider>,
    )

    await screen.findByText('运行配置')
    fireEvent.click(screen.getByRole('button', { name: /外观主题：默认/ }))
    expect(container.querySelectorAll('.theme-card')).toHaveLength(THEME_LIST.length)

    fireEvent.click(screen.getByRole('button', { name: /^暗色 \d+$/ }))

    const darkCount = THEME_LIST.filter((theme) => theme.isDark).length
    expect(container.querySelectorAll('.theme-card')).toHaveLength(darkCount)
    expect(container.querySelectorAll('.theme-card__mode--dark')).toHaveLength(darkCount)
    expect(screen.getByRole('heading', { name: '暗夜' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '极简白' })).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: /^全部 \d+$/ }))
    expect(container.querySelectorAll('.theme-card')).toHaveLength(THEME_LIST.length)
  })

  it('filters theme cards by the search keyword and restores the full grid on clear', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)

    const { container } = render(
      <ThemeProvider>
        <MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>
      </ThemeProvider>,
    )

    await screen.findByText('运行配置')
    fireEvent.click(screen.getByRole('button', { name: /外观主题：默认/ }))
    fireEvent.change(screen.getByRole('textbox', { name: '搜索主题' }), { target: { value: 'framer' } })
    expect(container.querySelectorAll('.theme-card')).toHaveLength(1)
    expect(container.querySelector('.theme-card')?.textContent).toContain('Framer')

    fireEvent.change(screen.getByRole('textbox', { name: '搜索主题' }), { target: { value: '不存在的主题' } })
    expect(container.querySelectorAll('.theme-card')).toHaveLength(0)
    expect(screen.getByText('没有匹配“不存在的主题”的主题。')).toBeInTheDocument()

    fireEvent.change(screen.getByRole('textbox', { name: '搜索主题' }), { target: { value: '' } })
    expect(container.querySelectorAll('.theme-card')).toHaveLength(THEME_LIST.length)
    expect(screen.queryByText(/没有匹配/)).toBeNull()
  })

  it('switches the live theme immediately when a card is clicked and stays expanded', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)

    const { container } = render(
      <ThemeProvider>
        <MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>
      </ThemeProvider>,
    )

    await screen.findByText('运行配置')
    expect(document.documentElement.dataset.theme).toBe('default')

    fireEvent.click(screen.getByRole('button', { name: /外观主题：默认/ }))
    fireEvent.click(screen.getByRole('button', { name: /Supabase/ }))

    expect(document.documentElement.dataset.theme).toBe('supabase')
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('supabase')
    expect(container.querySelector('.theme-card--active')?.textContent).toContain('Supabase')
    expect(container.querySelectorAll('.theme-card')).toHaveLength(THEME_LIST.length)
    expect(screen.getByRole('button', { name: /外观主题：Supabase/ })).toBeInTheDocument()
  })
})
