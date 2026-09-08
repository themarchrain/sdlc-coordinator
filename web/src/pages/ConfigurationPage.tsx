import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Checkbox,
  Divider,
  Form,
  Input,
  InputNumber,
  message,
  Select,
  Space,
  Steps,
  Switch,
  Tag,
  Typography,
} from 'antd'
import type { FormInstance } from 'antd'

type SecretFieldName = string | (string | number)[]
import { CheckCircleOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons'
import { useLocation, useNavigate } from 'react-router-dom'

import { adminApi } from '../api/client'
import { APP_VERSION } from '../app/version'
import type {
  ConfigurationSecretChanges,
  ConfigurationStatus,
  ConfigurationUpdateRequest,
  ConfigurationView,
  EditableRuntimeConfiguration,
  Person,
} from '../api/types'
import { ErrorState, LoadingState } from '../components/PageState'
import { PageHeading } from '../components/PageHeading'
import { AppearancePanel } from '../components/AppearancePanel'
import { buildRuntimePeopleFields, loadAllPeople, personCodeHostIdentity, personLabel, resolveConfigurationPeople, type ConfigurationPeopleSelection } from './configurationPeople'

type ConfigurationFormValues = Omit<EditableRuntimeConfiguration, 'agent_runner'> & {
  agent_runner: Omit<EditableRuntimeConfiguration['agent_runner'], 'task_timeout'> & {
    task_timeout_minutes: number
  }
  people: ConfigurationPeopleSelection
  secrets: ConfigurationSecretChanges
}

const stepItems = [
  { title: '运行边界' },
  { title: 'Agent Runner' },
  { title: '代码仓库' },
  { title: '通知' },
  { title: '调度' },
  { title: '保存启用' },
]

export function ConfigurationPage() {
  const location = useLocation()
  const navigate = useNavigate()
  const [form] = Form.useForm<ConfigurationFormValues>()
  const [status, setStatus] = useState<ConfigurationStatus>()
  const [view, setView] = useState<ConfigurationView>()
  const [people, setPeople] = useState<Person[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<unknown>()
  const [step, setStep] = useState(0)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)
  const [restarting, setRestarting] = useState(false)
  const isSetup = location.pathname === '/setup'

  useEffect(() => {
    Promise.all([adminApi.configurationStatus(), adminApi.configuration(), loadAllPeople()])
      .then(([nextStatus, nextView, allPeople]) => {
        setStatus(nextStatus)
        setView(nextView)
        setPeople(allPeople)
        form.setFieldsValue(toFormValues(nextView, allPeople))
      })
      .catch(setError)
      .finally(() => setLoading(false))
  }, [form])

  useEffect(() => {
    if (!restarting) return
    const timer = window.setInterval(() => {
      adminApi.configurationStatus().then((nextStatus) => {
        setStatus(nextStatus)
        if (nextStatus.mode === 'ready') {
          window.clearInterval(timer)
          setRestarting(false)
          message.success('Coordinator 已按新配置恢复运行')
          navigate('/', { replace: true })
        }
      }).catch(() => undefined)
    }, 1200)
    return () => window.clearInterval(timer)
  }, [navigate, restarting])

  const configuredCount = useMemo(() => view ? Object.values(view.secrets).filter(Boolean).length : 0, [view])
  const secretTotal = useMemo(() => view ? Object.values(view.secrets).length : 0, [view])

  const saveAndRestart = async () => {
    setSaving(true)
    setFieldErrors({})
    try {
      const values = await form.validateFields()
      const request = toUpdateRequest(values, people)
      const validation = await adminApi.validateConfiguration(request)
      if (!validation.valid) {
        setFieldErrors(validation.field_errors)
        message.error('配置仍有未通过的字段')
        return
      }
      const updated = await adminApi.updateConfiguration(request)
      setStatus(updated)
      setView(updated.configuration)
      await adminApi.restartConfiguration()
      setRestarting(true)
      message.success('配置已保存，Coordinator 正在重启')
    } catch (reason) {
      message.error(reason instanceof Error ? reason.message : '保存配置失败')
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <LoadingState />
  if (error) return <ErrorState error={error} />
  if (!status || !view) return <ErrorState error={new Error('配置状态不完整')} />

  return <div className="configuration-page">
    <PageHeading
      eyebrow={isSetup ? '首次启动' : '运行配置'}
      title={isSetup ? '初始化 Coordinator' : '系统配置'}
    />

    {status.mode === 'configuration_error' ? <Alert className="configuration-alert" type="error" showIcon message="已保存配置无法装配" description={status.configuration_error || '请修正配置并重新启用。'} /> : null}
    {restarting ? <Alert className="configuration-alert" type="info" showIcon icon={<ReloadOutlined spin />} message="服务正在重启" description="页面会持续检查服务状态。Docker 部署会自动拉起；本地 go run 需要手工重新启动。" /> : null}

    <AppearancePanel />

    <section className="surface-panel configuration-shell">
      <div className="configuration-status-rail">
        <div><span>当前模式</span><Tag color={status.mode === 'ready' ? 'success' : status.mode === 'configuration_error' ? 'error' : 'processing'}>{modeLabel(status.mode)}</Tag></div>
        <div><span>系统版本</span><strong>{APP_VERSION}</strong></div>
        <div><span>配置版本</span><strong>rev {status.revision}</strong></div>
        <div><span>凭证状态</span><strong>{configuredCount} / {secretTotal} 已配置</strong></div>
        <p>配置未完成时，Scheduler、Webhook 和 Agent Runner 均不会推进。</p>
      </div>

      <div className="configuration-workbench">
        <Steps current={step} items={stepItems} size="small" responsive={false} labelPlacement="vertical" />
        <Divider />
        <Form form={form} layout="vertical" requiredMark="optional" className="configuration-form">
          <div hidden={step !== 0}><RuntimeBoundaryStep status={status} /></div>
          <div hidden={step !== 1}><AgentRunnerStep secrets={view.secrets} /></div>
          <div hidden={step !== 2}><CodeHostStep secrets={view.secrets} people={people} onManagePeople={() => navigate('/people', { state: { returnTo: location.pathname } })} /></div>
          <div hidden={step !== 3}><NotificationStep secrets={view.secrets} people={people} /></div>
          <div hidden={step !== 4}><RuntimeSettingsStep /></div>
          <div hidden={step !== 5}><SummaryStep form={form} fieldErrors={fieldErrors} /></div>
        </Form>

        <div className="configuration-actions">
          <Button disabled={step === 0 || saving || restarting} onClick={() => setStep((current) => current - 1)}>上一步</Button>
          {step < stepItems.length - 1
            ? <Button type="primary" onClick={() => setStep((current) => current + 1)}>下一步</Button>
            : <Button type="primary" icon={<CheckCircleOutlined />} loading={saving || restarting} onClick={saveAndRestart}>保存并启用</Button>}
        </div>
      </div>
    </section>
  </div>
}

function RuntimeBoundaryStep({ status }: { status: ConfigurationStatus }) {
  const initialSetup = status.mode === 'setup_pending' && status.revision === 0
  const importedLegacy = status.source === 'env_import'
  const stateTitle = initialSetup
    ? '当前尚未完成初始化'
    : status.mode === 'configuration_error'
      ? '当前配置未能正常启动'
      : importedLegacy
        ? '已导入旧配置，等待你确认'
        : '准备更新当前运行配置'
  const stateDescription = initialSetup
    ? '自动化能力尚未启动。完成本向导并保存后，Coordinator 才会开始按你的配置运行。'
    : status.mode === 'configuration_error'
      ? '请检查并修正下面的配置，然后重新保存启用。'
      : '已有配置和历史数据会保留；最后保存时会以本次填写内容作为新的完整运行配置。'

  return <div className="configuration-step-copy">
    <h2>{stateTitle}</h2>
    <p>{stateDescription}</p>
    {initialSetup
      ? <Alert
          className="configuration-init-status"
          type="info"
          showIcon
          message="系统尚未初始化，请完成以下配置后启用。"
          description="请依次完成 Agent Runner、代码仓库、通知和调度设置。最后一步点击“保存并启用”后，系统会保存完整配置并自动重启；重启完成后自动化能力才会开始运行。"
        />
      : <Alert
          className="configuration-init-status"
          type={status.mode === 'configuration_error' ? 'error' : 'info'}
          showIcon
          message="系统已初始化，点击“下一步”开始更新配置，手动确认后，将重启并应用。"
          description="请依次完成 Agent Runner、代码仓库、通知和调度设置。最后一步点击“保存并启用”后，系统会用本次填写的完整配置覆盖当前运行配置，并自动重启；重启完成后新配置才会生效。"
        />}
    {importedLegacy ? <Alert type="success" showIcon message="旧环境变量已完成一次性导入；请确认后保存，后续以 SQLite 配置为准。" /> : null}
  </div>
}

function AgentRunnerStep({ secrets }: { secrets: ConfigurationView['secrets'] }) {
  const provider = Form.useWatch(['agent_runner', 'provider']) || 'codex-cli'
  return <div>
    <Alert
      className="configuration-alert"
      type="info"
      showIcon
      message="Reviewer 与 Developer 通过独立 Runner 执行"
      description="两个容器在同一 Docker 网络和共享工作区内通信。这里填写内部地址，不需要开放 Runner 的宿主机端口。"
    />
    <div className="configuration-form-grid">
      <Form.Item name={['agent_runner', 'service_url']} label="Runner 服务地址" rules={[{ required: true }, { validator: validateHTTPURL }]} extra="同一 Compose 网络内默认使用 http://agent-runner:8081。"><Input placeholder="http://agent-runner:8081" /></Form.Item>
      <Form.Item name={['agent_runner', 'workspace_root']} label="共享工作区" rules={[{ required: true }]} extra="Coordinator 与 Runner 容器内必须映射到同一路径。"><Input className="mono" placeholder="/app/workspaces" /></Form.Item>
      <Form.Item name={['agent_runner', 'provider']} label="执行 Provider" rules={[{ required: true }]}><Select options={[{ value: 'codex-cli', label: 'Codex CLI' }, { value: 'claude-cli', label: 'Claude Code CLI' }]} /></Form.Item>
      <div className="configuration-provider-note">{provider === 'codex-cli' ? '裸 Agent，不加载个人 Codex 配置、MCP、skills 或历史会话。协议要求：接入地址必须兼容 OpenAI Responses API。' : '裸 Agent，不加载个人 Claude 配置、MCP、skills 或历史会话。协议要求：接入地址必须兼容 Anthropic Messages API。'}</div>
      <Form.Item name={['agent_runner', 'provider_base_url']} label="模型接入地址" rules={[{ validator: validateHTTPURL }]} extra="由使用者自行提供，且协议必须兼容所选 CLI：codex-cli 要求 Responses API，claude-cli 要求 Messages API。Coordinator 只做透传，不做协议适配。"><Input placeholder="https://llm.example.com/v1" /></Form.Item>
      <Form.Item name={['agent_runner', 'model']} label="执行模型" rules={[{ required: true }]}><Input className="mono" placeholder="模型 ID" /></Form.Item>
      <Form.Item name={['agent_runner', 'effort']} label="推理强度" rules={[{ required: true }]}><Select options={['low', 'medium', 'high', 'xhigh', 'max'].map((value) => ({ value, label: value }))} /></Form.Item>
      <Form.Item name={['agent_runner', 'task_timeout_minutes']} label="单任务超时（分钟）" rules={[{ required: true }]} extra="默认 30 分钟；超时后 Runner 会终止当前 Agent 进程，本次 Run 按失败策略处理。"><InputNumber min={1} max={240} precision={0} /></Form.Item>
      <SecretField name="agent_runner_api_key" clearName="clear_agent_runner_api_key" label="模型 API Key" configured={secrets.agent_runner_api_key_configured} />
      <SecretField name="agent_runner_custom_headers_json" clearName="clear_agent_runner_custom_headers_json" label="自定义 Header JSON" configured={secrets.agent_runner_custom_headers_configured} optional />
    </div>
  </div>
}

function CodeHostStep({ secrets, people, onManagePeople }: { secrets: ConfigurationView['secrets']; people: Person[]; onManagePeople: () => void }) {
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

function NotificationStep({ secrets, people }: { secrets: ConfigurationView['secrets']; people: Person[] }) {
  const selected: string[] = Form.useWatch(['people', 'reviewer_person_ids']) || []
  const options = people.filter((person) => person.enabled || selected.includes(person.person_id)).map((person) => ({ value: person.person_id, label: personLabel(person), disabled: !person.enabled }))
  const provider = Form.useWatch(['im', 'provider']) || ''
  return <div className="configuration-form-grid">
    <Form.Item name={['im', 'provider']} label="IM 通知渠道" extra="不配置则跳过所有 IM 通知。">
      <Select aria-label="IM 通知渠道" allowClear options={[{ value: 'telegram', label: 'Telegram' }, { value: 'feishu', label: '飞书' }]} placeholder="选择通知渠道" />
    </Form.Item>
    {provider === 'telegram' ? <>
      <SecretField name={['secrets', 'telegram_bot_token']} clearName="clear_telegram_bot_token" label="Telegram Bot Token" configured={secrets.telegram_bot_token_configured} />
      <Form.Item name={['im', 'telegram', 'default_chat_id']} label="默认 Chat ID" extra="支持负数的群组 ID。"><Input className="mono" placeholder="-100123456789" /></Form.Item>
      <Form.Item name={['im', 'telegram', 'api_base']} label="API 地址（可选）" extra="自托管 bot-api server 时填写。"><Input className="mono" placeholder="https://api.telegram.org" /></Form.Item>
    </> : null}
    {provider === 'feishu' ? <>
      <Form.Item name={['im', 'feishu', 'webhook_url']} label="飞书群机器人 Webhook" rules={[{ validator: validateHTTPURL }]}><Input className="mono" placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..." /></Form.Item>
      <SecretField name={['secrets', 'feishu_secret']} clearName="clear_feishu_secret" label="签名 Secret（可选）" configured={secrets.feishu_secret_configured} />
    </> : null}
    <Form.Item name={['people', 'reviewer_person_ids']} label="审查完成后固定提醒人" extra="从人员资产选择;保存为人员显示名,供 IM 提及渲染。"><Select mode="multiple" options={options} placeholder="选择提醒人" /></Form.Item>
    <LegacyReviewerValues />
  </div>
}

function LegacyIDValues() {
  return <Form.List name={['people', 'legacy_rework_user_ids']}>{(fields, { remove }) => fields.length ? <div className="configuration-legacy-values"><span>历史未关联接续开发 ID（默认保留）</span>{fields.map((field, index) => <Form.Item noStyle shouldUpdate key={field.key}>{({ getFieldValue }) => { const value = getFieldValue(['people', 'legacy_rework_user_ids', index]); return <Tag closable aria-label={`移除历史接续开发 ID ${value}`} onClose={() => remove(field.name)}>{value}</Tag> }}</Form.Item>)}</div> : null}</Form.List>
}

function LegacyReviewerValues() {
  return <Form.List name={['people', 'legacy_reviewer_mentions']}>{(fields, { remove }) => fields.length ? <div className="configuration-legacy-values"><span>历史未关联提醒人（默认保留）</span>{fields.map((field, index) => <Form.Item noStyle shouldUpdate key={field.key}>{({ getFieldValue }) => { const value = getFieldValue(['people', 'legacy_reviewer_mentions', index]); return <Tag closable aria-label={`移除历史提醒邮箱 ${value}`} onClose={() => remove(field.name)}>{value}</Tag> }}</Form.Item>)}</div> : null}</Form.List>
}

function RuntimeSettingsStep() {
  const limits = [
    ['dispatch_limit', '派发'], ['poll_limit', '轮询'], ['reconciliation_limit', '写回对账'],
    ['finalization_limit', '结果完成'], ['recovery_limit', '租约恢复'], ['effect_limit', '通知执行'],
  ] as const
  return <div>
    <div className="configuration-inline-title"><div><h3>Scheduler</h3></div><Form.Item name={['scheduler', 'enabled']} valuePropName="checked"><Switch checkedChildren="启用" unCheckedChildren="停用" /></Form.Item></div>
    <div className="configuration-form-grid">
      <Form.Item name={['scheduler', 'interval']} label="执行间隔" rules={[{ required: true }]}><Input className="mono" placeholder="30s" /></Form.Item>
      <Form.Item name={['scheduler', 'callback_base_url']} label="回调基地址" rules={[{ required: true }, { validator: validateHTTPURL }]} extra="代码托管平台与 Agent Runner 回访 coordinator 使用的对外地址。"><Input className="mono" placeholder="http://127.0.0.1:8080" /></Form.Item>
      <Form.Item name={['scheduler', 'workspace_root']} label="工作区根目录" rules={[{ required: true }]} extra="通常与 Agent Runner 的共享工作区保持一致。"><Input className="mono" placeholder="D:/workspaces" /></Form.Item>
    </div>
    <div className="configuration-limit-grid">{limits.map(([name, label]) => <Form.Item key={name} name={['scheduler', name]} label={`${label}上限`} rules={[{ required: true }]}><InputNumber min={0} precision={0} /></Form.Item>)}</div>
  </div>
}

function SummaryStep({ form, fieldErrors }: { form: FormInstance<ConfigurationFormValues>; fieldErrors: Record<string, string> }) {
  const values = Form.useWatch([], form) as ConfigurationFormValues | undefined
  return <div className="configuration-summary">
    <h2>保存完整配置并重启</h2>
    <p>新版本会原子写入 SQLite。当前进程不会热替换已有 Client；收到重启请求后先返回响应，再执行优雅关闭。</p>
    <div className="configuration-summary-grid">
      <div><span>Agent Runner</span><strong>{values?.agent_runner?.model ? `${values.agent_runner.model} / ${values.agent_runner.service_url}` : '未填写'}</strong></div>
      <div><span>代码仓库</span><strong>{values?.codehost?.base_url || '未填写'}</strong></div>
      <div><span>Scheduler</span><strong>{values?.scheduler?.enabled ? `每 ${values.scheduler.interval}` : '停用'}</strong></div>
    </div>
    {Object.keys(fieldErrors).length > 0 ? <Alert type="error" showIcon message="以下字段未通过校验" description={<ul>{Object.entries(fieldErrors).map(([field, detail]) => <li key={field}><code>{field}</code>：{detail}</li>)}</ul>} /> : null}
  </div>
}

function SecretField({ name, clearName, label, configured, optional = false }: { name: SecretFieldName; clearName: SecretFieldName; label: string; configured: boolean; optional?: boolean }) {
  return <div className="configuration-secret-field">
    <div className="configuration-secret-label"><span>{label}{optional ? '（可选）' : ''}</span>{configured ? <Tag color="success">已配置</Tag> : <Tag>未配置</Tag>}</div>
    <Form.Item name={typeof name === 'string' ? ['secrets', name] : name}><Input.Password autoComplete="new-password" placeholder={configured ? '留空保留当前值；输入新值进行替换' : '输入新值'} /></Form.Item>
    <Form.Item name={typeof clearName === 'string' ? ['secrets', clearName] : clearName} valuePropName="checked"><Checkbox>明确清除当前值</Checkbox></Form.Item>
  </div>
}

function toFormValues(view: ConfigurationView, people: Person[]): ConfigurationFormValues {
  const { task_timeout, ...agentRunner } = view.config.agent_runner
  return {
    ...view.config,
    agent_runner: {
      ...agentRunner,
      task_timeout_minutes: durationMinutes(task_timeout),
    },
    codehost: { ...view.config.codehost, rework_allowed_user_ids: view.config.codehost.rework_allowed_user_ids || [] },
    im: { ...view.config.im, reviewer_mentions: view.config.im.reviewer_mentions || [] },
    people: resolveConfigurationPeople(view, people),
    secrets: {},
  }
}

function toUpdateRequest(values: ConfigurationFormValues, people: Person[]): ConfigurationUpdateRequest {
  const resolved = buildRuntimePeopleFields(values.people, people, values.codehost.provider, values.codehost.base_url)
  const { task_timeout_minutes, ...agentRunner } = values.agent_runner
  return {
    config: {
      agent_runner: {
        ...agentRunner,
        task_timeout: `${task_timeout_minutes}m`,
      },
      codehost: {
        ...values.codehost,
        rework_allowed_user_ids: resolved.rework_allowed_user_ids,
      },
      im: { ...values.im, reviewer_mentions: resolved.reviewer_mentions },
      scheduler: values.scheduler,
    },
    secrets: values.secrets || {},
  }
}

function durationMinutes(raw: string): number {
  const value = raw.trim()
  const match = /^(\d+(?:\.\d+)?)(ms|s|m|h)$/.exec(value)
  if (!match) return 30
  const amount = Number(match[1])
  switch (match[2]) {
    case 'ms': return Math.max(1, Math.round(amount / 60000))
    case 's': return Math.max(1, Math.round(amount / 60))
    case 'h': return Math.round(amount * 60)
    default: return Math.round(amount)
  }
}

function modeLabel(mode: ConfigurationStatus['mode']) {
  return ({ setup_pending: '等待初始化', ready: '运行中', restart_required: '等待重启', configuration_error: '配置错误' } as const)[mode]
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
