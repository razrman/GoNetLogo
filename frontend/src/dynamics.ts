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

// Shown under the World tab's dt field. A larger step finishes a run in
// fewer ticks and moves the stocks further from the continuous curve.
export const worldDtHelp =
  'A larger dt runs cycles faster, at an accuracy cost. The next setup and go use this value. It is the dt line on the Dynamics tab.'

// The World tab's numeric dt is only for a model that has a diagram.
export function showWorldDt(hasDynamics: boolean): boolean {
  return hasDynamics
}

// definitionDt is the dt line in the Dynamics text. Null when the text
// has no dt declaration. The World field shows this same text.
export function definitionDt(definition: string): string | null {
  const lines = definition.replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n')
  let i = 0
  while (i < lines.length && (lines[i].trim() === '' || lines[i].trim().startsWith(';'))) i++
  if (i >= lines.length || indentOf(lines[i]) !== 0) return null
  const header = splitDt(lines[i])
  if (!header) return null
  if (!isFenceOpener(header.rest)) return header.rest
  const fence = '>'.repeat(header.rest.length)
  const parts: string[] = []
  for (i++; i < lines.length; i++) {
    if (indentOf(lines[i]) === 0 && lines[i].trim() === fence) break
    parts.push(lines[i])
  }
  return parts.join('\n')
}

// replaceDefinitionDt writes dt into the Dynamics text and leaves every
// stock, flow, and converter line alone. The World field and the code
// stay on the same value.
export function replaceDefinitionDt(definition: string, dt: string): string {
  const text = definition.replace(/\r\n/g, '\n').replace(/\r/g, '\n')
  const lines = text.split('\n')
  let i = 0
  while (i < lines.length && (lines[i].trim() === '' || lines[i].trim().startsWith(';'))) i++
  const header = i < lines.length ? splitDt(lines[i]) : null
  if (header == null || indentOf(lines[i]) !== 0) {
    const body = text.replace(/^\n/, '')
    if (body.trim() === '') return `dt ${dt}\n`
    return `dt ${dt}\n\n${body}`
  }
  let end = i + 1
  if (isFenceOpener(header.rest)) {
    const fence = '>'.repeat(header.rest.length)
    while (end < lines.length && !(indentOf(lines[end]) === 0 && lines[end].trim() === fence)) end++
    if (end < lines.length) end++
  }
  lines.splice(i, end - i, `dt ${dt}`)
  return lines.join('\n')
}

export function worldDtError(raw: string): string | null {
  const text = raw.trim()
  if (!/^[+]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?$/.test(text)) return 'dt must be greater than 0.'
  const n = Number(text)
  if (!Number.isFinite(n) || n <= 0) return 'dt must be greater than 0.'
  return null
}

// dtApplyBlocked reports the lock when a dt edit would recompile during go.
// An unchanged dt is not blocked, so a seed-only World apply still goes through.
export function dtApplyBlocked(running: boolean, dtChanged: boolean): string | null {
  if (running && dtChanged) return dynamicsLockedMessage
  return null
}

function indentOf(line: string): number {
  let i = 0
  while (i < line.length && (line[i] === ' ' || line[i] === '\t')) i++
  return i
}

function splitDt(line: string): { rest: string } | null {
  const match = /^dt(?:[ \t]+(.*))?$/i.exec(line.trim())
  if (!match) return null
  return { rest: (match[1] ?? '').trim() }
}

function isFenceOpener(rest: string): boolean {
  return rest.length >= 2 && rest.replace(/</g, '') === ''
}

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
