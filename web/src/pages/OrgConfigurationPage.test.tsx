import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import type { TenantConfigurationView } from '../api/types'
import { OrgConfigurationPage } from './OrgConfigurationPage'

afterEach(() => {
  vi.restoreAllMocks()
  // antd message 渲染在 React 树外的 body 门户，不随组件卸载清理：残留的
  // 提示会让后续用例的按文本查询命中多个元素。
  document.body.querySelectorAll('.ant-message-notice').forEach(node => node.remove())
})

const view: TenantConfigurationView = {
  config: {
    codehost: { provider: 'github', base_url: 'https://github.com', rework_allowed_user_ids: ['42'] },
    im: { provider: '', telegram: {}, feishu: {}, reviewer_mentions: [] },
    runner: { provider: '' },
  },
  secrets: { telegram_bot_token_configured: false, feishu_secret_configured: false },
  revision: 3,
  configured: true,
}

beforeEach(() => {
  vi.spyOn(adminApi, 'listPeople').mockResolvedValue({
    items: [
      { person_id: 'person-owner', display_name: '负责人', email: 'owner@example.com', enabled: true, identities: [{ identity_id: 'identity-owner', provider: 'github', base_url: 'https://github.com', external_user_id: '42', username: 'owner' }], created_at: '', updated_at: '' },
      { person_id: 'person-reviewer', display_name: '审查人', email: 'reviewer@example.com', enabled: true, identities: [], created_at: '', updated_at: '' },
    ],
    page: 1, page_size: 100, total: 2,
  })
})

describe('OrgConfigurationPage', () => {
  it('renders the three org-scoped wizard steps', async () => {
    vi.spyOn(adminApi, 'tenantConfiguration').mockResolvedValue(view)

    render(<MemoryRouter initialEntries={['/org/settings']}><OrgConfigurationPage /></MemoryRouter>)

    expect(await screen.findByText('保存启用')).toBeInTheDocument()
    expect(screen.getAllByText('代码仓库').length).toBeGreaterThan(0)
    expect(screen.getAllByText('通知').length).toBeGreaterThan(0)
    expect(screen.getByText('保存启用')).toBeInTheDocument()
    expect(screen.queryByText('Agent Runner')).not.toBeInTheDocument()
    expect(screen.queryByText('调度')).not.toBeInTheDocument()
  })

  it('resolves the stored allowlist into a person selection', async () => {
    vi.spyOn(adminApi, 'tenantConfiguration').mockResolvedValue(view)

    render(<MemoryRouter initialEntries={['/org/settings']}><OrgConfigurationPage /></MemoryRouter>)
    await screen.findByText('保存启用')

    expect(screen.getByText('负责人 / owner / owner@example.com')).toBeInTheDocument()
    expect(adminApi.listPeople).toHaveBeenCalledWith({ page: 1, page_size: 100 })
  })

  it('shows telegram settings after selecting the telegram provider', async () => {
    vi.spyOn(adminApi, 'tenantConfiguration').mockResolvedValue(view)

    render(<MemoryRouter initialEntries={['/org/settings']}><OrgConfigurationPage /></MemoryRouter>)
    await screen.findByText('保存启用')
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))

    expect(screen.getByText('IM 通知渠道')).toBeInTheDocument()
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'IM 通知渠道' }))
    fireEvent.click(await screen.findByText('Telegram'))

    expect(await screen.findByText('Telegram Bot Token')).toBeInTheDocument()
    expect(screen.getByText('默认 Chat ID')).toBeInTheDocument()
  })

  it('validates, saves, and hot-applies the org configuration', async () => {
    vi.spyOn(adminApi, 'tenantConfiguration').mockResolvedValue(view)
    vi.spyOn(adminApi, 'validateTenantConfiguration').mockResolvedValue({ valid: true, field_errors: {} })
    const update = vi.spyOn(adminApi, 'updateTenantConfiguration').mockResolvedValue({ ...view, revision: 4 })

    render(<MemoryRouter initialEntries={['/org/settings']}><OrgConfigurationPage /></MemoryRouter>)
    await screen.findByText('保存启用')
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: /保存并生效/ }))

    await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
    expect(update).toHaveBeenCalledWith(expect.objectContaining({
      expected_revision: 3,
      config: expect.objectContaining({
        codehost: expect.objectContaining({ provider: 'github', base_url: 'https://github.com', rework_allowed_user_ids: ['42'] }),
        im: expect.objectContaining({ provider: '' }),
      }),
    }))
    expect(await screen.findByText('组织配置已保存并生效')).toBeInTheDocument()
  })

  it('preserves api-managed im mention maps when saving', async () => {
    const withMentions = {
      ...view,
      config: {
        ...view.config,
        im: {
          ...view.config.im,
          telegram: { ...view.config.im.telegram, mention_map: { alice: '@alice' } },
          feishu: { ...view.config.im.feishu, mention_map: { bob: '@bob' } },
        },
      },
    }
    vi.spyOn(adminApi, 'tenantConfiguration').mockResolvedValue(withMentions)
    vi.spyOn(adminApi, 'validateTenantConfiguration').mockResolvedValue({ valid: true, field_errors: {} })
    const update = vi.spyOn(adminApi, 'updateTenantConfiguration').mockResolvedValue(withMentions)

    render(<MemoryRouter initialEntries={['/org/settings']}><OrgConfigurationPage /></MemoryRouter>)
    await screen.findByText('保存启用')
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: /保存并生效/ }))

    await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
    expect(update).toHaveBeenCalledWith(expect.objectContaining({
      config: expect.objectContaining({
        im: expect.objectContaining({
          telegram: expect.objectContaining({ mention_map: { alice: '@alice' } }),
          feishu: expect.objectContaining({ mention_map: { bob: '@bob' } }),
        }),
      }),
    }))
  })

  it('surfaces the conflict message when a concurrent save wins', async () => {
    vi.spyOn(adminApi, 'tenantConfiguration').mockResolvedValue(view)
    vi.spyOn(adminApi, 'validateTenantConfiguration').mockResolvedValue({ valid: true, field_errors: {} })
    const update = vi.spyOn(adminApi, 'updateTenantConfiguration').mockRejectedValue(new Error('配置已被他人更新，请刷新后重试'))

    render(<MemoryRouter initialEntries={['/org/settings']}><OrgConfigurationPage /></MemoryRouter>)
    await screen.findByText('保存启用')
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: /保存并生效/ }))

    expect(await screen.findByText('配置已被他人更新，请刷新后重试')).toBeInTheDocument()
    expect(update).toHaveBeenCalledWith(expect.objectContaining({ expected_revision: 3 }))
  })

  it('renders an error state when the initial load fails', async () => {
    vi.spyOn(adminApi, 'tenantConfiguration').mockRejectedValue(new Error('读取组织配置失败'))

    render(<MemoryRouter initialEntries={['/org/settings']}><OrgConfigurationPage /></MemoryRouter>)

    expect(await screen.findByText('读取组织配置失败')).toBeInTheDocument()
  })

  it('rejects a save that sets both a clear flag and a replacement token', async () => {
    vi.spyOn(adminApi, 'tenantConfiguration').mockResolvedValue({ ...view, secrets: { telegram_bot_token_configured: true, feishu_secret_configured: false } })
    vi.spyOn(adminApi, 'validateTenantConfiguration').mockResolvedValue({ valid: true, field_errors: {} })
    const update = vi.spyOn(adminApi, 'updateTenantConfiguration').mockResolvedValue(view)

    render(<MemoryRouter initialEntries={['/org/settings']}><OrgConfigurationPage /></MemoryRouter>)
    await screen.findByText('保存启用')
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'IM 通知渠道' }))
    fireEvent.click(await screen.findByText('Telegram'))
    fireEvent.change(screen.getByPlaceholderText('留空保留当前值；输入新值进行替换'), { target: { value: 'new-token' } })
    fireEvent.click(screen.getByRole('checkbox'))
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: /保存并生效/ }))

    expect(await screen.findByText('「明确清除」与新值不能同时提交：请取消勾选或清空新值')).toBeInTheDocument()
    expect(update).not.toHaveBeenCalled()
  })

  it('clears the secret subform after a successful save', async () => {
    const configured: TenantConfigurationView = {
      ...view,
      config: { ...view.config, im: { ...view.config.im, provider: 'telegram', telegram: {} } },
      secrets: { telegram_bot_token_configured: true, feishu_secret_configured: false },
    }
    vi.spyOn(adminApi, 'tenantConfiguration').mockResolvedValue(configured)
    vi.spyOn(adminApi, 'validateTenantConfiguration').mockResolvedValue({ valid: true, field_errors: {} })
    vi.spyOn(adminApi, 'updateTenantConfiguration').mockResolvedValue(configured)

    render(<MemoryRouter initialEntries={['/org/settings']}><OrgConfigurationPage /></MemoryRouter>)
    await screen.findByText('保存启用')
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(await screen.findByRole('checkbox'))
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: /保存并生效/ }))

    expect(await screen.findByText('组织配置已保存并生效')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('checkbox', { hidden: true })).not.toBeChecked())
  })

  it('surfaces field errors when validation rejects the payload', async () => {
    vi.spyOn(adminApi, 'tenantConfiguration').mockResolvedValue(view)
    vi.spyOn(adminApi, 'validateTenantConfiguration').mockResolvedValue({ valid: false, field_errors: { 'codehost.base_url': '必须是完整的 http 或 https 地址' } })
    const update = vi.spyOn(adminApi, 'updateTenantConfiguration').mockResolvedValue(view)

    render(<MemoryRouter initialEntries={['/org/settings']}><OrgConfigurationPage /></MemoryRouter>)
    await screen.findByText('保存启用')
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: /保存并生效/ }))

    expect(await screen.findByText('以下字段未通过校验')).toBeInTheDocument()
    expect(screen.getByText(/必须是完整的 http 或 https 地址/)).toBeInTheDocument()
    expect(update).not.toHaveBeenCalled()
  })

  it('selects the runner CLI on the first step and saves it', async () => {
    vi.spyOn(adminApi, 'tenantConfiguration').mockResolvedValue(view)
    vi.spyOn(adminApi, 'validateTenantConfiguration').mockResolvedValue({ valid: true, field_errors: {} })
    const update = vi.spyOn(adminApi, 'updateTenantConfiguration').mockResolvedValue({ ...view, revision: 4 })

    render(<MemoryRouter initialEntries={['/org/settings']}><OrgConfigurationPage /></MemoryRouter>)
    await screen.findByText('保存启用')

    expect(screen.getAllByText('执行 CLI').length).toBeGreaterThan(0)
    fireEvent.mouseDown(screen.getByRole('combobox', { name: '执行 CLI' }))
    fireEvent.click(await screen.findByText('Codex CLI'))

    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: '下一步' }))
    fireEvent.click(screen.getByRole('button', { name: /保存并生效/ }))

    await waitFor(() => expect(update).toHaveBeenCalledTimes(1))
    expect(update).toHaveBeenCalledWith(expect.objectContaining({
      expected_revision: 3,
      config: expect.objectContaining({
        runner: { provider: 'codex-cli' },
      }),
    }))
    // antd message 门户跨测试残留,用 All 变体容忍先前用例的未过期提示。
    expect((await screen.findAllByText('组织配置已保存并生效')).length).toBeGreaterThan(0)
  })
})
