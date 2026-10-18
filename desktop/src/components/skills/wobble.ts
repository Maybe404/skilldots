/*
 * The hand-drawn wobble of the wire view's curves (doodle style).
 *
 * It used to be an SVG filter (feTurbulence + feDisplacementMap) on the whole
 * wire <svg>. WebKit renders a filtered element through an offscreen buffer
 * the size of the filter region and draws nothing when that buffer is larger
 * than about 4096 x 4096 device pixels; the wire <svg> is as tall as the skill
 * list, so a region that covers it fails on a 2x screen (the native window)
 * while Chrome copes. Bending the path itself has no such limit, costs no
 * buffer, and looks the same in both engines.
 */

// Smooth two-channel noise with the filter's feel: ~33px wavelength, a finer ripple on top, about 2px of travel.
const FREQ = 0.03
const AMPLITUDE = 2.1
const STEP = 5

function hash(ix: number, iy: number, ch: number) {
  let h = (ix * 374761393 + iy * 668265263 + ch * 2147483647 + 11 * 1274126177) | 0
  h = Math.imul(h ^ (h >>> 13), 1274126177)
  h ^= h >>> 16
  return (h >>> 0) / 4294967295
}

function valueNoise(x: number, y: number, ch: number) {
  const ix = Math.floor(x)
  const iy = Math.floor(y)
  const fx = x - ix
  const fy = y - iy
  const u = fx * fx * (3 - 2 * fx)
  const v = fy * fy * (3 - 2 * fy)
  const a = hash(ix, iy, ch)
  const b = hash(ix + 1, iy, ch)
  const c = hash(ix, iy + 1, ch)
  const d = hash(ix + 1, iy + 1, ch)
  return a + (b - a) * u + (c - a) * v + (a - b - c + d) * u * v
}

/** Displacement in [-1, 1] of the point (x, y), the same wherever the curve is drawn. */
function offset(x: number, y: number, ch: number) {
  const n =
    valueNoise(x * FREQ, y * FREQ, ch) * 0.5 +
    valueNoise(x * FREQ * 2, y * FREQ * 2, ch + 2) * 0.3 +
    valueNoise(x * FREQ * 4, y * FREQ * 4, ch + 4) * 0.2
  return (n - 0.5) * 2
}

const cache = new Map<string, string>()

/**
 * A cubic Bezier from (x1, y1) to (x2, y2), sampled and nudged by the noise.
 * Pass wobble=false for the plain curve (the clean style).
 */
export function cubicPath(wobble: boolean, x1: number, y1: number, c1x: number, c1y: number, c2x: number, c2y: number, x2: number, y2: number) {
  const exact = `M${x1} ${y1} C${c1x} ${c1y}, ${c2x} ${c2y}, ${x2} ${y2}`
  if (!wobble) return exact
  const hit = cache.get(exact)
  if (hit) return hit
  // The control polygon is never shorter than the curve's chord and never much longer than the curve.
  const len = Math.hypot(c1x - x1, c1y - y1) + Math.hypot(c2x - c1x, c2y - c1y) + Math.hypot(x2 - c2x, y2 - c2y)
  const n = Math.max(8, Math.min(240, Math.ceil(len / STEP)))
  let d = ""
  for (let i = 0; i <= n; i++) {
    const t = i / n
    const m = 1 - t
    const x = m * m * m * x1 + 3 * m * m * t * c1x + 3 * m * t * t * c2x + t * t * t * x2
    const y = m * m * m * y1 + 3 * m * m * t * c1y + 3 * m * t * t * c2y + t * t * t * y2
    const px = x + offset(x, y, 0) * AMPLITUDE
    const py = y + offset(x, y, 1) * AMPLITUDE
    d += (i ? "L" : "M") + px.toFixed(1) + " " + py.toFixed(1)
  }
  if (cache.size > 4000) cache.clear()
  cache.set(exact, d)
  return d
}
