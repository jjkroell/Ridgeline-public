// Shareable map view state. A map URL captures where you are looking (centre,
// zoom), what you are looking at (basemap, role filter) and, on the live map,
// whether the animation is running — so pasting the link to someone drops them
// on exactly your view.
//
// Deliberate rule: following a shared link NEVER writes to the viewer's saved
// preferences. A link's basemap applies for that visit only; the viewer's own
// stored choice is untouched and returns on their next normal visit. Otherwise
// opening someone else's link would silently reconfigure your app.

import { BASEMAP_IDS } from './map-basemap';

/** Every role the map can filter on. Used to validate an incoming link. */
export const SHAREABLE_ROLES = ['Repeater', 'RoomServer', 'ChatNode', 'Sensor'];

export interface MapView {
	lat?: number;
	lon?: number;
	zoom?: number;
	basemap?: string;
	roles?: string[];
	live?: boolean;
}

/** True when the URL carries any view state at all — i.e. this is a shared
 *  link and the page should honour it instead of its usual default framing. */
export function hasMapView(v: MapView): boolean {
	return v.lat != null || v.zoom != null || v.basemap != null || v.roles != null || v.live != null;
}

function num(sp: URLSearchParams, key: string, lo: number, hi: number): number | undefined {
	const raw = sp.get(key);
	if (raw == null) return undefined;
	const n = Number(raw);
	// Reject NaN and out-of-range rather than letting a malformed link throw the
	// map into an invalid camera.
	if (!Number.isFinite(n) || n < lo || n > hi) return undefined;
	return n;
}

/** Parse view state out of a URL's query string. Unknown or malformed values
 *  are dropped, never thrown — a bad link should degrade to the default view. */
export function parseMapView(url: URL): MapView {
	const sp = url.searchParams;
	const v: MapView = {};

	const lat = num(sp, 'lat', -90, 90);
	const lon = num(sp, 'lon', -180, 180);
	// Centre only counts when BOTH halves are present and valid; half a
	// coordinate is meaningless.
	if (lat != null && lon != null) {
		v.lat = lat;
		v.lon = lon;
	}

	const z = num(sp, 'z', 0, 24);
	if (z != null) v.zoom = z;

	const b = sp.get('base');
	if (b && BASEMAP_IDS.has(b)) v.basemap = b;

	const roles = sp.get('roles');
	if (roles != null) {
		const wanted = roles
			.split(',')
			.map((r) => r.trim())
			.filter((r) => SHAREABLE_ROLES.includes(r));
		// An explicit empty list is legitimate (everything filtered off), so only
		// a list containing nothing valid at all is discarded.
		if (wanted.length > 0 || roles === '') v.roles = wanted;
	}

	if (sp.get('live') === '1') v.live = true;

	return v;
}

/** Write view state onto a URL, dropping keys that are absent. Rounding keeps
 *  the link short and stops a one-pixel pan producing a different string. */
export function applyMapViewToUrl(url: URL, v: MapView): URL {
	const sp = url.searchParams;
	const set = (k: string, val: string | undefined) => (val == null ? sp.delete(k) : sp.set(k, val));

	set('lat', v.lat?.toFixed(5));
	set('lon', v.lon?.toFixed(5));
	set('z', v.zoom?.toFixed(2));
	set('base', v.basemap);
	// Only pin the role filter when it is not the default "everything", so a
	// plain view link stays clean.
	set(
		'roles',
		v.roles && v.roles.length !== SHAREABLE_ROLES.length ? v.roles.join(',') : undefined
	);
	set('live', v.live ? '1' : undefined);
	return url;
}

/** Absolute shareable URL for the current view. */
export function shareUrl(base: URL, v: MapView): string {
	return applyMapViewToUrl(new URL(base.toString()), v).toString();
}

/** Copy text to the clipboard, falling back for non-secure contexts where the
 *  async Clipboard API is unavailable. Resolves false when copying failed so
 *  the caller can show the URL instead of silently doing nothing. */
export async function copyText(text: string): Promise<boolean> {
	try {
		if (navigator.clipboard && window.isSecureContext) {
			await navigator.clipboard.writeText(text);
			return true;
		}
	} catch {
		/* fall through to the legacy path */
	}
	try {
		const ta = document.createElement('textarea');
		ta.value = text;
		ta.setAttribute('readonly', '');
		ta.style.position = 'fixed';
		ta.style.opacity = '0';
		document.body.appendChild(ta);
		ta.select();
		const ok = document.execCommand('copy');
		document.body.removeChild(ta);
		return ok;
	} catch {
		return false;
	}
}
