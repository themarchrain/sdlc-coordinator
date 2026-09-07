import React from 'react'
import ReactDOM from 'react-dom/client'
import { ConfigProvider, message } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import { BrowserRouter } from 'react-router-dom'

import { App } from './app/App'
import './styles/index.css'

// 全局操作提示最多同时显示 3 条，超出后最早的提示被关闭
message.config({ maxCount: 3 })

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider
      locale={zhCN}
      theme={{
        token: {
          colorPrimary: '#2F6BFF',
          colorSuccess: '#0F9F8F',
          colorText: '#152033',
          colorTextSecondary: '#667085',
          colorBgLayout: '#F3F6FA',
          borderRadius: 8,
          fontFamily: 'Inter, Segoe UI, PingFang SC, Microsoft YaHei, sans-serif',
        },
      }}
    >
      <BrowserRouter basename="/admin"><App /></BrowserRouter>
    </ConfigProvider>
  </React.StrictMode>,
)
