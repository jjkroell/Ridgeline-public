// Reactive WebSocket connection to /api/live. Exposes a rolling buffer of the
// most recent live events plus connection state, as Svelte 5 runes.
import { api, type LiveEvent, type LiveNode } from './api';

// Keep roughly the last hour of events in the rolling buffer. The live feed
// renders this whole window; live-map takes its own newest-N slices from it.
const RETAIN_MS = 60 * 60 * 1000;
// Hard safety cap so a busy mesh can't grow the buffer without bound.
const MAX_EVENTS = 5000;

/** A collapsed set of related live events shown as one feed row. */
export interface LiveGroup {
	key: string;
	/** 'node' = all of a node's packets of one type; 'hash' = one transmission. */
	kind: 'node' | 'hash';
	payloadType: string;
	routeType: string;
	node?: LiveNode;
	messageHash: string; // representative (latest)
	events: LiveEvent[]; // underlying observations, newest first
	count: number;
	observers: string[];
	bestSnr?: number;
	latest: string;
}

/**
 * Stable, vivid colour for a packet hash. Used to tie a live-map comet to its
 * Recent Packets row: both the animated pulse and the row's flag derive their
 * colour from the same message hash, so it's obvious which comet is which. The
 * hex hash is folded to a hue; saturation/lightness are fixed to read on the
 * dark map and keep distinct packets visually separable.
 */
export function hashColor(hash: string): string {
	let h = 0;
	for (let i = 0; i < hash.length; i++) h = (Math.imul(h, 31) + hash.charCodeAt(i)) >>> 0;
	return `hsl(${h % 360}, 78%, 62%)`;
}

/**
 * Collapse a newest-first list of live events into feed groups. Adverts (and
 * any node-resolved packet) group by node + payload type, so all of node X's
 * adverts share one row; packets with no resolved node group by message hash,
 * so one transmission heard by many observers is a single row.
 */
export function groupLive(events: LiveEvent[]): LiveGroup[] {
	const map = new Map<string, LiveGroup>();
	for (const ev of events) {
		const key = ev.node ? `node:${ev.node.publicKey}:${ev.payloadType}` : `hash:${ev.messageHash}`;
		let g = map.get(key);
		if (!g) {
			g = {
				key,
				kind: ev.node ? 'node' : 'hash',
				payloadType: ev.payloadType,
				routeType: ev.routeType,
				node: ev.node,
				messageHash: ev.messageHash,
				events: [],
				count: 0,
				observers: [],
				latest: ev.receivedAt
			};
			map.set(key, g);
		}
		g.events.push(ev);
		g.count++;
		if (ev.snr != null && (g.bestSnr == null || ev.snr > g.bestSnr)) g.bestSnr = ev.snr;
		if (ev.observerId && !g.observers.includes(ev.observerId)) g.observers.push(ev.observerId);
	}
	return [...map.values()];
}

class LiveFeed {
	events = $state<LiveEvent[]>([]);
	connected = $state(false);
	total = $state(0);

	#ws: WebSocket | null = null;
	#retry = 0;
	#timer: ReturnType<typeof setTimeout> | null = null;
	#started = false;

	start() {
		if (this.#started) return;
		this.#started = true;
		this.#hydrate();
		this.#connect();
	}

	// Seed the buffer with the last hour of history so the feed renders
	// immediately instead of waiting for fresh packets.
	async #hydrate() {
		try {
			const recent = await api.recent(3600);
			const cutoff = Date.now() - RETAIN_MS;
			const seed = recent.filter((e) => +new Date(e.receivedAt) >= cutoff);
			// Any events that streamed in while fetching take precedence; append
			// the (older) history after them, de-duped by observer+hash+time.
			const seen = new Set(this.events.map((e) => e.observerId + e.messageHash + e.receivedAt));
			const merged = [
				...this.events,
				...seed.filter((e) => !seen.has(e.observerId + e.messageHash + e.receivedAt))
			];
			this.events = merged.slice(0, MAX_EVENTS);
		} catch {
			/* history is best-effort; live stream still works */
		}
	}

	#connect() {
		const proto = location.protocol === 'https:' ? 'wss' : 'ws';
		const ws = new WebSocket(`${proto}://${location.host}/api/live`);
		this.#ws = ws;

		ws.onopen = () => {
			this.connected = true;
			this.#retry = 0;
		};
		ws.onmessage = (e) => {
			try {
				const ev = JSON.parse(e.data) as LiveEvent;
				const cutoff = Date.now() - RETAIN_MS;
				this.events = [ev, ...this.events]
					.filter((x) => +new Date(x.receivedAt) >= cutoff)
					.slice(0, MAX_EVENTS);
				this.total += 1;
			} catch {
				/* ignore malformed frames */
			}
		};
		ws.onclose = () => {
			this.connected = false;
			this.#scheduleReconnect();
		};
		ws.onerror = () => ws.close();
	}

	#scheduleReconnect() {
		if (this.#timer) return;
		const delay = Math.min(1000 * 2 ** this.#retry, 15000);
		this.#retry += 1;
		this.#timer = setTimeout(() => {
			this.#timer = null;
			this.#connect();
		}, delay);
	}
}

export const live = new LiveFeed();
