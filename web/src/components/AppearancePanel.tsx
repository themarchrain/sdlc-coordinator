import { useState } from 'react'
import { useTheme } from '../theme/useTheme'
import { THEME_LIST } from '../themes/registry'

/**
 * 外观主题面板：本地浏览器偏好（localStorage），与 6 步配置向导/
 * restartConfiguration 完全正交，不写入服务端配置。
 * 折叠交互：刷新/首载默认收起为一行摘要；展开后选中主题仅换肤、
 * 保持展开（便于连续比对），不再主动折叠。
 */
export function AppearancePanel() {
  const { themeId, theme, setTheme } = useTheme()
  const [open, setOpen] = useState(false)

  return (
    <section className="surface-panel appearance-panel" aria-label="外观主题">
      <button
        type="button"
        className={`appearance-summary${open ? ' appearance-summary--open' : ''}`}
        aria-expanded={open}
        aria-controls="appearance-theme-grid"
        onClick={() => setOpen((v) => !v)}
      >
        <span className="appearance-summary__chevron" aria-hidden="true">▸</span>
        <strong>外观主题：{theme.label}</strong>
        <span className="appearance-summary__swatches" aria-hidden="true">
          <i style={{ background: theme.swatches.canvas }} />
          <i style={{ background: theme.swatches.surface }} />
          <i style={{ background: theme.swatches.ink }} />
          <i style={{ background: theme.swatches.primary }} />
          <i style={{ background: theme.swatches.accent }} />
        </span>
        <span className="appearance-summary__hint">{open ? '收起主题列表' : '展开主题列表'}</span>
      </button>
      {open ? (
        <>
          <p className="appearance-panel__note">本地浏览器偏好，不写入服务端配置。</p>
          <div className="theme-card-grid" id="appearance-theme-grid">
            {THEME_LIST.map((candidate) => {
              const active = candidate.id === themeId
              return (
                <button
                  key={candidate.id}
                  type="button"
                  className={`theme-card${active ? ' theme-card--active' : ''}`}
                  aria-pressed={active}
                  onClick={() => setTheme(candidate.id)}
                >
                  <span className="theme-card__head">
                    <strong>{candidate.label}</strong>
                    <span className={`theme-card__mode${candidate.isDark ? ' theme-card__mode--dark' : ''}`}>{candidate.isDark ? '暗色' : '亮色'}</span>
                  </span>
                  <span className="theme-card__swatches" aria-hidden="true">
                    <i style={{ background: candidate.swatches.canvas }} />
                    <i style={{ background: candidate.swatches.surface }} />
                    <i style={{ background: candidate.swatches.ink }} />
                    <i style={{ background: candidate.swatches.primary }} />
                    <i style={{ background: candidate.swatches.accent }} />
                  </span>
                  {active ? <span className="theme-card__current">当前使用</span> : null}
                </button>
              )
            })}
          </div>
        </>
      ) : null}
    </section>
  )
}
