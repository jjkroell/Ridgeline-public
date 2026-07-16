// Display helpers shared across the UI.

export type StatusTier = 'online' | 'idle' | 'offline' | 'unknown';
export interface NodeStatus {
	label: string;
	color: string;
	tier: StatusTier;
}

// True "alive in the mesh" status. A node is alive if it's done ANYTHING
// recently — broadcast its own advert OR relayed someone else's traffic — so we
// score on the most recent of the two. With a ~30h recommended advert interval,
// advert recency alone is a weak signal; relay activity is the strong proof a
// node is up and working. Thresholds: Online <6h, Idle <33h (one advert cycle +
// grace), Offline beyond that.
const HOUR = 3_600_000;
/** The most recent time a node was heard — its latest advert or relay. */
export function lastHeard(n: { lastSeen?: string; lastRelayed?: string }): string | undefined {
	return n.lastRelayed && (!n.lastSeen || n.lastRelayed > n.lastSeen) ? n.lastRelayed : n.lastSeen;
}

export function nodeStatus(n: {
	lastSeen?: string;
	lastRelayed?: string;
}): NodeStatus {
	const times: number[] = [];
	for (const t of [n.lastSeen, n.lastRelayed]) {
		if (t) {
			const ms = new Date(t).getTime();
			if (!Number.isNaN(ms)) times.push(ms);
		}
	}
	if (!times.length) return { label: 'Unknown', color: 'var(--color-fg-faint)', tier: 'unknown' };
	const age = Date.now() - Math.max(...times);
	if (age < 6 * HOUR) return { label: 'Online', color: 'var(--color-signal)', tier: 'online' };
	if (age < 33 * HOUR) return { label: 'Idle', color: 'var(--color-amber)', tier: 'idle' };
	return { label: 'Offline', color: 'var(--color-coral)', tier: 'offline' };
}

const ROLE_COLORS: Record<string, string> = {
	Repeater: 'var(--color-role-repeater)',
	ChatNode: 'var(--color-role-companion)',
	RoomServer: 'var(--color-role-room)',
	Sensor: 'var(--color-role-sensor)',
	Observer: 'var(--color-role-observer)'
};

const ROLE_LABELS: Record<string, string> = {
	Repeater: 'Repeater',
	ChatNode: 'Companion',
	RoomServer: 'Room',
	Sensor: 'Sensor',
	Observer: 'Observer'
};

export function roleColor(role: string): string {
	return ROLE_COLORS[role] ?? 'var(--color-fg-faint)';
}

export function roleLabel(role: string): string {
	return ROLE_LABELS[role] ?? role ?? 'Unknown';
}

/** Short form of a 64-hex public key: AB12…F9 */
export function shortKey(key: string, head = 4, tail = 2): string {
	if (!key) return '—';
	if (key.length <= head + tail) return key;
	return `${key.slice(0, head)}…${key.slice(-tail)}`;
}

/** Compact relative time, e.g. "12s", "4m", "3h", "2d". */
export function ago(iso?: string): string {
	if (!iso) return '—';
	const then = new Date(iso).getTime();
	if (Number.isNaN(then)) return '—';
	const s = Math.max(0, (Date.now() - then) / 1000);
	if (s < 60) return `${Math.floor(s)}s`;
	if (s < 3600) return `${Math.floor(s / 60)}m`;
	if (s < 86400) return `${Math.floor(s / 3600)}h`;
	return `${Math.floor(s / 86400)}d`;
}

/** Maps an SNR value (dB) to a color on the signal scale. */
export function snrColor(snr?: number): string {
	if (snr == null) return 'var(--color-fg-faint)';
	if (snr >= 5) return 'var(--color-lime)';
	if (snr >= -5) return 'var(--color-signal)';
	if (snr >= -12) return 'var(--color-amber)';
	return 'var(--color-coral)';
}

export function fmtSnr(snr?: number): string {
	return snr == null ? '—' : `${snr > 0 ? '+' : ''}${snr.toFixed(1)}`;
}

export function fmtNum(n: number): string {
	return n.toLocaleString('en-US');
}

// Observer clock-skew (median RX-time deviation, ms): tight = healthy clock,
// large = drifting. Shared by the analytics + observer views.
export function skewColor(ms?: number): string {
	if (ms == null) return 'var(--color-fg-faint)';
	const a = Math.abs(ms);
	return a < 50 ? 'var(--color-lime)' : a < 250 ? 'var(--color-amber)' : 'var(--color-coral)';
}

export function fmtSkew(ms?: number): string {
	if (ms == null) return '—';
	return (ms >= 0 ? '+' : '') + Math.round(ms) + ' ms';
}

export function fmtCoord(lat?: number, lon?: number): string {
	if (lat == null || lon == null) return '—';
	return `${lat.toFixed(4)}, ${lon.toFixed(4)}`;
}

/** Format a node's "freq,bw,sf,cr" radio config as "915.0 · 62.5k · SF7 · CR5"
 *  (frequency rounded to 3 decimals). */
export function fmtRadio(r?: string): string {
	if (!r) return '—';
	const [f, b, s, c] = r.split(',');
	const parts: string[] = [];
	if (f) parts.push(`${+(+f).toFixed(3)}`);
	if (b) parts.push(`${b}k`);
	if (s) parts.push(`SF${s}`);
	if (c) parts.push(`CR${c}`);
	return parts.join(' · ') || '—';
}

/** True when a timestamp is within the last 5 minutes (an observer "reporting"). */
export function isFresh(iso?: string): boolean {
	return !!iso && Date.now() - new Date(iso).getTime() < 5 * 60 * 1000;
}
