import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../api/client'
import { PeoplePage } from './PeoplePage'

afterEach(() => vi.restoreAllMocks())

describe('PeoplePage', () => {
  it('lists people and opens the person editor', async () => {
    vi.spyOn(adminApi, 'listPeople').mockResolvedValue({
      items: [{
        person_id: 'person-1', display_name: 'Alice', email: 'user@example.com', enabled: true,
        identities: [{ identity_id: 'identity-1', provider: 'gitlab', base_url: 'https://gitlab.example.com', external_user_id: '7101', username: 'alice' }],
        created_at: '2026-08-11T01:00:00Z', updated_at: '2026-08-11T01:00:00Z',
      }],
      page: 1, page_size: 20, total: 1,
    })

    render(<MemoryRouter><PeoplePage /></MemoryRouter>)

    expect(await screen.findByText('Alice')).toBeInTheDocument()
    expect(screen.getByText('alice / ID 7101')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /新增人员/ }))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByLabelText('姓名')).toBeInTheDocument()
    expect(screen.getByLabelText('邮箱')).toBeInTheDocument()
  })

  it('does not offer GitLab lookup for a Gitea identity row', async () => {
    vi.spyOn(adminApi, 'listPeople').mockResolvedValue({ items: [], page: 1, page_size: 100, total: 0 })
    render(<MemoryRouter><PeoplePage /></MemoryRouter>)
    fireEvent.click(await screen.findByRole('button', { name: /新增人员/ }))

    fireEvent.mouseDown(screen.getByLabelText('平台'))
    fireEvent.click(await screen.findByText('Gitea'))

    expect(screen.getByRole('button', { name: '从 GitLab 查询' })).toBeDisabled()
  })

  it('queries the selected server-side page', async () => {
    const list = vi.spyOn(adminApi, 'listPeople').mockResolvedValue({
      items: [{ person_id: 'person-1', display_name: '成员1', email: 'user1@example.com', enabled: true, identities: [], created_at: '', updated_at: '' }],
      page: 1, page_size: 20, total: 21,
    })
    render(<MemoryRouter><PeoplePage /></MemoryRouter>)
    await screen.findByText('成员1')
    fireEvent.click(screen.getByTitle('2'))
    expect(list).toHaveBeenLastCalledWith({ q: '', page: 2, page_size: 20 })
  })

  it('saves a person status change from the table switch', async () => {
    const person = {
      person_id: 'person-1', display_name: '成员1', email: 'user1@example.com', enabled: true,
      identities: [{ identity_id: 'identity-1', provider: 'gitlab' as const, base_url: 'https://gitlab.example.com', external_user_id: '7', username: 'user1' }],
      created_at: '', updated_at: '',
    }
    vi.spyOn(adminApi, 'listPeople').mockResolvedValue({ items: [person], page: 1, page_size: 20, total: 1 })
    const update = vi.spyOn(adminApi, 'updatePerson').mockResolvedValue(person)

    render(<MemoryRouter><PeoplePage /></MemoryRouter>)
    const statusSwitch = await screen.findByRole('switch', { name: '停用成员1' })
    fireEvent.click(statusSwitch)

    expect(update).toHaveBeenCalledWith('person-1', {
      display_name: '成员1', email: 'user1@example.com', enabled: false,
      identities: [{ identity_id: 'identity-1', provider: 'gitlab', base_url: 'https://gitlab.example.com', external_user_id: '7', username: 'user1' }],
    })
  })

  it('clears a prior candidate while a new GitLab lookup is in flight', async () => {
    vi.spyOn(adminApi, 'listPeople').mockResolvedValue({ items: [], page: 1, page_size: 20, total: 0 })
    let resolveSecond: ((value: never) => void) | undefined
    vi.spyOn(adminApi, 'lookupCodeHostIdentity')
      .mockResolvedValueOnce({ candidates: [{ external_user_id: '7', username: 'user', display_name: '用户' }], exact_match_index: 0 })
      .mockImplementationOnce(() => new Promise((resolve) => { resolveSecond = resolve }))
    render(<MemoryRouter><PeoplePage /></MemoryRouter>)
    fireEvent.click(await screen.findByRole('button', { name: /新增人员/ }))
    fireEvent.change(screen.getByLabelText('邮箱'), { target: { value: 'user@example.com' } })
    fireEvent.change(screen.getByLabelText('平台地址'), { target: { value: 'https://gitlab.example.com' } })
    fireEvent.click(screen.getByRole('button', { name: '从 GitLab 查询' }))
    fireEvent.change(screen.getByLabelText('临时 GitLab Token'), { target: { value: 'temporary-token' } })
    fireEvent.click(screen.getByRole('button', { name: '查询身份' }))
    expect(await screen.findByRole('button', { name: '确认回填' })).toBeEnabled()

    fireEvent.click(screen.getByRole('button', { name: '查询身份' }))
    expect(screen.getByRole('button', { name: '确认回填' })).toBeDisabled()
    resolveSecond?.({ candidates: [], exact_match_index: null } as never)
  })
})
