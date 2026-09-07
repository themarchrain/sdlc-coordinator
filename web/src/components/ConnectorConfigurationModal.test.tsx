import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import type { Connector } from '../api/types'
import { ConnectorConfigurationModal } from './ConnectorConfigurationModal'

afterEach(() => vi.restoreAllMocks())

const connector: Connector = {
  connector_id: 'connector-1',
  kind: 'state_source_sync',
  display_name: '状态源同步器',
  base_url: 'http://localhost:8989',
  enabled: true,
  target_project_path: '',
  capabilities: ['configuration'],
  last_health_status: 'healthy',
  last_health_message: '',
  created_at: '',
  updated_at: '',
}

describe('ConnectorConfigurationModal', () => {
  it('loads settings as JSON, saves opaque settings and preserves omitted secret fields', async () => {
    vi.spyOn(adminApi, 'connectorConfiguration').mockResolvedValue({
      mode: 'ready',
      version: 3,
      restart_required: false,
      settings: { api_key: '***', region: 'cn' },
      secret_fields: ['api_key'],
    })
    const update = vi.spyOn(adminApi, 'updateConnectorConfiguration').mockResolvedValue({
      mode: 'ready',
      version: 4,
      restart_required: true,
      settings: { region: 'cn' },
      secret_fields: ['api_key'],
    })

    render(<ConnectorConfigurationModal connector={connector} onClose={() => undefined} onSaved={() => undefined} />)

    const editor = await screen.findByLabelText('连接器设置 JSON')
    expect(editor).toHaveValue(JSON.stringify({ api_key: '***', region: 'cn' }, null, 2))

    fireEvent.change(editor, { target: { value: JSON.stringify({ region: 'cn', limit: 5 }) } })
    fireEvent.click(screen.getByRole('button', { name: /保存/ }))

    await waitFor(() => expect(update).toHaveBeenCalledWith('connector-1', expect.objectContaining({
      settings: { region: 'cn', limit: 5 },
      // api_key 未回传 → 进入 preserve_secrets,由连接器保留原值
      preserve_secrets: ['api_key'],
    })))
  })

  it('rejects invalid JSON without calling the API', async () => {
    vi.spyOn(adminApi, 'connectorConfiguration').mockResolvedValue({
      mode: 'ready',
      version: 1,
      restart_required: false,
      settings: {},
      secret_fields: [],
    })
    const update = vi.spyOn(adminApi, 'updateConnectorConfiguration')

    render(<ConnectorConfigurationModal connector={connector} onClose={() => undefined} onSaved={() => undefined} />)

    const editor = await screen.findByLabelText('连接器设置 JSON')
    fireEvent.change(editor, { target: { value: '{not-json' } })
    fireEvent.click(screen.getByRole('button', { name: /保存/ }))

    expect(await screen.findByText('JSON 解析失败')).toBeInTheDocument()
    expect(update).not.toHaveBeenCalled()
  })

  it('shows the load error but still saves what the operator typed', async () => {
    vi.spyOn(adminApi, 'connectorConfiguration').mockRejectedValue(new Error('Connector 请求失败'))
    const update = vi.spyOn(adminApi, 'updateConnectorConfiguration').mockResolvedValue({
      mode: 'setup_pending',
      version: 1,
      restart_required: true,
      settings: {},
      secret_fields: [],
    })

    render(<ConnectorConfigurationModal connector={connector} onClose={() => undefined} onSaved={() => undefined} />)

    expect(await screen.findByText('读取连接器配置失败')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('连接器设置 JSON'), { target: { value: '{"endpoint":"https://sync.example.test"}' } })
    fireEvent.click(screen.getByRole('button', { name: /保存/ }))

    await waitFor(() => expect(update).toHaveBeenCalledWith('connector-1', expect.objectContaining({
      settings: { endpoint: 'https://sync.example.test' },
    })))
  })
})
