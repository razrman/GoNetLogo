import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  dynamicsApplyEnabled,
  dynamicsDefinitionReadOnly,
  dynamicsErrorTitle,
  dynamicsLockedMessage,
  generatedProceduresReadOnly,
  hasSystemDynamics,
  mainTabLabel,
  presentDynamicsError,
  settleDynamicsApply,
  tabAfterModelLoad,
  visibleMainTabs,
} from './dynamics.ts'

test('the Dynamics tab is shown only while a system dynamics model is loaded', () => {
  assert.equal(hasSystemDynamics(null), false)
  assert.equal(hasSystemDynamics(undefined), false)
  assert.equal(hasSystemDynamics({ definition: 'dt 1\n' }), true)

  assert.deepEqual(visibleMainTabs(false).map(mainTabLabel), ['Interface', 'World', 'Info', 'Code'])
  assert.deepEqual(visibleMainTabs(true).map(mainTabLabel), ['Interface', 'World', 'Info', 'Code', 'Dynamics'])

  // A non-SDM model loaded from the Dynamics tab goes back to Interface.
  assert.equal(tabAfterModelLoad('dynamics', false), 'interface')
  assert.equal(tabAfterModelLoad('dynamics', true), 'dynamics')
  assert.equal(tabAfterModelLoad('code', false), 'code')
  assert.equal(tabAfterModelLoad('world', false), 'world')
  assert.equal(tabAfterModelLoad('info', false), 'info')
  assert.equal(tabAfterModelLoad('interface', true), 'interface')
})

test('Apply stays locked while go is running', () => {
  assert.equal(dynamicsApplyEnabled(true, false), true)
  assert.equal(dynamicsApplyEnabled(true, true), false)
  assert.equal(dynamicsApplyEnabled(false, false), false)
  assert.equal(dynamicsDefinitionReadOnly(true), true)
  assert.equal(dynamicsDefinitionReadOnly(false), false)
  assert.equal(generatedProceduresReadOnly(), true)
  assert.equal(dynamicsLockedMessage, 'Stop the model before changing the system dynamics.')
})

test('a rejected edit stays in the editor and quotes the bad line', () => {
  const draft = 'dt 1\n\nstock\n  initial 1\n'
  const rejected = settleDynamicsApply(draft, {
    ok: false,
    error: 'line 3: stock needs a name',
    definition: 'dt 1\n',
  })
  assert.equal(rejected.definition, draft)
  assert.equal(dynamicsErrorTitle(rejected.error!), 'Could not read the definition')
  const shown = presentDynamicsError(rejected.error!, rejected.definition)
  assert.match(shown, /stock needs a name/)
  assert.match(shown, /3 \| stock/)
})

test('a compile error keeps the edit and shows the generated line', () => {
  const draft = 'dt 1\n\nstock s\n  initial 1\n  allow-negative true\n\nflow f\n  expression (\n  sink s\n'
  const message = 'generated procedures, line 4: expected end\n   4 | to-report inflow'
  const rejected = settleDynamicsApply(draft, { ok: false, error: message, definition: draft })
  assert.equal(rejected.definition, draft)
  assert.equal(dynamicsErrorTitle(message), 'Could not compile')
  assert.equal(presentDynamicsError(message, draft), message)
})

test('a successful Apply installs the canonical definition', () => {
  const draft = 'dt 1\n\nstock a\n  initial 2\n  allow-negative true\n\n'
  const canonical = 'dt 1\n\nstock a\n  initial 2\n  allow-negative true\n'
  const applied = settleDynamicsApply(draft, { ok: true, error: null, definition: canonical })
  assert.equal(applied.definition, canonical)
  assert.equal(applied.error, null)
})
