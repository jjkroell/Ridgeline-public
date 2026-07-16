// Renderer-agnostic propagation-pulse engine for the live map. It owns all the
// lon/lat geometry — resolving a packet's relay-hop prefixes to located nodes,
// animating a comet along that path, and scheduling node ripples — and emits
// per-frame geometry that a renderer turns into pixels. The MapLibre live map
// feeds the output into GeoJSON sources; the WebGL-free Leaflet fallback draws it
// onto a 2-D canvas. Keeping the logic here means both stay identical.
import type { Node, LiveEvent } from './api';

export type LngLat = [number, number];

const FADE = 700;
const RIPPLE_DUR = 650;
const TRAIL_NODES = 2;

const dist2 = (a: LngLat, b: LngLat) => (a[0] - b[0]) ** 2 + (a[1] - b[1]) ** 2;

// A packet's hops are all at the originating node's hash size, so the hop length
// is the prefix length to match. Each hop may match several located nodes (a
// short/1-byte hop is ambiguous); resolve by: (1) preferring repeaters, (2)
// anchoring hops with a single candidate, (3) picking the rest nearest a resolved
// neighbour. Returns the located points in path order plus whether any hop was
// ambiguous (so the path can be drawn with less confidence).
export function resolvePath(
	located: Node[],
	path: string[]
): { pts: LngLat[]; uncertain: boolean } {
	const cands: LngLat[][] = path.map((hop) => {
		let c = located.filter((n) => n.publicKey.startsWith(hop));
		const reps = c.filter((n) => n.role === 'Repeater');
		if (reps.length) c = reps;
		return c.map((n) => [n.longitude!, n.latitude!] as LngLat);
	});

	const resolved: (LngLat | null)[] = path.map(() => null);
	let uncertain = false;

	for (let i = 0; i < cands.length; i++) if (cands[i].length === 1) resolved[i] = cands[i][0];

	for (let i = 0; i < cands.length; i++) {
		if (resolved[i] || cands[i].length === 0) continue;
		uncertain = true;
		let ref: LngLat | null = null;
		for (let d = 1; d < cands.length && !ref; d++) {
			if (i - d >= 0 && resolved[i - d]) ref = resolved[i - d];
			else if (i + d < cands.length && resolved[i + d]) ref = resolved[i + d];
		}
		resolved[i] = ref
			? cands[i].reduce((best, p) => (dist2(p, ref!) < dist2(best, ref!) ? p : best))
			: cands[i][0];
	}

	return { pts: resolved.filter((p): p is LngLat => p !== null), uncertain };
}

interface Anim {
	pts: LngLat[];
	seglen: number[]; // cumulative length at each vertex
	total: number;
	color: string;
	born: number;
	dur: number;
	uncertain: boolean;
}
interface Ripple {
	at: LngLat;
	born: number; // when the dot reaches this node (may be in the future)
	color: string;
	played?: boolean; // ripple has begun (emitted as an arrival once)
}

export interface PulseLine {
	coords: LngLat[];
	color: string;
	opacity: number;
}
export interface PulseRing {
	at: LngLat;
	r: number; // radius factor (renderer scales to pixels)
	o: number; // opacity
	color: string;
}
export interface PulseFrame {
	lines: PulseLine[];
	dots: { at: LngLat; color: string }[];
	rings: PulseRing[];
	count: number; // active comets (for the on-screen counter)
	arrivals: LngLat[]; // nodes the dot reached this frame (for audio)
}

export class PulseEngine {
	private anims: Anim[] = [];
	private ripples: Ripple[] = [];
	// Fire-on-arrival dedupe: "hash:path" -> time. Observers report the same
	// transmission over several seconds, so firing on arrival animates the flood
	// spreading in real time rather than dumping every branch at once.
	private fired = new Map<string, number>();
	private colorFor: (ev: LiveEvent) => string;

	// colorFor picks a comet's colour from its event — keyed on the message hash
	// so the pulse matches its Recent Packets row (see hashColor in live.svelte).
	constructor(colorFor: (ev: LiveEvent) => string) {
		this.colorFor = colorFor;
	}

	private addAnim(pts: LngLat[], color: string, uncertain: boolean) {
		const seglen = [0];
		for (let i = 1; i < pts.length; i++) {
			const dx = pts[i][0] - pts[i - 1][0],
				dy = pts[i][1] - pts[i - 1][1];
			seglen.push(seglen[i - 1] + Math.hypot(dx, dy));
		}
		const total = seglen[seglen.length - 1];
		const born = performance.now();
		const dur = 900 + pts.length * 280;
		this.anims.push({ pts, seglen, total, color, born, dur, uncertain });
		if (this.anims.length > 60) this.anims.shift();

		for (let i = 0; i < pts.length; i++) {
			this.ripples.push({
				at: pts[i],
				born: born + (total > 0 ? seglen[i] / total : 0) * dur,
				color
			});
		}
		if (this.ripples.length > 500) this.ripples.splice(0, this.ripples.length - 500);
	}

	// Ingest the newest live events, pulsing each freshly-seen header path once.
	ingest(events: LiveEvent[], located: Node[]) {
		const now = performance.now();
		for (const ev of events.slice(0, 60)) {
			if (!ev.path || ev.path.length < 1) continue;
			// Only pulse genuinely fresh arrivals — skip the last-hour history the
			// feed seeds into the shared buffer, so the map doesn't burst on load.
			if (Date.now() - +new Date(ev.receivedAt) > 20000) continue;
			const key = ev.messageHash + ':' + ev.path.join(',');
			if (this.fired.has(key)) continue;
			this.fired.set(key, now);
			const { pts, uncertain } = resolvePath(located, ev.path);
			if (pts.length >= 2) this.addAnim(pts, this.colorFor(ev), uncertain);
		}
		const cutoff = now - 90000;
		for (const [k, t] of this.fired) if (t < cutoff) this.fired.delete(k);
	}

	// position along the polyline at fraction t (0..1)
	private along(a: Anim, t: number): LngLat {
		const d = t * a.total;
		for (let i = 1; i < a.pts.length; i++) {
			if (d <= a.seglen[i]) {
				const segStart = a.seglen[i - 1];
				const f = (d - segStart) / (a.seglen[i] - segStart || 1);
				return [
					a.pts[i - 1][0] + (a.pts[i][0] - a.pts[i - 1][0]) * f,
					a.pts[i - 1][1] + (a.pts[i][1] - a.pts[i - 1][1]) * f
				];
			}
		}
		return a.pts[a.pts.length - 1];
	}

	// The visible "comet" line: from the head position back ~two nodes behind it.
	private trailCoords(a: Anim, travel: number): LngLat[] {
		const d = travel * a.total;
		let lastV = 0;
		for (let i = 1; i < a.pts.length; i++) {
			if (a.seglen[i] <= d) lastV = i;
			else break;
		}
		const tailV = Math.max(0, lastV - (TRAIL_NODES - 1));
		const coords = a.pts.slice(tailV, lastV + 1);
		if (d > a.seglen[lastV]) coords.push(this.along(a, travel)); // head mid-segment
		return coords;
	}

	// Compute the geometry to render this frame. `arrivals` lists nodes the dot
	// just reached (caller may play a sound per arrival).
	frame(now: number): PulseFrame {
		const lines: PulseLine[] = [];
		const dots: { at: LngLat; color: string }[] = [];
		this.anims = this.anims.filter((a) => now - a.born < a.dur + FADE);
		for (const a of this.anims) {
			const age = now - a.born;
			const travel = Math.min(1, age / a.dur);
			const base = a.uncertain ? 0.45 : 0.85;
			const opacity = age < a.dur ? base : base * (1 - (age - a.dur) / FADE);
			const trail = this.trailCoords(a, travel);
			if (trail.length >= 2) lines.push({ coords: trail, color: a.color, opacity });
			if (age < a.dur) dots.push({ at: this.along(a, travel), color: a.color });
		}

		const rings: PulseRing[] = [];
		const arrivals: LngLat[] = [];
		this.ripples = this.ripples.filter((r) => now - r.born < RIPPLE_DUR);
		for (const r of this.ripples) {
			const age = now - r.born;
			if (age < 0) continue; // scheduled but not yet reached
			if (!r.played) {
				r.played = true;
				arrivals.push(r.at);
			}
			const t = age / RIPPLE_DUR;
			rings.push({ at: r.at, r: 3 + t * 11, o: 0.85 * (1 - t), color: r.color });
		}

		return { lines, dots, rings, count: this.anims.length, arrivals };
	}
}
