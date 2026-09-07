export type SearchParamPatch = Record<string, string | number | null | undefined>

export function repositoryFilter(searchParams: URLSearchParams): { repository_id?: string } {
  const repositoryID = searchParams.get('repository_id')
  return repositoryID ? { repository_id: repositoryID } : {}
}

export function patchSearchParams(current: URLSearchParams, patch: SearchParamPatch): URLSearchParams {
  const next = new URLSearchParams(current)
  Object.entries(patch).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') {
      next.delete(key)
      return
    }
    next.set(key, String(value))
  })
  return next
}
