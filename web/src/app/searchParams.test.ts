import { describe, expect, it } from 'vitest'

import { patchSearchParams, repositoryFilter } from './searchParams'

describe('patchSearchParams', () => {
  it('updates selected values while preserving unrelated parameters', () => {
    const params = patchSearchParams(new URLSearchParams('status=running&task_id=task-1'), { task_id: undefined, role: 'DeveloperWorker' })

    expect(params.toString()).toBe('status=running&role=DeveloperWorker')
  })

  it('removes empty values', () => {
    const params = patchSearchParams(new URLSearchParams('status=running'), { status: '', repository: null })

    expect(params.toString()).toBe('')
  })
})

describe('repositoryFilter', () => {
  it('passes through repository_id and omits it when absent', () => {
    expect(repositoryFilter(new URLSearchParams('repository_id=repo_a&status=running'))).toEqual({ repository_id: 'repo_a' })
    expect(repositoryFilter(new URLSearchParams('status=running'))).toEqual({})
  })
})
