import { useCallback, useEffect, useState } from 'react'
import { Alert, Button, Input, message, Modal, Space, Spin, Tag } from 'antd'

import { adminApi } from '../api/client'
import type { Connector, ConnectorConfiguration, ConnectorConfigurationSettings } from '../api/types'
import { TermHint } from './TermHint'

type Props = {
  connector?: Connector
  onClose: () => void
  onSaved: () => void
}

// 连接器配置是插件自定义的不透明 JSON:表单字段由连接器自身语义决定,
// 这里只做 JSON 合法性校验与透传保存。
export function ConnectorConfigurationModal({ connector, onClose, onSaved }: Props) {
  const [settingsText, setSettingsText] = useState('')
  const [configuration, setConfiguration] = useState<ConnectorConfiguration>()
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState<string>()
  const [parseError, setParseError] = useState<string>()
  const [saving, setSaving] = useState(false)
  const connectorID = connector?.connector_id

  const loadConfiguration = useCallback(async () => {
    if (!connectorID) return
    setLoading(true)
    setLoadError(undefined)
    try {
      const remote = await adminApi.connectorConfiguration(connectorID)
      setConfiguration(remote)
      setSettingsText(JSON.stringify(remote.settings ?? {}, null, 2))
    } catch (error) {
      setConfiguration(undefined)
      const text = error instanceof Error ? error.message : '读取连接器配置失败'
      setLoadError(text)
    } finally {
      setLoading(false)
    }
  }, [connectorID])

  useEffect(() => {
    if (!connectorID) {
      setConfiguration(undefined)
      setLoadError(undefined)
      setParseError(undefined)
      setSettingsText('')
      setLoading(false)
      return
    }
    void loadConfiguration()
  }, [connectorID, loadConfiguration])

  const save = async () => {
    if (!connector) return
    let settings: ConnectorConfigurationSettings
    try {
      settings = JSON.parse(settingsText || '{}') as ConnectorConfigurationSettings
      if (settings === null || typeof settings !== 'object' || Array.isArray(settings)) {
        throw new Error('settings must be a JSON object')
      }
    } catch (error) {
      setParseError(error instanceof Error ? error.message : '设置必须是合法的 JSON 对象')
      message.warning('设置必须是合法的 JSON 对象')
      return
    }
    setParseError(undefined)
    const previousSecrets = new Set(configuration?.secret_fields ?? [])
    const currentKeys = new Set(Object.keys(settings))
    const preserveSecrets = [...previousSecrets].filter((key) => !currentKeys.has(key))
    setSaving(true)
    try {
      await adminApi.updateConnectorConfiguration(connector.connector_id, {
        settings,
        preserve_secrets: preserveSecrets,
      })
      message.success('实例配置已保存')
      onSaved()
    } catch (error) {
      message.error(error instanceof Error ? error.message : '保存连接器配置失败')
    } finally {
      setSaving(false)
    }
  }

  const secretFields = configuration?.secret_fields ?? []

  return <Modal
    width={640}
    open={Boolean(connector)}
    title={`实例配置 ${connector?.display_name ?? ''}`}
    onCancel={onClose}
    footer={<Space>
      <Button onClick={onClose} disabled={saving}>取消</Button>
      <Button type="primary" aria-label="保存" loading={saving} disabled={loading} onClick={() => void save()}>保存</Button>
    </Space>}
    destroyOnClose
  >
    <Spin spinning={loading}>
      {loadError ? (
        <Alert
          type="error"
          showIcon
          message="读取连接器配置失败"
          description={<>
            <p>{loadError}</p>
            <p className="form-note">连接器刚启动时 HTTP 端口可能尚未就绪，稍后重试即可。<TermHint title="读取说明" content="这里的保存只更新实例配置，由连接器自行校验字段。" /></p>
            <Button size="small" onClick={() => void loadConfiguration()}>重试读取</Button>
          </>}
        />
      ) : (
        <Alert
          type={configuration?.mode === 'setup_pending' ? 'warning' : 'info'}
          showIcon
          style={{ marginBottom: 12 }}
          message={configuration?.mode === 'setup_pending' ? '连接器正在等待首次配置' : configuration ? `当前配置版本 ${configuration.version}` : '正在读取连接器配置…'}
          description={<Space size={4}>
            配置字段由连接器插件自定义，coordinator 只做透传保存。
            <TermHint title="实例配置说明" content="settings 是连接器自定义的 JSON 对象；提交时未回传的密钥字段会按 preserve_secrets 保留原值。" />
          </Space>}
        />
      )}
      {secretFields.length > 0 ? (
        <div style={{ marginBottom: 8 }}>
          {secretFields.map((key) => <Tag key={key} color="blue">密钥字段: {key}</Tag>)}
        </div>
      ) : null}
      <Input.TextArea
        aria-label="连接器设置 JSON"
        className="mono"
        rows={14}
        value={settingsText}
        onChange={(event) => { setSettingsText(event.target.value); setParseError(undefined) }}
        status={parseError ? 'error' : undefined}
        placeholder='{"api_key": "..."}'
      />
      {parseError ? <Alert type="error" showIcon style={{ marginTop: 8 }} message="JSON 解析失败" description={parseError} /> : null}
    </Spin>
  </Modal>
}
