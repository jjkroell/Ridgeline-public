// The user's Overview dashboard layout: which widget cards are shown, and in
// what order. Like favorites/channels/theme, this lives only in the browser
// (localStorage) — no backend or account needed — and is shared by the desktop
// (/) and mobile (/m) Overview pages so a user's layout follows them.
//
// The top stat row (Nodes/Observers/…) is fixed and NOT part of this — only the
// cards below it are user-arrangeable.

export type WidgetSize = 'full' | 'half';

export interface WidgetMeta {
	id: string;
	/** Section heading shown on the card and in the customizer. */
	title: string;
	/** One-line explanation shown under the title in the customizer. */
	desc: string;
	/** Inline SVG path for the customizer icon. */
	icon: string;
	/** Preferred width in the 2-column desktop grid (mobile is always 1 column). */
	size: WidgetSize;
	/** Widget only has content for a signed-in user (still listable, shows a hint). */
	requiresAuth?: boolean;
	/** Whether it's shown by default for a first-time visitor. */
	defaultVisible: boolean;
}

// The widget catalogue. Order here is the default layout order. The first three
// (favorites/live feed/recent nodes) are visible by default to preserve the
// original Overview; the rest are opt-in via the customizer.
export const WIDGETS: WidgetMeta[] = [
	{ id: 'favorites', title: 'Favorites', desc: 'Nodes you’ve pinned', size: 'half', defaultVisible: true,
		icon: 'M12 2.5l2.9 5.9 6.5.95-4.7 4.6 1.1 6.45L12 17.9l-5.8 3.05 1.1-6.45-4.7-4.6 6.5-.95z' },
	{ id: 'livefeed', title: 'Live Feed', desc: 'Latest packets across the mesh', size: 'half', defaultVisible: true,
		icon: 'M2 12h4l3 8 4-16 3 8h6' },
	{ id: 'recentnodes', title: 'Recent Nodes', desc: 'Most recently heard nodes', size: 'half', defaultVisible: true,
		icon: 'M12 8a4 4 0 100 8 4 4 0 000-8zM12 2v4m0 12v4M2 12h4m12 0h4' },
	{ id: 'claimed', title: 'Claimed Nodes', desc: 'Every node on the mesh with a verified owner', size: 'half', defaultVisible: false,
		icon: 'M12 3 4 6v6c0 4.4 3.2 7.6 8 9 4.8-1.4 8-4.6 8-9V6l-8-3zM9 12l2 2 4-4' },
	{ id: 'mynodes', title: 'My Nodes', desc: 'Nodes you’ve claimed, plus ones shared with you', size: 'half', defaultVisible: false, requiresAuth: true,
		icon: 'M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2M12 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8z' },
	{ id: 'newnodes', title: 'New Nodes', desc: 'Nodes first heard in the last 7 days', size: 'half', defaultVisible: false,
		icon: 'M12 5v14M5 12h14' },
	{ id: 'backbone', title: 'Top Relays', desc: 'Busiest relays carrying the mesh right now', size: 'half', defaultVisible: false,
		icon: 'M6 3v12M6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM6 6a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 9a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 6a9 9 0 0 1-9 9' },
	{ id: 'minimap', title: 'Mini Map', desc: 'Compact map of node locations', size: 'full', defaultVisible: false,
		icon: 'M9 3 3 6v15l6-3 6 3 6-3V3l-6 3-6-3zM9 3v15m6-12v15' },
	{ id: 'activity', title: 'Network Activity', desc: 'Active nodes and channel utilisation', size: 'half', defaultVisible: false,
		icon: 'M3 3v18h18M7 14l3-4 3 3 4-6' },
	{ id: 'observers', title: 'Observers', desc: 'Observer health — online status & battery', size: 'half', defaultVisible: false,
		icon: 'M2 12s4-7 10-7 10 7 10 7-4 7-10 7-10-7-10-7zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z' },
	{ id: 'channels', title: 'Channels', desc: 'Latest messages from your saved channels', size: 'half', defaultVisible: false,
		icon: 'M4 9h16M4 15h16M10 3 8 21M16 3l-2 18' }
];

const META = new Map(WIDGETS.map((w) => [w.id, w]));
const KEY = 'ridgeline-overview-layout';

interface Entry {
	id: string;
	visible: boolean;
}

function defaults(): Entry[] {
	return WIDGETS.map((w) => ({ id: w.id, visible: w.defaultVisible }));
}

class Overview {
	// Ordered layout — every known widget appears exactly once (visible or not).
	entries = $state<Entry[]>(defaults());
	// Whether the customize panel is open.
	editing = $state(false);

	init() {
		try {
			const raw = localStorage.getItem(KEY);
			if (raw) {
				const parsed = JSON.parse(raw);
				if (Array.isArray(parsed)) this.entries = this.#reconcile(parsed);
			}
		} catch {
			/* storage unavailable or malformed — keep defaults */
		}
	}

	// Keep saved order/visibility for known widgets, drop unknown ones, and append
	// any widgets added since the layout was saved (so upgrades surface new cards).
	#reconcile(saved: unknown[]): Entry[] {
		const out: Entry[] = [];
		const seen = new Set<string>();
		for (const s of saved) {
			const e = s as Partial<Entry>;
			if (e && typeof e.id === 'string' && META.has(e.id) && !seen.has(e.id)) {
				out.push({ id: e.id, visible: e.visible !== false });
				seen.add(e.id);
			}
		}
		for (const w of WIDGETS) {
			if (!seen.has(w.id)) out.push({ id: w.id, visible: w.defaultVisible });
		}
		return out;
	}

	meta(id: string): WidgetMeta | undefined {
		return META.get(id);
	}

	get visibleIds(): string[] {
		return this.entries.filter((e) => e.visible).map((e) => e.id);
	}

	isVisible(id: string): boolean {
		return this.entries.find((e) => e.id === id)?.visible ?? false;
	}

	toggle(id: string) {
		this.entries = this.entries.map((e) => (e.id === id ? { ...e, visible: !e.visible } : e));
		this.#persist();
	}

	// Reorder: move the dragged widget so it sits at the target widget's position.
	move(fromId: string, toId: string) {
		if (fromId === toId) return;
		const from = this.entries.findIndex((e) => e.id === fromId);
		const to = this.entries.findIndex((e) => e.id === toId);
		if (from < 0 || to < 0) return;
		const next = [...this.entries];
		const [moved] = next.splice(from, 1);
		next.splice(to, 0, moved);
		this.entries = next;
		this.#persist();
	}

	reset() {
		this.entries = defaults();
		this.#persist();
	}

	#persist() {
		try {
			localStorage.setItem(KEY, JSON.stringify(this.entries));
		} catch {
			/* storage unavailable */
		}
	}
}

export const overview = new Overview();
