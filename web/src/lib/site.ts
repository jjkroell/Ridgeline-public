// Central, build-time site configuration for the public build.
//
// setup.sh writes these values into web/.env from your install answers (as
// VITE_SITE_* variables); Vite inlines them at build time. Everything has a
// sensible default so a plain `npm run build` with no .env still produces a
// working, generic "Ridgeline" site.
//
// These are compile-time constants (baked into the bundle). Runtime, per-deploy
// values that the server knows about are exposed separately via /api/site.

/** Display name shown in the sidebar, tab title suffix, and SEO. */
export const SITE_NAME: string = import.meta.env.VITE_SITE_NAME || 'Ridgeline';

/** Short tagline under the wordmark in the sidebar. */
export const SITE_TAGLINE: string = import.meta.env.VITE_SITE_TAGLINE || 'MeshCore Observatory';

/** Absolute site origin (e.g. https://mesh.example.com), no trailing slash.
 * Empty string -> canonical/OG URLs fall back to path-relative. */
export const SITE_URL: string = (import.meta.env.VITE_SITE_URL || '').replace(/\/+$/, '');

/** One-line description of the mesh/region, used as an SEO fallback. */
export const SITE_DESCRIPTION: string =
	import.meta.env.VITE_SITE_DESCRIPTION ||
	'A live observatory for a MeshCore LoRa mesh network — nodes, coverage and packets in real time.';

/** Privacy / data-controller contact shown on the /privacy page. Empty renders a
 * generic "the operator of this site" with no email; setup.sh fills it in. */
export const PRIVACY_CONTACT: string = import.meta.env.VITE_PRIVACY_CONTACT || '';

// --- Default map view for your region -------------------------------------
// setup.sh writes these from the install Q&A. The default is a wide, neutral
// view so a fresh install with no nodes still shows a sensible map; once you
// pick a region in the installer the maps open centred on it.
export const MAP_CENTER_LAT: number = Number(import.meta.env.VITE_MAP_CENTER_LAT ?? 20);
export const MAP_CENTER_LON: number = Number(import.meta.env.VITE_MAP_CENTER_LON ?? 0);
export const MAP_ZOOM: number = Number(import.meta.env.VITE_MAP_ZOOM ?? 3);

/** Centre as [lon, lat] — the order MapLibre GL expects. */
export const MAP_CENTER_LONLAT: [number, number] = [MAP_CENTER_LON, MAP_CENTER_LAT];
/** Centre as [lat, lon] — the order Leaflet expects. */
export const MAP_CENTER_LATLON: [number, number] = [MAP_CENTER_LAT, MAP_CENTER_LON];
