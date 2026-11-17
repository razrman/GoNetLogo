// View update mode, matching NetLogo's view updates dropdown.
// "ticks" redraws at tick and when a button or command finishes.
// "continuous" redraws while a button runs.

export type ViewUpdates = 'ticks' | 'continuous'

export const viewUpdateChoices: { value: ViewUpdates; label: string }[] = [
  { value: 'ticks', label: 'on ticks' },
  { value: 'continuous', label: 'continuous' },
]

// viewUpdatesFromModel is the dropdown value for a loaded model.
// 1, "1", and "ticks" are NetLogo's on ticks. Anything else is continuous,
// including a file that omits the setting.
export function viewUpdatesFromModel(mode: string | number | null | undefined): ViewUpdates {
  if (mode === 1 || mode === '1' || mode === 'ticks' || mode === 'on ticks') return 'ticks'
  return 'continuous'
}
