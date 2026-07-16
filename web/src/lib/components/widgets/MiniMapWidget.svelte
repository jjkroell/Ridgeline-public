<!--
  Compact node map for the Overview dashboard. Leaflet + CARTO raster tiles (no
  WebGL, so it renders for everyone), non-interactive — the whole card links to
  the full /map. Dots are colored by node role and the view fits their bounds.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import WidgetShell from './WidgetShell.svelte';
	import { overview } from '$lib/overview.svelte';
	import { theme } from '$lib/theme.svelte';
	import { isLight } from '$lib/map-util';
	import { MAP_CENTER_LATLON } from '$lib/site';
	import { roleColor } from '$lib/format';
	import type { Node } from '$lib/api';

	let { base = '', nodes = [] }: { base?: string; nodes?: Node[] } = $props();
	const m = overview.meta('minimap')!;

	let el: HTMLDivElement;
	/* eslint-disable @typescript-eslint/no-explicit-any */
	let L: any = null;
	let map: any = null;
	let tiles: any = null;
	let layer: any = null;
	/* eslint-enable @typescript-eslint/no-explicit-any */
	let ro: ResizeObserver | null = null;
	let curLight = false;

	// Exclude gpsSuspect nodes — one corrupt-GPS outlier would blow the fitBounds
	// box out to the whole globe (the full maps hide them for the same reason).
	const located = $derived(
		nodes.filter((n) => n.hasLocation && !n.gpsSuspect && n.latitude != null && n.longitude != null)
	);
	const tileUrl = (light: boolean) =>
		`https://{s}.basemaps.cartocdn.com/${light ? 'light_all' : 'dark_all'}/{z}/{x}/{y}{r}.png`;

	function drawNodes() {
		if (!map || !L) return;
		if (layer) layer.remove();
		layer = L.layerGroup();
		const pts: [number, number][] = [];
		for (const n of located) {
			const lat = n.latitude as number;
			const lon = n.longitude as number;
			pts.push([lat, lon]);
			L.circleMarker([lat, lon], {
				radius: 4,
				color: '#0b1f1a',
				weight: 1,
				fillColor: roleColor(n.role),
				fillOpacity: 0.95
			}).addTo(layer);
		}
		layer.addTo(map);
		// fitBounds is a no-op (or wrong) while the container has zero size, so only
		// fit once Leaflet reports real dimensions (the ResizeObserver re-fits later).
		if (pts.length && map.getSize().x > 0) {
			map.fitBounds(L.latLngBounds(pts), { padding: [24, 24], maxZoom: 11 });
		}
	}

	onMount(() => {
		let destroyed = false;
		(async () => {
			await import('leaflet/dist/leaflet.css');
			L = (await import('leaflet')).default ?? (await import('leaflet'));
			if (destroyed || !el) return;
			curLight = isLight();
			map = L.map(el, {
				center: MAP_CENTER_LATLON,
				zoom: 8,
				zoomControl: false,
				attributionControl: false,
				dragging: false,
				scrollWheelZoom: false,
				doubleClickZoom: false,
				boxZoom: false,
				keyboard: false,
				touchZoom: false
			});
			tiles = L.tileLayer(tileUrl(curLight), { subdomains: 'abcd', maxZoom: 19 }).addTo(map);
			drawNodes();
			// The card lays out after mount; when the container first gets a real
			// size, invalidate + fit so the map isn't stuck at the world view.
			ro = new ResizeObserver(() => {
				if (!map) return;
				map.invalidateSize();
				drawNodes();
			});
			ro.observe(el);
		})();
		return () => {
			destroyed = true;
			if (ro) ro.disconnect();
			if (map) map.remove();
			map = null;
		};
	});

	// Redraw dots when the located set changes.
	$effect(() => {
		void located.length;
		if (map) drawNodes();
	});

	// Swap tiles on theme change.
	$effect(() => {
		void theme.mode;
		if (!map || !L) return;
		const light = isLight();
		if (light === curLight) return;
		curLight = light;
		if (tiles) tiles.remove();
		tiles = L.tileLayer(tileUrl(light), { subdomains: 'abcd', maxZoom: 19 }).addTo(map);
	});
</script>

<WidgetShell title={m.title} icon={m.icon} href="{base}/map" linkLabel="Full map →">
	<a href="{base}/map" class="relative block" aria-label="Open full map">
		<div bind:this={el} class="h-60 w-full" style="background:var(--color-ink-2)"></div>
		<div class="absolute inset-0"></div>
		<div class="label text-fg-dim bg-panel/80 absolute bottom-2 right-2 rounded-[var(--radius)] px-2 py-1 backdrop-blur">
			{located.length} located
		</div>
	</a>
</WidgetShell>
