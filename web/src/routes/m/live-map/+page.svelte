<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import maplibregl from 'maplibre-gl';
	import 'maplibre-gl/dist/maplibre-gl.css';
	import type { FeatureCollection } from 'geojson';
	import { api, type Node } from '$lib/api';
	import { live, hashColor } from '$lib/live.svelte';
	import { theme } from '$lib/theme.svelte';
	import { basemapStyle, basemapHasHillshade, basemapHasLocalTerrain, collapseAttribution, ensureLocalTerrain } from '$lib/map-basemap';
	import { basemap } from '$lib/basemap.svelte';
	import { ensureHillshade } from '$lib/map-hillshade';
	import { ROLE_HEX, locatedNodes } from '$lib/map-util';
	import { MAP_CENTER_LONLAT } from '$lib/site';
	import BasemapSelector from '$lib/components/BasemapSelector.svelte';
	import FallbackMap from '$lib/components/FallbackMap.svelte';
	import { hasWebGL } from '$lib/webgl';

	let mapEl: HTMLDivElement;
	let map: maplibregl.Map | null = null;
	let webglOk = $state(true);
	let fbBanner = $state(true); // WebGL-free banner open? (offsets the basemap selector below it)
	let nodes = $state<Node[]>([]);
	let basemapLight = false;
	let didFit = false;
	let pulseCount = $state(0);

	// resolve a path-hop prefix to a located node's coordinates (unique prefix)
	function resolveHop(hop: string): [number, number] | null {
		const h = hop.toUpperCase();
		const hit = nodes.filter((n) => n.publicKey.toUpperCase().startsWith(h));
		return hit.length === 1 ? [hit[0].longitude!, hit[0].latitude!] : null;
	}

	function nodeFeatures(): FeatureCollection {
		return {
			type: 'FeatureCollection',
			features: nodes.map((n) => ({
				type: 'Feature',
				geometry: { type: 'Point', coordinates: [n.longitude!, n.latitude!] },
				properties: { color: ROLE_HEX[n.role] ?? '#8394a1', pubkey: n.publicKey }
			}))
		};
	}

	// active pulses: a node coord + colour + start time
	type Pulse = { lng: number; lat: number; color: string; start: number };
	let pulses: Pulse[] = [];
	const PULSE_MS = 1600;

	// active trails: the polyline through a packet's resolved path hops, fading out.
	type Trail = { coords: [number, number][]; color: string; start: number };
	let trails: Trail[] = [];
	const TRAIL_MS = 2600;

	function trailFeatures(now: number): FeatureCollection {
		return {
			type: 'FeatureCollection',
			features: trails.map((t) => {
				const age = (now - t.start) / TRAIL_MS; // 0..1
				return {
					type: 'Feature',
					geometry: { type: 'LineString', coordinates: t.coords },
					properties: { color: t.color, o: Math.max(0, 1 - age) * 0.85 }
				};
			})
		};
	}

	function pulseFeatures(now: number): FeatureCollection {
		return {
			type: 'FeatureCollection',
			features: pulses.map((p) => {
				const age = (now - p.start) / PULSE_MS; // 0..1
				return {
					type: 'Feature',
					geometry: { type: 'Point', coordinates: [p.lng, p.lat] },
					properties: { color: p.color, r: 4 + age * 22, o: Math.max(0, 1 - age) }
				};
			})
		};
	}

	function spawn(ev: { path?: string[]; payloadType: string; messageHash: string }) {
		if (!ev.path?.length) return;
		// Colour per message hash, matching the desktop comets (and the Recent
		// Packets flag there); keeps the propagation colours consistent app-wide.
		const color = hashColor(ev.messageHash);
		const now = performance.now();
		const coords: [number, number][] = [];
		ev.path.forEach((hop, i) => {
			const c = resolveHop(hop);
			if (c) {
				coords.push(c);
				pulses.push({ lng: c[0], lat: c[1], color, start: now + i * 180 });
			}
		});
		if (coords.length >= 2) trails.push({ coords, color, start: now });
	}

	let raf = 0;
	function frame() {
		const now = performance.now();
		pulses = pulses.filter((p) => now - p.start < PULSE_MS);
		trails = trails.filter((t) => now - t.start < TRAIL_MS);
		pulseCount = pulses.length;
		(map?.getSource('pulses') as maplibregl.GeoJSONSource | undefined)?.setData(pulseFeatures(now));
		(map?.getSource('trails') as maplibregl.GeoJSONSource | undefined)?.setData(trailFeatures(now));
		raf = requestAnimationFrame(frame);
	}

	// watch the live store for new events
	let lastSeen = 0;
	$effect(() => {
		const evs = live.events;
		void evs.length;
		if (!map) return;
		const now = Date.now();
		for (const ev of evs) {
			const t = +new Date(ev.receivedAt);
			if (t > lastSeen && now - t < 20000) spawn(ev);
		}
		lastSeen = Math.max(lastSeen, ...evs.map((e) => +new Date(e.receivedAt)), lastSeen);
	});

	function addLayers() {
		if (!map || map.getSource('nodes')) return;
		map.addSource('nodes', { type: 'geojson', data: nodeFeatures() });
		map.addSource('pulses', { type: 'geojson', data: { type: 'FeatureCollection', features: [] } });
		map.addSource('trails', { type: 'geojson', data: { type: 'FeatureCollection', features: [] } });
		map.addLayer({
			id: 'pulse-lines', type: 'line', source: 'trails',
			layout: { 'line-cap': 'round', 'line-join': 'round' },
			paint: { 'line-color': ['get', 'color'], 'line-width': 2, 'line-opacity': ['get', 'o'], 'line-blur': 0.5 }
		});
		map.addLayer({
			id: 'pulse-rings', type: 'circle', source: 'pulses',
			paint: { 'circle-radius': ['get', 'r'], 'circle-color': 'transparent', 'circle-stroke-color': ['get', 'color'], 'circle-stroke-width': 2, 'circle-stroke-opacity': ['get', 'o'] }
		});
		map.addLayer({
			id: 'node-dots', type: 'circle', source: 'nodes',
			paint: { 'circle-radius': ['interpolate', ['linear'], ['zoom'], 4, 2.5, 11, 5], 'circle-color': ['get', 'color'], 'circle-opacity': 0.85 }
		});
		// Transparent, larger hit target so the small dots are tappable on a phone.
		map.addLayer({
			id: 'node-hit', type: 'circle', source: 'nodes',
			paint: { 'circle-radius': 13, 'circle-color': 'transparent' }
		});
		map.on('click', 'node-hit', (e) => {
			const pk = e.features?.[0]?.properties?.pubkey as string | undefined;
			if (pk) goto('/m/nodes/' + pk);
		});
		map.on('mouseenter', 'node-hit', () => { if (map) map.getCanvas().style.cursor = 'pointer'; });
	}
	let currentBasemap = basemap.id;
	function ensureOverlays() {
		if (!map || !map.isStyleLoaded()) return;
		if (basemapHasHillshade(currentBasemap)) ensureHillshade(map, basemapLight);
		if (basemapHasLocalTerrain(currentBasemap)) ensureLocalTerrain(map, basemapLight);
		if (!map.getSource('nodes')) addLayers();
	}
	function fit() {
		if (!map || didFit || nodes.length === 0) return;
		const b = new maplibregl.LngLatBounds();
		for (const n of nodes) b.extend([n.longitude!, n.latitude!]);
		map.fitBounds(b, { padding: 56, maxZoom: 11, duration: 0 });
		didFit = true;
	}

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
			(map?.getSource('nodes') as maplibregl.GeoJSONSource | undefined)?.setData(nodeFeatures());
			fit();
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
		map = new maplibregl.Map({ container: mapEl, style: basemapStyle(currentBasemap, basemapLight), center: MAP_CENTER_LONLAT, zoom: 7, attributionControl: { compact: true } });
		map.addControl(new maplibregl.NavigationControl({ showCompass: true, visualizePitch: true }), 'top-right');
		map.on('load', () => {
			if (!map) return;
			map.resize();
			if (basemapHasHillshade(currentBasemap)) ensureHillshade(map, basemapLight);
			if (basemapHasLocalTerrain(currentBasemap)) ensureLocalTerrain(map, basemapLight);
			addLayers();
			collapseAttribution(map);
			refresh();
			raf = requestAnimationFrame(frame);
		});
		const t = setInterval(refresh, 15000);
		return () => { clearInterval(t); cancelAnimationFrame(raf); map?.remove(); map = null; };
	});
</script>

<div class="relative h-full w-full">
	{#if !webglOk}
		<FallbackMap
			nodes={nodes}
			center={MAP_CENTER_LONLAT}
			zoom={7}
			live
			bind:bannerOpen={fbBanner}
			onselect={(k) => goto('/m/nodes/' + k)}
			notice="WebGL is disabled — showing the basic live map. Enable WebGL for terrain and the full-fidelity animation."
		/>
		<BasemapSelector compact posClass={fbBanner ? 'top-16 left-3' : 'top-3 left-3'} />
	{:else}
	<div bind:this={mapEl} class="h-full w-full"></div>
	<div class="border-line/60 bg-ink-2/80 absolute top-3 left-3 z-10 flex items-center gap-2 rounded-full border px-3 py-1.5 backdrop-blur-md">
		{#if live.connected}<span class="live-dot"></span>{:else}<span class="bg-coral/70 h-2 w-2 rounded-full"></span>{/if}
		<span class="text-fg-dim font-mono text-[0.62rem] tnum">{nodes.length} nodes · {pulseCount} live</span>
	</div>
	<BasemapSelector compact posClass="top-14 left-3" />
	{/if}
</div>
