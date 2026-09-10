import { useEffect, useState } from 'react'
import type { FormInstance } from 'antd'
import {
  Alert,
  Button,
  Checkbox,
  Divider,
  Form,
  Input,
  message,
  Select,
  Steps,
  Tag,
  Typography,
} from 'antd'

import { CheckCircleOutlined, PlusOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'

import { adminApi } from '../api/client'
import type {
  ConfigurationValidationResult,
  Person,
  TenantConfigurationUpdateRequest,
  TenantConfigurationView,
} from '../api/types'
import { ErrorState, LoadingState } from '../components/PageState'
import { PageHeading } from '../components/PageHeading'
import {
  buildRuntimePeopleFields,
  loadAllPeople,
  personCodeHostIdentity,
  personLabel,
  resolveTenantConfigurationPeople,
  type ConfigurationPeopleSelection,
} from './configurationPeople'

type OrgConfigSecrets = NonNullable<TenantConfigurationUpdateRequest['secrets']>
type OrgConfigFormValues = TenantConfigurationUpdateRequest['config'] & {
  people: ConfigurationPeopleSelection
  secrets: OrgConfigSecrets
}

const stepItems = [
  { title: '代码仓库' },
  { title: '通知' },
  { title: '保存启用' },
]

// OrgConfigurationPage 是组织管理员的组织段配置向导：代码仓库默认值、
// 接续开发白名单、IM 通知与固定提醒人。保存即热生效，无需重启。
export function OrgConfigurationPage() {
  const navigate = useNavigate()
  const [form] = Form.useForm<OrgConfigFormValues>()
  const [view, setView] = useState<TenantConfigurationView>()
  const [people, setPeople] = useState<Person[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<unknown>()
  const [step, setStep] = useState(0)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    Promise.all([adminApi.tenantConfiguration(), loadAllPeople()])
      .then(([nextView, allPeople]) => {
        setView(nextView)
        setPeople(allPeople)
        form.setFieldsValue(toFormValues(nextView, allPeople))
      })
      .catch(setError)
      .finally(() => setLoading(false))
  }, [form])

  const save = async () => {
    setSaving(true)
    setFieldErrors({})
    try {
      const values = await form.validateFields()
      const secrets = values.secrets || {}
      const clearWithNewValue =
        (secrets.clear_telegram_bot_token && secrets.telegram_bot_token) ||
        (secrets.clear_feishu_secret && secrets.feishu_secret)
      if (clearWithNewValue) {
        message.error('「明确清除」与新值不能同时提交：请取消勾选或清空新值')
        return
      }
      const request = toUpdateRequest(values, people, view?.revision ?? 0)
      const validation: ConfigurationValidationResult = await adminApi.validateTenantConfiguration(request)
      if (!validation.valid) {
        setFieldErrors(validation.field_errors)
        message.error('配置仍有未通过的字段')
        return
      }
      const updated = await adminApi.updateTenantConfiguration(request)
      setView(updated)
      // 密钥子树整体重置：已勾选的「明确清除」与残留输入若不重置，会在
      // 下一次保存被深合并保留，导致新密钥被 clear 优先级静默吞掉。
      form.resetFields([['secrets']])
      form.setFieldsValue(toFormValues(updated, people))
      message.success('组织配置已保存并生效')
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '保存组织配置失败')
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <LoadingState />
  if (error) return <ErrorState error={error} />
  if (!view) return <ErrorState error={new Error('组织配置状态不完整')} />

  return <div className="org-configuration-page">
    <PageHeading
      eyebrow="组织管理"
      title="组织配置"
      action={<Button onClick={() => navigate('/org')}>返回组织工作台</Button>}
    />

    <section className="surface-panel configuration-shell">
      <div className="configuration-workbench">
        <Steps current={step} items={stepItems} size="small" responsive={false} labelPlacement="vertical" />
        <Divider />
        <Form form={form} layout="vertical" requiredMark="optional" className="configuration-form">
          <div hidden={step !== 0}><CodeHostStep people={people} onManagePeople={() => navigate('/people', { state: { returnTo: '/org/settings' } })} /></div>
          <div hidden={step !== 1}><NotificationStep secrets={view.secrets} people={people} /></div>
          <div hidden={step !== 2}><SummaryStep form={form} fieldErrors={fieldErrors} /></div>
        </Form>
        <div className="configuration-actions">
          <Button disabled={step === 0 || saving} onClick={() => setStep((current) => current - 1)}>上一步</Button>
          {step < stepItems.length - 1
            ? <Button type="primary" onClick={() => setStep((current) => current + 1)}>下一步</Button>
            : <Button type="primary" icon={<CheckCircleOutlined />} loading={saving} onClick={save}>保存并生效</Button>}
        </div>
      </div>
    </section>
  </div>
}

function CodeHostStep({ people, onManagePeople }: { people: Person[]; onManagePeople: () => void }) {
  const provider = Form.useWatch(['codehost', 'provider']) || 'gitlab'
  const baseURL = Form.useWatch(['codehost', 'base_url']) || ''

  const selectedRework: string[] = Form.useWatch(['people', 'rework_person_ids']) || []
  const codeHostOptions = people.filter((person) => {
    const identity = personCodeHostIdentity(person, provider, baseURL)
    return identity && Number.isInteger(Number(identity.external_user_id)) && Number(identity.external_user_id) > 0 && (person.enabled || selectedRework.includes(person.person_id))
  }).map((person) => ({ value: person.person_id, label: personLabel(person, provider, baseURL), disabled: !person.enabled }))
  return <div>
    <div className="configuration-form-grid">
      <Form.Item name={['codehost', 'provider']} label="代码托管平台" rules={[{ required: true }]}><Select aria-label="代码托管平台" options={[{ value: 'gitlab', label: 'GitLab' }, { value: 'gitea', label: 'Gitea' }, { value: 'github', label: 'GitHub' }]} /></Form.Item>
      <Form.Item name={['codehost', 'base_url']} label="平台地址" rules={[{ required: true }, { validator: validateHTTPURL }]}><Input placeholder="https://gitlab.example.com" /></Form.Item>
      <p className="form-note">此处的平台与地址作为新增仓库表单的默认值；每个仓库可在仓库管理中单独指定平台与地址。</p>
      <Form.Item name={['people', 'rework_person_ids']} label="额外允许接续开发的人员" extra="从人员资产选择；MR 作者、负责人和 Reviewer 无需重复填写。"><Select mode="multiple" options={codeHostOptions} placeholder="选择人员" /></Form.Item>
    </div>
    <div className="configuration-inline-title"><Typography.Text type="secondary">额外允许人员按平台身份解析;无匹配时接续开发授权退化为作者 ∪ 白名单。</Typography.Text><Button icon={<PlusOutlined />} onClick={onManagePeople}>管理人员资产</Button></div>
    <LegacyIDValues />
  </div>
}

function NotificationStep({ secrets, people }: { secrets: TenantConfigurationView['secrets']; people: Person[] }) {
  const selected: string[] = Form.useWatch(['people', 'reviewer_person_ids']) || []
  const options = people.filter((person) => person.enabled || selected.includes(person.person_id)).map((person) => ({ value: person.person_id, label: personLabel(person), disabled: !person.enabled }))
  const provider = Form.useWatch(['im', 'provider']) || ''
  return <div className="configuration-form-grid">
    <Form.Item name={['im', 'provider']} label="IM 通知渠道" extra="不配置则跳过所有 IM 通知。">
      <Select aria-label="IM 通知渠道" allowClear options={[{ value: 'telegram', label: 'Telegram' }, { value: 'feishu', label: '飞书' }]} placeholder="选择通知渠道" />
    </Form.Item>
    {provider === 'telegram' ? <>
      <SecretField name="telegram_bot_token" clearName="clear_telegram_bot_token" label="Telegram Bot Token" configured={secrets.telegram_bot_token_configured} />
      <Form.Item name={['im', 'telegram', 'default_chat_id']} label="默认 Chat ID" extra="支持负数的群组 ID。"><Input className="mono" placeholder="-100123456789" /></Form.Item>
      <Form.Item name={['im', 'telegram', 'api_base']} label="API 地址（可选）" extra="自托管 bot-api server 时填写。"><Input className="mono" placeholder="https://api.telegram.org" /></Form.Item>
    </> : null}
    {provider === 'feishu' ? <>
      <Form.Item name={['im', 'feishu', 'webhook_url']} label="飞书群机器人 Webhook" rules={[{ validator: validateHTTPURL }]}><Input className="mono" placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..." /></Form.Item>
      <SecretField name="feishu_secret" clearName="clear_feishu_secret" label="签名 Secret（可选）" configured={secrets.feishu_secret_configured} optional />
    </> : null}
    <Form.Item name={['people', 'reviewer_person_ids']} label="审查完成后固定提醒人" extra="从人员资产选择;保存为人员显示名,供 IM 提及渲染。"><Select mode="multiple" options={options} placeholder="选择提醒人" /></Form.Item>
    <LegacyReviewerValues />
  </div>
}

function LegacyIDValues() {
  return <Form.List name={['people', 'legacy_rework_user_ids']}>{(fields, { remove }) => fields.length ? <div className="configuration-legacy-values"><span>历史未关联接续开发 ID（默认保留）</span>{fields.map((field, index) => <Form.Item noStyle shouldUpdate key={field.key}>{({ getFieldValue }) => { const value = getFieldValue(['people', 'legacy_rework_user_ids', index]); return <Tag closable aria-label={`移除历史接续开发 ID ${value}`} onClose={() => remove(field.name)}>{value}</Tag> }}</Form.Item>)}</div> : null}</Form.List>
}

function LegacyReviewerValues() {
  return <Form.List name={['people', 'legacy_reviewer_mentions']}>{(fields, { remove }) => fields.length ? <div className="configuration-legacy-values"><span>历史未关联提醒人（默认保留）</span>{fields.map((field, index) => <Form.Item noStyle shouldUpdate key={field.key}>{({ getFieldValue }) => { const value = getFieldValue(['people', 'legacy_reviewer_mentions', index]); return <Tag closable aria-label={`移除历史提醒人 ${value}`} onClose={() => remove(field.name)}>{value}</Tag> }}</Form.Item>)}</div> : null}</Form.List>
}

function SummaryStep({ form, fieldErrors }: { form: FormInstance<OrgConfigFormValues>; fieldErrors: Record<string, string> }) {
  const values = Form.useWatch([], form) as OrgConfigFormValues | undefined
  return <div className="configuration-summary">
    <h2>保存组织配置并即时生效</h2>
    <p>配置原子写入本组织的租户库，保存后立即生效，不需要重启服务，也不影响其他组织。</p>
    <div className="configuration-summary-grid">
      <div><span>代码仓库</span><strong>{values?.codehost?.base_url || '未填写'}</strong></div>
      <div><span>IM 通知</span><strong>{values?.im?.provider ? values.im.provider : '未配置'}</strong></div>
    </div>
    {Object.keys(fieldErrors).length > 0 ? <Alert type="error" showIcon message="以下字段未通过校验" description={<ul>{Object.entries(fieldErrors).map(([field, detail]) => <li key={field}><code>{field}</code>：{detail}</li>)}</ul>} /> : null}
  </div>
}

type SecretFieldName = string | (string | number)[]
function SecretField({ name, clearName, label, configured, optional = false }: { name: SecretFieldName; clearName: SecretFieldName; label: string; configured: boolean; optional?: boolean }) {
  return <div className="configuration-secret-field">
    <div className="configuration-secret-label"><span>{label}{optional ? '（可选）' : ''}</span>{configured ? <Tag color="success">已配置</Tag> : <Tag>未配置</Tag>}</div>
    <Form.Item name={typeof name === 'string' ? ['secrets', name] : name}><Input.Password autoComplete="new-password" placeholder={configured ? '留空保留当前值；输入新值进行替换' : '输入新值'} /></Form.Item>
    <Form.Item name={typeof clearName === 'string' ? ['secrets', clearName] : clearName} valuePropName="checked"><Checkbox>明确清除当前值</Checkbox></Form.Item>
  </div>
}

function toFormValues(view: TenantConfigurationView, people: Person[]): OrgConfigFormValues {
  return {
    codehost: { ...view.config.codehost, rework_allowed_user_ids: view.config.codehost.rework_allowed_user_ids || [] },
    im: { ...view.config.im, reviewer_mentions: view.config.im.reviewer_mentions || [] },
    people: resolveTenantConfigurationPeople(view, people),
    secrets: {},
  }
}

function toUpdateRequest(values: OrgConfigFormValues, people: Person[], expectedRevision: number): TenantConfigurationUpdateRequest {
  const resolved = buildRuntimePeopleFields(values.people, people, values.codehost.provider, values.codehost.base_url)
  return {
    config: {
      codehost: {
        ...values.codehost,
        rework_allowed_user_ids: resolved.rework_allowed_user_ids,
      },
      im: { ...values.im, reviewer_mentions: resolved.reviewer_mentions },
    },
    secrets: values.secrets || {},
    expected_revision: expectedRevision,
  }
}

function validateHTTPURL(_: unknown, value?: string) {
  if (!value) return Promise.resolve()
  try {
    const parsed = new URL(value)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
      ? Promise.resolve()
      : Promise.reject(new Error('仅支持 http 或 https 地址'))
  } catch {
    return Promise.reject(new Error('请输入完整的 http 或 https 地址'))
  }
}
