import React from 'react'
import ReactDOM from 'react-dom/client'
import { ConfigProvider, message } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { BrowserRouter } from 'react-router-dom'

import { App } from './app/App'
import { ThemeProvider } from './theme/ThemeProvider'
import { useTheme } from './theme/useTheme'
import './styles/index.css'
// 必须在 index.css 之后引入：主题令牌块以同特异性覆盖 :root 回退令牌。
import './themes/css/themes.css'

// 全局操作提示最多同时显示 3 条，超出后最早的提示被关闭
message.config({ maxCount: 3 })

function ThemedRoot() {
  const { theme } = useTheme()
  return (
    <ConfigProvider locale={zhCN} theme={theme.antd}>
      <BrowserRouter basename="/admin"><App /></BrowserRouter>
    </ConfigProvider>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ThemeProvider>
      <ThemedRoot />
    </ThemeProvider>
  </React.StrictMode>,
)
