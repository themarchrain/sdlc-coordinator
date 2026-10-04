import { createContext, createElement, useContext, type ReactNode } from 'react'

import type { ConfigurationStatus } from '../api/types'

export type ConfigurationStatusSyncOptions = {
  // 标记本次状态来自配置向导的重启轮询：边界据此把落地路由交给
  // 自己接管（重启完成送首页），与直接访问已配置系统 /setup 的
  // 既有导航（送系统配置页）区分开。
  fromRestart?: boolean
}

export type ConfigurationStatusValue = {
  status?: ConfigurationStatus
  // 配置页保存或轮询到新状态时回写共享层，避免边界停留在旧模式
  // （首次初始化后边界仍认为 setup_pending，会把后续导航弹回 /setup）。
  sync: (value: ConfigurationStatus, options?: ConfigurationStatusSyncOptions) => void
}

// Provider 外误用按无状态处理（fail-closed）：读不到状态、同步是 no-op，
// 绝不伪造 ready 让门禁放行。
const ConfigurationStatusContext = createContext<ConfigurationStatusValue>({
  status: undefined,
  sync: () => undefined,
})

export function ConfigurationStatusProvider({
  value,
  children,
}: {
  value: ConfigurationStatusValue
  children: ReactNode
}) {
  return createElement(ConfigurationStatusContext.Provider, { value }, children)
}

export function useConfigurationStatus(): ConfigurationStatusValue {
  return useContext(ConfigurationStatusContext)
}
