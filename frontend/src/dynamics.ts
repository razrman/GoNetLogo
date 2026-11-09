// Rules for the Dynamics tab. The tab is the text of a System Dynamics
// diagram: dt, stocks, flows, and converters. It is not a diagram canvas.

export type MainTab = 'interface' | 'world' | 'info' | 'code' | 'dynamics'

export function hasSystemDynamics(dynamics: { definition: string } | null | undefined): boolean {
  return dynamics != null
}

// visibleMainTabs hides Dynamics unless the loaded model has a System
// Dynamics section. Every other model keeps the original four tabs.
export function visibleMainTabs(hasDynamics: boolean): MainTab[] {
  const tabs: MainTab[] = ['interface', 'world', 'info', 'code']
  if (hasDynamics) tabs.push('dynamics')
  return tabs
}

export function mainTabLabel(tab: MainTab): string {
  switch (tab) {
    case 'interface':
      return 'Interface'
    case 'world':
      return 'World'
    case 'info':
      return 'Info'
    case 'code':
      return 'Code'
    case 'dynamics':
      return 'Dynamics'
  }
}

// tabAfterModelLoad leaves the Dynamics tab when the new model has no
// diagram. Other tabs stay where they are.
export function tabAfterModelLoad(current: MainTab, hasDynamics: boolean): MainTab {
  if (current === 'dynamics' && !hasDynamics) return 'interface'
  return current
}

// Same sentence the server returns when go is running.
export const dynamicsLockedMessage = 'Stop the model before changing the system dynamics.'

export function dynamicsApplyEnabled(hasSession: boolean, running: boolean): boolean {
  return hasSession && !running
}

export function dynamicsDefinitionReadOnly(running: boolean): boolean {
  return running
}

// The generated procedures are what Compile appends. They are not edited.
export function generatedProceduresReadOnly(): boolean {
  return true
}

// presentDynamicsError quotes the definition line for a parse error.
// A compile error already carries its own snippet, so it is left as sent.
export function presentDynamicsError(message: string, definition: string): string {
  const cleaned = message.replace(/^\[GraphQL\]\s*/i, '').trim()
  if (cleaned.includes('\n')) return cleaned
  const match = /^line (\d+):/i.exec(cleaned)
  if (!match) return cleaned
  const n = Number(match[1])
  const lines = definition.replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n')
  const text = lines[n - 1]
  if (text == null) return cleaned
  return `${cleaned}\n${String(n).padStart(4, ' ')} | ${text}`
}

export function dynamicsErrorTitle(message: string): string {
  const cleaned = message.replace(/^\[GraphQL\]\s*/i, '').trim()
  if (/stop the model/i.test(cleaned)) return 'Model is running'
  if (/generated procedures|model code did not compile/i.test(cleaned)) return 'Could not compile'
  if (/^line \d+:/i.test(cleaned)) return 'Could not read the definition'
  return 'Could not apply'
}

// settleDynamicsApply keeps the editor text when Apply is rejected, and
// installs the canonical definition when the model recompiles.
export function settleDynamicsApply(
  draft: string,
  result: { ok: boolean; error: string | null; definition: string | null },
): { definition: string; error: string | null } {
  if (!result.ok) return { definition: draft, error: result.error ?? 'Could not apply' }
  return { definition: result.definition ?? draft, error: null }
}
