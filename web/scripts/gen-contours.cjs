// Generates static/contours.svg — a topographic contour field (Gaussian peaks
// + domain-warped value noise) traced with marching squares. Index contours
// (every 5th) are drawn thicker. Usage: node scripts/gen-contours.cjs [seed]
// The SVG is used as a CSS mask so contour color follows the theme.
const W = 1600, H = 1000;
const SEED = Number(process.argv[2] || 7);

// deterministic PRNG
let s = SEED >>> 0;
const rng = () => ((s = (s * 1664525 + 1013904223) >>> 0) / 4294967296);

// value noise + fbm for organic terrain
const hash = (x, y) => {
  let n = (x * 374761393 + y * 668265263) | 0;
  n = (n ^ (n >> 13)) * 1274126177;
  return ((n ^ (n >> 16)) >>> 0) / 4294967296;
};
const vnoise = (x, y) => {
  const xi = Math.floor(x), yi = Math.floor(y), xf = x - xi, yf = y - yi;
  const u = xf * xf * (3 - 2 * xf), v = yf * yf * (3 - 2 * yf);
  const a = hash(xi, yi), b = hash(xi + 1, yi), c = hash(xi, yi + 1), d = hash(xi + 1, yi + 1);
  return a * (1 - u) * (1 - v) + b * u * (1 - v) + c * (1 - u) * v + d * u * v;
};
const fbm = (x, y) => {
  let sum = 0, amp = 0.5, f = 1;
  for (let o = 0; o < 4; o++) { sum += amp * vnoise(x * f, y * f); f *= 2; amp *= 0.5; }
  return sum;
};

// a handful of peaks/hills of varying size
const peaks = [];
const N = 3 + Math.floor(rng() * 3);
for (let i = 0; i < N; i++)
  peaks.push({ x: (0.12 + rng() * 0.76) * W, y: (0.12 + rng() * 0.76) * H, amp: 0.55 + rng() * 0.95, r: 150 + rng() * 300 });

const height = (px, py) => {
  // domain warp so hills aren't symmetric blobs
  const wx = 70 * fbm(px / 230 + 11, py / 230 + 3);
  const wy = 70 * fbm(px / 230 + 4, py / 230 + 19);
  let h = 0;
  for (const p of peaks) {
    const dx = px + wx - p.x, dy = py + wy - p.y;
    h += p.amp * Math.exp(-(dx * dx + dy * dy) / (2 * p.r * p.r));
  }
  h += 0.45 * fbm(px / 300, py / 300); // broad rolling terrain
  return h;
};

// sample grid
const GX = 200, GY = 125, dx = W / GX, dy = H / GY;
const X = [...Array(GX + 1)].map((_, i) => i * dx);
const Y = [...Array(GY + 1)].map((_, j) => j * dy);
const V = [];
let lo = Infinity, hi = -Infinity;
for (let i = 0; i <= GX; i++) { V[i] = []; for (let j = 0; j <= GY; j++) { const v = height(X[i], Y[j]); V[i][j] = v; if (v < lo) lo = v; if (v > hi) hi = v; } }

// marching squares per level
const lerp = (ax, ay, bx, by, va, vb, L) => { let t = (L - va) / (vb - va); if (!isFinite(t)) t = 0.5; return [ax + (bx - ax) * t, ay + (by - ay) * t]; };
const LEVELS = 24;
let pathsIndex = '', pathsInter = '';
for (let k = 1; k < LEVELS; k++) {
  const L = lo + ((hi - lo) * k) / LEVELS;
  let segs = '';
  for (let i = 0; i < GX; i++) for (let j = 0; j < GY; j++) {
    const v0 = V[i][j], v1 = V[i + 1][j], v2 = V[i + 1][j + 1], v3 = V[i][j + 1];
    const c = (v0 >= L ? 1 : 0) | (v1 >= L ? 2 : 0) | (v2 >= L ? 4 : 0) | (v3 >= L ? 8 : 0);
    if (c === 0 || c === 15) continue;
    const T = () => lerp(X[i], Y[j], X[i + 1], Y[j], v0, v1, L);
    const R = () => lerp(X[i + 1], Y[j], X[i + 1], Y[j + 1], v1, v2, L);
    const B = () => lerp(X[i], Y[j + 1], X[i + 1], Y[j + 1], v3, v2, L);
    const Lf = () => lerp(X[i], Y[j], X[i], Y[j + 1], v0, v3, L);
    const seg = (a, b) => { segs += `M${a[0].toFixed(1)} ${a[1].toFixed(1)}L${b[0].toFixed(1)} ${b[1].toFixed(1)}`; };
    switch (c) {
      case 1: case 14: seg(T(), Lf()); break;
      case 2: case 13: seg(T(), R()); break;
      case 3: case 12: seg(Lf(), R()); break;
      case 4: case 11: seg(R(), B()); break;
      case 6: case 9: seg(T(), B()); break;
      case 7: case 8: seg(Lf(), B()); break;
      case 5: seg(T(), Lf()); seg(R(), B()); break;
      case 10: seg(T(), R()); seg(Lf(), B()); break;
    }
  }
  if (k % 5 === 0) pathsIndex += segs; else pathsInter += segs;
}
const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${W} ${H}" preserveAspectRatio="xMidYMid slice">
<g fill="none" stroke="#000" stroke-linecap="round" stroke-linejoin="round">
<path stroke-width="0.85" d="${pathsInter}"/>
<path stroke-width="1.7" d="${pathsIndex}"/>
</g></svg>`;
require('fs').writeFileSync('static/contours.svg', svg);
console.log('seed', SEED, '| peaks', N, '| bytes', svg.length);
