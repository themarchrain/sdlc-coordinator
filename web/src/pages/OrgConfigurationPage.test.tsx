import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import type { TenantConfigurationView } from '../api/types'
import { OrgConfigurationPage } from './OrgConfigurationPage'

afterEach(() => vi.restoreAllMocks())

const view: TenantConfigurationView = {
  config: {
    codehost: { provider: 'github', base_url: 'https://github.com', rework_allowed_user_ids: ['42'] },
    im: { provider: '', telegram: {}, feishu: {}, reviewer_mentions: [] },
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
})
