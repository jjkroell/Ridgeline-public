// Shared helpers for the MapLibre views (/map, /live-map, and the node-detail
// inset). Keeps the role palette, theme probes and the located-node filter in
// one place rather than copied across every map.
import type { Node } from './api';

/** True when the light theme class is active on <html>. */
export function isLight(): boolean {
	return document.documentElement.classList.contains('theme-light');
}

/** Current --color-ink value (used as the node-dot stroke), with a dark fallback. */
export function inkColor(): string {
	return getComputedStyle(document.documentElement).getPropertyValue('--color-ink').trim() || '#070a0e';
}

// Raw hex role colours. MapLibre paint expressions can't read CSS custom
// properties, so the map needs literal hex (the rest of the UI uses the
// --color-role-* vars via format.ts roleColor()). Kept in sync with app.css.
export const ROLE_HEX: Record<string, string> = {
	Repeater: '#ff6b6b',
	ChatNode: '#5b9dff',
	RoomServer: '#6ee7a8',
	Sensor: '#f472b6',
	Observer: '#a78bfa'
};

/** Amber ring drawn around favourited nodes on the maps. */
export const FAV_COLOR = '#e8b454';

/** Nodes with valid, non-suspect coordinates — the set safe to plot. */
export function locatedNodes(nodes: Node[]): Node[] {
	return nodes.filter(
		(n) =>
			n.hasLocation &&
			!n.gpsSuspect &&
			n.latitude != null &&
			n.longitude != null &&
			Math.abs(n.latitude) <= 90 &&
			Math.abs(n.longitude) <= 180
	);
}
