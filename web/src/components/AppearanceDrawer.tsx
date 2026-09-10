import { useState } from 'react'
import { BgColorsOutlined } from '@ant-design/icons'
import { Button, Drawer } from 'antd'

import { AppearancePanel } from './AppearancePanel'

/**
 * 外观主题入口（全局外壳用）：图标按钮 + 右侧抽屉承载 AppearancePanel。
 * 主题是本地浏览器偏好，入口从系统配置页（超管专属路由）迁到两个外壳后，
 * 成员/组织管理员/超管都能随时换肤；面板本体保持自包含、原样复用。
 */
export function AppearanceDrawerButton() {
  const [open, setOpen] = useState(false)

  return (
    <>
      <Button
        className="appearance-trigger"
        type="text"
        icon={<BgColorsOutlined />}
        aria-label="外观主题"
        title="外观主题"
        onClick={() => setOpen(true)}
      />
      <Drawer
        className="appearance-drawer"
        title="外观主题"
        placement="right"
        width={480}
        open={open}
        onClose={() => setOpen(false)}
        destroyOnClose
      >
        <AppearancePanel />
      </Drawer>
    </>
  )
}
