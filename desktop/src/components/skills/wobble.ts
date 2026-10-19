/*
 * The hand-drawn wobble of the wire view's curves (doodle style).
 *
 * It used to be an SVG filter (feTurbulence + feDisplacementMap) on the whole
 * wire <svg>, then paths bent by noise taken at each point's position on the
 * page. Both shimmered while scrolling: the agent end of every curve follows
 * the scroll, so each frame moved the curve through a different patch of
 * noise. The noise now belongs to the wire: it is read along the curve's own
 * length (from a phase given by the wire's seed), so a moving curve only
 * stretches its wobble. It fades out at both ends, so curves still meet the
 * ports exactly.
 */

// Smooth two-channel noise with the filter's feel: ~33px wavelength, a finer ripple on top, about 2px of travel.
const FREQ = 0.03
const AMPLITUDE = 2.1
const STEP = 5
/** Distance (px) over which the wobble fades in at each end. */
const FADE = 24
/** Spacing (px) of the precomputed noise along a wire. */
const SPACING = 3

function hash(ix: number, iy: number, ch: number) {
  let h = (ix * 374761393 + iy * 668265263 + ch * 2147483647 + 11 * 1274126177) | 0
  h = Math.imul(h ^ (h >>> 13), 1274126177)
  h ^= h >>> 16
  return (h >>> 0) / 4294967295
}

function valueNoise(x: number, ch: number) {
  const ix = Math.floor(x)
  const fx = x - ix
  const u = fx * fx * (3 - 2 * fx)
  const a = hash(ix, 0, ch)
  return a + (hash(ix + 1, 0, ch) - a) * u
}

/** Displacement in [-1, 1] at distance s along a wire. */
function offset(s: number, ch: number) {
  const n = valueNoise(s * FREQ, ch) * 0.5 + valueNoise(s * FREQ * 2, ch + 2) * 0.3 + valueNoise(s * FREQ * 4, ch + 4) * 0.2
  return (n - 0.5) * 2
}

// The noise is the same for every frame, so it is computed once, as far along as a wire has needed.
const table: [number[], number[]] = [[], []]

function along(s: number, ch: 0 | 1) {
  const col = table[ch]
  const i = s / SPACING
  const i0 = Math.floor(i)
  while (col.length <= i0 + 1) col.push(offset(col.length * SPACING, ch))
  return col[i0] + (col[i0 + 1] - col[i0]) * (i - i0)
}

/** A stable seed for a wire, from its key (skill and agent). */
export function wireSeed(key: string) {
  let h = 2166136261
  for (let i = 0; i < key.length; i++) h = Math.imul(h ^ key.charCodeAt(i), 16777619)
  return h >>> 0
}

/**
 * Adds a cubic Bezier from (x1, y1) to (x2, y2) to the canvas path. With
 * wobble it is sampled and nudged by the wire's own noise (seed); without, it
 * is the plain curve (the clean style).
 */
export function traceCubic(
  ctx: CanvasRenderingContext2D,
  wobble: boolean,
  seed: number,
  x1: number,
  y1: number,
  c1x: number,
  c1y: number,
  c2x: number,
  c2y: number,
  x2: number,
  y2: number,
) {
  ctx.moveTo(x1, y1)
  if (!wobble) {
    ctx.bezierCurveTo(c1x, c1y, c2x, c2y, x2, y2)
    return
  }
  // The control polygon is never shorter than the curve's chord and never much longer than the curve.
  const len = Math.hypot(c1x - x1, c1y - y1) + Math.hypot(c2x - c1x, c2y - c1y) + Math.hypot(x2 - c2x, y2 - c2y)
  const n = Math.max(8, Math.min(240, Math.ceil(len / STEP)))
  const phase = (seed % 509) * 23
  for (let i = 1; i <= n; i++) {
    const t = i / n
    const m = 1 - t
    const x = m * m * m * x1 + 3 * m * m * t * c1x + 3 * m * t * t * c2x + t * t * t * x2
    const y = m * m * m * y1 + 3 * m * m * t * c1y + 3 * m * t * t * c2y + t * t * t * y2
    const s = t * len
    const a = AMPLITUDE * Math.max(0, Math.min(1, s / FADE, (len - s) / FADE))
    ctx.lineTo(x + along(phase + s, 0) * a, y + along(phase + s, 1) * a)
  }
}
