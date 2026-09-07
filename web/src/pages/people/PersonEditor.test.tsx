import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../../api/client'
import type { Person } from '../../api/types'
import { PersonEditor } from './PersonEditor'

afterEach(() => vi.restoreAllMocks())

const existingPerson: Person = {
  person_id: 'person-1', display_name: 'Alice', email: 'alice@example.com', enabled: true,
  identities: [{ identity_id: 'identity-1', provider: 'gitlab', base_url: 'https://gitlab.example.com', external_user_id: '7101', username: 'alice' }],
  created_at: '', updated_at: '',
}

describe('PersonEditor', () => {
  it('shows every existing value when editing a person', async () => {
    render(<PersonEditor person={existingPerson} onClose={vi.fn()} onSaved={vi.fn()} />)

    expect(await screen.findByLabelText('姓名')).toHaveValue('Alice')
    expect(screen.getByLabelText('邮箱')).toHaveValue('alice@example.com')
    expect(screen.getByLabelText('平台地址')).toHaveValue('https://gitlab.example.com')
    expect(screen.getByLabelText('用户名')).toHaveValue('alice')
    expect(screen.getByLabelText('用户 ID')).toHaveValue('7101')
    expect(screen.getByRole('switch', { name: '允许在新配置中选择' })).toBeChecked()
  })

  it('keeps a new person disabled by default', async () => {
    render(<PersonEditor person="new" onClose={vi.fn()} onSaved={vi.fn()} />)

    expect(await screen.findByRole('switch', { name: '允许在新配置中选择' })).not.toBeChecked()
  })

  it('queries GitLab from the person email without requiring a username', async () => {
    const lookup = vi.spyOn(adminApi, 'lookupCodeHostIdentity').mockResolvedValue({
      candidates: [{ external_user_id: '7101', username: 'alice', display_name: 'Alice' }], exact_match_index: 0,
    })
    render(<PersonEditor person="new" onClose={vi.fn()} onSaved={vi.fn()} />)
    fireEvent.change(await screen.findByLabelText('邮箱'), { target: { value: 'alice@example.com' } })
    fireEvent.change(screen.getByLabelText('平台地址'), { target: { value: 'https://gitlab.example.com' } })
    fireEvent.click(screen.getByRole('button', { name: '从 GitLab 查询' }))
    fireEvent.change(await screen.findByLabelText('临时 GitLab Token'), { target: { value: 'temporary-token' } })
    fireEvent.click(screen.getByRole('button', { name: '查询身份' }))

    await waitFor(() => expect(lookup).toHaveBeenCalledWith({
      provider: 'gitlab', base_url: 'https://gitlab.example.com', token: 'temporary-token', email: 'alice@example.com',
    }))
    expect(screen.queryByLabelText(/username/)).not.toBeInTheDocument()
  })

  it('blocks an incomplete enabled person but saves the same data as a disabled draft', async () => {
    const create = vi.spyOn(adminApi, 'createPerson').mockResolvedValue(existingPerson)
    const onSaved = vi.fn().mockResolvedValue(undefined)
    render(<PersonEditor person="new" onClose={vi.fn()} onSaved={onSaved} />)
    fireEvent.change(await screen.findByLabelText('姓名'), { target: { value: 'Alice' } })
    fireEvent.change(screen.getByLabelText('邮箱'), { target: { value: 'alice@example.com' } })
    fireEvent.change(screen.getByLabelText('平台地址'), { target: { value: 'https://gitlab.example.com' } })
    fireEvent.click(screen.getByRole('switch', { name: '允许在新配置中选择' }))

    fireEvent.click(screen.getByRole('button', { name: '保存人员' }))

    expect(await screen.findByText('启用人员前请补全用户名和用户 ID')).toBeInTheDocument()
    expect(create).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('switch', { name: '允许在新配置中选择' }))
    fireEvent.click(screen.getByRole('button', { name: '保存人员' }))
    await waitFor(() => expect(create).toHaveBeenCalledWith(expect.objectContaining({ enabled: false })))
  })
})
