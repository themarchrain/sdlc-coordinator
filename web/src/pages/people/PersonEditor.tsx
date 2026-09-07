import { useState } from 'react'
import { Button, Divider, Form, Input, message, Modal, Radio, Select, Switch } from 'antd'
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons'

import { adminApi } from '../../api/client'
import type { IdentityLookupCandidate, Person, PersonInput } from '../../api/types'

type IdentityFormValue = PersonInput['identities'][number]

export function PersonEditor({ person, onClose, onSaved }: { person: Person | 'new'; onClose: () => void; onSaved: () => Promise<void> }) {
  const [form] = Form.useForm<PersonInput>()
  const [saving, setSaving] = useState(false)
  const [lookupIndex, setLookupIndex] = useState<number>()
  const [lookupToken, setLookupToken] = useState('')
  const [lookupLoading, setLookupLoading] = useState(false)
  const [candidates, setCandidates] = useState<IdentityLookupCandidate[]>([])
  const [selectedCandidate, setSelectedCandidate] = useState<number>()
  const initialValues: PersonInput = person === 'new'
    ? { display_name: '', email: '', enabled: false, identities: [{ provider: 'gitlab', base_url: '', external_user_id: '', username: '' }] }
    : {
        display_name: person.display_name,
        email: person.email,
        enabled: person.enabled,
        identities: person.identities.map(({ provider, base_url, external_user_id, username, identity_id }) => ({ provider, base_url, external_user_id, username, identity_id })),
      }

  const closeLookup = () => {
    setLookupIndex(undefined)
    setLookupToken('')
    setCandidates([])
    setSelectedCandidate(undefined)
  }

  const closeEditor = () => {
    closeLookup()
    onClose()
  }

  const queryIdentity = async () => {
    if (lookupIndex === undefined) return
    const identity = form.getFieldValue(['identities', lookupIndex]) as IdentityFormValue | undefined
    if (!identity) return
    setCandidates([])
    setSelectedCandidate(undefined)
    setLookupLoading(true)
    try {
      const result = await adminApi.lookupCodeHostIdentity({
        provider: 'gitlab', base_url: identity.base_url, token: lookupToken, email: form.getFieldValue('email'),
      })
      setCandidates(result.candidates)
      setSelectedCandidate(result.exact_match_index ?? undefined)
      if (result.candidates.length === 0) message.warning('未找到候选用户，可返回手动填写')
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : 'GitLab 用户查询失败')
    } finally {
      setLookupLoading(false)
    }
  }

  const confirmCandidate = () => {
    if (lookupIndex === undefined || selectedCandidate === undefined || !candidates[selectedCandidate]) return
    const identities = [...(form.getFieldValue('identities') || [])]
    identities[lookupIndex] = {
      ...identities[lookupIndex],
      external_user_id: candidates[selectedCandidate].external_user_id,
      username: candidates[selectedCandidate].username,
    }
    form.setFieldValue('identities', identities)
    closeLookup()
    message.success('GitLab 身份已回填，请保存人员后生效')
  }

  const validateIdentityCompleteness = (values: PersonInput) => {
    const identities = values.identities || []
    const incompleteIndexes = identities.flatMap((identity, index) => {
      const hasUsername = Boolean(identity.username?.trim())
      const hasUserID = Boolean(identity.external_user_id?.trim())
      return hasUsername !== hasUserID || (values.enabled && (!hasUsername || !hasUserID)) ? [index] : []
    })
    if (values.enabled && identities.length === 0) {
      message.error('启用人员前请至少添加一个 CodeHost 身份')
      return false
    }
    if (incompleteIndexes.length === 0) return true
    form.setFields(incompleteIndexes.flatMap((index) => [
      { name: ['identities', index, 'username'], errors: ['用户名和用户 ID 需要同时补全'] },
      { name: ['identities', index, 'external_user_id'], errors: ['用户名和用户 ID 需要同时补全'] },
    ]))
    message.error(values.enabled ? '启用人员前请补全用户名和用户 ID' : '用户名和用户 ID 需要同时填写或同时留空')
    return false
  }

  const save = async () => {
    setSaving(true)
    try {
      const values = await form.validateFields()
      if (!validateIdentityCompleteness(values)) return
      if (person === 'new') await adminApi.createPerson(values)
      else await adminApi.updatePerson(person.person_id, values)
      message.success('人员信息已保存')
      await onSaved()
    } catch (reason) {
      if (reason instanceof Error) message.error(reason.message)
    } finally {
      setSaving(false)
    }
  }

  return <>
    <Modal title={person === 'new' ? '新增人员' : '编辑人员'} open onCancel={closeEditor} onOk={save} okText="保存人员" cancelText="取消" confirmLoading={saving} width={820} destroyOnClose>
      <Form form={form} layout="vertical" initialValues={initialValues} preserve={false}>
        <Divider orientation="left" plain>系统资产</Divider>
        <div className="configuration-form-grid">
          <Form.Item name="display_name" label="姓名" rules={[{ required: true, message: '请输入姓名' }]}><Input /></Form.Item>
          <Form.Item name="email" label="邮箱" rules={[{ required: true, message: '请输入邮箱' }, { type: 'email', message: '请输入有效邮箱' }]}><Input /></Form.Item>
          <Form.Item name="enabled" label="允许在新配置中选择" valuePropName="checked"><Switch aria-label="允许在新配置中选择" /></Form.Item>
        </div>
        <Divider orientation="left" plain>平台身份<span className="ant-divider-inner-text-desc">与代码托管平台强绑定;系统资产与 IM 层永不依赖此区</span></Divider>
        <Form.List name="identities">{(fields, { add, remove }) => <div className="people-identity-editor">
          {fields.map((field, index) => <div className="people-identity-row" key={field.key}>
            <Form.Item name={[field.name, 'provider']} label="平台" rules={[{ required: true, message: '请选择平台' }]}><Select options={[{ value: 'gitlab', label: 'GitLab' }, { value: 'gitea', label: 'Gitea' }, { value: 'github', label: 'GitHub' }]} /></Form.Item>
            <Form.Item name={[field.name, 'base_url']} label="平台地址" rules={[{ required: true, message: '请输入平台地址' }, { type: 'url', message: '请输入有效的 http 或 https 地址' }]}><Input placeholder="https://gitlab.example.com" /></Form.Item>
            <Form.Item name={[field.name, 'username']} label="用户名"><Input placeholder="查询后自动回填" /></Form.Item>
            <Form.Item name={[field.name, 'external_user_id']} label="用户 ID"><Input placeholder="可暂不填写" /></Form.Item>
            <div className="people-identity-actions">
              <Form.Item noStyle shouldUpdate={(previous, current) => previous.identities?.[index]?.provider !== current.identities?.[index]?.provider}>{({ getFieldValue }) => <Button disabled={getFieldValue(['identities', index, 'provider']) !== 'gitlab'} onClick={() => setLookupIndex(index)}>从 GitLab 查询</Button>}</Form.Item>
              <Button type="text" danger aria-label={`删除身份 ${index + 1}`} icon={<DeleteOutlined />} onClick={() => remove(field.name)} />
            </div>
          </div>)}
          <Button icon={<PlusOutlined />} onClick={() => add({ provider: 'gitlab', base_url: '', external_user_id: '', username: '' })}>添加 CodeHost 身份</Button>
        </div>}</Form.List>
      </Form>
    </Modal>
    <Modal title="从 GitLab 查询身份" open={lookupIndex !== undefined} zIndex={1200} maskClosable={false} onCancel={closeLookup} footer={[
      <Button key="cancel" onClick={closeLookup}>取消</Button>,
      <Button key="query" loading={lookupLoading} disabled={!lookupToken} onClick={queryIdentity}>查询身份</Button>,
      <Button key="confirm" type="primary" disabled={lookupLoading || selectedCandidate === undefined} onClick={confirmCandidate}>确认回填</Button>,
    ]}>
      <p className="form-note">将根据人员邮箱查询 GitLab 用户。Token 仅用于本次请求，不会保存。</p>
      <label className="people-lookup-field">临时 GitLab Token<Input.Password aria-label="临时 GitLab Token" autoComplete="new-password" value={lookupToken} onChange={(event) => setLookupToken(event.target.value)} /></label>
      {candidates.length > 0 ? <Radio.Group className="people-candidates" value={selectedCandidate} onChange={(event) => setSelectedCandidate(event.target.value)}>{candidates.map((candidate, index) => <Radio key={`${candidate.external_user_id}:${candidate.username}`} value={index}><strong>{candidate.username}</strong><span>ID {candidate.external_user_id}{candidate.display_name ? ` / ${candidate.display_name}` : ''}</span></Radio>)}</Radio.Group> : null}
    </Modal>
  </>
}
