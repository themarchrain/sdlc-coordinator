import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import type { RegisteredRepository, Requirement } from '../api/types'
import { WorkspaceSessionProvider } from '../app/workspaceSession'
import { RequirementsPage } from './RequirementsPage'

vi.mock('../api/client', () => ({
  adminApi: {
    listRequirements: vi.fn(),
    listRepositories: vi.fn(),
    createRequirement: vi.fn(),
    patchRequirement: vi.fn(),
    publishRequirement: vi.fn(),
    cancelRequirement: vi.fn(),
    completeRequirement: vi.fn(),
    triggerRequirementSync: vi.fn(),
  },
}))

const mockedApi = vi.mocked(adminApi)

const requirement: Requirement = {
  requirement_id: 'req_1',
  title: '支付回调幂等',
  description: '防重',
  status: 'queued',
  repository_id: 'repo-1',
  labels: ['backend'],
  priority: 'high',
  issue_iid: 0,
  issue_url: '',
  last_error: '',
  published_at: null,
  completed_at: null,
  created_at: '2026-08-28T10:00:00Z',
  updated_at: '2026-08-28T10:00:00Z',
}

const repository: RegisteredRepository = {
  repository_id: 'repo-1',
  display_name: 'Demo',
provider: 'gitlab',
base_url: 'https://gitlab.example.com',
  codehost_project_id: '',
  path_with_namespace: 'group/demo',
  token_configured: true,
  webhook_secret_configured: true,
  default_branch: 'master',
  status: 'enabled',

  last_configuration_error: '',

}

beforeEach(() => {
  vi.clearAllMocks()
  mockedApi.listRequirements.mockResolvedValue({ items: [requirement], page: 1, page_size: 20, total: 1 })
  mockedApi.listRepositories.mockResolvedValue({ items: [repository] })
})

function renderPage() {
  return render(<MemoryRouter><RequirementsPage /></MemoryRouter>)
}

describe('RequirementsPage', () => {
  it('renders requirements with status tag', async () => {
    renderPage()
    expect(await screen.findByText('支付回调幂等')).toBeInTheDocument()
    expect(screen.getByText('待同步')).toBeInTheDocument()
    expect(screen.getByText('高')).toBeInTheDocument()
  })

  it('creates a requirement through the modal form', async () => {
    mockedApi.createRequirement.mockResolvedValue(requirement)
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: /新建需求/ }))
    fireEvent.change(await screen.findByPlaceholderText('需求标题'), { target: { value: '新需求' } })
    // 第一个下拉是"目标仓库"(其后为优先级、标签)
    fireEvent.mouseDown(screen.getAllByRole('combobox')[0])
    const option = await screen.findByText('Demo', { selector: '.ant-select-item-option-content' })
    fireEvent.click(option.closest('.ant-select-item')!)
    fireEvent.click(screen.getByRole('button', { name: /保\s*存/ }))
    await waitFor(() => expect(mockedApi.createRequirement).toHaveBeenCalled())
  })

  it('triggers manual sync', async () => {
    mockedApi.triggerRequirementSync.mockResolvedValue({
      result: { created: 1, recovered: 0, failed: 0, skipped: 0, mirrored: 0, mirror_errors: 0 },
    })
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: /立即同步/ }))
    await waitFor(() => expect(mockedApi.triggerRequirementSync).toHaveBeenCalled())
  })

  it('hides write actions for organization members', async () => {
    render(
      <MemoryRouter>
        <WorkspaceSessionProvider value={{ session: null, role: 'member', superAdmin: false }}>
          <RequirementsPage />
        </WorkspaceSessionProvider>
      </MemoryRouter>,
    )
    expect(await screen.findByText('支付回调幂等')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /新建需求/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /立即同步/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('columnheader', { name: '操作' })).not.toBeInTheDocument()
  })
})
