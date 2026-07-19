// Shared filter state + ordering for the Nodes lists, so desktop (/nodes) and
// mobile (/m/nodes) constrain and sort the inventory identically. Each page
// creates its own instance; nothing is persisted.
import type { Node } from '$lib/api';
import { lastHeard } from '$lib/format';
import { favorites } from '$lib/favorites.svelte';
import { auth } from '$lib/auth.svelte';

/** Role values as the API reports them — NOT display labels. The mobile chips
 *  previously filtered on 'Companion', which no node ever reports, so that chip
 *  matched nothing; the value here is the wire value 'ChatNode'. */
export type RoleKey = 'all' | 'Repeater' | 'ChatNode' | 'RoomServer' | 'Sensor';

export const ROLE_OPTIONS: { key: RoleKey; label: string }[] = [
	{ key: 'all', label: 'All roles' },
	{ key: 'Repeater', label: 'Repeaters' },
	{ key: 'ChatNode', label: 'Companions' },
	{ key: 'RoomServer', label: 'Rooms' },
	{ key: 'Sensor', label: 'Sensors' }
];

const roleLabel = (k: RoleKey) => ROLE_OPTIONS.find((r) => r.key === k)?.label ?? '';

export class NodeFilters {
	query = $state('');
	role = $state<RoleKey>('all');
	favOnly = $state(false);
	claimedOnly = $state(false);

	/** Filters in effect, excluding the search box (which is always visible). */
	get activeCount(): number {
		return (this.role !== 'all' ? 1 : 0) + (this.favOnly ? 1 : 0) + (this.claimedOnly ? 1 : 0);
	}

	/** Reads the active filters back on the control itself, e.g. "Repeaters · Claimed",
	 *  so the constraint on the list is legible without opening the modal. */
	get summary(): string {
		const parts: string[] = [];
		if (this.role !== 'all') parts.push(roleLabel(this.role));
		if (this.favOnly) parts.push('Favorites');
		if (this.claimedOnly) parts.push('Claimed');
		return parts.join(' · ');
	}

	clear() {
		this.role = 'all';
		this.favOnly = false;
		this.claimedOnly = false;
	}

	/** Apply the filters, then order the result. */
	apply(nodes: Node[]): Node[] {
		const term = this.query.trim().toLowerCase();
		const list = nodes.filter((n) => {
			if (this.favOnly && !favorites.has(n.publicKey)) return false;
			if (this.claimedOnly && !n.claimed) return false;
			if (this.role !== 'all' && n.role !== this.role) return false;
			if (!term) return true;
			return (
				(n.name ?? '').toLowerCase().includes(term) || n.publicKey.toLowerCase().includes(term)
			);
		});

		// When filtering to claimed nodes, ownership leads: your own nodes first,
		// then other operators'. Otherwise favorites pin to the top. Ties break on
		// lastHeard — the same value the lists render in their "Heard" column, so
		// the order always matches what's on screen. Sorting on lastSeen instead
		// would put a relay-only node (recent relay, stale advert) below a node
		// showing an older "Heard", which reads as a broken sort.
		const rank = (n: Node) => ({
			mine: this.claimedOnly && auth.ownsNode(n.publicKey) ? 1 : 0,
			fav: favorites.has(n.publicKey) ? 1 : 0
		});
		return [...list].sort((a, b) => {
			const ra = rank(a);
			const rb = rank(b);
			if (ra.mine !== rb.mine) return rb.mine - ra.mine;
			if (ra.fav !== rb.fav) return rb.fav - ra.fav;
			return (lastHeard(b) ?? '').localeCompare(lastHeard(a) ?? '');
		});
	}
}
