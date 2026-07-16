// MapLibre GL requires a WebGL context. Some users disable WebGL (commonly to
// reduce fingerprinting surface), which leaves every MapLibre map blank. We probe
// once here so the map views can fall back to a plain raster map (Leaflet) and
// skip constructing MapLibre entirely when WebGL is unavailable.
let cached: boolean | null = null;

/** True if the browser can give us a WebGL context. Cached after the first probe. */
export function hasWebGL(): boolean {
	if (cached !== null) return cached;
	if (typeof document === 'undefined') return false; // SSR/prerender
	try {
		const canvas = document.createElement('canvas');
		const gl =
			canvas.getContext('webgl2') ||
			canvas.getContext('webgl') ||
			canvas.getContext('experimental-webgl');
		cached = !!gl && typeof (gl as WebGLRenderingContext).getParameter === 'function';
	} catch {
		cached = false;
	}
	return cached;
}
