<!--
  A small, locked Leaflet map thumbnail of a node's location, shown on the node
  detail views when WebGL is disabled (MapLibre can't run). Mirrors the locked
  MapLibre inset: theme-aware CARTO raster tiles, a single teal marker, and no
  interaction so the page scrolls past it instead of the map panning/zooming.
  Leaflet + its CSS load lazily (only WebGL-off visitors pay for them).
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { theme } from '$lib/theme.svelte';
	import { isLight } from '$lib/map-util';

	let { lat, lon, zoom = 11 }: { lat: number; lon: number; zoom?: number } = $props();

	let el: HTMLDivElement;
	/* eslint-disable @typescript-eslint/no-explicit-any */
	let L: any = null;
	let map: any = null;
	let tiles: any = null;
	let dot: any = null;
	/* eslint-enable @typescript-eslint/no-explicit-any */
	let curLight = false;

	const tileUrl = (light: boolean) =>
		`https://{s}.basemaps.cartocdn.com/${light ? 'light_all' : 'dark_all'}/{z}/{x}/{y}{r}.png`;

	onMount(() => {
		let destroyed = false;
		(async () => {
			await import('leaflet/dist/leaflet.css');
			L = (await import('leaflet')).default ?? (await import('leaflet'));
			if (destroyed || !el) return;
			curLight = isLight();
			map = L.map(el, {
				center: [lat, lon],
				zoom,
				zoomControl: false,
				attributionControl: true,
				// Locked thumbnail — every interaction off so wheel/touch scroll the page.
				dragging: false,
				scrollWheelZoom: false,
				doubleClickZoom: false,
				boxZoom: false,
				keyboard: false,
				touchZoom: false
			});
			tiles = L.tileLayer(tileUrl(curLight), {
				subdomains: 'abcd',
				maxZoom: 19,
				attribution:
					'© <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a> © <a href="https://carto.com/attributions" target="_blank" rel="noopener">CARTO</a>'
			}).addTo(map);
			dot = L.circleMarker([lat, lon], {
				radius: 6,
				color: '#0b1f1a',
				weight: 1.5,
				fillColor: '#34e3c4',
				fillOpacity: 1
			}).addTo(map);
			// The container is often laid out (or revealed in a modal) after mount.
			setTimeout(() => map?.invalidateSize(), 80);
		})();
		return () => {
			destroyed = true;
			map?.remove();
			map = null;
		};
	});

	// Recentre/move the marker if the coords change without recreating the map.
	$effect(() => {
		if (!map || !dot) return;
		map.setView([lat, lon], zoom, { animate: false });
		dot.setLatLng([lat, lon]);
	});

	// Swap tile theme when the UI theme toggles.
	$effect(() => {
		void theme.mode;
		const light = isLight();
		if (!map || !tiles || light === curLight) return;
		curLight = light;
		tiles.setUrl(tileUrl(light));
	});
</script>

<div bind:this={el} class="h-full w-full"></div>

<style>
	:global(.leaflet-container) {
		background: var(--color-ink-2);
		font-family: inherit;
	}
	:global(.leaflet-control-attribution) {
		background: color-mix(in srgb, var(--color-ink-2) 80%, transparent);
		color: var(--color-fg-faint);
		font-size: 9px;
	}
	:global(.leaflet-control-attribution a) {
		color: var(--color-fg-dim);
	}
</style>
