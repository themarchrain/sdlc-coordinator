import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { adminApi } from '../../api/client'
import type { Person } from '../../api/types'
import { BatchIdentityLookupModal } from './BatchIdentityLookupModal'

afterEach(() => vi.restoreAllMocks())

const draftPerson: Person = {
  person_id: 'person-1', display_name: 'Alice', email: 'alice@example.com', enabled: false,
  identities: [{ identity_id: 'identity-1', provider: 'gitlab', base_url: 'https://gitlab.example.com', external_user_id: '', username: '' }],
  created_at: '', updated_at: '',
}

describe('BatchIdentityLookupModal', () => {
  it('loads GitLab draft addresses, previews exact matches, and only saves confirmed rows', async () => {
    vi.spyOn(adminApi, 'listPeople').mockResolvedValue({ items: [draftPerson], page: 1, page_size: 100, total: 1 })
    const lookup = vi.spyOn(adminApi, 'lookupCodeHostIdentities').mockResolvedValue({
      items: [{
        person_id: 'person-1', display_name: 'Alice', email: 'alice@example.com', identity_id: 'identity-1',
        candidates: [{ external_user_id: '7101', username: 'alice', display_name: 'Alice' }], exact_match_index: 0,
      }],
    })
    vi.spyOn(adminApi, 'getPerson').mockResolvedValue(draftPerson)
    const update = vi.spyOn(adminApi, 'updatePerson').mockResolvedValue({
      ...draftPerson, identities: [{ ...draftPerson.identities[0], external_user_id: '7101', username: 'alice' }],
    })
    const onSaved = vi.fn().mockResolvedValue(undefined)

    render(<BatchIdentityLookupModal open onClose={vi.fn()} onSaved={onSaved} />)
    expect(await screen.findByText('https://gitlab.example.com')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('临时 GitLab Token'), { target: { value: 'temporary-token' } })
    fireEvent.click(screen.getByRole('button', { name: '查询待补全人员' }))

    await waitFor(() => expect(lookup).toHaveBeenCalledWith({ provider: 'gitlab', base_url: 'https://gitlab.example.com', token: 'temporary-token' }))
    expect(await screen.findByText('alice / ID 7101')).toBeInTheDocument()
    expect(screen.getByRole('checkbox', { name: '保存 Alice' })).toBeChecked()

    fireEvent.click(screen.getByRole('button', { name: '保存选中结果' }))
    await waitFor(() => expect(update).toHaveBeenCalledWith('person-1', {
      display_name: 'Alice', email: 'alice@example.com', enabled: false,
      identities: [{ identity_id: 'identity-1', provider: 'gitlab', base_url: 'https://gitlab.example.com', external_user_id: '7101', username: 'alice' }],
    }))
    expect(onSaved).toHaveBeenCalled()
  })

  it('keeps ambiguous and failed rows unselected', async () => {
    vi.spyOn(adminApi, 'listPeople').mockResolvedValue({ items: [draftPerson], page: 1, page_size: 100, total: 1 })
    vi.spyOn(adminApi, 'lookupCodeHostIdentities').mockResolvedValue({
      items: [{
        person_id: 'person-1', display_name: 'Alice', email: 'alice@example.com', identity_id: 'identity-1',
        candidates: [
          { external_user_id: '7101', username: 'alice', display_name: 'Alice' },
          { external_user_id: '7102', username: 'alice-old', display_name: '旧账号' },
        ], exact_match_index: null,
      }],
    })
    render(<BatchIdentityLookupModal open onClose={vi.fn()} onSaved={vi.fn()} />)
    await screen.findByText('https://gitlab.example.com')
    fireEvent.change(screen.getByLabelText('临时 GitLab Token'), { target: { value: 'temporary-token' } })
    fireEvent.click(screen.getByRole('button', { name: '查询待补全人员' }))

    expect(await screen.findByRole('checkbox', { name: '保存 Alice' })).not.toBeChecked()
    expect(screen.getByRole('button', { name: '保存选中结果' })).toBeDisabled()
  })
})
