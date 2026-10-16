import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { AdminApiError, adminApi } from '../api/client'
import type { ConfigurationView } from '../api/types'
import { ConfigurationBoundary } from '../app/ConfigurationBoundary'
import { WorkspaceSessionProvider } from '../app/workspaceSession'
import { APP_VERSION } from '../app/version'
import { ConfigurationPage } from './ConfigurationPage'

afterEach(() => vi.restoreAllMocks())

function RouteProbe({ label }: { label: string }) {
  const location = useLocation()
  const navigate = useNavigate()
  return (
    <div>
      <span data-testid={`location-${label}`}>{location.pathname}</span>
      {label === 'home' ? <button onClick={() => navigate('/tasks')}>go-tasks</button> : null}
    </div>
  )
}

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
    // 执行 Provider 已撤出向导：不再提供选择 UI，只按加载值展示协议说明。
    expect(screen.queryByLabelText('执行 Provider')).not.toBeInTheDocument()
    expect(screen.queryByText('Codex CLI')).not.toBeInTheDocument()
    expect(screen.getByLabelText('执行模型')).toHaveValue('deepseek-v4-flash-0731')
    // 推理强度与单任务超时已下沉组织配置：系统向导不再渲染编辑 UI。
    expect(screen.queryByLabelText('推理强度')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('单任务超时（分钟）')).not.toBeInTheDocument()
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
    // provider 与 effort 已无 UI 编辑，但保存 payload 必须原样回传加载值：
    // 后端整行替换语义下缺键会把系统兜底值静默清空。
    expect(update).toHaveBeenCalledWith(expect.objectContaining({
      expected_revision: 0,
      config: expect.objectContaining({
        agent_runner: expect.objectContaining({
          service_url: 'http://agent-runner:8081',
          provider: 'codex-cli',
          model: 'deepseek-v4-flash-0731',
          effort: 'high',
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

  it('explains that a stale runtime configuration must be reloaded', async () => {
    vi.spyOn(adminApi, 'configurationStatus').mockResolvedValue({ mode: 'ready', revision: 3, source: 'ui' })
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)
    vi.spyOn(adminApi, 'validateConfiguration').mockResolvedValue({ valid: true, field_errors: {} })
    vi.spyOn(adminApi, 'updateConfiguration').mockRejectedValue(new AdminApiError(409, 'revision_conflict', '运行配置已被其他管理员更新，请刷新页面后重新提交'))

    render(<MemoryRouter initialEntries={['/settings']}><ConfigurationPage /></MemoryRouter>)
    await screen.findByText('运行配置')
    for (let index = 0; index < 3; index += 1) fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: /保存并启用/ }))

    expect(await screen.findByText('运行配置已被其他管理员更新，请刷新页面后重新提交')).toBeInTheDocument()
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

  it('lands on the workspace after restart and navigates admin routes without bouncing back to setup', async () => {
    // 02-admin-setup-stale-configuration-boundary 全链回归：
    // 初始 setup_pending → 保存重启 → 轮询 ready → 访问各管理路由不再弹回 /setup。
    const status = vi.spyOn(adminApi, 'configurationStatus')
    status.mockResolvedValueOnce({ mode: 'setup_pending', revision: 0 }) // 边界挂载
      .mockResolvedValueOnce({ mode: 'setup_pending', revision: 0 }) // 页面初始加载
      .mockResolvedValue({ mode: 'ready', revision: 1, source: 'ui' }) // 重启轮询
    vi.spyOn(adminApi, 'configuration').mockResolvedValue(configuration)
    vi.spyOn(adminApi, 'validateConfiguration').mockResolvedValue({ valid: true, field_errors: {} })
    vi.spyOn(adminApi, 'updateConfiguration').mockResolvedValue({
      mode: 'restart_required', revision: 1, source: 'ui', configuration,
    })
    vi.spyOn(adminApi, 'restartConfiguration').mockResolvedValue({ status: 'shutdown_scheduled' })

    render(
      <MemoryRouter initialEntries={['/setup']}>
        <WorkspaceSessionProvider value={{ session: null, role: 'super_admin', superAdmin: true }}>
          <ConfigurationBoundary>
            <Routes>
              <Route path="/" element={<RouteProbe label="home" />} />
              <Route path="/setup" element={<ConfigurationPage />} />
              <Route path="/tasks" element={<RouteProbe label="tasks" />} />
            </Routes>
          </ConfigurationBoundary>
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )

    await screen.findByText('首次启动')
    for (let index = 0; index < 3; index += 1) fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: /保存并启用/ }))

    // 轮询首个 tick 在 1.2s 后；读到 ready 即导航首页。
    await waitFor(() => expect(screen.getByTestId('location-home')).toHaveTextContent('/'), { timeout: 4000 })

    // 不刷新浏览器直接访问其他管理路由，边界按同步后的 ready 放行。
    fireEvent.click(screen.getByRole('button', { name: 'go-tasks' }))
    await waitFor(() => expect(screen.getByTestId('location-tasks')).toHaveTextContent('/tasks'))
  }, 10000)
})
