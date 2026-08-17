<!--
  A small, locked Leaflet map thumbnail of a node's location, shown on the node
  detail views when WebGL is disabled (MapLibre can't run). Mirrors the locked
  MapLibre inset: the themed "Hillshade" basemap (CARTO base + Esri shaded
  relief), a single teal marker, and no interaction so the page scrolls past it
  instead of the map panning/zooming. Tiles come from the shared leafletBasemap
  spec so the raster hillshade stays defined in one place.
  Leaflet + its CSS load lazily (only WebGL-off visitors pay for them).
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { theme } from '$lib/theme.svelte';
	import { isLight } from '$lib/map-util';
	import { leafletBasemap } from '$lib/leaflet-basemap';

	let { lat, lon, zoom = 11 }: { lat: number; lon: number; zoom?: number } = $props();

	let el: HTMLDivElement;
	/* eslint-disable @typescript-eslint/no-explicit-any */
	let L: any = null;
	let map: any = null;
	let tiles: any = null;
	let hillTiles: any = null;
	let dot: any = null;
	/* eslint-enable @typescript-eslint/no-explicit-any */
	let curLight = false;

	// Theme-aware blend + weight, same reasoning as FallbackMap: the relief tiles
	// are grayscale, so on the dark base 'screen' lifts the lit slopes and on the
	// light base 'multiply' drops the shadows in. Plain multiply over dark is
	// near-invisible, and multiply over the near-white light base needs the extra
	// opacity or the relief washes out.
	const hillBlend = (light: boolean) => (light ? 'multiply' : 'screen');
	const hillOpacity = (light: boolean) => (light ? 0.7 : 0.22);

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
			const spec = leafletBasemap('topo', curLight);
			tiles = L.tileLayer(spec.base.url, {
				subdomains: spec.base.subdomains ?? 'abc',
				maxZoom: spec.base.maxZoom,
				attribution: spec.base.attribution
			}).addTo(map);
			if (spec.hillshade) {
				// Dedicated pane so the blend applies to the relief alone: above the
				// base tiles (200), below the marker (400+).
				const pane = map.createPane('hillshade');
				pane.style.zIndex = '250';
				pane.style.pointerEvents = 'none';
				pane.style.mixBlendMode = hillBlend(curLight);
				hillTiles = L.tileLayer(spec.hillshade.url, {
					pane: 'hillshade',
					opacity: hillOpacity(curLight),
					maxZoom: spec.hillshade.maxZoom,
					attribution: spec.hillshade.attribution
				}).addTo(map);
			}
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
		void theme.id;
		const light = isLight();
		if (!map || !tiles || light === curLight) return;
		curLight = light;
		// Only the base is themed — the Esri relief tiles are fixed grayscale, so
		// the theme swap re-tunes its blend and weight rather than its URL.
		tiles.setUrl(leafletBasemap('topo', light).base.url);
		if (hillTiles) {
			map.getPane('hillshade').style.mixBlendMode = hillBlend(light);
			hillTiles.setOpacity(hillOpacity(light));
		}
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
