import assert from 'node:assert/strict'
import { test } from 'node:test'
import { mainTabLabel, visibleMainTabs } from './dynamics.ts'
import { presentWidgetError, settleWidgetApply, widgetsApplyEnabled, widgetsLockedMessage } from './widgetsTab.ts'

test('Widgets sits next to Code', () => {
  assert.deepEqual(visibleMainTabs(false).map(mainTabLabel), ['Interface', 'World', 'Info', 'Code', 'Widgets'])
  assert.deepEqual(visibleMainTabs(true).map(mainTabLabel), [
    'Interface',
    'World',
    'Info',
    'Code',
    'Widgets',
    'Dynamics',
  ])
})

test('Apply is locked while go is running', () => {
  assert.equal(widgetsApplyEnabled(true, false), true)
  assert.equal(widgetsApplyEnabled(true, true), false)
  assert.equal(widgetsApplyEnabled(false, false), false)
  assert.equal(widgetsLockedMessage, 'Stop the model before changing the widgets.')
})

test('a parse error keeps the draft and quotes the line', () => {
  const draft = 'GRAPHICS-WINDOW\n0\n0\n'
  const rejected = settleWidgetApply(draft, {
    ok: false,
    error: 'line 1: view has an empty world',
    widgetText: 'old',
  })
  assert.equal(rejected.widgetText, draft)
  assert.match(presentWidgetError(rejected.error!, draft), /view has an empty world/)
  assert.match(presentWidgetError(rejected.error!, draft), /1 \| GRAPHICS-WINDOW/)
})

test('a successful apply keeps the edited interface text', () => {
  const draft = '<widgets></widgets>'
  const applied = settleWidgetApply(draft, { ok: true, widgetText: draft })
  assert.equal(applied.widgetText, draft)
  assert.equal(applied.error, null)
})
