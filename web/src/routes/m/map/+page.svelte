<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import maplibregl from 'maplibre-gl';
	import 'maplibre-gl/dist/maplibre-gl.css';
	import type { FeatureCollection } from 'geojson';
	import { api, type Node } from '$lib/api';
	import { roleLabel } from '$lib/format';
	import { theme } from '$lib/theme.svelte';
	import { favorites } from '$lib/favorites.svelte';
	import { basemapStyle, basemapHasHillshade, basemapHasLocalTerrain, collapseAttribution, ensureLocalTerrain } from '$lib/map-basemap';
	import { basemap } from '$lib/basemap.svelte';
	import { ensureHillshade } from '$lib/map-hillshade';
	import { ROLE_HEX, FAV_COLOR, locatedNodes } from '$lib/map-util';
	import { computeCoverage, covered, distKm, type CoverageResult } from '$lib/coverage';
	import { MAP_CENTER_LONLAT } from '$lib/site';
	import BasemapSelector from '$lib/components/BasemapSelector.svelte';
	import FallbackMap from '$lib/components/FallbackMap.svelte';
	import { hasWebGL } from '$lib/webgl';

	let mapEl: HTMLDivElement;
	let map: maplibregl.Map | null = null;
	let webglOk = $state(true);
	let fbBanner = $state(true); // WebGL-free banner open? (offsets the basemap selector below it)
	let nodes = $state<Node[]>([]);
	let didFit = false;
	let basemapLight = false;

	// RF coverage prediction
	let coverageMode = $state(false);
	let pin = $state<{ lat: number; lon: number } | null>(null);
	let txHeight = $state(6);
	let rangeKm = $state(15);
	let computing = $state(false);
	let coverage = $state<CoverageResult | null>(null);
	let pinMarker: maplibregl.Marker | null = null;
	let showNodes = $state(false);

	const nodesInCoverage = $derived(
		coverage && pin
			? nodes
					.filter((n) => covered(coverage!, n.longitude!, n.latitude!))
					.map((n) => ({ n, d: distKm(pin!.lat, pin!.lon, n.latitude!, n.longitude!) }))
					.sort((a, b) => a.d - b.d)
			: []
	);

	function placePin(lat: number, lon: number) {
		pin = { lat, lon };
		if (!pinMarker) {
			pinMarker = new maplibregl.Marker({ color: '#e8b454', draggable: true }).setLngLat([lon, lat]).addTo(map!);
			pinMarker.on('dragend', () => { const ll = pinMarker!.getLngLat(); pin = { lat: ll.lat, lon: ll.lng }; runCoverage(); });
		} else pinMarker.setLngLat([lon, lat]);
	}
	function drawCoverage() {
		const src = map?.getSource('coverage') as maplibregl.ImageSource | undefined;
		if (!src) return;
		if (coverage) {
			src.updateImage({ url: coverage.dataUrl, coordinates: coverage.imageCoords });
			map?.setLayoutProperty('coverage', 'visibility', 'visible');
		} else {
			map?.setLayoutProperty('coverage', 'visibility', 'none');
		}
	}
	async function runCoverage() {
		if (!pin) return;
		computing = true;
		try {
			coverage = await computeCoverage({ lat: pin.lat, lon: pin.lon, txHeightM: txHeight, rxHeightM: 2, maxRangeKm: rangeKm });
			drawCoverage();
		} finally {
			computing = false;
		}
	}
	function clearCoverage() {
		coverage = null;
		pin = null;
		pinMarker?.remove();
		pinMarker = null;
		drawCoverage();
	}
	function toggleCoverage() {
		coverageMode = !coverageMode;
		if (!coverageMode) clearCoverage();
	}

	function features(): FeatureCollection {
		return {
			type: 'FeatureCollection',
			features: nodes.map((n) => ({
				type: 'Feature',
				geometry: { type: 'Point', coordinates: [n.longitude!, n.latitude!] },
				properties: {
					color: ROLE_HEX[n.role] ?? '#8394a1',
					name: n.name || n.publicKey.slice(0, 10),
					roleLabel: roleLabel(n.role),
					pubkey: n.publicKey,
					fav: favorites.has(n.publicKey)
				}
			}))
		};
	}
	function updateSource() {
		(map?.getSource('nodes') as maplibregl.GeoJSONSource | undefined)?.setData(features());
		fit();
	}
	function fit() {
		if (!map || didFit || nodes.length === 0) return;
		const b = new maplibregl.LngLatBounds();
		for (const n of nodes) b.extend([n.longitude!, n.latitude!]);
		map.fitBounds(b, { padding: 56, maxZoom: 11, duration: 0 });
		didFit = true;
	}

	function addLayers() {
		if (!map || map.getSource('nodes')) return;
		const blank = document.createElement('canvas');
		blank.width = blank.height = 1;
		map.addSource('coverage', {
			type: 'image',
			url: blank.toDataURL(),
			coordinates: [
				[0, 0.001],
				[0.001, 0.001],
				[0.001, 0],
				[0, 0]
			]
		});
		map.addLayer({
			id: 'coverage',
			type: 'raster',
			source: 'coverage',
			layout: { visibility: 'none' },
			paint: { 'raster-opacity': 0.85, 'raster-resampling': 'linear', 'raster-fade-duration': 0 }
		});
		map.addSource('nodes', { type: 'geojson', data: features() });
		map.addLayer({
			id: 'fav-halo',
			type: 'circle',
			source: 'nodes',
			filter: ['==', ['get', 'fav'], true],
			paint: {
				'circle-radius': ['interpolate', ['linear'], ['zoom'], 4, 6, 11, 11],
				'circle-color': 'transparent',
				'circle-stroke-color': FAV_COLOR,
				'circle-stroke-width': 2
			}
		});
		map.addLayer({
			id: 'node-dots',
			type: 'circle',
			source: 'nodes',
			paint: {
				'circle-radius': ['interpolate', ['linear'], ['zoom'], 4, 3.5, 11, 7],
				'circle-color': ['get', 'color'],
				'circle-stroke-color': 'rgba(0,0,0,0.4)',
				'circle-stroke-width': 1
			}
		});
		map.on('click', 'node-dots', (e) => {
			if (coverageMode) return;
			const f = e.features?.[0];
			const pk = f?.properties?.pubkey as string | undefined;
			if (pk) goto('/m/nodes/' + pk);
		});
		map.on('click', (e) => {
			if (coverageMode) { placePin(e.lngLat.lat, e.lngLat.lng); runCoverage(); }
		});
		map.on('mouseenter', 'node-dots', () => { if (map) map.getCanvas().style.cursor = 'pointer'; });
	}

	let currentBasemap = basemap.id;
	function ensureOverlays() {
		if (!map || !map.isStyleLoaded()) return;
		if (basemapHasHillshade(currentBasemap)) ensureHillshade(map, basemapLight);
		if (basemapHasLocalTerrain(currentBasemap)) ensureLocalTerrain(map, basemapLight);
		if (!map.getSource('nodes')) { addLayers(); updateSource(); drawCoverage(); }
	}

	// Theme swap → restyle basemap, re-add overlays on idle.
	$effect(() => {
		void theme.mode;
		if (!map) return;
		const light = theme.mode === 'light';
		if (light === basemapLight) return;
		basemapLight = light;
		map.setStyle(basemapStyle(currentBasemap, light));
		map.once('idle', ensureOverlays);
	});

	// Swap the basemap when the user picks a different one.
	$effect(() => {
		const id = basemap.id;
		if (!map || id === currentBasemap) return;
		currentBasemap = id;
		basemapLight = theme.mode === 'light';
		map.setStyle(basemapStyle(id, basemapLight));
		map.once('idle', ensureOverlays);
	});

	async function refresh() {
		try {
			nodes = locatedNodes(await api.nodes());
			updateSource();
		} catch {
			/* keep */
		}
	}

	onMount(() => {
		basemap.init();
		webglOk = hasWebGL();
		if (!webglOk) {
			refresh();
			const t = setInterval(refresh, 30000);
			return () => clearInterval(t);
		}
		currentBasemap = basemap.id;
		basemapLight = theme.mode === 'light';
		map = new maplibregl.Map({
			container: mapEl,
			style: basemapStyle(currentBasemap, basemapLight),
			center: MAP_CENTER_LONLAT,
			zoom: 7,
			attributionControl: { compact: true }
		});
		map.addControl(new maplibregl.NavigationControl({ showCompass: true, visualizePitch: true }), 'top-right');
		map.on('load', () => {
			if (!map) return;
			map.resize();
			if (basemapHasHillshade(currentBasemap)) ensureHillshade(map, basemapLight);
			if (basemapHasLocalTerrain(currentBasemap)) ensureLocalTerrain(map, basemapLight);
			addLayers();
			updateSource();
			collapseAttribution(map);
		});
		refresh();
		const t = setInterval(refresh, 15000);
		return () => { clearInterval(t); map?.remove(); map = null; };
	});
</script>

<div class="relative h-full w-full">
	{#if !webglOk}
		<FallbackMap
			nodes={nodes}
			center={MAP_CENTER_LONLAT}
			zoom={7}
			cluster
			bind:bannerOpen={fbBanner}
			{coverage}
			{pin}
			{coverageMode}
			onmapclick={(lat, lon) => {
				pin = { lat, lon };
				runCoverage();
			}}
			onpinmove={(lat, lon) => {
				pin = { lat, lon };
				runCoverage();
			}}
			onselect={(k) => goto('/m/nodes/' + k)}
		/>
		<BasemapSelector compact posClass={fbBanner ? 'top-16 left-3' : 'top-3 left-3'} />
	{:else}
	<div bind:this={mapEl} class="h-full w-full"></div>

	<div class="border-line/60 bg-ink-2/80 pointer-events-none absolute top-3 left-3 z-10 rounded-full border px-3 py-1.5 backdrop-blur-md">
		<span class="text-fg-dim font-mono text-[0.62rem] tnum">{nodes.length} located</span>
	</div>

	<BasemapSelector compact posClass="top-14 left-3" />
	{/if}

	<!-- Coverage toggle (shared by the MapLibre + WebGL-free maps) -->
	<button
		onclick={toggleCoverage}
		class="border-line/60 bg-ink-2/85 absolute {!webglOk && fbBanner ? 'top-16' : 'top-3'} right-14 z-10 flex items-center gap-1.5 rounded-full border px-3 py-1.5 backdrop-blur-md {coverageMode ? 'text-signal border-signal/50' : 'text-fg-dim'}"
	>
		<svg viewBox="0 0 24 24" class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M4.9 4.9a10 10 0 0 0 0 14.2M19.1 4.9a10 10 0 0 1 0 14.2M8 8a5 5 0 0 0 0 8M16 8a5 5 0 0 1 0 8M12 11.2a1 1 0 1 0 0 1.6 1 1 0 0 0 0-1.6z" /></svg>
		<span class="text-[0.62rem] font-600">Coverage</span>
	</button>

	<!-- Coverage panel -->
	{#if coverageMode}
		<div class="border-line/60 bg-ink-2/95 absolute inset-x-2 bottom-2 z-10 rounded-2xl border p-3 backdrop-blur-md">
			{#if !pin}
				<p class="text-fg-faint text-center text-xs">Tap the map to drop a planned repeater.</p>
			{:else}
				<div class="flex items-end gap-2">
					<label class="flex-1">
						<span class="label !text-[0.55rem]">Antenna m</span>
						<input type="number" min="0" bind:value={txHeight} onchange={runCoverage} class="border-line bg-ink text-fg mt-1 w-full rounded-lg border px-2 py-1.5 font-mono text-sm outline-none" />
					</label>
					<label class="flex-1">
						<span class="label !text-[0.55rem]">Range km</span>
						<input type="number" min="1" max="60" bind:value={rangeKm} onchange={runCoverage} class="border-line bg-ink text-fg mt-1 w-full rounded-lg border px-2 py-1.5 font-mono text-sm outline-none" />
					</label>
					<button onclick={runCoverage} disabled={computing} class="border-signal/40 bg-signal/15 text-signal shrink-0 rounded-lg border px-3 py-1.5 text-xs font-600 disabled:opacity-50">{computing ? '…' : 'Go'}</button>
					<button onclick={clearCoverage} class="border-line text-fg-faint shrink-0 rounded-lg border px-2.5 py-1.5 text-xs font-600">×</button>
				</div>
				{#if coverage}
					<button onclick={() => (showNodes = !showNodes)} class="text-fg-faint mt-2 flex w-full items-center justify-between text-[0.62rem]">
						<span>reaches {coverage.maxReachKm.toFixed(1)} km · ground {Number.isFinite(coverage.groundElevM) ? coverage.groundElevM.toFixed(0) + 'm' : '—'}</span>
						<span class="text-signal">{nodesInCoverage.length} nodes {showNodes ? '▾' : '▸'}</span>
					</button>
					{#if showNodes && nodesInCoverage.length}
						<div class="mt-1 max-h-40 space-y-0.5 overflow-y-auto">
							{#each nodesInCoverage as { n, d } (n.publicKey)}
								<a href="/m/nodes/{n.publicKey}" class="active:bg-line/40 flex items-center gap-2 rounded-lg px-1.5 py-1">
									<span class="h-2 w-2 shrink-0 rounded-full" style="background:{ROLE_HEX[n.role] ?? '#8394a1'}"></span>
									<span class="text-fg-dim min-w-0 flex-1 truncate text-xs">{n.name || n.publicKey.slice(0, 10)}</span>
									<span class="text-fg-faint font-mono text-[0.6rem] tnum">{d.toFixed(1)}km</span>
								</a>
							{/each}
						</div>
					{/if}
				{/if}
			{/if}
		</div>
	{/if}
</div>

<style>
	:global(.maplibregl-ctrl-attrib) {
		font-size: 9px;
	}
</style>
