<!--
  WebGL-free map shown when MapLibre can't run (WebGL disabled). Uses Leaflet — a
  DOM/canvas slippy map — with theme-aware CARTO raster tiles and clickable node
  markers. Two optional modes give parity with the WebGL maps without WebGL:
    • cluster — group nodes with Leaflet.markercluster (the static /map view).
    • live    — animate propagation pulses onto a 2-D canvas overlay driven by the
                shared PulseEngine (the /live-map view).
  Leaflet + the cluster plugin (and their CSS) are dynamically imported so they
  only load for the minority of visitors with WebGL disabled. A dismissible banner
  points them at enabling WebGL for the full MapLibre maps.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import type { Node } from '$lib/api';
	import type { CoverageResult } from '$lib/coverage';
	import { theme } from '$lib/theme.svelte';
	import { isLight, inkColor, ROLE_HEX, FAV_COLOR, locatedNodes } from '$lib/map-util';
	import { favorites } from '$lib/favorites.svelte';
	import { roleLabel } from '$lib/format';
	import { live, hashColor } from '$lib/live.svelte';
	import { PulseEngine } from '$lib/live-pulse';
	import { chime } from '$lib/live-audio.svelte';
	import { basemap } from '$lib/basemap.svelte';
	import { leafletBasemap } from '$lib/leaflet-basemap';
	import { MAP_CENTER_LONLAT } from '$lib/site';

	interface Props {
		nodes: Node[];
		/** Initial centre as [lon, lat] (matches the MapLibre maps' convention). */
		center?: [number, number];
		zoom?: number;
		onselect?: (pubkey: string) => void;
		/** Banner copy; defaults to the generic "enable WebGL" notice. */
		notice?: string;
		/** Group nodes into clusters (static map). */
		cluster?: boolean;
		/** Animate propagation pulses from the live feed (live map). */
		live?: boolean;
		/** Play a chime as pulses reach nodes (live map). The on/off + tuning UI lives
		 * in the route's Map Control panel (ChimeControls), bound to the same singleton. */
		audio?: boolean;
		/** Restrict which node roles are drawn. Pulse resolution still uses every
		 * located node, so hidden-role relays keep animating correctly. */
		roleFilter?: Set<string>;
		/** Two-way: whether the WebGL-disabled banner is showing. Callers bind this to
		 * offset their own overlays (e.g. map controls) below the banner. */
		bannerOpen?: boolean;
		/** Coverage-prediction overlay (static map). Rendered as an image overlay. */
		coverage?: CoverageResult | null;
		/** Planned-transmitter pin [lat, lon] — draggable when coverage mode is on. */
		pin?: { lat: number; lon: number } | null;
		/** In coverage mode, map clicks place the pin (not select nodes). */
		coverageMode?: boolean;
		/** Map click while coverage mode is on (lat, lon). */
		onmapclick?: (lat: number, lon: number) => void;
		/** Pin dragged to a new spot (lat, lon). */
		onpinmove?: (lat: number, lon: number) => void;
	}
	let {
		nodes,
		center = MAP_CENTER_LONLAT,
		zoom = 9,
		onselect,
		notice = 'Basic map — WebGL is disabled in your browser. Enable it for the full interactive map with terrain, clustering, coverage and live propagation.',
		cluster = false,
		live: liveMode = false,
		audio = false,
		roleFilter = undefined,
		bannerOpen = $bindable(true),
		coverage = null,
		pin = null,
		coverageMode = false,
		onmapclick = undefined,
		onpinmove = undefined
	}: Props = $props();

	let el: HTMLDivElement;
	// Leaflet is loaded lazily; keep the namespace + instances as `any` rather than
	// pulling the types into the eager bundle.
	/* eslint-disable @typescript-eslint/no-explicit-any */
	let L: any = null;
	let map: any = null;
	let baseLayer: any = null;
	let hillLayer: any = null;
	let labelLayer: any = null;
	let coverageLayer: any = null;
	let pinMarker: any = null;
	let markers: any = null;
	let canvasRenderer: any = null;
	/* eslint-enable @typescript-eslint/no-explicit-any */
	let didFit = false;
	let curLight = false;

	const located = $derived(locatedNodes(nodes));
	// Markers honour the role filter; pulse resolution (below) keeps using `located`.
	const displayed = $derived(
		roleFilter ? located.filter((n) => roleFilter!.has(n.role)) : located
	);

	// Build (or rebuild) the tile layers for the selected basemap + current theme.
	// `topo` adds a shaded-relief overlay in a dedicated, multiply-blended pane that
	// sits above the base tiles but below the node markers/pulses.
	function applyBasemap() {
		if (!map || !L) return;
		if (baseLayer) map.removeLayer(baseLayer);
		if (hillLayer) map.removeLayer(hillLayer);
		if (labelLayer) map.removeLayer(labelLayer);
		baseLayer = hillLayer = labelLayer = null;
		const spec = leafletBasemap(basemap.id, curLight);
		baseLayer = L.tileLayer(spec.base.url, {
			subdomains: spec.base.subdomains ?? 'abc',
			maxZoom: spec.base.maxZoom,
			maxNativeZoom: spec.base.maxNativeZoom,
			attribution: spec.base.attribution
		}).addTo(map);
		if (spec.hillshade) {
			if (!map.getPane('hillshade')) {
				const p = map.createPane('hillshade');
				p.style.zIndex = '250'; // above base tiles (200), below overlay/markers (400+)
				p.style.pointerEvents = 'none';
			}
			// Theme-aware blend so the grayscale relief is actually visible: on the dark
			// base 'screen' lifts lit slopes (relief reads light-on-dark); on the light
			// base 'multiply' drops shadows in. (Plain 'multiply' over the dark base was
			// near-invisible.) Set each render so a theme toggle updates it.
			map.getPane('hillshade').style.mixBlendMode = curLight ? 'multiply' : 'screen';
			hillLayer = L.tileLayer(spec.hillshade.url, {
				pane: 'hillshade',
				// 'screen' lifts aggressively on the dark base (light hand); 'multiply'
				// only darkens, and on the near-white light base it needs more weight or
				// the relief washes out.
				opacity: curLight ? 0.7 : 0.22,
				maxZoom: spec.hillshade.maxZoom,
				attribution: spec.hillshade.attribution
			}).addTo(map);
		}
		if (spec.labels) {
			if (!map.getPane('labels')) {
				const lp = map.createPane('labels');
				lp.style.zIndex = '350'; // above base/hillshade (≤250), below markers (400+)
				lp.style.pointerEvents = 'none';
			}
			labelLayer = L.tileLayer(spec.labels.url, {
				pane: 'labels',
				subdomains: spec.labels.subdomains ?? 'abc',
				maxZoom: spec.labels.maxZoom,
				attribution: spec.labels.attribution
			}).addTo(map);
		}
	}

	// Teal cluster bubble matching the MapLibre cluster style.
	function clusterIcon(count: number) {
		const d = count >= 30 ? 48 : count >= 10 ? 36 : 26;
		return L.divIcon({
			html:
				`<div style="width:${d}px;height:${d}px;display:flex;align-items:center;justify-content:center;` +
				`border-radius:50%;background:rgba(21,158,139,.85);border:1.5px solid #34e3c4;` +
				`color:#04140f;font:600 12px/1 var(--font-mono,monospace)">${count}</div>`,
			className: 'rl-cluster',
			iconSize: [d, d]
		});
	}

	// A node rendered as a real marker with a divIcon. Leaflet.markercluster only
	// shows/hides leaf layers through their `_icon` DOM element, which circleMarkers
	// (canvas OR svg) don't have — so in cluster mode each node must be a divIcon
	// marker or it never appears once clustered.
	function nodeIcon(n: Node, stroke: string, zoom: number) {
		const repeater = n.role === 'Repeater';
		const d = Math.round(nodeRadius(repeater, zoom) * 2); // match the live circleMarker diameter
		const color = ROLE_HEX[n.role] ?? '#8394a1';
		const ring = favorites.has(n.publicKey) ? `box-shadow:0 0 0 2px ${FAV_COLOR};` : '';
		return L.divIcon({
			html:
				`<div style="width:${d}px;height:${d}px;border-radius:50%;` +
				`background:${color};border:1px solid ${stroke};${ring}box-sizing:border-box"></div>`,
			className: 'rl-node',
			iconSize: [d, d],
			iconAnchor: [d / 2, d / 2]
		});
	}

	// One node-dot radius (px) shared by the static (divIcon) and live (circleMarker)
	// maps, so a node is the same size on both at any zoom. Dots ease DOWN as you zoom
	// in — large enough to spot the network when zoomed out, small and precise when
	// zoomed in (they ballooned at high zoom before). Clamped so they never vanish.
	function nodeRadius(repeater: boolean, zoom: number): number {
		const r = (repeater ? 4.5 : 3.5) - (zoom - 9) * 0.2;
		return Math.max(repeater ? 3 : 2.6, Math.min(repeater ? 5 : 4, r));
	}

	function renderMarkers() {
		if (!map || !L) return;
		markers.clearLayers();
		const stroke = inkColor();
		const zoom = map.getZoom();
		for (const n of displayed) {
			const lat = n.latitude!;
			const lon = n.longitude!;
			// Cluster mode: divIcon markers so markercluster can cluster/decluster them.
			if (cluster) {
				const m = L.marker([lat, lon], { icon: nodeIcon(n, stroke, zoom) });
				m.bindTooltip(`${n.name || n.publicKey.slice(0, 10)} · ${roleLabel(n.role)}`, {
					direction: 'top'
				});
				m.on('click', () => {
					if (!coverageMode) onselect?.(n.publicKey);
				});
				m.addTo(markers);
				continue;
			}
			// Non-cluster (live map): canvas circleMarkers — cheap to draw in bulk.
			const color = ROLE_HEX[n.role] ?? '#8394a1';
			const repeater = n.role === 'Repeater';
			const r = nodeRadius(repeater, zoom);
			if (favorites.has(n.publicKey)) {
				L.circleMarker([lat, lon], {
					renderer: canvasRenderer,
					radius: r + 2.5,
					color: FAV_COLOR,
					weight: 1.75,
					opacity: 0.95,
					fill: false,
					interactive: false
				}).addTo(markers);
			}
			const m = L.circleMarker([lat, lon], {
				renderer: canvasRenderer,
				radius: r,
				color: stroke,
				weight: 1,
				fillColor: color,
				fillOpacity: 0.9
			});
			m.bindTooltip(`${n.name || n.publicKey.slice(0, 10)} · ${roleLabel(n.role)}`, {
				direction: 'top'
			});
			m.on('click', () => onselect?.(n.publicKey));
			m.addTo(markers);
		}
	}

	function fit() {
		if (!map || !L || didFit || displayed.length === 0) return;
		const b = L.latLngBounds(displayed.map((n) => [n.latitude!, n.longitude!]));
		map.fitBounds(b, { padding: [40, 40], maxZoom: 12 });
		didFit = true;
	}

	// ── coverage prediction overlay + draggable transmitter pin ───────────────
	// Renders the viewshed PNG (computeCoverage) as an image overlay in its own pane
	// beneath the node markers, plus a draggable pin the route recomputes around.
	function applyCoverage() {
		if (!map || !L) return;
		if (coverageLayer) {
			map.removeLayer(coverageLayer);
			coverageLayer = null;
		}
		if (!coverage) return;
		if (!map.getPane('coverage')) {
			const p = map.createPane('coverage');
			p.style.zIndex = '350'; // above tiles/hillshade, below node markers (400+)
			p.style.pointerEvents = 'none';
		}
		// imageCoords = [TL, TR, BR, BL] as [lon, lat]; Leaflet wants [[S,W],[N,E]].
		const ic = coverage.imageCoords;
		const bounds = [
			[ic[2][1], ic[0][0]],
			[ic[0][1], ic[2][0]]
		];
		coverageLayer = L.imageOverlay(coverage.dataUrl, bounds, {
			pane: 'coverage',
			opacity: 0.85
		}).addTo(map);
	}

	function pinIcon() {
		return L.divIcon({
			html:
				'<div style="width:16px;height:16px;border-radius:50%;background:#e8b454;' +
				'border:2px solid #1a1407;box-shadow:0 0 0 2px rgba(232,180,84,.35),0 1px 4px rgba(0,0,0,.5);' +
				'box-sizing:border-box"></div>',
			className: 'rl-pin',
			iconSize: [16, 16],
			iconAnchor: [8, 8]
		});
	}

	function applyPin() {
		if (!map || !L) return;
		if (!pin) {
			if (pinMarker) {
				map.removeLayer(pinMarker);
				pinMarker = null;
			}
			return;
		}
		if (!pinMarker) {
			pinMarker = L.marker([pin.lat, pin.lon], {
				draggable: true,
				icon: pinIcon(),
				zIndexOffset: 1000
			}).addTo(map);
			pinMarker.on('dragend', () => {
				const ll = pinMarker.getLatLng();
				onpinmove?.(ll.lat, ll.lng);
			});
		} else {
			pinMarker.setLatLng([pin.lat, pin.lon]);
		}
	}

	// ── live propagation overlay (2-D canvas, no WebGL) ────────────────────
	let engine: PulseEngine | null = null;
	let pulseCanvas: HTMLCanvasElement | null = null;
	let raf = 0;

	function sizeCanvas() {
		if (!pulseCanvas || !map) return;
		const size = map.getSize();
		const dpr = window.devicePixelRatio || 1;
		pulseCanvas.width = size.x * dpr;
		pulseCanvas.height = size.y * dpr;
		pulseCanvas.style.width = size.x + 'px';
		pulseCanvas.style.height = size.y + 'px';
		const ctx = pulseCanvas.getContext('2d');
		if (ctx) ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
	}

	function drawPulses() {
		raf = requestAnimationFrame(drawPulses);
		if (!engine || !pulseCanvas || !map) return;
		const ctx = pulseCanvas.getContext('2d');
		if (!ctx) return;
		const size = map.getSize();
		ctx.clearRect(0, 0, size.x, size.y);
		const out = engine.frame(performance.now());
		// Soft wind-chime as each pulse reaches a node (same seed as the MapLibre map).
		if (audio) for (const at of out.arrivals) chime.play(Math.round((at[0] + at[1]) * 100));
		// Project an engine [lon,lat] to a container pixel.
		const px = (c: [number, number]) => map.latLngToContainerPoint([c[1], c[0]]);

		// Comet trails (line-width 2, soft glow).
		ctx.lineCap = 'round';
		ctx.lineJoin = 'round';
		ctx.lineWidth = 2;
		for (const l of out.lines) {
			if (l.coords.length < 2) continue;
			ctx.globalAlpha = l.opacity;
			ctx.strokeStyle = l.color;
			ctx.shadowColor = l.color;
			ctx.shadowBlur = 4;
			ctx.beginPath();
			const p0 = px(l.coords[0]);
			ctx.moveTo(p0.x, p0.y);
			for (let i = 1; i < l.coords.length; i++) {
				const p = px(l.coords[i]);
				ctx.lineTo(p.x, p.y);
			}
			ctx.stroke();
		}

		// Node ripples (expanding stroked rings).
		ctx.shadowBlur = 0;
		ctx.lineWidth = 1.5;
		for (const r of out.rings) {
			const p = px(r.at);
			ctx.globalAlpha = r.o;
			ctx.strokeStyle = r.color;
			ctx.beginPath();
			ctx.arc(p.x, p.y, r.r, 0, Math.PI * 2);
			ctx.stroke();
		}

		// Travelling head: a glow halo under a white dot.
		for (const d of out.dots) {
			const p = px(d.at);
			ctx.globalAlpha = 0.5;
			ctx.fillStyle = d.color;
			ctx.shadowColor = d.color;
			ctx.shadowBlur = 9;
			ctx.beginPath();
			ctx.arc(p.x, p.y, 7, 0, Math.PI * 2);
			ctx.fill();
			ctx.shadowBlur = 0;
			ctx.globalAlpha = 1;
			ctx.fillStyle = '#ffffff';
			ctx.strokeStyle = d.color;
			ctx.lineWidth = 2;
			ctx.beginPath();
			ctx.arc(p.x, p.y, 4.5, 0, Math.PI * 2);
			ctx.fill();
			ctx.stroke();
		}
		ctx.globalAlpha = 1;
	}

	onMount(() => {
		if (audio) chime.load();
		let destroyed = false;
		(async () => {
			// CSS first so controls/tiles are styled the moment the map appears.
			await import('leaflet/dist/leaflet.css');
			L = (await import('leaflet')).default ?? (await import('leaflet'));
			if (cluster) {
				await import('leaflet.markercluster/dist/MarkerCluster.css');
				await import('leaflet.markercluster');
			}
			if (destroyed || !el) return;
			curLight = isLight();
			canvasRenderer = L.canvas({ padding: 0.5 });
			map = L.map(el, {
				center: [center[1], center[0]],
				zoom,
				zoomControl: false,
				preferCanvas: true,
				attributionControl: true
			});
			L.control.zoom({ position: 'bottomright' }).addTo(map);
			applyBasemap();
			markers =
				cluster && L.markerClusterGroup
					? L.markerClusterGroup({
							maxClusterRadius: 46,
							showCoverageOnHover: false,
							disableClusteringAtZoom: 12,
							iconCreateFunction: (c: { getChildCount(): number }) => clusterIcon(c.getChildCount())
						})
					: L.layerGroup();
			markers.addTo(map);
			renderMarkers();
			fit();
			applyCoverage();
			applyPin();

			// Coverage mode: a map click drops/moves the planned-transmitter pin.
			map.on('click', (e: { latlng: { lat: number; lng: number } }) => {
				if (coverageMode) onmapclick?.(e.latlng.lat, e.latlng.lng);
			});

			// Re-plot dots at the new zoom-scaled radius — both the live circleMarkers
			// and the static divIcons (their size is baked at creation, so they need a
			// rebuild to resize).
			map.on('zoomend', renderMarkers);

			if (liveMode) {
				engine = new PulseEngine((ev) => hashColor(ev.messageHash));
				pulseCanvas = document.createElement('canvas');
				pulseCanvas.style.cssText =
					'position:absolute;inset:0;pointer-events:none;z-index:400';
				map.getContainer().appendChild(pulseCanvas);
				sizeCanvas();
				map.on('resize', sizeCanvas);
				raf = requestAnimationFrame(drawPulses);
			}
		})();
		return () => {
			destroyed = true;
			cancelAnimationFrame(raf);
			map?.remove();
			map = null;
			pulseCanvas = null;
			engine = null;
		};
	});

	// Re-plot markers as nodes load / the role filter or favourites change.
	$effect(() => {
		void displayed;
		void favorites.keys;
		if (map) {
			renderMarkers();
			fit();
		}
	});

	// Feed the pulse engine from the live event stream.
	$effect(() => {
		if (!liveMode) return;
		void live.events.length;
		engine?.ingest(live.events, located);
	});

	// Rebuild tiles + marker strokes when the UI theme toggles (themed bases change).
	$effect(() => {
		void theme.mode;
		const light = isLight();
		if (!map || light === curLight) return;
		curLight = light;
		applyBasemap();
		renderMarkers(); // marker stroke follows the theme ink colour
	});

	// Swap tiles when the user picks a different basemap.
	$effect(() => {
		void basemap.id;
		if (map) applyBasemap();
	});

	// Re-render the coverage overlay / pin when the route updates them.
	$effect(() => {
		void coverage;
		if (map) applyCoverage();
	});
	$effect(() => {
		void pin;
		if (map) applyPin();
	});
</script>

<!-- `isolate` contains Leaflet's high z-indices (panes/controls up to ~1000) plus
     our banner/audio overlays in one stacking context, so a page-level modal
     (Modal.svelte is z-50) isn't painted underneath the map. -->
<div class="relative isolate h-full w-full">
	<div bind:this={el} class="h-full w-full"></div>

	{#if bannerOpen}
		<div
			class="border-line bg-ink-2/90 absolute top-3 right-3 left-3 z-[1000] flex items-start gap-2 rounded-[var(--radius)] border px-3 py-2 shadow-lg backdrop-blur-md"
		>
			<svg
				viewBox="0 0 24 24"
				class="text-signal mt-0.5 h-4 w-4 shrink-0"
				fill="none"
				stroke="currentColor"
				stroke-width="1.7"
				stroke-linecap="round"
				stroke-linejoin="round"
			>
				<circle cx="12" cy="12" r="10" /><path d="M12 16v-4M12 8h.01" />
			</svg>
			<p class="text-fg-dim flex-1 text-xs leading-snug">{notice}</p>
			<button
				onclick={() => (bannerOpen = false)}
				aria-label="Dismiss"
				class="text-fg-faint hover:text-fg -mt-0.5 shrink-0 text-lg leading-none">×</button
			>
		</div>
	{/if}
</div>

<style>
	/* Leaflet renders its own controls/tiles; keep its attribution unobtrusive and
	   on-theme rather than the default white box. */
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
	:global(.leaflet-bar a) {
		background: var(--color-panel);
		color: var(--color-fg-dim);
		border-color: var(--color-line);
	}
	:global(.leaflet-bar a:hover) {
		background: var(--color-panel-2);
		color: var(--color-fg);
	}
	/* Node hover tooltips are styled site-wide in app.css (.leaflet-tooltip), to
	   match the Tooltip.svelte bubble used across the app. */
	:global(.rl-cluster) {
		background: transparent;
		border: 0;
	}
	:global(.rl-node) {
		background: transparent;
		border: 0;
	}
</style>
