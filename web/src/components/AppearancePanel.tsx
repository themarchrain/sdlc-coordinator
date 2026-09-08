import { useState } from 'react'
import { useTheme } from '../theme/useTheme'
import { groupedThemes, THEME_LIST, type ThemeLightFilter } from '../themes/registry'

/**
 * 外观主题面板：本地浏览器偏好（localStorage），与 6 步配置向导/
 * restartConfiguration 完全正交，不写入服务端配置。
 * 折叠交互：刷新/首载默认收起为一行摘要；展开后选中主题仅换肤、
 * 保持展开（便于连续比对），不再主动折叠。
 * 分类管理（多维度）：明暗筛选 chips（全部/亮色/暗色）× 策展家族分节
 * （groupedThemes，空节自动隐藏）× 关键词搜索（label/id，77 主题
 * 触发的 README 后置项）。
 */
const LIGHT_FILTERS: { id: ThemeLightFilter; label: string }[] = [
  { id: 'all', label: '全部' },
  { id: 'light', label: '亮色' },
  { id: 'dark', label: '暗色' },
]

export function AppearancePanel() {
  const { themeId, theme, setTheme } = useTheme()
  const [open, setOpen] = useState(false)
  const [filter, setFilter] = useState<ThemeLightFilter>('all')
  const [query, setQuery] = useState('')

  const counts = {
    all: THEME_LIST.length,
    light: THEME_LIST.filter((candidate) => !candidate.isDark).length,
    dark: THEME_LIST.filter((candidate) => candidate.isDark).length,
  }
  const sections = groupedThemes(filter, query)
  const matchCount = sections.reduce((total, section) => total + section.themes.length, 0)

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
          <input
            className="appearance-search"
            type="text"
            value={query}
            placeholder="搜索主题名称或 ID"
            aria-label="搜索主题"
            onChange={(event) => setQuery(event.target.value)}
          />
          <div className="appearance-chips" role="group" aria-label="明暗筛选">
            {LIGHT_FILTERS.map(({ id, label }) => (
              <button
                key={id}
                type="button"
                className={`appearance-chip${filter === id ? ' appearance-chip--on' : ''}`}
                aria-pressed={filter === id}
                onClick={() => setFilter(id)}
              >
                {label} <b>{counts[id]}</b>
              </button>
            ))}
          </div>
          {matchCount === 0 ? (
            <p className="appearance-panel__empty">没有匹配“{query.trim()}”的主题。</p>
          ) : (
            <div id="appearance-theme-grid">
              {sections.map(({ family, themes }) => (
              <div key={family.id} className="theme-family">
                <div className="theme-family__head">
                  <h3 className="theme-family__title">{family.label}</h3>
                  <span className="theme-family__count">{themes.length}</span>
                  <span className="theme-family__desc">{family.description}</span>
                </div>
                <div className="theme-card-grid">
                  {themes.map((candidate) => {
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
              </div>
            ))}
            </div>
          )}
        </>
      ) : null}
    </section>
  )
}
