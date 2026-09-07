import { useCallback, useEffect, useState, type DependencyList } from 'react'

export function useRemote<T>(loader: () => Promise<T>, dependencies: DependencyList, enabled = true) {
  const [data, setData] = useState<T>()
  const [error, setError] = useState<unknown>()
  const [loading, setLoading] = useState(true)
  const [version, setVersion] = useState(0)

  const reload = useCallback(() => setVersion((current) => current + 1), [])

  useEffect(() => {
    if (!enabled) {
      setLoading(false)
      return
    }
    let active = true
    setLoading(true)
    setError(undefined)
    loader().then((result) => {
      if (active) setData(result)
    }).catch((reason) => {
      if (active) setError(reason)
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
    // Loader is intentionally defined by the caller; dependency identity is explicit.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...dependencies, version, enabled])

  return { data, error, loading, reload }
}
