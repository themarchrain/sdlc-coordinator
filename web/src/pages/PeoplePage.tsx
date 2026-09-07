import { useEffect, useState } from 'react'
import { Button, Input, message, Popconfirm, Space, Switch, Table, Tag } from 'antd'
import { DeleteOutlined, EditOutlined, PlusOutlined, SearchOutlined, SyncOutlined } from '@ant-design/icons'
import { useLocation, useNavigate } from 'react-router-dom'

import { adminApi } from '../api/client'
import type { Person } from '../api/types'
import { ErrorState } from '../components/PageState'
import { PageHeading } from '../components/PageHeading'
import { PersonEditor } from './people/PersonEditor'
import { BatchIdentityLookupModal } from './people/BatchIdentityLookupModal'

export function PeoplePage() {
  const location = useLocation()
  const navigate = useNavigate()
  const [items, setItems] = useState<Person[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<unknown>()
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [editing, setEditing] = useState<Person | 'new'>()
  const [batchLookupOpen, setBatchLookupOpen] = useState(false)

  const load = async (q = query, nextPage = page) => {
    setLoading(true)
    setError(undefined)
    try {
      const result = await adminApi.listPeople({ q, page: nextPage, page_size: 20 })
      setItems(result.items)
      setPage(result.page)
      setTotal(result.total)
    } catch (reason) {
      setError(reason)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load('', 1) }, [])

  const remove = async (person: Person) => {
    try {
      await adminApi.deletePerson(person.person_id)
      message.success('人员已删除')
      await load(query, page)
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '删除失败')
    }
  }

  const updateEnabled = async (person: Person, enabled: boolean) => {
    try {
      await adminApi.updatePerson(person.person_id, {
        display_name: person.display_name,
        email: person.email,
        enabled,
        identities: person.identities.map(({ identity_id, provider, base_url, external_user_id, username }) => ({
          identity_id, provider, base_url, external_user_id, username,
        })),
      })
      message.success(enabled ? '人员已启用' : '人员已禁用')
      await load(query, page)
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '状态更新失败')
    }
  }

  const returnTo = (location.state as { returnTo?: string } | null)?.returnTo
  return <div className="people-page">
    <PageHeading eyebrow="团队资产" title="人员资产" />
    <section className="surface-panel people-panel">
      <div className="people-toolbar">
        <Space.Compact><Input value={query} allowClear placeholder="搜索姓名、邮箱或用户名" onChange={(event) => setQuery(event.target.value)} onPressEnter={() => void load(query, 1)} /><Button icon={<SearchOutlined />} onClick={() => void load(query, 1)}>查询</Button></Space.Compact>
        <Space>{returnTo ? <Button onClick={() => navigate(returnTo)}>返回配置</Button> : null}<Button icon={<SyncOutlined />} onClick={() => setBatchLookupOpen(true)}>统一查询用户 ID</Button><Button type="primary" icon={<PlusOutlined />} onClick={() => setEditing('new')}>新增人员</Button></Space>
      </div>
      {error ? <ErrorState error={error} /> : <Table<Person>
        size="small" loading={loading} rowKey="person_id" dataSource={items} pagination={{ current: page, pageSize: 20, total, showSizeChanger: false, onChange: (nextPage) => void load(query, nextPage) }}
        columns={[
          { title: '人员', render: (_, person) => <div><strong>{person.display_name}</strong><small className="table-secondary">{person.email}</small></div> },
          { title: 'CodeHost 身份', render: (_, person) => <div className="people-identities">{person.identities.length === 0 ? '—' : person.identities.map((identity) => <div key={identity.identity_id || `${identity.provider}:${identity.base_url}:${identity.username}`}><Tag>{{ gitlab: 'GitLab', gitea: 'Gitea', github: 'GitHub' }[identity.provider] || identity.provider}</Tag>{identity.username && identity.external_user_id ? <span>{identity.username} / ID {identity.external_user_id}</span> : <Tag className="people-draft-tag">待补全</Tag>}<small>{identity.base_url}</small></div>)}</div> },
          { title: '状态', width: 100, render: (_, person) => <Switch checked={person.enabled} checkedChildren="启用" unCheckedChildren="禁用" aria-label={`${person.enabled ? '停用' : '启用'}${person.display_name}`} onChange={(enabled) => void updateEnabled(person, enabled)} /> },
          { title: '更新时间', width: 180, dataIndex: 'updated_at', render: (value: string) => value ? new Date(value).toLocaleString() : '—' },
          { title: '操作', width: 150, render: (_, person) => <Space><Button type="text" icon={<EditOutlined />} onClick={() => setEditing(person)}>编辑</Button><Popconfirm title="删除该人员？" description="历史运行快照不会改变。" okText="删除" cancelText="取消" onConfirm={() => remove(person)}><Button type="text" danger icon={<DeleteOutlined />}>删除</Button></Popconfirm></Space> },
        ]}
      />}
    </section>
    {editing ? <PersonEditor person={editing} onClose={() => setEditing(undefined)} onSaved={async () => { setEditing(undefined); await load(query, page) }} /> : null}
    <BatchIdentityLookupModal open={batchLookupOpen} onClose={() => setBatchLookupOpen(false)} onSaved={() => load(query, page)} />
  </div>
}
