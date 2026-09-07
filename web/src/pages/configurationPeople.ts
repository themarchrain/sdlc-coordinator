import { adminApi } from '../api/client'
import type { ConfigurationView, Person } from '../api/types'

export interface ConfigurationPeopleSelection {
  rework_person_ids: string[]
  reviewer_person_ids: string[]
  legacy_rework_user_ids: string[]
  legacy_reviewer_mentions: string[]
}

export interface RuntimePeopleFields {
  rework_allowed_user_ids: string[]
  reviewer_mentions: string[]
}

export function resolveConfigurationPeople(view: ConfigurationView, people: Person[]): ConfigurationPeopleSelection {
  const provider = view.config.codehost.provider
  const baseURL = normalizeBaseURL(view.config.codehost.base_url)
  const reworkPersonIDs: string[] = []
  const reviewerPersonIDs: string[] = []
  const legacyReworkUserIDs: string[] = []
  const legacyReviewerMentions: string[] = []

  ;(view.config.codehost.rework_allowed_user_ids || []).forEach((userID) => {
    const person = people.find((candidate) => identityFor(candidate, provider, baseURL)?.external_user_id === userID)
    if (person) addUnique(reworkPersonIDs, person.person_id)
    else legacyReworkUserIDs.push(userID)
  })

  ;(view.config.im.reviewer_mentions || []).forEach((mention) => {
    const person = people.find((candidate) => candidate.display_name.toLowerCase() === mention.toLowerCase())
    if (person) addUnique(reviewerPersonIDs, person.person_id)
    else legacyReviewerMentions.push(mention)
  })

  return {
    rework_person_ids: reworkPersonIDs,
    reviewer_person_ids: reviewerPersonIDs,
    legacy_rework_user_ids: legacyReworkUserIDs,
    legacy_reviewer_mentions: legacyReviewerMentions,
  }
}

export function buildRuntimePeopleFields(selection: ConfigurationPeopleSelection, people: Person[], provider: 'gitlab' | 'gitea' | 'github', rawBaseURL: string): RuntimePeopleFields {
  const baseURL = normalizeBaseURL(rawBaseURL)
  const reworkIDs: string[] = []
  const reviewerMentions: string[] = []
  const unresolved: string[] = []

  selection.rework_person_ids.forEach((personID) => {
    const person = people.find((candidate) => candidate.person_id === personID)
    const identity = person && identityFor(person, provider, baseURL)
    if (identity && positiveInteger(identity.external_user_id)) addUnique(reworkIDs, identity.external_user_id)
    else addUnique(unresolved, person?.display_name || personID)
  })
  selection.legacy_rework_user_ids.forEach((userID) => addUnique(reworkIDs, userID))

  selection.reviewer_person_ids.forEach((personID) => {
    const person = people.find((candidate) => candidate.person_id === personID)
    if (person && person.display_name.trim()) addUnique(reviewerMentions, person.display_name.trim())
    else addUnique(unresolved, person?.display_name || personID)
  })
  selection.legacy_reviewer_mentions.forEach((mention) => addUnique(reviewerMentions, mention))

  if (unresolved.length > 0) {
    throw new Error(`人员“${unresolved.join('、')}”缺少当前 CodeHost 的有效身份，请重新选择或补全人员资产`)
  }
  return { rework_allowed_user_ids: reworkIDs, reviewer_mentions: reviewerMentions }
}

export function eligibleCodeHostPeople(people: Person[], provider: 'gitlab' | 'gitea', baseURL: string): Person[] {
  const normalized = normalizeBaseURL(baseURL)
  return people.filter((person) => person.enabled && Boolean(identityFor(person, provider, normalized)) && positiveInteger(identityFor(person, provider, normalized)?.external_user_id || ''))
}

export function personLabel(person: Person, provider?: 'gitlab' | 'gitea', baseURL?: string): string {
  const identity = provider && baseURL ? personCodeHostIdentity(person, provider, baseURL) : undefined
  return [person.display_name, identity?.username, person.email].filter(Boolean).join(' / ')
}

export function personCodeHostIdentity(person: Person, provider: 'gitlab' | 'gitea', baseURL: string) {
  return identityFor(person, provider, normalizeBaseURL(baseURL))
}

export async function loadAllPeople(): Promise<Person[]> {
  const items: Person[] = []
  let page = 1
  while (true) {
    const result = await adminApi.listPeople({ page, page_size: 100 })
    items.push(...result.items)
    if (items.length >= result.total || result.items.length === 0) return items
    page += 1
  }
}

function identityFor(person: Person, provider: 'gitlab' | 'gitea' | 'github', baseURL: string) {
  return person.identities.find((identity) => identity.provider === provider && normalizeBaseURL(identity.base_url) === baseURL)
}

function normalizeBaseURL(value: string) {
  try {
    const parsed = new URL(value.trim())
    const path = parsed.pathname.replace(/\/+$/, '')
    return `${parsed.protocol.toLowerCase()}//${parsed.host.toLowerCase()}${path}`
  } catch {
    return value.trim().replace(/\/+$/, '')
  }
}
function positiveInteger(value: string) { const parsed = Number(value); return Number.isInteger(parsed) && parsed > 0 }
function addUnique(values: string[], value: string) { if (!values.includes(value)) values.push(value) }
