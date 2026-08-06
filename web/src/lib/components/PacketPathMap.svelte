<script lang="ts">
	// A self-contained map of ONE transmission's route(s). Each observer that
	// heard the packet reports the path it saw, and those often differ — the
	// flood takes different branches to different corners of the mesh. Drawing
	// one coloured route per observation shows that spread directly, instead of
	// collapsing it into a single "best" path that never actually existed.
	//
	// Honesty rule (matches how the rest of Ridgeline treats short prefixes): a
	// hop whose 1-byte prefix matches several located nodes is a GUESS. Guessed
	// hops are drawn dashed and their nodes hollow, and the legend says how many
	// a route contains, so nobody reads an inference as a measurement.
	import { untrack } from 'svelte';
	import maplibregl from 'maplibre-gl';
	import 'maplibre-gl/dist/maplibre-gl.css';
	import type { Node, LiveEvent } from '$lib/api';
	import { resolvePathNodes, type ResolvedHop } from '$lib/live-pulse';
	import { basemapStyle, collapseAttribution } from '$lib/map-basemap';
	import { basemap } from '$lib/basemap.svelte';
	import { isLight, locatedNodes } from '$lib/map-util';
	import { hasWebGL } from '$lib/webgl';
	import { theme } from '$lib/theme.svelte';
	import { shortKey } from '$lib/format';

	// UTC with milliseconds, matching LiveGroupModal: these timestamps are
	// compared against each other for one transmission, so the sub-second spread
	// between observers is the interesting part, not the wall-clock hour.
	const absTime = (iso: string) => new Date(iso).toISOString().slice(11, 23) + 'Z';

	interface Props {
		events: LiveEvent[];
		nodes: Node[];
	}
	let { events, nodes }: Props = $props();

	// Distinct hues per route. Chosen to stay apart on both light and dark
	// basemaps rather than sampled from a continuous ramp.
	const ROUTE_HEX = ['#34e3c4', '#e8b454', '#7aa2f7', '#ff6b6b', '#bb9af7', '#9ece6a', '#f7768e', '#2ac3de'];

	const located = $derived(locatedNodes(nodes));

	interface Route {
		key: string;
		observer: string;
		receivedAt: string;
		hops: ResolvedHop[];
		color: string;
		/** Hops that resolved to a located node — what can actually be drawn. */
		pts: [number, number][];
		guessed: number;
	}

	// One route per observation that carried a path, fastest (first heard) first
	// — the earliest reception is the closest thing to "how it actually went".
	const routes = $derived.by((): Route[] => {
		const withPath = events.filter((e) => (e.path ?? []).length > 0);
		const sorted = [...withPath].sort((a, b) => a.receivedAt.localeCompare(b.receivedAt));
		return sorted.map((e, i) => {
			const hops = resolvePathNodes(located, e.path ?? []);
			const pts = hops
				.filter((h) => h.node)
				.map((h) => [h.node!.longitude!, h.node!.latitude!] as [number, number]);
			return {
				key: `${e.observerId ?? i}-${e.receivedAt}`,
				observer: e.observerName || e.observerId || 'unknown',
				receivedAt: e.receivedAt,
				hops,
				color: ROUTE_HEX[i % ROUTE_HEX.length],
				pts,
				guessed: hops.filter((h) => h.ambiguous).length
			};
		});
	});

	// null = show every route overlaid.
	let isolated = $state<string | null>(null);
	const shown = $derived(isolated ? routes.filter((r) => r.key === isolated) : routes);
	const drawable = $derived(routes.filter((r) => r.pts.length >= 1));

	// $state, because the map container only exists once there is something to
	// draw — the node list arrives asynchronously, so the element is bound after
	// mount. Creating the map has to wait for it.
	let mapEl = $state<HTMLDivElement | null>(null);
	let map: maplibregl.Map | null = null;
	let ready = $state(false);
	const webglOk = hasWebGL();

	function lineFC(): GeoJSON.FeatureCollection {
		return {
			type: 'FeatureCollection',
			features: shown
				.filter((r) => r.pts.length >= 2)
				.map((r) => ({
					type: 'Feature' as const,
					properties: { color: r.color, dashed: r.guessed > 0 },
					geometry: { type: 'LineString' as const, coordinates: r.pts }
				}))
		};
	}
	function pointFC(): GeoJSON.FeatureCollection {
		const seen = new Set<string>();
		const feats: GeoJSON.Feature[] = [];
		for (const r of shown) {
			r.hops.forEach((h, i) => {
				if (!h.node) return;
				const id = `${r.key}:${h.node.publicKey}:${i}`;
				if (seen.has(id)) return;
				seen.add(id);
				feats.push({
					type: 'Feature',
					properties: {
						color: r.color,
						guess: h.ambiguous,
						label: h.node.name || shortKey(h.node.publicKey, 4, 4)
					},
					geometry: { type: 'Point', coordinates: [h.node.longitude!, h.node.latitude!] }
				});
			});
		}
		return { type: 'FeatureCollection', features: feats };
	}

	function refresh() {
		if (!map || !ready) return;
		(map.getSource('route-lines') as maplibregl.GeoJSONSource | undefined)?.setData(lineFC());
		(map.getSource('route-pts') as maplibregl.GeoJSONSource | undefined)?.setData(pointFC());
	}

	function fit() {
		if (!map) return;
		const all = drawable.flatMap((r) => r.pts);
		if (all.length === 0) return;
		const b = new maplibregl.LngLatBounds(all[0], all[0]);
		for (const p of all) b.extend(p);
		map.fitBounds(b, { padding: 48, maxZoom: 12, duration: 0 });
	}

	function addLayers() {
		if (!map) return;
		map.addSource('route-lines', { type: 'geojson', data: lineFC() });
		map.addSource('route-pts', { type: 'geojson', data: pointFC() });
		// Solid routes: every hop resolved uniquely.
		map.addLayer({
			id: 'route-line',
			type: 'line',
			source: 'route-lines',
			filter: ['!', ['get', 'dashed']],
			paint: { 'line-color': ['get', 'color'], 'line-width': 2.4, 'line-opacity': 0.85 },
			layout: { 'line-cap': 'round', 'line-join': 'round' }
		});
		// Dashed: contains at least one guessed hop.
		map.addLayer({
			id: 'route-line-guess',
			type: 'line',
			source: 'route-lines',
			filter: ['get', 'dashed'],
			paint: {
				'line-color': ['get', 'color'],
				'line-width': 2,
				'line-opacity': 0.6,
				'line-dasharray': [2, 1.6]
			},
			layout: { 'line-cap': 'round', 'line-join': 'round' }
		});
		map.addLayer({
			id: 'route-pt',
			type: 'circle',
			source: 'route-pts',
			paint: {
				'circle-radius': 5,
				// Hollow for a guessed hop, filled when it is a certain match.
				'circle-color': ['case', ['get', 'guess'], 'rgba(0,0,0,0)', ['get', 'color']],
				'circle-stroke-color': ['get', 'color'],
				'circle-stroke-width': 2
			}
		});
		map.addLayer({
			id: 'route-label',
			type: 'symbol',
			source: 'route-pts',
			layout: {
				'text-field': ['get', 'label'],
				'text-size': 10,
				'text-offset': [0, 1.3],
				'text-anchor': 'top',
				'text-allow-overlap': false
			},
			paint: {
				'text-color': isLight() ? '#243b53' : '#cfe8e2',
				'text-halo-color': isLight() ? '#ffffff' : '#04100e',
				'text-halo-width': 1.2
			}
		});
	}

	// Build the map as soon as its container is in the DOM. This is an $effect
	// rather than onMount because the container is inside an {#if} gated on
	// having a drawable route, and the node list that decides that arrives
	// after mount — at onMount time the element does not exist yet.
	$effect(() => {
		const el = mapEl;
		if (!webglOk || !el || map) return;
		untrack(() => {
			basemap.init();
			map = new maplibregl.Map({
				container: el,
				style: basemapStyle(basemap.id, isLight()),
				center: [-123.65, 49.25],
				zoom: 7,
				attributionControl: { compact: true }
			});
			map.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'bottom-right');
			map.on('load', () => {
				map?.resize();
				collapseAttribution(map!);
				addLayers();
				ready = true;
				untrack(fit);
			});
		});
		return () => {
			map?.remove();
			map = null;
			ready = false;
		};
	});

	// Redraw when the isolation changes or new observations land.
	$effect(() => {
		void shown;
		void theme.id;
		refresh();
	});
</script>

{#if !webglOk}
	<div class="text-fg-faint px-1 py-3 text-xs">
		Route map needs WebGL, which this browser has disabled. The hop list below still shows the
		full path.
	</div>
{:else if drawable.length === 0}
	<div class="text-fg-faint px-1 py-3 text-xs">
		No hop in this packet's path resolved to a node with a known location, so there is nothing to
		plot. The hop list below shows the raw prefixes.
	</div>
{:else}
	<div class="border-line/60 relative h-64 w-full overflow-hidden rounded-lg border">
		<div bind:this={mapEl} class="h-full w-full"></div>
	</div>

	<!-- The route selector only earns its space when there is more than one route
	     to choose between; a single repeat's map needs no chooser. -->
	{#if routes.length > 1}
		<div class="mt-2 flex flex-wrap items-center gap-1.5">
			<button
				onclick={() => (isolated = null)}
				class="rounded-full border px-2.5 py-1 font-mono text-[0.65rem] transition-colors {isolated ===
				null
					? 'border-signal/60 text-signal'
					: 'border-line/70 text-fg-faint hover:text-fg-dim'}"
			>
				All paths · {routes.length}
			</button>
			{#each routes as r (r.key)}
				<button
					onclick={() => (isolated = isolated === r.key ? null : r.key)}
					class="flex items-center gap-1.5 rounded-full border px-2.5 py-1 font-mono text-[0.65rem] transition-colors {isolated ===
					r.key
						? 'border-line text-fg'
						: 'border-line/70 text-fg-faint hover:text-fg-dim'}"
					title="{r.observer} · heard {absTime(r.receivedAt)} · {r.hops.length} hops{r.guessed
						? ` · ${r.guessed} guessed`
						: ''}"
				>
					<span class="h-2 w-2 shrink-0 rounded-full" style="background:{r.color}"></span>
					<span class="max-w-32 truncate">{r.observer}</span>
					{#if r.guessed}<span class="text-amber">~</span>{/if}
				</button>
			{/each}
		</div>
	{/if}

	<p class="text-fg-faint mt-2 text-[0.68rem] leading-relaxed">
		{#if routes.length > 1}
			One route per observer, earliest reception first.
		{:else}
			The route as this observer heard it.
		{/if}
		A <span class="text-fg-dim">dashed</span> line and hollow nodes mean at least one hop's prefix
		matched more than one located node — that hop is this app's best guess, not a fact.
	</p>
{/if}
