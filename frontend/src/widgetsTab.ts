// The Widgets tab edits the model's interface as text. Apply sends that
// text to the server. A parse error stays in the editor.

export const widgetsLockedMessage = 'Stop the model before changing the widgets.'

export function widgetsApplyEnabled(hasSession: boolean, running: boolean): boolean {
  return hasSession && !running
}

// presentWidgetError quotes the numbered line from the interface text
// when the server names one.
export function presentWidgetError(message: string, source: string): string {
  const cleaned = message.replace(/^\[GraphQL\]\s*/i, '').trim()
  const match = /line (\d+)/i.exec(cleaned)
  if (!match) return cleaned
  const n = Number(match[1])
  const lines = source.replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n')
  const text = lines[n - 1]
  if (text == null) return cleaned
  return `${cleaned}\n${String(n).padStart(4, ' ')} | ${text}`
}

// settleWidgetYAML keeps the user's text when the server rejects it.
export function settleWidgetYAML(
  draft: string,
  result: { ok: boolean; error?: string | null; spec?: string | null },
): { draft: string; error: string | null } {
  if (!result.ok) {
    const message = (result.error ?? '').replace(/^\[GraphQL\]\s*/i, '').trim()
    return { draft, error: message || 'Could not apply the widget' }
  }
  return { draft: result.spec ?? draft, error: null }
}

export function settleWidgetApply(
  draft: string,
  result: { ok: boolean; error?: string | null; widgetText?: string | null },
): { widgetText: string; error: string | null } {
  if (!result.ok) {
    return { widgetText: draft, error: result.error ?? 'Could not read the interface' }
  }
  return { widgetText: result.widgetText ?? draft, error: null }
}
