import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { StrictMode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import type { Connector } from '../api/types'
import { ConnectorsPage } from './ConnectorsPage'

afterEach(() => vi.restoreAllMocks())

function connector(overrides: Partial<Connector> = {}): Connector {
  return {
    connector_id: 'connector-1',
    kind: 'state_source_sync',
    display_name: '状态源同步器',
    base_url: 'http://sync:8989',
    enabled: true,
    target_project_path: 'group/repo',
    capabilities: ['configuration', 'manual_trigger', 'run_history', 'record_query', 'metrics'],
    last_health_status: 'healthy',
    last_health_message: '',
    created_at: '2026-08-08T08:00:00Z',
    updated_at: '2026-08-08T08:10:00Z',
    ...overrides,
  }
}

function mockList(items: Connector[]) {
  vi.spyOn(adminApi, 'listConnectors').mockResolvedValue({ items, page: 1, page_size: 20, total: items.length })
}

describe('ConnectorsPage', () => {
  it('shows 连接器实例 and not 状态源连接器', async () => {
    mockList([connector()])
    render(<ConnectorsPage />)
    expect(await screen.findByText('连接器实例')).toBeInTheDocument()
    expect(screen.queryByText('状态源连接器')).not.toBeInTheDocument()
  })

  it('has no team UUID input and no mixed records table', async () => {
    mockList([connector({ last_health_status: 'unavailable', last_health_message: '连接超时' })])
    const records = vi.spyOn(adminApi, 'connectorRecords').mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })
    const runs = vi.spyOn(adminApi, 'connectorRuns').mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })
    const metrics = vi.spyOn(adminApi, 'connectorMetrics').mockResolvedValue({ values: {} })

    render(<ConnectorsPage />)
    fireEvent.click(await screen.findByRole('button', { name: /^api 状态源同步器$/ }))

    expect((await screen.findAllByText('连接超时')).length).toBeGreaterThan(0)
    expect(screen.queryByText(/同步记录/)).not.toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: /运行历史|指标/ })).not.toBeInTheDocument()
    expect(records).not.toHaveBeenCalled()
    expect(runs).not.toHaveBeenCalled()
    expect(metrics).not.toHaveBeenCalled()
  })

  it('does not offer unscoped trigger or a business-configuration wizard', async () => {
    mockList([connector()])
    render(<ConnectorsPage />)
    await screen.findByText('状态源同步器')

    expect(screen.queryByRole('button', { name: /触发/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /setting 配置/ })).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Team UUID')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Project UUID')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('项目 ID')).not.toBeInTheDocument()
  })

  it('keeps probe, register, edit URL/name/kind, enable switch, and delete', async () => {
    mockList([connector({ last_health_status: 'unknown' })])
    vi.spyOn(adminApi, 'probeConnector').mockResolvedValue({ health: { status: 'healthy' } })

    render(<ConnectorsPage />)
    expect(await screen.findByRole('button', { name: /注册连接器/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /探测/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '编辑 状态源同步器' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '删除 状态源同步器' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /探测/ }))
    expect(await screen.findByText('健康检查完成')).toBeInTheDocument()
    expect(adminApi.probeConnector).toHaveBeenCalledWith('connector-1')

    fireEvent.click(screen.getByRole('button', { name: '编辑 状态源同步器' }))
    expect(await screen.findByLabelText('显示名称')).toHaveValue('状态源同步器')
    expect(screen.getByLabelText('连接器类型')).toHaveValue('state_source_sync')
    expect(screen.getByLabelText('服务地址')).toHaveValue('http://sync:8989')
    expect(screen.getByRole('switch', { name: '允许管理操作' })).toBeChecked()
    expect(screen.queryByLabelText('目标仓库')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Team UUID')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('GitLab Token')).not.toBeInTheDocument()
  })

  it('saves opaque instance settings through the JSON editor', async () => {
    mockList([connector({ last_health_status: 'degraded', last_health_message: 'configuration required' })])
    vi.spyOn(adminApi, 'connectorConfiguration').mockResolvedValue({
      mode: 'setup_pending', version: 0, restart_required: false,
      settings: { endpoint: 'https://sync.example.test', limit: 3 },
      secret_fields: ['api_key'],
    })
    const update = vi.spyOn(adminApi, 'updateConnectorConfiguration').mockResolvedValue({
      mode: 'ready', version: 1, restart_required: true, settings: { endpoint: 'https://sync.example.test', limit: 5 }, secret_fields: ['api_key'],
    })

    render(<StrictMode><ConnectorsPage /></StrictMode>)
    fireEvent.click(await screen.findByRole('button', { name: /实例配置/ }))

    const editor = await screen.findByLabelText('连接器设置 JSON')
    expect(editor).toHaveValue(JSON.stringify({ endpoint: 'https://sync.example.test', limit: 3 }, null, 2))

    fireEvent.change(editor, { target: { value: JSON.stringify({ endpoint: 'https://sync.example.test', limit: 5 }) } })
    fireEvent.click(screen.getByRole('button', { name: /保存/ }))
    await waitFor(() => expect(update).toHaveBeenCalledWith('connector-1', expect.objectContaining({
      settings: { endpoint: 'https://sync.example.test', limit: 5 },
      preserve_secrets: ['api_key'],
    })))
  }, 10_000)
})
