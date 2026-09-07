import { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Checkbox, Form, Input, message, Modal, Select, Table, Tag } from 'antd'

import { adminApi } from '../../api/client'
import type { BatchIdentityLookupItem, IdentityLookupCandidate, Person, PersonInput } from '../../api/types'

type SaveState = { succeeded: number; failed: number }

export function BatchIdentityLookupModal({ open, onClose, onSaved }: { open: boolean; onClose: () => void; onSaved: () => Promise<void> }) {
  const [baseURLs, setBaseURLs] = useState<string[]>([])
  const [baseURL, setBaseURL] = useState('')
  const [token, setToken] = useState('')
  const [loadingAddresses, setLoadingAddresses] = useState(false)
  const [querying, setQuerying] = useState(false)
  const [saving, setSaving] = useState(false)
  const [items, setItems] = useState<BatchIdentityLookupItem[]>([])
  const [candidateByKey, setCandidateByKey] = useState<Record<string, number>>({})
  const [selectedKeys, setSelectedKeys] = useState<string[]>([])
  const [saveState, setSaveState] = useState<SaveState>()

  useEffect(() => {
    if (!open) {
      setBaseURLs([])
      setBaseURL('')
      setToken('')
      setItems([])
      setCandidateByKey({})
      setSelectedKeys([])
      setSaveState(undefined)
      return
    }
    let active = true
    setLoadingAddresses(true)
    loadAllPeople().then((people) => {
      if (!active) return
      const urls = Array.from(new Set(people.flatMap((person) => person.identities
        .filter((identity) => identity.provider === 'gitlab' && (!identity.username || !identity.external_user_id))
        .map((identity) => identity.base_url)))).sort()
      setBaseURLs(urls)
      setBaseURL((current) => current && urls.includes(current) ? current : (urls[0] || ''))
    }).catch((reason) => {
      if (active) message.error(reason instanceof Error ? reason.message : '读取待补全人员失败')
    }).finally(() => {
      if (active) setLoadingAddresses(false)
    })
    return () => { active = false }
  }, [open])

  const query = async () => {
    setQuerying(true)
    setSaveState(undefined)
    try {
      const result = await adminApi.lookupCodeHostIdentities({ provider: 'gitlab', base_url: baseURL, token })
      setItems(result.items)
      const choices: Record<string, number> = {}
      const selected: string[] = []
      result.items.forEach((item) => {
        const key = itemKey(item)
        const exact = item.exact_match_index
        if (exact !== null && exact >= 0 && exact < item.candidates.length) {
          choices[key] = exact
          selected.push(key)
        }
      })
      setCandidateByKey(choices)
      setSelectedKeys(selected)
      if (result.items.length === 0) message.info('当前地址下没有待补全的 GitLab 身份')
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '批量查询 GitLab 身份失败')
    } finally {
      setQuerying(false)
      setToken('')
    }
  }

  const selectCandidate = (item: BatchIdentityLookupItem, candidateIndex: number) => {
    const key = itemKey(item)
    setCandidateByKey((current) => ({ ...current, [key]: candidateIndex }))
    setSelectedKeys((current) => current.includes(key) ? current : [...current, key])
  }

  const toggleSelected = (item: BatchIdentityLookupItem, checked: boolean) => {
    const key = itemKey(item)
    setSelectedKeys((current) => checked ? Array.from(new Set([...current, key])) : current.filter((value) => value !== key))
  }

  const saveSelected = async () => {
    setSaving(true)
    try {
      const selectedItems = items.filter((item) => selectedKeys.includes(itemKey(item)))
      const results = await Promise.allSettled(selectedItems.map(async (item) => {
        const candidate = item.candidates[candidateByKey[itemKey(item)]]
        if (!candidate) throw new Error(`人员 ${item.display_name} 尚未选择候选身份`)
        const person = await adminApi.getPerson(item.person_id)
        await adminApi.updatePerson(person.person_id, buildPersonUpdate(person, item.identity_id, candidate))
      }))
      const succeeded = results.filter((result) => result.status === 'fulfilled').length
      const failed = results.length - succeeded
      setSaveState({ succeeded, failed })
      if (succeeded > 0) await onSaved()
      if (failed === 0) {
        message.success(`已保存 ${succeeded} 个人员的 GitLab 身份`)
        onClose()
      } else {
        message.warning(`已保存 ${succeeded} 人，${failed} 人保存失败`)
      }
    } finally {
      setSaving(false)
    }
  }

  const columns = useMemo(() => [
    {
      title: '保存', width: 66,
      render: (_: unknown, item: BatchIdentityLookupItem) => {
        const key = itemKey(item)
        return <Checkbox aria-label={`保存 ${item.display_name}`} checked={selectedKeys.includes(key)} disabled={candidateByKey[key] === undefined} onChange={(event) => toggleSelected(item, event.target.checked)} />
      },
    },
    { title: '人员', width: 180, render: (_: unknown, item: BatchIdentityLookupItem) => <div><strong>{item.display_name}</strong><small className="table-secondary">{item.email}</small></div> },
    {
      title: '匹配结果',
      render: (_: unknown, item: BatchIdentityLookupItem) => {
        const key = itemKey(item)
        if (item.error_code) return <Tag>查询失败</Tag>
        if (item.candidates.length === 0) return <Tag>未找到</Tag>
        return <Select
          aria-label={`候选身份 ${item.display_name}`}
          value={candidateByKey[key]}
          placeholder="请选择候选身份"
          options={item.candidates.map((candidate, index) => ({ value: index, label: `${candidate.username} / ID ${candidate.external_user_id}` }))}
          onChange={(value) => selectCandidate(item, value)}
        />
      },
    },
    {
      title: '状态', width: 110,
      render: (_: unknown, item: BatchIdentityLookupItem) => item.error_code
        ? <Tag>查询失败</Tag>
        : item.exact_match_index !== null ? <Tag color="processing">精准匹配</Tag> : item.candidates.length > 0 ? <Tag color="warning">待确认</Tag> : <Tag>未找到</Tag>,
    },
  ], [candidateByKey, selectedKeys])

  return <Modal title="统一查询 GitLab 用户 ID" open={open} width={880} destroyOnClose maskClosable={false} onCancel={onClose} footer={[
    <Button key="cancel" onClick={onClose}>关闭</Button>,
    <Button key="save" type="primary" loading={saving} disabled={selectedKeys.length === 0} onClick={saveSelected}>保存选中结果</Button>,
  ]}>
    <p className="form-note">对同一 GitLab 地址下的待补全人员统一查询。Token 只用于本次预览，查询结束后立即清空。</p>
    <div className="people-batch-controls">
      <Form.Item label="GitLab 地址" required>
        <Select aria-label="GitLab 地址" loading={loadingAddresses} value={baseURL || undefined} options={baseURLs.map((value) => ({ value, label: value }))} onChange={(value) => { setBaseURL(value); setItems([]); setSelectedKeys([]) }} placeholder="暂无待补全的 GitLab 地址" />
      </Form.Item>
      <Form.Item label="临时 GitLab Token" required>
        <Input.Password aria-label="临时 GitLab Token" autoComplete="new-password" value={token} onChange={(event) => setToken(event.target.value)} />
      </Form.Item>
      <Button className="people-batch-query" loading={querying} disabled={!baseURL || !token} onClick={query}>查询待补全人员</Button>
    </div>
    {saveState ? <Alert type={saveState.failed > 0 ? 'warning' : 'success'} showIcon message={`保存结果：成功 ${saveState.succeeded} 人，失败 ${saveState.failed} 人`} /> : null}
    {items.length > 0 ? <Table<BatchIdentityLookupItem> className="people-batch-results" size="small" rowKey={itemKey} dataSource={items} columns={columns} pagination={false} /> : null}
  </Modal>
}

async function loadAllPeople(): Promise<Person[]> {
  const people: Person[] = []
  for (let page = 1; ; page++) {
    const result = await adminApi.listPeople({ provider: 'gitlab', page, page_size: 100 })
    people.push(...result.items)
    if (people.length >= result.total || result.items.length === 0) return people
  }
}

function itemKey(item: BatchIdentityLookupItem) {
  return `${item.person_id}:${item.identity_id}`
}

function buildPersonUpdate(person: Person, identityID: string, candidate: IdentityLookupCandidate): PersonInput {
  return {
    display_name: person.display_name,
    email: person.email,
    enabled: person.enabled,
    identities: person.identities.map(({ identity_id, provider, base_url, external_user_id, username }) => ({
      identity_id,
      provider,
      base_url,
      external_user_id: identity_id === identityID ? candidate.external_user_id : external_user_id,
      username: identity_id === identityID ? candidate.username : username,
    })),
  }
}
