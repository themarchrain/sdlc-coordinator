import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  MAX_SAVED_ACCOUNTS,
  clearSavedAccounts,
  SESSION_COOKIE_NAME,
  listSavedAccounts,
  otherSavedAccounts,
  removeSavedAccount,
  saveCurrentAccount,
} from './accountRegistry'

const COOKIE_NAME = SESSION_COOKIE_NAME

function setCookieToken(value: string) {
  // jsdom 允许写非 HttpOnly cookie；过期删除用 Max-Age=0。
  document.cookie = `${COOKIE_NAME}=${value}; path=/`
}

function clearCookieToken() {
  document.cookie = `${COOKIE_NAME}=; path=/; max-age=0`
}

describe('accountRegistry', () => {
  beforeEach(() => {
    window.localStorage.clear()
    clearCookieToken()
  })

  it('saves the given session token as an account entry', () => {
    setCookieToken('token-a')
    saveCurrentAccount(11, 'alice', 'token-a')
    const list = listSavedAccounts()
    expect(list).toHaveLength(1)
    expect(list[0]).toMatchObject({ token: 'token-a', userId: 11, username: 'alice' })
  })

  it('does nothing when there is no session token', () => {
    saveCurrentAccount(11, 'alice', '')
    expect(listSavedAccounts()).toHaveLength(0)
  })

  it('deduplicates by user id, overwriting the older token', () => {
    setCookieToken('token-a-1')
    saveCurrentAccount(11, 'alice', 'token-a-1')
    setCookieToken('token-a-2')
    saveCurrentAccount(11, 'alice', 'token-a-2')
    const list = listSavedAccounts()
    expect(list).toHaveLength(1)
    expect(list[0].token).toBe('token-a-2')
  })

  it('keeps distinct accounts and evicts the oldest beyond capacity', () => {
    for (let i = 0; i < MAX_SAVED_ACCOUNTS + 1; i += 1) {
      setCookieToken(`token-${i}`)
      saveCurrentAccount(100 + i, `user-${i}`, `token-${i}`)
    }
    const list = listSavedAccounts()
    expect(list).toHaveLength(MAX_SAVED_ACCOUNTS)
    // 最早保存的 user-0 被淘汰，最新 user-5 保留。
    expect(list.map((item) => item.username)).not.toContain('user-0')
    expect(list.map((item) => item.username)).toContain(`user-${MAX_SAVED_ACCOUNTS}`)
  })

  it('removes a single account by user id', () => {
    setCookieToken('token-a')
    saveCurrentAccount(11, 'alice', 'token-a')
    setCookieToken('token-b')
    saveCurrentAccount(22, 'bob', 'token-b')
    removeSavedAccount(11)
    expect(listSavedAccounts().map((item) => item.username)).toEqual(['bob'])
  })

  it('clears the whole registry', () => {
    setCookieToken('token-a')
    saveCurrentAccount(11, 'alice', 'token-a')
    clearSavedAccounts()
    expect(listSavedAccounts()).toHaveLength(0)
  })

  it('lists other accounts relative to the current user', () => {
    setCookieToken('token-a')
    saveCurrentAccount(11, 'alice', 'token-a')
    setCookieToken('token-b')
    saveCurrentAccount(22, 'bob', 'token-b')
    expect(otherSavedAccounts(22).map((item) => item.username)).toEqual(['alice'])
    expect(otherSavedAccounts(null)).toHaveLength(2)
  })

  it('ignores corrupted registry payloads', () => {
    window.localStorage.setItem('coordinator_portal_accounts', 'not-json{')
    expect(listSavedAccounts()).toHaveLength(0)
    window.localStorage.setItem('coordinator_portal_accounts', JSON.stringify([{ bogus: true }]))
    expect(listSavedAccounts()).toHaveLength(0)
  })

  it('survives a missing localStorage (quota/private mode) without throwing', () => {
    const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('quota exceeded')
    })
    setCookieToken('token-a')
    expect(() => saveCurrentAccount(11, 'alice', 'token-a')).not.toThrow()
    spy.mockRestore()
  })
})
