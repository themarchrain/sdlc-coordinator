import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { RepositorySwitcher } from './RepositorySwitcher'

afterEach(() => vi.restoreAllMocks())

const repositories = [
  {
    repository_id: 'repo_a',
    display_name: 'Repo A',
provider: 'gitlab',
base_url: 'https://gitlab.example.com',
    codehost_project_id: '1',
    path_with_namespace: 'group/repo-a',
    token_configured: true,
    webhook_secret_configured: true,
    default_branch: 'main',
    status: 'enabled',

    last_configuration_error: '',

  },
  {
    repository_id: 'repo_b',
    display_name: 'Repo B',
provider: 'gitlab',
base_url: 'https://gitlab.example.com',
    codehost_project_id: '2',
    path_with_namespace: 'group/repo-b',
    token_configured: true,
    webhook_secret_configured: true,
    default_branch: 'main',
    status: 'configuring',

    last_configuration_error: 'probe failed',

  },
  {
    repository_id: 'repo_c',
    display_name: 'Repo C',
provider: 'gitlab',
base_url: 'https://gitlab.example.com',
    codehost_project_id: '3',
    path_with_namespace: 'group/repo-c',
    token_configured: false,
    webhook_secret_configured: false,
    default_branch: 'main',
    status: 'disabled',

    last_configuration_error: '',

  },
]

function LocationProbe() {
  const location = useLocation()
  return <span data-testid="location">{location.pathname}{location.search}</span>
}

function renderSwitcher(path = '/tasks') {
  vi.spyOn(adminApi, 'listRepositories').mockResolvedValue({ items: repositories })
  return render(
    <MemoryRouter initialEntries={[path]}>
      <RepositorySwitcher />
      <LocationProbe />
    </MemoryRouter>,
  )
}

describe('RepositorySwitcher', () => {
  it('defaults to 全部仓库', async () => {
    renderSwitcher()

    expect(await screen.findByText('全部仓库')).toBeInTheDocument()
    expect(screen.getByText('仓库：')).toBeInTheDocument()
    expect(screen.getByTestId('location')).toHaveTextContent('/tasks')
    expect(screen.getByTestId('location')).not.toHaveTextContent('repository_id')
  })

  it('writes repository_id=repo_a into the location when Repo A is selected', async () => {
    renderSwitcher('/tasks?page=2')

    await screen.findByText('全部仓库')
    fireEvent.mouseDown(screen.getByRole('combobox', { name: '仓库' }))
    expect(await screen.findByText('Repo A')).toBeInTheDocument()
    expect(screen.getByText('enabled')).toBeInTheDocument()
    expect(screen.getByText('configuring')).toBeInTheDocument()
    expect(screen.getByText('disabled')).toBeInTheDocument()

    fireEvent.click(screen.getByText('Repo A'))
    expect(screen.getByTestId('location')).toHaveTextContent('repository_id=repo_a')
    expect(screen.getByTestId('location')).not.toHaveTextContent('page=2')
  })

  it('restores the selected repository from the URL and clearing it omits the param', async () => {
    renderSwitcher('/tasks?repository_id=repo_a&status=running')

    expect(await screen.findByText('Repo A')).toBeInTheDocument()
    expect(screen.getByTestId('location')).toHaveTextContent('repository_id=repo_a')

    fireEvent.mouseDown(screen.getByRole('combobox', { name: '仓库' }))
    fireEvent.click(await screen.findByText('全部仓库'))
    expect(screen.getByTestId('location')).toHaveTextContent('/tasks?status=running')
    expect(screen.getByTestId('location')).not.toHaveTextContent('repository_id')
  })
})
