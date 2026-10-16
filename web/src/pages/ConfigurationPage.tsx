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
  Steps,
  Switch,
  Tag,
} from 'antd'
import type { FormInstance } from 'antd'

type SecretFieldName = string | (string | number)[]
import { CheckCircleOutlined, ReloadOutlined } from '@ant-design/icons'
import { useLocation, useNavigate } from 'react-router-dom'

import { AdminApiError, adminApi } from '../api/client'
import { APP_VERSION } from '../app/version'
import type {
  ConfigurationSecretChanges,
  ConfigurationStatus,
  ConfigurationUpdateRequest,
  ConfigurationView,
  EditableRuntimeConfiguration,
} from '../api/types'
import { ErrorState, LoadingState } from '../components/PageState'
import { PageHeading } from '../components/PageHeading'
import { useConfigurationStatus } from '../app/configurationStatus'

type ConfigurationFormValues = Omit<EditableRuntimeConfiguration, 'agent_runner'> & {
  agent_runner: Omit<EditableRuntimeConfiguration['agent_runner'], 'task_timeout'> & {
    task_timeout_minutes: number
  }
  secrets: ConfigurationSecretChanges
}

const stepItems = [
  { title: '运行边界' },
  { title: 'Agent Runner' },
  { title: '调度' },
  { title: '保存启用' },
]

export function ConfigurationPage() {
  const location = useLocation()
  const navigate = useNavigate()
  const { sync } = useConfigurationStatus()
  const [form] = Form.useForm<ConfigurationFormValues>()
  const [status, setStatus] = useState<ConfigurationStatus>()
  const [view, setView] = useState<ConfigurationView>()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<unknown>()
  const [step, setStep] = useState(0)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)
  const [restarting, setRestarting] = useState(false)
  const isSetup = location.pathname === '/setup'

  useEffect(() => {
    Promise.all([adminApi.configurationStatus(), adminApi.configuration()])
      .then(([nextStatus, nextView]) => {
        setStatus(nextStatus)
        sync(nextStatus)
        setView(nextView)
        form.setFieldsValue(toFormValues(nextView))
      })
      .catch(setError)
      .finally(() => setLoading(false))
  }, [form, sync])

  useEffect(() => {
    if (!restarting) return
    const timer = window.setInterval(() => {
      adminApi.configurationStatus().then((nextStatus) => {
        setStatus(nextStatus)
        // 每次轮询都回写共享层：否则边界停留在旧模式（首次初始化时是
        // setup_pending），会把重启完成后的导航弹回 /setup。
        if (nextStatus.mode === 'ready') {
          window.clearInterval(timer)
          setRestarting(false)
          message.success('Coordinator 已按新配置恢复运行')
          // 向导（/setup）的落地由边界接管（送首页）：navigate 走
          // transition，会输给边界的紧急重定向。/settings 重新保存时
          // 边界的 ready 重定向不触发，沿用既有落地首页。
          sync(nextStatus, { fromRestart: true })
          if (!isSetup) navigate('/', { replace: true })
        } else {
          sync(nextStatus)
        }
      }).catch(() => undefined)
    }, 1200)
    return () => window.clearInterval(timer)
  }, [isSetup, navigate, restarting, sync])

  const configuredCount = useMemo(() => view ? Object.values(view.secrets).filter(Boolean).length : 0, [view])
  const secretTotal = useMemo(() => view ? Object.values(view.secrets).length : 0, [view])

  const saveAndRestart = async () => {
    setSaving(true)
    setFieldErrors({})
    try {
      const values = await form.validateFields()
      const request = toUpdateRequest(values, status?.revision ?? 0)
      const validation = await adminApi.validateConfiguration(request)
      if (!validation.valid) {
        setFieldErrors(validation.field_errors)
        message.error('配置仍有未通过的字段')
        return
      }
      const updated = await adminApi.updateConfiguration(request)
      setStatus(updated)
      sync(updated)
      setView(updated.configuration)
      await adminApi.restartConfiguration()
      setRestarting(true)
      message.success('配置已保存，Coordinator 正在重启')
    } catch (reason) {
      message.error(reason instanceof AdminApiError && reason.code === 'revision_conflict'
        ? '运行配置已被其他管理员更新，请刷新页面后重新提交'
        : reason instanceof Error ? reason.message : '保存配置失败')
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

    {/* 外观主题入口已迁至全局外壳顶栏（AppearanceDrawerButton），本页不再渲染主题块。 */}

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
          <div hidden={step !== 2}><RuntimeSettingsStep /></div>
          <div hidden={step !== 3}><SummaryStep form={form} fieldErrors={fieldErrors} /></div>
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
          description="请依次完成 Agent Runner 和调度设置。最后一步点击“保存并启用”后，系统会保存完整配置并自动重启；重启完成后自动化能力才会开始运行。组织级配置（代码仓库默认值、通知）由组织管理员在组织工作台维护。"
        />
      : <Alert
          className="configuration-init-status"
          type={status.mode === 'configuration_error' ? 'error' : 'info'}
          showIcon
          message="系统已初始化，点击“下一步”开始更新配置，手动确认后，将重启并应用。"
          description="请依次完成 Agent Runner 和调度设置。最后一步点击“保存并启用”后，系统会用本次填写的完整配置覆盖当前运行配置，并自动重启；重启完成后新配置才会生效。组织级配置（代码仓库默认值、通知）由组织管理员在组织工作台维护。"
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
      {/* 执行 Provider 不再提供 UI 编辑（切换属系统级变更，不进配置向导）；
          保留隐藏注册字段让保存 payload 原样回传加载值——后端整行替换语义下
          缺键会把系统默认 CLI 静默清空。 */}
      <Form.Item name={['agent_runner', 'provider']} hidden><Input className="mono" /></Form.Item>
      <div className="configuration-provider-note">{provider === 'codex-cli' ? '裸 Agent，不加载个人 Codex 配置、MCP、skills 或历史会话。协议要求：接入地址必须兼容 OpenAI Responses API。' : '裸 Agent，不加载个人 Claude 配置、MCP、skills 或历史会话。协议要求：接入地址必须兼容 Anthropic Messages API。'}</div>
      <Form.Item name={['agent_runner', 'provider_base_url']} label="模型接入地址" rules={[{ validator: validateHTTPURL }]} extra="由使用者自行提供，且协议必须兼容当前执行 Provider：codex-cli 要求 Responses API，claude-cli 要求 Messages API。Coordinator 只做透传，不做协议适配。"><Input placeholder="https://llm.example.com/v1" /></Form.Item>
      <Form.Item name={['agent_runner', 'model']} label="执行模型" rules={[{ required: true }]}><Input className="mono" placeholder="模型 ID" /></Form.Item>
      {/* 推理强度已下沉组织配置（组织可选、系统兜底）；保留隐藏注册字段让
          保存 payload 原样回传加载值——后端整行替换语义下缺键会把系统兜底
          值静默清空。 */}
      <Form.Item name={['agent_runner', 'effort']} hidden><Input className="mono" /></Form.Item>
      {/* 单任务超时已下沉组织配置（组织可选、系统兜底）；保留隐藏注册字段让
          保存 payload 原样回传加载值——后端整行替换语义下缺键会把系统兜底
          值静默清空。 */}
      <Form.Item name={['agent_runner', 'task_timeout_minutes']} hidden><Input className="mono" /></Form.Item>
      <SecretField name="agent_runner_api_key" clearName="clear_agent_runner_api_key" label="模型 API Key" configured={secrets.agent_runner_api_key_configured} />
      <SecretField name="agent_runner_custom_headers_json" clearName="clear_agent_runner_custom_headers_json" label="自定义 Header JSON" configured={secrets.agent_runner_custom_headers_configured} optional />
    </div>
  </div>
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
      <div><span>Scheduler</span><strong>{values?.scheduler?.enabled ? `每 ${values.scheduler.interval}` : '停用'}</strong></div>
    </div>
    {Object.keys(fieldErrors).length > 0 ? <Alert type="error" showIcon message="以下字段未通过校验" description={<ul>{Object.entries(fieldErrors).map(([field, detail]) => <li key={field}><code>{field}</code>：{detail}</li>)}</ul> } /> : null}
  </div>
}

function SecretField({ name, clearName, label, configured, optional = false }: { name: SecretFieldName; clearName: SecretFieldName; label: string; configured: boolean; optional?: boolean }) {
  return <div className="configuration-secret-field">
    <div className="configuration-secret-label"><span>{label}{optional ? '（可选）' : ''}</span>{configured ? <Tag color="success">已配置</Tag> : <Tag>未配置</Tag>}</div>
    <Form.Item name={typeof name === 'string' ? ['secrets', name] : name}><Input.Password autoComplete="new-password" placeholder={configured ? '留空保留当前值；输入新值进行替换' : '输入新值'} /></Form.Item>
    <Form.Item name={typeof clearName === 'string' ? ['secrets', clearName] : clearName} valuePropName="checked"><Checkbox>明确清除当前值</Checkbox></Form.Item>
  </div>
}

function toFormValues(view: ConfigurationView): ConfigurationFormValues {
  const { task_timeout, ...agentRunner } = view.config.agent_runner
  return {
    ...view.config,
    agent_runner: {
      ...agentRunner,
      task_timeout_minutes: durationMinutes(task_timeout),
    },
    secrets: {},
  }
}

function toUpdateRequest(values: ConfigurationFormValues, expectedRevision: number): ConfigurationUpdateRequest {
  const { task_timeout_minutes, ...agentRunner } = values.agent_runner
  return {
    config: {
      agent_runner: {
        ...agentRunner,
        task_timeout: `${task_timeout_minutes}m`,
      },
      scheduler: values.scheduler,
    },
    secrets: values.secrets || {},
    expected_revision: expectedRevision,
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
