// Anchors for turtle, patch, and link labels in the 3D view.
//
// NetLogo pins a label at the bottom-right of the agent's box. The text
// grows up and to the left. The 3D view keeps that corner and billboards
// the text so it faces the camera. z comes from the turtle, from the
// patch's pzcor, or from the midpoint of a link.

export type LabelAnchor = {
  text: string
  x: number
  y: number
  z: number
  r: number
  g: number
  b: number
}

export type LabelSource = {
  text: string
  who?: number | null
  px?: number | null
  py?: number | null
  lx?: number | null
  ly?: number | null
  z?: number
  r: number
  g: number
  b: number
}

export type LabelTurtle = {
  who: number
  x: number
  y: number
  z?: number
  size: number
  hidden: boolean
}

export function labelAnchors(labels: readonly LabelSource[], turtles: readonly LabelTurtle[]): LabelAnchor[] {
  const byWho = new Map<number, LabelTurtle>()
  for (const t of turtles) byWho.set(t.who, t)
  const out: LabelAnchor[] = []
  for (const label of labels) {
    if (!label.text) continue
    if (label.who != null) {
      const t = byWho.get(label.who)
      if (!t || t.hidden) continue
      const size = t.size || 1
      out.push({
        text: label.text,
        x: t.x + size * 0.5,
        y: t.y - size * 0.5,
        z: t.z ?? label.z ?? 0,
        r: label.r,
        g: label.g,
        b: label.b,
      })
      continue
    }
    if (label.px != null && label.py != null) {
      out.push({
        text: label.text,
        x: label.px + 0.5,
        y: label.py - 0.5,
        z: label.z ?? 0,
        r: label.r,
        g: label.g,
        b: label.b,
      })
      continue
    }
    if (label.lx != null && label.ly != null) {
      out.push({
        text: label.text,
        x: label.lx,
        y: label.ly,
        z: label.z ?? 0,
        r: label.r,
        g: label.g,
        b: label.b,
      })
    }
  }
  return out
}
