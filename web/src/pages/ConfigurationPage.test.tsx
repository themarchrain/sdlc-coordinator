import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import type { ConfigurationView } from '../api/types'
import { APP_VERSION } from '../app/version'
import { ConfigurationPage } from './ConfigurationPage'

afterEach(() => vi.restoreAllMocks())

const configuration: ConfigurationView = {
  config: {
    agent_runner: { service_url: 'http://agent-runner:8081', workspace_root: '/app/workspaces', provider: 'codex-cli', provider_base_url: 'https://gateway.example', model: 'deepseek-v4-flash-0731', effort: 'high', task_timeout: '30m' },
    codehost: { provider: 'gitlab', base_url: 'https://gitlab.example.com', rework_allowed_user_ids: ['7'] },
    im: { provider: 'none', telegram: {}, feishu: {}, reviewer_mentions: ['审查人'] },
    scheduler: {
      enabled: true, interval: '30s',
      callback_base_url: 'http://127.0.0.1:8080', workspace_root: '/app/workspaces',
      dispatch_limit: 1, poll_limit: 5, reconciliation_limit: 5, finalization_limit: 5, recovery_limit: 5, effect_limit: 5,
    },
  },
  secrets: {
    agent_runner_api_key_configured: true,
    agent_runner_custom_headers_configured: true,
    telegram_bot_token_configured: false,
    feishu_secret_configured: false,
  },
}

beforeEach(() => {
  vi.spyOn(adminApi, 'listPeople').mockResolvedValue({
    items: [
      { person_id: 'person-owner', display_name: '负责人', email: 'owner@example.com', enabled: true, identities: [{ identity_id: 'identity-owner', provider: 'gitlab', base_url: 'https://gitlab.example.com', external_user_id: '7', username: 'owner' }], created_at: '', updated_at: '' },
      { person_id: 'person-reviewer', display_name: '审查人', email: 'reviewer@example.com', enabled: true, identities: [], created_at: '', updated_at: '' },
    ],
    page: 1, page_size: 100, total: 2,
  })
})

describe('ConfigurationPage', () => {
  it('shows masked secret state without rendering secret values', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)

    expect(await screen.findByText('运行配置')).toBeInTheDocument()
    expect(screen.queryByText('配置引导')).not.toBeInTheDocument()
    expect(screen.queryByText('状态推进')).not.toBeInTheDocument()
    expect(screen.queryByText('只读采集')).not.toBeInTheDocument()
    expect(screen.queryByText('生效摘要')).not.toBeInTheDocument()
    expect(screen.getByText('2 / 4 已配置')).toBeInTheDocument()
    expect(screen.getByText('系统版本')).toBeInTheDocument()
    expect(screen.getByText(APP_VERSION)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    expect(screen.getAllByText('已配置').length).toBeGreaterThan(0)
    expect(screen.queryByDisplayValue('session-secret')).not.toBeInTheDocument()
    expect(screen.queryByDisplayValue('codehost-secret')).not.toBeInTheDocument()
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
    expect(screen.queryByLabelText('转换器地址')).not.toBeInTheDocument()
    expect(screen.queryByText('归一化为 Chat Completions')).not.toBeInTheDocument()
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

  it('shows telegram settings after selecting the telegram provider', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)

    await screen.findByText('运行配置')
    for (let index = 0; index < 3; index += 1) fireEvent.click(screen.getByRole('button', { name: '下一步' }))

    expect(screen.getByText('IM 通知渠道')).toBeInTheDocument()

    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'IM 通知渠道' }))
    fireEvent.click(await screen.findByText('Telegram'))

    expect(await screen.findByText('Telegram Bot Token')).toBeInTheDocument()
    expect(screen.getByText('默认 Chat ID')).toBeInTheDocument()
  })

  it('accepts legacy configuration responses with null collection fields', async () => {
    const legacyView = {
      ...configuration,
      config: {
        ...configuration.config,
        codehost: {
          ...configuration.config.codehost,
          rework_allowed_user_ids: null,
        },
        im: { ...configuration.config.im, reviewer_mentions: null },
      },
    } as unknown as ConfigurationView
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 1, source: 'env_import' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(legacyView)

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)

    expect(await screen.findByText('运行配置')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    expect(screen.getByText('额外允许接续开发的人员')).toBeInTheDocument()
  })

  it('resolves existing runtime IDs and emails into person selections', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)
    await screen.findByText('运行配置')
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))

    expect(screen.getAllByText('负责人 / owner / owner@example.com').length).toBeGreaterThan(0)
    expect(adminApi.listPeople).toHaveBeenCalledWith({ page: 1, page_size: 100 })
  })

  it('loads every person page for configuration selectors', async () => {
    const personPage = Array.from({ length: 100 }, (_, index) => ({
      person_id: `person-${index}`, display_name: `成员${index}`, email: `user${index}@example.com`, enabled: true,
      identities: [], created_at: '', updated_at: '',
    }))
    vi.mocked(adminApi.listPeople).mockReset()
      .mockResolvedValueOnce({ items: personPage, page: 1, page_size: 100, total: 101 })
      .mockResolvedValueOnce({ items: [{ person_id: 'person-100', display_name: '成员100', email: 'user100@example.com', enabled: true, identities: [], created_at: '', updated_at: '' }], page: 2, page_size: 100, total: 101 })
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)
    await screen.findByText('运行配置')

    expect(adminApi.listPeople).toHaveBeenNthCalledWith(1, { page: 1, page_size: 100 })
    expect(adminApi.listPeople).toHaveBeenNthCalledWith(2, { page: 2, page_size: 100 })
  })

  it('does not show the platform API token anywhere', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)
    await screen.findByText('运行配置')
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))

    // 平台 API Token 不是全局配置项,每仓 Token 在仓库管理中维护。
    expect(screen.queryByText('平台 API Token')).not.toBeInTheDocument()
    expect(screen.getByLabelText('平台地址')).toBeInTheDocument()
    // webhook 验签凭据归属注册仓,配置页没有该输入。
    expect(screen.queryByText('Webhook Secret')).not.toBeInTheDocument()
  })

  it('does not present GitLab token or clone push name as daily inputs', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 2, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue({
      ...configuration,
      config: { ...configuration.config },
    })

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)
    await screen.findByText('运行配置')
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    expect(screen.queryByLabelText('Clone / Push Token 变量名')).not.toBeInTheDocument()
    expect(screen.queryByText('Clone / Push Token 变量名')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    expect(screen.queryByText('平台 API Token')).not.toBeInTheDocument()
    // webhook 验签凭据归属注册仓,配置页没有该输入。
    expect(screen.queryByText('Webhook Secret')).not.toBeInTheDocument()
    expect(screen.getByLabelText('平台地址')).toBeInTheDocument()
  })

  it('validates, saves, and requests restart from the final step', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'setup_pending', revision: 0 })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)
    vi.spyOn(adminApi, 'validateConfiguration').mockResolvedValue({ valid: true, field_errors: {} })
    const update = vi.spyOn(adminApi, 'updateConfiguration').mockResolvedValue({
      mode: 'restart_required', revision: 1, source: 'ui', configuration,
    })
    const restart = vi.spyOn(adminApi, 'restartConfiguration').mockResolvedValue({ status: 'shutdown_scheduled' })

    render(<MemoryRouter initialEntries={['/setup']}><ConfigurationPage /></MemoryRouter>)
    await screen.findByText('首次启动')
    for (let index = 0; index < 5; index += 1) fireEvent.click(screen.getByRole('button', { name: '下一步' }))
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
        codehost: expect.objectContaining({ rework_allowed_user_ids: ['7'] }),
        im: expect.objectContaining({ reviewer_mentions: ['审查人'] }),
        scheduler: expect.objectContaining({ callback_base_url: 'http://127.0.0.1:8080', workspace_root: '/app/workspaces' }),
      }),
    }))
    expect(restart).toHaveBeenCalledTimes(1)
  })

  it('can save when clone push token name is empty because the field is not editable', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({
      mode: 'configuration_error',
      revision: 4,
      configuration_error: 'runtime configuration validation failed: runtime configuration has 4 validation error(s)',
    })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue({
      ...configuration,
      config: { ...configuration.config },
    })
    vi.spyOn(adminApi, 'validateConfiguration').mockResolvedValue({ valid: true, field_errors: {} })
    const update = vi.spyOn(adminApi, 'updateConfiguration').mockResolvedValue({
      mode: 'restart_required', revision: 5, source: 'ui', configuration,
    })
    vi.spyOn(adminApi, 'restartConfiguration').mockResolvedValue({ status: 'shutdown_scheduled' })

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)
    expect(await screen.findByText('已保存配置无法装配')).toBeInTheDocument()
    for (let index = 0; index < 5; index += 1) fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: /保存并启用/ }))

    await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
    expect(update.mock.calls[0][0].config.codehost.base_url).toBe(configuration.config.codehost.base_url)
  })
})

describe('ConfigurationPage codehost providers', () => {
  it('offers GitHub alongside GitLab and Gitea', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 1, source: 'env_import' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)

    await screen.findByText('运行配置')
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.mouseDown(screen.getByRole('combobox', { name: '代码托管平台' }))
    expect(await screen.findByText('GitHub')).toBeInTheDocument()
  })
})
