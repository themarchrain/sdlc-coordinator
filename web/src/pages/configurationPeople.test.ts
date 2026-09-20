import { describe, expect, it } from 'vitest'

import type { Person, TenantConfigurationView } from '../api/types'
import { buildRuntimePeopleFields, personCodeHostIdentity, resolveTenantConfigurationPeople } from './configurationPeople'

const people: Person[] = [
  {
    person_id: 'person-owner', display_name: '负责人', email: 'owner@example.com', enabled: true,
    identities: [{ identity_id: 'identity-owner', provider: 'gitlab', base_url: 'https://gitlab.example.com', external_user_id: '7', username: 'owner' }],
    created_at: '', updated_at: '',
  },
  {
    person_id: 'person-reviewer', display_name: '审查人', email: 'reviewer@example.com', enabled: true,
    identities: [], created_at: '', updated_at: '',
  },
]

const view = {
  config: {
    codehost: { provider: 'gitlab', base_url: 'https://gitlab.example.com/', rework_allowed_user_ids: ['7', '99'] },
    im: { reviewer_mentions: ['审查人', '历史提醒人'] },
  },
} as unknown as TenantConfigurationView

describe('configuration people resolution', () => {
  it('matches people and keeps unmatched runtime values as explicit legacy values', () => {
    expect(resolveTenantConfigurationPeople(view, people)).toEqual({
      rework_person_ids: ['person-owner'],
      reviewer_person_ids: ['person-reviewer'],
      legacy_rework_user_ids: ['99'],
      legacy_reviewer_mentions: ['历史提醒人'],
    })
  })

  it('builds runtime fields from selected people and retained legacy values', () => {
    const fields = buildRuntimePeopleFields({
      rework_person_ids: ['person-owner'], reviewer_person_ids: ['person-reviewer'],
      legacy_rework_user_ids: ['99'], legacy_reviewer_mentions: ['历史提醒人'],
    }, people, 'gitlab', 'https://gitlab.example.com')

    expect(fields).toEqual({
      rework_allowed_user_ids: ['7', '99'],
      reviewer_mentions: ['审查人', '历史提醒人'],
    })
  })

  it('rejects selected rework people that cannot resolve against the current CodeHost', () => {
    expect(() => buildRuntimePeopleFields({
      rework_person_ids: ['person-owner'], reviewer_person_ids: [],
      legacy_rework_user_ids: [], legacy_reviewer_mentions: [],
    }, people, 'gitlab', 'https://another-gitlab.example.com')).toThrow(/负责人.*缺少.*CodeHost/)
  })

  it('preserves case-sensitive URL paths while matching CodeHost identities', () => {
    const person = { ...people[0], identities: [{ ...people[0].identities[0], base_url: 'https://gitlab.example.com/Team' }] }
    expect(personCodeHostIdentity(person, 'gitlab', 'https://gitlab.example.com/team')).toBeUndefined()
  })

})
