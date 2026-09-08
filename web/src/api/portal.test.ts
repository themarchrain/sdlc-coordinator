import { afterEach, describe, expect, it, vi } from 'vitest'

import { portalApi, PortalApiError } from './portal'

afterEach(() => vi.restoreAllMocks())

function mockFetchOnce(status: number, payload: unknown) {
  return vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify(payload), {
          status,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    ),
  )
}

describe('portalApi', () => {
  it('logs in and returns the session payload', async () => {
    const payload = {
      user: { id: 1, username: 'u', role: 'user', must_change_password: false },
      scope: { kind: 'tenant', role: 'member', tenant: { id: 2, slug: 'acme', display_name: 'Acme', status: 'active' } },
    }
    mockFetchOnce(200, payload)
    const result = await portalApi.login('u', 'p')
    expect(result.scope.kind).toBe('tenant')
    expect(fetch).toHaveBeenCalledWith(
      '/api/portal/auth/login',
      expect.objectContaining({ method: 'POST', credentials: 'same-origin' }),
    )
  })

  it('throws PortalApiError with backend code on failure', async () => {
    mockFetchOnce(401, { error: { code: 'invalid_credentials', message: '账号或密码错误' } })
    const error = await portalApi.login('u', 'bad').catch((reason: unknown) => reason)
    expect(error).toBeInstanceOf(PortalApiError)
    expect((error as PortalApiError).status).toBe(401)
    expect((error as PortalApiError).code).toBe('invalid_credentials')
    expect((error as PortalApiError).message).toBe('账号或密码错误')
  })

  it('registers with an invite code and selects tenants', async () => {
    const fetchMock = mockFetchOnce(200, { user: { id: 3, username: 'n', role: 'user', must_change_password: false }, scope: { kind: 'tenant' } })
    await portalApi.register('n', 'password-001', 'ABCD-EFGH-JKLM')
    const [path, init] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit]
    expect(path).toBe('/api/portal/auth/register')
    expect(JSON.parse(String(init.body))).toEqual({ username: 'n', password: 'password-001', invite_code: 'ABCD-EFGH-JKLM' })

    mockFetchOnce(200, { scope: { kind: 'tenant' } })
    await portalApi.selectTenant(7)
    const [selectPath] = (fetch as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit]
    expect(selectPath).toBe('/api/portal/auth/select-tenant')
  })
})
