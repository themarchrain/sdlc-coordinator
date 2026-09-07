import { describe, expect, it } from 'vitest'

import { getTheme, THEMES, THEME_LIST } from './registry'

describe('themes registry', () => {
  it('registers exactly 10 unique theme ids with default first', () => {
    expect(THEME_LIST).toHaveLength(10)
    const ids = THEME_LIST.map((theme) => theme.id)
    expect(new Set(ids).size).toBe(10)
    expect(ids).toEqual(['default', 'apple', 'claude', 'minimax', 'supabase', 'vercel', 'xai', 'cursor', 'ollama', 'voltagent'])
  })

  it('gives every theme complete metadata, swatches, antd config and slots', () => {
    for (const theme of THEME_LIST) {
      expect(theme.id, `${theme.id}: id`).toBeTruthy()
      expect(theme.label, `${theme.id}: label`).toBeTruthy()
      expect(typeof theme.isDark, `${theme.id}: isDark`).toBe('boolean')
      expect(theme.chromeColor, `${theme.id}: chromeColor`).toMatch(/^#/)
      expect(theme.swatches, `${theme.id}: swatches`).toMatchObject({
        canvas: expect.stringMatching(/^#/),
        surface: expect.stringMatching(/^#/),
        ink: expect.stringMatching(/^#/),
        primary: expect.stringMatching(/^#/),
        accent: expect.stringMatching(/^#/),
      })
      expect(theme.antd.token, `${theme.id}: antd token`).toMatchObject({
        colorPrimary: expect.stringMatching(/^#/),
        colorText: expect.stringMatching(/^#/),
        borderRadius: expect.any(Number),
        fontFamily: expect.any(String),
      })
      expect(theme.slots.chart.trends, `${theme.id}: chart.trends`).toMatchObject({
        mr: expect.any(String), crDone: expect.any(String), crStart: expect.any(String), avgDuration: expect.any(String),
      })
      expect(theme.slots.chart.ratios, `${theme.id}: chart.ratios`).toMatchObject({
        success: expect.any(String), firstRun: expect.any(String), intervention: expect.any(String), recovery: expect.any(String),
      })
      expect(theme.slots.timeline, `${theme.id}: timeline`).toMatchObject({
        error: expect.any(String), warning: expect.any(String), success: expect.any(String), info: expect.any(String),
      })
      expect(theme.slots.eventType, `${theme.id}: eventType`).toMatchObject({
        error: expect.any(String), usage: expect.any(String), info: expect.any(String),
      })
      expect(theme.slots.sparkline, `${theme.id}: sparkline`).toMatchObject({
        default: expect.any(String), hoverGrid: expect.any(String),
      })
    }
  })

  it('marks only xai and voltagent as dark themes', () => {
    expect(THEME_LIST.filter((theme) => theme.isDark).map((theme) => theme.id)).toEqual(['xai', 'voltagent'])
  })

  it('falls back to default for unknown or missing ids', () => {
    expect(THEMES.default.id).toBe('default')
    expect(getTheme('default').id).toBe('default')
    expect(getTheme('not-a-theme').id).toBe('default')
    expect(getTheme('').id).toBe('default')
    expect(getTheme(undefined).id).toBe('default')
    expect(getTheme(null).id).toBe('default')
  })
})
