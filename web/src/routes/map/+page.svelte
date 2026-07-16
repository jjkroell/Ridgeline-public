<script lang="ts">
	import { onMount } from 'svelte';
	import Seo from '$lib/components/Seo.svelte';
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
	import { isLight, inkColor, ROLE_HEX, FAV_COLOR, locatedNodes } from '$lib/map-util';
	import { computeCoverage, covered, distKm, type CoverageResult } from '$lib/coverage';
	import { MAP_CENTER_LONLAT } from '$lib/site';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import MapRoleFilter from '$lib/components/MapRoleFilter.svelte';
	import BasemapSelector from '$lib/components/BasemapSelector.svelte';
	import NodeModal from '$lib/components/NodeModal.svelte';
	import FallbackMap from '$lib/components/FallbackMap.svelte';
	import { hasWebGL } from '$lib/webgl';

	let mapEl: HTMLDivElement;
	let map: maplibregl.Map | null = null;
	let webglOk = $state(true);
	let fbBanner = $state(true); // WebGL-free banner open? (offsets the role filter below it)
	let ready = false;
	let didFit = false;
	let allLocated = $state<Node[]>([]);
	let selectedRoles = $state(new Set(['Repeater', 'RoomServer', 'ChatNode', 'Sensor']));
	let nodeKey = $state<string | null>(null);

	const visible = $derived(allLocated.filter((n) => selectedRoles.has(n.role)));

	// ── RF coverage prediction (terrain line-of-sight from a planned repeater) ──
	let coverageMode = $state(false);
	let pin = $state<{ lat: number; lon: number } | null>(null);
	let txHeight = $state(6);
	let rxHeight = $state(2);
	let rangeKm = $state(15);
	let computing = $state(false);
	let coverage = $state<CoverageResult | null>(null);
	let pinMarker: maplibregl.Marker | null = null;

	// Known located nodes that fall inside the computed coverage, nearest first.
	const nodesInCoverage = $derived(
		coverage && pin
			? allLocated
					.filter((n) => covered(coverage!, n.longitude!, n.latitude!))
					.map((n) => ({ n, d: distKm(pin!.lat, pin!.lon, n.latitude!, n.longitude!) }))
					.sort((a, b) => a.d - b.d)
			: []
	);

	function placePin(lat: number, lon: number) {
		pin = { lat, lon };
		if (!pinMarker) {
			pinMarker = new maplibregl.Marker({ color: '#e8b454', draggable: true })
				.setLngLat([lon, lat])
				.addTo(map!);
			pinMarker.on('dragend', () => {
				const ll = pinMarker!.getLngLat();
				pin = { lat: ll.lat, lon: ll.lng };
				runCoverage();
			});
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
			coverage = await computeCoverage({
				lat: pin.lat,
				lon: pin.lon,
				txHeightM: txHeight,
				rxHeightM: rxHeight,
				maxRangeKm: rangeKm
			});
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

	function nodeFeatures(): FeatureCollection {
		return {
			type: 'FeatureCollection',
			features: visible.map((n) => ({
				type: 'Feature',
				geometry: { type: 'Point', coordinates: [n.longitude!, n.latitude!] },
				properties: {
					role: n.role,
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
		(map?.getSource('nodes') as maplibregl.GeoJSONSource | undefined)?.setData(nodeFeatures());
	}

	// Re-add overlays after a basemap style swap (theme or basemap change) drops them.
	let basemapLight = false;
	let currentBasemap = basemap.id;
	function ensureOverlays() {
		if (!map || !map.isStyleLoaded()) return;
		if (basemapHasHillshade(currentBasemap)) ensureHillshade(map, basemapLight);
		if (basemapHasLocalTerrain(currentBasemap)) ensureLocalTerrain(map, basemapLight);
		if (map.getSource('nodes')) return;
		addLayers();
		updateSource();
		drawCoverage();
	}

	$effect(() => {
		void theme.mode;
		const light = isLight();
		if (!map || light === basemapLight) return;
		basemapLight = light;
		map.setStyle(basemapStyle(currentBasemap, light));
		// styledata fires mid-load with isStyleLoaded()===false, so ensureOverlays
		// bails; `idle` is guaranteed once the new style has fully settled.
		map.once('idle', ensureOverlays);
	});

	// Swap the basemap when the user picks a different one.
	$effect(() => {
		const id = basemap.id;
		if (!map || id === currentBasemap) return;
		currentBasemap = id;
		basemapLight = isLight();
		map.setStyle(basemapStyle(id, basemapLight));
		map.once('idle', ensureOverlays);
	});

	// re-filter / re-style when the role selection or favorites change
	$effect(() => {
		void selectedRoles;
		void favorites.keys;
		if (ready) updateSource();
	});

	function addLayers() {
		if (!map) return;
		// Coverage viewshed (a teal raster with terrain shadows) sits beneath the
		// nodes/clusters. Seeded with a transparent placeholder + hidden until computed.
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
		map.addSource('nodes', {
			type: 'geojson',
			data: nodeFeatures(),
			cluster: true,
			clusterRadius: 46,
			clusterMaxZoom: 11
		});
		map.addLayer({
			id: 'clusters',
			type: 'circle',
			source: 'nodes',
			filter: ['has', 'point_count'],
			paint: {
				'circle-color': '#159e8b',
				'circle-opacity': 0.85,
				'circle-radius': ['step', ['get', 'point_count'], 13, 10, 18, 30, 24],
				'circle-stroke-width': 1.5,
				'circle-stroke-color': '#34e3c4'
			}
		});
		map.addLayer({
			id: 'cluster-count',
			type: 'symbol',
			source: 'nodes',
			filter: ['has', 'point_count'],
			layout: {
				'text-field': ['get', 'point_count_abbreviated'],
				'text-font': ['Noto Sans Regular'],
				'text-size': 12
			},
			paint: { 'text-color': '#04140f' }
		});
		// Amber ring around favorited individual nodes, drawn beneath the dots.
		map.addLayer({
			id: 'fav-halo',
			type: 'circle',
			source: 'nodes',
			filter: ['all', ['!', ['has', 'point_count']], ['==', ['get', 'fav'], true]],
			paint: {
				// Ring hugging the dot: ~2px outside the node radius at each zoom.
				'circle-radius': [
					'interpolate', ['linear'], ['zoom'],
					6, ['match', ['get', 'role'], 'Repeater', 4.5, 3.8],
					11, ['match', ['get', 'role'], 'Repeater', 7, 5.2],
					15, ['match', ['get', 'role'], 'Repeater', 10, 7.5]
				],
				'circle-color': 'rgba(0,0,0,0)',
				'circle-stroke-color': FAV_COLOR,
				'circle-stroke-width': 1.75,
				'circle-stroke-opacity': 0.95
			}
		});
		map.addLayer({
			id: 'unclustered',
			type: 'circle',
			source: 'nodes',
			filter: ['!', ['has', 'point_count']],
			paint: {
				'circle-color': ['get', 'color'],
				// Smaller when zoomed out, scaling up as you zoom in.
				'circle-radius': [
					'interpolate', ['linear'], ['zoom'],
					6, ['match', ['get', 'role'], 'Repeater', 2.5, 1.8],
					11, ['match', ['get', 'role'], 'Repeater', 5, 3.2],
					15, ['match', ['get', 'role'], 'Repeater', 8, 5.5]
				],
				'circle-opacity': 0.9,
				'circle-stroke-width': 1,
				'circle-stroke-color': inkColor()
			}
		});
	}

	// Interaction handlers — bound once. MapLibre keeps layer-id listeners across
	// removeLayer/addLayer, so they survive a basemap style swap.
	function bindEvents() {
		if (!map) return;
		// Cluster click → zoom to expand.
		map.on('click', 'clusters', async (e) => {
			const f = map!.queryRenderedFeatures(e.point, { layers: ['clusters'] })[0];
			const id = f.properties!.cluster_id;
			const src = map!.getSource('nodes') as maplibregl.GeoJSONSource;
			const zoom = await src.getClusterExpansionZoom(id);
			map!.easeTo({ center: (f.geometry as GeoJSON.Point).coordinates as [number, number], zoom });
		});
		// Node click → full node-detail modal (suppressed in coverage mode).
		map.on('click', 'unclustered', (e) => {
			if (coverageMode) return;
			const p = e.features![0].properties as { pubkey?: string };
			if (p?.pubkey) nodeKey = p.pubkey;
		});
		// Coverage mode: a map click drops/moves the transmitter pin and recomputes.
		map.on('click', (e) => {
			if (!coverageMode) return;
			placePin(e.lngLat.lat, e.lngLat.lng);
			runCoverage();
		});
		for (const layer of ['clusters', 'unclustered']) {
			map.on('mouseenter', layer, () => (map!.getCanvas().style.cursor = 'pointer'));
			map.on('mouseleave', layer, () => (map!.getCanvas().style.cursor = ''));
		}
	}

	// Fit to the bulk of nodes, rejecting geographic outliers (bad GPS or far
	// regions) via the 1.5×IQR rule so the view focuses where the mesh is.
	function fitToNodes() {
		if (!map || allLocated.length === 0) return;
		const q = (arr: number[], p: number) =>
			arr[Math.min(arr.length - 1, Math.max(0, Math.round((arr.length - 1) * p)))];
		const whisker = (vals: number[]): [number, number] => {
			const s = [...vals].sort((a, b) => a - b);
			const q1 = q(s, 0.25),
				q3 = q(s, 0.75),
				iqr = q3 - q1;
			return [q1 - 1.5 * iqr, q3 + 1.5 * iqr];
		};
		const [latLo, latHi] = whisker(allLocated.map((n) => n.latitude!));
		const [lonLo, lonHi] = whisker(allLocated.map((n) => n.longitude!));
		const inliers = allLocated.filter(
			(n) =>
				n.latitude! >= latLo && n.latitude! <= latHi && n.longitude! >= lonLo && n.longitude! <= lonHi
		);
		const pts = inliers.length ? inliers : allLocated;
		const b = new maplibregl.LngLatBounds();
		pts.forEach((n) => b.extend([n.longitude!, n.latitude!]));
		map.fitBounds(b, { padding: 80, maxZoom: 12, duration: 600 });
	}

	async function plot() {
		const nodes = await api.nodes();
		allLocated = locatedNodes(nodes);
		// No map = WebGL-free fallback: it renders straight from `allLocated`, so we
		// must still fetch even though the MapLibre source/fit below are skipped.
		if (!map) return;
		if (ready) updateSource();
		if (!didFit && allLocated.length > 0) {
			fitToNodes();
			didFit = true;
		}
	}

	onMount(() => {
		basemap.init();
		webglOk = hasWebGL();
		if (!webglOk) {
			// No WebGL → skip MapLibre entirely; the Leaflet FallbackMap just needs
			// the located-node list, refreshed on the same cadence.
			plot();
			const t = setInterval(plot, 30000);
			return () => clearInterval(t);
		}
		currentBasemap = basemap.id;
		basemapLight = isLight();
		map = new maplibregl.Map({
			container: mapEl,
			style: basemapStyle(currentBasemap, basemapLight),
			center: MAP_CENTER_LONLAT,
			zoom: 9,
			attributionControl: { compact: true }
		});
		map.addControl(new maplibregl.NavigationControl({ showCompass: true, visualizePitch: true }), 'bottom-right');
		map.on('load', () => {
			map?.resize();
			if (basemapHasHillshade(currentBasemap)) ensureHillshade(map!, basemapLight);
			if (basemapHasLocalTerrain(currentBasemap)) ensureLocalTerrain(map!, basemapLight);
			collapseAttribution(map!);
			addLayers();
			bindEvents();
			ready = true;
			plot();
		});
		// Re-add overlays after a basemap (theme) style swap drops them.
		map.on('styledata', ensureOverlays);
		const t = setInterval(plot, 10000);
		return () => {
			clearInterval(t);
			map?.remove();
		};
	});
</script>

<Seo
	title="MeshCore Coverage Map"
	description="Interactive coverage map of the MeshCore LoRa mesh — node and repeater locations and estimated reach."
	path="/map"
/>

<PageHeader eyebrow="Terrain & Topology" title="Network Map">
	<div class="font-mono text-fg-dim text-xs">
		<span class="text-signal tnum">{visible.length}</span> <span class="text-fg-faint">shown</span>
	</div>
</PageHeader>

<div class="px-6 py-6 md:px-10">
	<div class="panel relative overflow-hidden" style="height:calc(100vh - 220px);min-height:420px">
		{#if !webglOk}
			<FallbackMap
				nodes={allLocated}
				roleFilter={selectedRoles}
				center={MAP_CENTER_LONLAT}
				zoom={9}
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
				onselect={(k) => (nodeKey = k)}
			/>
			<MapRoleFilter bind:selected={selectedRoles} pos={fbBanner ? 'top-16' : 'top-3'} />
			<BasemapSelector posClass={fbBanner ? 'top-16 right-3' : 'top-3 right-3'} />
		{:else}
		<div bind:this={mapEl} class="h-full w-full"></div>
		<MapRoleFilter bind:selected={selectedRoles} />
		<BasemapSelector />
		{/if}

		<!-- Coverage prediction control (bottom-left; panel expands upward) — shared by
		     the MapLibre map and the WebGL-free FallbackMap. -->
		<div class="absolute bottom-3 left-3 z-10 flex w-64 max-w-[80vw] flex-col-reverse gap-2">
			<button
				onclick={toggleCoverage}
				class="panel flex w-full items-center gap-2 px-3 py-2 text-sm font-600 transition-colors {coverageMode ? 'border-signal/50 text-signal' : 'text-fg-dim hover:text-fg'}"
			>
				<svg viewBox="0 0 24 24" class="h-4 w-4" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M4.9 4.9a10 10 0 0 0 0 14.2M19.1 4.9a10 10 0 0 1 0 14.2M8 8a5 5 0 0 0 0 8M16 8a5 5 0 0 1 0 8M12 11.2a1 1 0 1 0 0 1.6 1 1 0 0 0 0-1.6z" /></svg>
				Coverage prediction
				<span class="ml-auto text-xs">{coverageMode ? '×' : '+'}</span>
			</button>

			{#if coverageMode}
				<div class="panel rise px-4 py-3">
					<p class="text-fg-faint mb-3 text-xs">
						{pin ? 'Drag the pin or tap to move it.' : 'Tap the map to drop a planned repeater.'}
					</p>
					<div class="flex gap-2">
						<label class="flex-1">
							<span class="label">Antenna m</span>
							<input type="number" min="0" bind:value={txHeight} onchange={runCoverage} class="border-line bg-ink-2 text-fg focus:border-signal mt-1 w-full rounded-[var(--radius)] border px-2 py-1 font-mono text-sm outline-none" />
						</label>
						<label class="flex-1">
							<span class="label">Range km</span>
							<input type="number" min="1" max="60" bind:value={rangeKm} onchange={runCoverage} class="border-line bg-ink-2 text-fg focus:border-signal mt-1 w-full rounded-[var(--radius)] border px-2 py-1 font-mono text-sm outline-none" />
						</label>
					</div>

					<div class="mt-3 flex items-center gap-2">
						<button onclick={runCoverage} disabled={!pin || computing} class="border-signal/40 bg-signal/15 text-signal flex-1 rounded-[var(--radius)] border px-3 py-1.5 text-xs font-600 disabled:opacity-50">
							{computing ? 'Computing…' : 'Recompute'}
						</button>
						{#if pin}<button onclick={clearCoverage} class="border-line text-fg-dim hover:text-coral rounded-[var(--radius)] border px-3 py-1.5 text-xs font-600">Clear</button>{/if}
					</div>

					{#if coverage}
						<div class="border-line/60 text-fg-faint mt-3 border-t pt-2 font-mono text-[0.62rem]">
							ground {Number.isFinite(coverage.groundElevM) ? coverage.groundElevM.toFixed(0) + ' m' : '—'} · reaches up to {coverage.maxReachKm.toFixed(1)} km
						</div>

						<!-- Nodes reachable inside the coverage -->
						<div class="border-line/60 mt-2 border-t pt-2">
							<div class="label mb-1.5 flex items-center justify-between">
								<span>Nodes in coverage</span>
								<span class="text-signal tnum">{nodesInCoverage.length}</span>
							</div>
							{#if nodesInCoverage.length === 0}
								<div class="text-fg-faint text-xs">No known nodes inside.</div>
							{:else}
								<div class="-mr-1 max-h-48 space-y-0.5 overflow-y-auto pr-1">
									{#each nodesInCoverage as { n, d } (n.publicKey)}
										<button onclick={() => (nodeKey = n.publicKey)} class="hover:bg-panel-2/50 flex w-full items-center gap-2 rounded-[var(--radius)] px-1.5 py-1 text-left">
											<span class="h-2 w-2 shrink-0 rounded-full" style="background:{ROLE_HEX[n.role] ?? '#8394a1'}"></span>
											<span class="text-fg-dim min-w-0 flex-1 truncate text-xs">{n.name || n.publicKey.slice(0, 10)}</span>
											<span class="text-fg-faint font-mono text-[0.62rem] tnum">{d.toFixed(1)} km</span>
										</button>
									{/each}
								</div>
							{/if}
						</div>
					{/if}
				</div>
			{/if}
		</div>
	</div>
</div>

<NodeModal pubkey={nodeKey} onclose={() => (nodeKey = null)} />

<style>
	:global(.maplibregl-popup-content) {
		background: var(--color-panel);
		border: 1px solid var(--color-line-bright);
		border-radius: 2px;
		padding: 8px 10px;
		box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
	}
	:global(.maplibregl-popup-tip) {
		border-top-color: var(--color-line-bright) !important;
		border-bottom-color: var(--color-line-bright) !important;
	}
</style>
