/// <reference types="@sveltejs/kit" />
/// <reference lib="webworker" />
import { build, files, version } from '$service-worker';

const sw = self as unknown as ServiceWorkerGlobalScope;

// A caching SW against the Vite dev server is a foot-gun: it caches the dev app
// shell, whose module URLs change on every restart, leaving stale references that
// 404. So in dev the SW is a self-destructing no-op (clears caches + unregisters);
// full PWA caching only runs against a production build.
const DEV = import.meta.env.DEV;

const CACHE = `ridgeline-${version}`;
const SHELL = [...build, ...files];

sw.addEventListener('install', (event) => {
	sw.skipWaiting();
	if (DEV) return;
	event.waitUntil(caches.open(CACHE).then((c) => c.addAll(SHELL)));
});

sw.addEventListener('activate', (event) => {
	event.waitUntil(
		(async () => {
			if (DEV) {
				// Dev: drop everything and unregister so a previously-cached bad shell
				// can't keep serving 404s.
				for (const key of await caches.keys()) await caches.delete(key);
				await sw.clients.claim();
				await sw.registration.unregister();
				return;
			}
			// Keep the current cache AND the immediately previous one. A client still
			// running the previous build needs to lazy-load that build's hashed chunks,
			// which the new deploy has deleted from the server — serving them from the
			// retained cache is what stops mobile getting stuck on "Loading…" after a
			// redeploy. caches.keys() is insertion-ordered (oldest first), so the last
			// two entries are the previous + current builds; older caches are pruned.
			const mine = (await caches.keys()).filter((k) => k.startsWith('ridgeline-'));
			const keep = new Set([CACHE, ...mine.slice(-2)]);
			for (const k of mine) if (!keep.has(k)) await caches.delete(k);
			await sw.clients.claim();
		})()
	);
});

sw.addEventListener('fetch', (event) => {
	if (DEV) return; // pass through to the network in dev
	const req = event.request;
	if (req.method !== 'GET') return;
	const url = new URL(req.url);
	if (url.origin !== location.origin) return;

	// Hashed, immutable build assets — including lazily-loaded route chunks.
	// Cache-first across ALL cache versions (caches.match searches every cache),
	// so a client that updated mid-session can still load a previous build's chunk
	// from the retained cache instead of 404-ing on a file the deploy removed.
	if (url.pathname.startsWith('/_app/')) {
		event.respondWith(caches.match(req).then((hit) => hit ?? fetch(req)));
		return;
	}

	// Other app-shell + static assets → cache-first.
	if (SHELL.includes(url.pathname)) {
		event.respondWith(caches.match(req).then((hit) => hit ?? fetch(req)));
		return;
	}

	// API reads → network-first, fall back to the last cached response offline.
	if (url.pathname.startsWith('/api/')) {
		event.respondWith(
			(async () => {
				try {
					const res = await fetch(req);
					if (res.ok) (await caches.open(CACHE)).put(req, res.clone());
					return res;
				} catch {
					const cached = await caches.match(req);
					if (cached) return cached;
					return new Response(JSON.stringify({ error: 'offline' }), {
						status: 503,
						headers: { 'content-type': 'application/json' }
					});
				}
			})()
		);
		return;
	}

	// SPA navigations → network-first, fall back to the cached shell offline.
	if (req.mode === 'navigate') {
		event.respondWith(
			(async () => {
				try {
					return await fetch(req);
				} catch {
					return (
						(await caches.match('/index.html')) ??
						(await caches.match('/')) ??
						new Response('offline', { status: 503 })
					);
				}
			})()
		);
	}
});
