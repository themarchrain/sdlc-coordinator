import { Select } from 'antd'
import { useSearchParams } from 'react-router-dom'

import { adminApi } from '../api/client'
import { patchSearchParams } from '../app/searchParams'
import { useRemote } from '../hooks/useRemote'
import { StatusTag } from './StatusTag'

export function RepositorySwitcher() {
  const [searchParams, setSearchParams] = useSearchParams()
  const selected = searchParams.get('repository_id') ?? ''
  const repositories = useRemote(() => adminApi.listRepositories(), [])

  return (
    <div className="repository-switcher">
      <span>仓库：</span>
      <Select
        aria-label="仓库"
        value={selected}
        popupMatchSelectWidth={false}
        style={{ minWidth: 188 }}
        onChange={(value) => {
          setSearchParams(patchSearchParams(searchParams, {
            repository_id: value || undefined,
            page: undefined,
          }))
        }}
        options={[
          { value: '', label: '全部仓库' },
          ...(repositories.data?.items ?? []).map((repo) => ({
            value: repo.repository_id,
            label: (
              <span className="repository-switcher__option">
                {repo.display_name} <StatusTag status={repo.status} />
              </span>
            ),
          })),
        ]}
      />
    </div>
  )
}
