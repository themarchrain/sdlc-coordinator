import { afterEach, describe, expect, it, vi } from 'vitest'

import { AdminApiError, adminApi, queryString } from './client'

function response(body: unknown, status = 200): Response {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

afterEach(() => vi.unstubAllGlobals())

describe('admin API client', () => {
  it('builds stable query strings and omits empty values', () => {
    expect(queryString({ page: 2, status: 'running', role: '', enabled: false, missing: undefined })).toBe('?page=2&status=running&enabled=false')
  })

  it('queries tasks through the same-origin admin API', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(response({ items: [], page: 1, page_size: 20, total: 0 })))
    vi.stubGlobal('fetch', fetchMock)
    await adminApi.listTasks({ page: 1, page_size: 20, status: 'human_required' })
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/tasks?page=1&page_size=20&status=human_required', expect.any(Object))
  })

  it('queries automation metrics with window and task type filters', async () => {
    const fetchMock = vi.fn().mockResolvedValue(response({ window: '7d', task_sample_count: 0 }))
    vi.stubGlobal('fetch', fetchMock)
    await adminApi.metrics({ window: '7d', type: 'develop_from_issue' })
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/metrics?window=7d&type=develop_from_issue', expect.any(Object))
  })

  it('normalizes the admin error envelope', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({ error: { code: 'connector_unavailable', message: '连接器当前不可用', details: { retryable: true } } }, 502)))
    await expect(adminApi.probeConnector('connector-1')).rejects.toEqual(expect.objectContaining<Partial<AdminApiError>>({
      status: 502,
      code: 'connector_unavailable',
      message: '连接器当前不可用',
    }))
  })

  it('only addresses a registered connector by ID', async () => {
    const fetchMock = vi.fn().mockResolvedValue(response({ request_id: 'request-1', run: { run_id: 'run-1' } }))
    vi.stubGlobal('fetch', fetchMock)
    await adminApi.triggerConnector('connector/with spaces')
    expect(fetchMock.mock.calls[0][0]).toBe('/api/admin/connectors/connector%2Fwith%20spaces/runs')
    expect(fetchMock.mock.calls[0][1]).toEqual(expect.objectContaining({ method: 'POST' }))
  })

  it('posts effect retry by effect ID without exposing its dedupe key', async () => {
    const fetchMock = vi.fn().mockResolvedValue(response({ effect_id: 'effect-1', status: 'pending' }, 202))
    vi.stubGlobal('fetch', fetchMock)
    await adminApi.retryEffect('effect:1')
    expect(fetchMock.mock.calls[0][0]).toBe('/api/admin/effects/effect%3A1/retry')
  })

  it('uses the coordinator configuration management contract', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(response({ mode: 'setup_pending', revision: 0 })))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.configurationStatus()
    await adminApi.configuration()
    await adminApi.validateConfiguration({ config: {} } as never)
    await adminApi.updateConfiguration({ config: {} } as never)
    await adminApi.restartConfiguration()

    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      '/api/admin/configuration/status',
      '/api/admin/configuration',
      '/api/admin/configuration/validate',
      '/api/admin/configuration',
      '/api/admin/configuration/restart',
    ])
    expect(fetchMock.mock.calls[2][1]).toEqual(expect.objectContaining({ method: 'POST' }))
    expect(fetchMock.mock.calls[3][1]).toEqual(expect.objectContaining({ method: 'PUT' }))
    expect(fetchMock.mock.calls[4][1]).toEqual(expect.objectContaining({ method: 'POST' }))
  })

  it('uses the person directory and transient GitLab lookup contracts', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(response({ items: [], page: 1, page_size: 20, total: 0 })))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.listPeople({ q: 'alice', enabled: true, page: 1, page_size: 20 })
    await adminApi.lookupCodeHostIdentity({
      provider: 'gitlab', base_url: 'https://gitlab.example.com', token: 'temporary-token', email: 'user@example.com', username: '',
    })
    await adminApi.lookupCodeHostIdentities({
      provider: 'gitlab', base_url: 'https://gitlab.example.com', token: 'temporary-token',
    })

    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      '/api/admin/people?q=alice&enabled=true&page=1&page_size=20',
      '/api/admin/people/codehost-identities/lookup',
      '/api/admin/people/codehost-identities/lookup-batch',
    ])
    expect(fetchMock.mock.calls[1][1]).toEqual(expect.objectContaining({ method: 'POST' }))
    expect(fetchMock.mock.calls[2][1]).toEqual(expect.objectContaining({ method: 'POST' }))
  })

  it('uses the registered repository admin contract and never puts a token in the URL', async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(response({ items: [] })))
    vi.stubGlobal('fetch', fetchMock)

    await adminApi.listRepositories()
    await adminApi.getRepository('repo_a')
    await adminApi.createRepository({
      display_name: 'Coordinator',
provider: 'gitlab',
base_url: 'https://gitlab.example.com',
      path_with_namespace: 'group/coordinator',
      token: 'repo-token-value',
      token_scope_attested: true,
      webhook_secret: 'repo-hook-secret',
    })
    await adminApi.patchRepository('repo_a', { display_name: 'Coordinator',
provider: 'gitlab',
base_url: 'https://gitlab.example.com', token: 'rotated-token' })
    await adminApi.disableRepository('repo_a')
    await adminApi.enableRepository('repo_a')
    expect(fetchMock.mock.calls.map((call) => call[0])).toEqual([
      '/api/admin/repositories',
      '/api/admin/repositories/repo_a',
      '/api/admin/repositories',
      '/api/admin/repositories/repo_a',
      '/api/admin/repositories/repo_a/disable',
      '/api/admin/repositories/repo_a/enable',
    ])
    expect(fetchMock.mock.calls.every((call) => !String(call[0]).includes('repo-token-value'))).toBe(true)
    expect(fetchMock.mock.calls.every((call) => !String(call[0]).includes('rotated-token'))).toBe(true)
    expect(JSON.parse(String(fetchMock.mock.calls[2][1].body))).toEqual(expect.objectContaining({
      token: 'repo-token-value',
      token_scope_attested: true,
    }))
    expect(fetchMock.mock.calls[2][1]).toEqual(expect.objectContaining({ method: 'POST' }))
    expect(fetchMock.mock.calls[3][1]).toEqual(expect.objectContaining({ method: 'PATCH' }))
    expect(fetchMock.mock.calls[4][1]).toEqual(expect.objectContaining({ method: 'POST' }))
    expect(fetchMock.mock.calls[5][1]).toEqual(expect.objectContaining({ method: 'POST' }))
  })
})
