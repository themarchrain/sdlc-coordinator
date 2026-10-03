import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { RepositoriesPage } from './RepositoriesPage'

afterEach(() => vi.restoreAllMocks())

const repositories = [
  {
    repository_id: 'repo_a',
    display_name: 'Coordinator',
provider: 'gitlab',
base_url: 'https://gitlab.example.com',
    codehost_project_id: '24207',
    path_with_namespace: 'group/coordinator',
    token_configured: true,
    webhook_secret_configured: true,
    codehost_issue_labels: [],
    default_branch: 'main',
    status: 'enabled',
    last_configuration_error: '',
    verify_command: 'go test ./...',
  },
  {
    repository_id: 'repo_b',
    display_name: 'Legacy',
provider: 'gitlab',
base_url: 'https://gitlab.example.com',
    codehost_project_id: '9',
    path_with_namespace: 'group/legacy',
    token_configured: true,
    webhook_secret_configured: true,
    codehost_issue_labels: ['developer-agent'],
    default_branch: 'master',
    status: 'configuring',
    last_configuration_error: 'webhook secret is required',
  },
]

function renderPage(items = repositories) {
  vi.spyOn(adminApi, 'listRepositories').mockResolvedValue({ items })
  vi.spyOn(adminApi, 'tenantConfiguration').mockResolvedValue({
    config: {
      codehost: { provider: 'gitea', base_url: 'https://gitea.example.com', rework_allowed_user_ids: [] },
      im: { provider: '', telegram: {}, feishu: {}, reviewer_mentions: [] },
      runner: { provider: '', effort: '', task_timeout: '' },
    },
    secrets: { telegram_bot_token_configured: false, feishu_secret_configured: false },
    revision: 1,
    configured: true,
  })
  return render(<MemoryRouter><RepositoriesPage /></MemoryRouter>)
}

describe('RepositoriesPage', () => {
  it('places the latest configuration error after the action column', async () => {
    renderPage()

    const headers = await screen.findAllByRole('columnheader')
    const headerNames = headers.map((header) => header.textContent?.trim())
    expect(headerNames.indexOf('操作')).toBeGreaterThanOrEqual(0)
    expect(headerNames.indexOf('最近配置错误')).toBe(headerNames.indexOf('操作') + 1)
  })

  it('uses a switch to enable a disabled repository', async () => {
    renderPage([{ ...repositories[1], status: 'disabled' }])
    const enable = vi.spyOn(adminApi, 'enableRepository').mockResolvedValue(repositories[1])

    const row = await screen.findByText('Legacy').then((node) => node.closest('tr')!)
    const toggle = within(row).getByRole('switch')
    expect(toggle).not.toBeChecked()
    fireEvent.click(toggle)
    await waitFor(() => expect(enable).toHaveBeenCalledWith('repo_b'))
  })

  it('renders repository list fields and never shows a token', async () => {
    renderPage()

    expect(await screen.findByText('Coordinator')).toBeInTheDocument()
    expect(screen.getByText('group/coordinator')).toBeInTheDocument()
    const coordinatorRow = screen.getByText('Coordinator').closest('tr')!
    expect(within(coordinatorRow).getByText('enabled')).toBeInTheDocument()
    expect(screen.getByText('Legacy')).toBeInTheDocument()
    const legacyRow = screen.getByText('Legacy').closest('tr')!
    expect(within(legacyRow).getByText('configuring')).toBeInTheDocument()
    expect(within(legacyRow).getByText('webhook secret is required')).toBeInTheDocument()
    expect(screen.queryByText('repo-token-value')).not.toBeInTheDocument()
    expect(screen.queryByText(/glpat-/)).not.toBeInTheDocument()
    expect(screen.queryByText('super-secret-token')).not.toBeInTheDocument()
    expect(document.body.textContent).not.toMatch(/"token"\s*:/)
  })

  it('requires a project-scoped token attestation on the create form', async () => {
    renderPage([])

    fireEvent.click(await screen.findByRole('button', { name: /新增仓库/ }))

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByLabelText('显示名称')).toBeInTheDocument()
    expect(within(dialog).getByLabelText('平台')).toBeInTheDocument()
    expect(within(dialog).getByLabelText('平台地址')).toBeInTheDocument()
    expect(within(dialog).getByLabelText('仓库路径')).toBeInTheDocument()
    expect(within(dialog).getByLabelText('项目级 Token')).toBeInTheDocument()
    expect(within(dialog).getByRole('checkbox', { name: /项目级（repository 级）token/ })).toBeInTheDocument()
    expect(within(dialog).getByText(/操作者确认这是该平台项目级（repository 级）token，不是用户\/组\/实例 token/)).toBeInTheDocument()
    // 全局 codehost 配置降级为表单默认值:新表单预填配置中的平台与地址。
    await waitFor(() => expect(within(dialog).getByLabelText('平台地址')).toHaveValue('https://gitea.example.com'))
  })

  it('submits per-repository platform identity on create', async () => {
    renderPage([])
    const create = vi.spyOn(adminApi, 'createRepository').mockResolvedValue(repositories[0])

    fireEvent.click(await screen.findByRole('button', { name: /新增仓库/ }))
    const dialog = await screen.findByRole('dialog')
    await waitFor(() => expect(within(dialog).getByLabelText('平台地址')).toHaveValue('https://gitea.example.com'))
    fireEvent.change(within(dialog).getByLabelText('显示名称'), { target: { value: 'Gitea 仓' } })
    fireEvent.change(within(dialog).getByLabelText('仓库路径'), { target: { value: 'group/gitea-repo' } })
    fireEvent.change(within(dialog).getByLabelText('项目级 Token'), { target: { value: 'token-input' } })
    fireEvent.change(within(dialog).getByLabelText('Webhook Secret'), { target: { value: 'hook-secret' } })
    fireEvent.click(within(dialog).getByRole('checkbox', { name: /项目级（repository 级）token/ }))
    fireEvent.click(within(dialog).getByRole('button', { name: '保 存' }))

    await waitFor(() => expect(create).toHaveBeenCalledWith(expect.objectContaining({
      provider: 'gitea',
      base_url: 'https://gitea.example.com',
    })))
  })

  it('submits the verification command on create', async () => {
    renderPage([])
    const create = vi.spyOn(adminApi, 'createRepository').mockResolvedValue(repositories[0])

    fireEvent.click(await screen.findByRole('button', { name: /新增仓库/ }))
    const dialog = await screen.findByRole('dialog')
    await waitFor(() => expect(within(dialog).getByLabelText('平台地址')).toHaveValue('https://gitea.example.com'))
    fireEvent.change(within(dialog).getByLabelText('显示名称'), { target: { value: 'Gitea 仓' } })
    fireEvent.change(within(dialog).getByLabelText('仓库路径'), { target: { value: 'group/gitea-repo' } })
    fireEvent.change(within(dialog).getByLabelText('项目级 Token'), { target: { value: 'token-input' } })
    fireEvent.change(within(dialog).getByLabelText('Webhook Secret'), { target: { value: 'hook-secret' } })
    fireEvent.change(within(dialog).getByLabelText('验证命令'), { target: { value: '  go test ./...  ' } })
    fireEvent.click(within(dialog).getByRole('checkbox', { name: /项目级（repository 级）token/ }))
    fireEvent.click(within(dialog).getByRole('button', { name: '保 存' }))

    await waitFor(() => expect(create).toHaveBeenCalledWith(expect.objectContaining({
      verify_command: '  go test ./...  ',
    })))
  })

  it('prefills and updates the verification command on edit', async () => {
    renderPage()
    const patch = vi.spyOn(adminApi, 'patchRepository').mockResolvedValue(repositories[0])

    const row = await screen.findByText('Coordinator').then((node) => node.closest('tr')!)
    fireEvent.click(within(row).getByRole('button', { name: '编辑仓库' }))

    const dialog = await screen.findByRole('dialog')
    await waitFor(() => expect(within(dialog).getByLabelText('验证命令')).toHaveValue('go test ./...'))
    fireEvent.change(within(dialog).getByLabelText('验证命令'), { target: { value: 'make test' } })
    fireEvent.click(within(dialog).getByRole('button', { name: '保 存' }))
    await waitFor(() => expect(patch).toHaveBeenCalledWith('repo_a', expect.objectContaining({ verify_command: 'make test' })))
  })

  it('opens the edit dialog with repository basic fields', async () => {
    renderPage()
    const patch = vi.spyOn(adminApi, 'patchRepository').mockResolvedValue(repositories[0])

    const row = await screen.findByText('Coordinator').then((node) => node.closest('tr')!)
    fireEvent.click(within(row).getByRole('button', { name: '编辑仓库' }))

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByLabelText('显示名称')).toBeInTheDocument()
    expect(within(dialog).getByLabelText('仓库路径')).toBeInTheDocument()

    fireEvent.change(within(dialog).getByLabelText('显示名称'), { target: { value: 'Coordinator 2' } })
    fireEvent.click(within(dialog).getByRole('button', { name: '保 存' }))
    await waitFor(() => expect(patch).toHaveBeenCalledWith('repo_a', expect.objectContaining({ display_name: 'Coordinator 2' })))
  })
})

describe('RepositoriesPage git identity', () => {
  it('prefills, edits and submits git identity on a disabled repository', async () => {
    const repo = { ...repositories[1], status: 'disabled', git_author_name: 'Release Bot', git_author_email: 'bot@example.com' }
    renderPage([repo])
    const patch = vi.spyOn(adminApi, 'patchRepository').mockResolvedValue(repo)

    const row = await screen.findByText('Legacy').then((node) => node.closest('tr')!)
    fireEvent.click(within(row).getByRole('button', { name: '编辑仓库' }))

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByLabelText('提交作者姓名')).toHaveValue('Release Bot')
    expect(within(dialog).getByLabelText('提交作者邮箱')).toHaveValue('bot@example.com')
    expect(within(dialog).getByText(/sdlc-coordinator-dev/)).toBeInTheDocument()
    fireEvent.change(within(dialog).getByLabelText('提交作者姓名'), { target: { value: 'Renamed Bot' } })
    fireEvent.change(within(dialog).getByLabelText('提交作者邮箱'), { target: { value: 'renamed@example.com' } })
    fireEvent.click(within(dialog).getByRole('button', { name: '保 存' }))

    await waitFor(() => expect(patch).toHaveBeenCalledWith('repo_b', expect.objectContaining({
      git_author_name: 'Renamed Bot',
      git_author_email: 'renamed@example.com',
    })))
  })

  it('offers disable for a configuring repository so identity becomes editable', async () => {
    renderPage()
    const disable = vi.spyOn(adminApi, 'disableRepository').mockResolvedValue(repositories[1])

    const row = await screen.findByText('Legacy').then((node) => node.closest('tr')!)
    fireEvent.click(within(row).getByRole('button', { name: /停\s*用/ }))
    const confirmRoot = await screen.findByText('停用该仓库？').then((node) => node.closest('.ant-modal-confirm') as HTMLElement)
    fireEvent.click(within(confirmRoot).getByRole('button', { name: /停\s*用/ }))

    await waitFor(() => expect(disable).toHaveBeenCalledWith('repo_b'))
  })

  it('keeps git identity read-only while the repository is enabled', async () => {
    renderPage()

    const row = await screen.findByText('Coordinator').then((node) => node.closest('tr')!)
    fireEvent.click(within(row).getByRole('button', { name: '编辑仓库' }))

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByLabelText('提交作者姓名')).toBeDisabled()
    expect(within(dialog).getByLabelText('提交作者邮箱')).toBeDisabled()
  })
})
