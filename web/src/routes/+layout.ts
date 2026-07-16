// Pure client-side SPA: no server-side rendering or prerendering. The Go
// daemon serves the static bundle and falls back to index.html.
import { browser } from '$app/environment';
import { redirect } from '@sveltejs/kit';
import type { LayoutLoad } from './$types';

export const ssr = false;
export const prerender = false;

// Desktop section roots that have a mobile equivalent under /m. Dynamic detail
// routes (/nodes/[k], /observers/[id]) are omitted until their /m screens exist.
const MOBILE_ROOTS = ['/', '/nodes', '/live', '/map', '/live-map', '/analytics', '/channels', '/observers', '/admin'];

// Send phones to the mobile app.
export const load: LayoutLoad = ({ url }) => {
	if (!browser) return;
	if (url.pathname === '/m' || url.pathname.startsWith('/m/')) return;
	const isPhone = window.matchMedia('(pointer: coarse)').matches && window.innerWidth <= 820;
	if (isPhone && MOBILE_ROOTS.includes(url.pathname)) {
		redirect(307, url.pathname === '/' ? '/m' : '/m' + url.pathname);
	}
};
