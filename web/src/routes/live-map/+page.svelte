<script lang="ts">
	import { onMount } from 'svelte';
	import Seo from '$lib/components/Seo.svelte';
	import maplibregl from 'maplibre-gl';
	import 'maplibre-gl/dist/maplibre-gl.css';
	import type { Feature, FeatureCollection } from 'geojson';
	import { api, type Node } from '$lib/api';
	import { live, groupLive, hashColor, type LiveGroup } from '$lib/live.svelte';
	import { theme } from '$lib/theme.svelte';
	import { favorites } from '$lib/favorites.svelte';
	import { basemapStyle, basemapHasHillshade, basemapHasLocalTerrain, collapseAttribution, ensureLocalTerrain } from '$lib/map-basemap';
	import { basemap } from '$lib/basemap.svelte';
	import { ensureHillshade } from '$lib/map-hillshade';
	import { isLight, inkColor, ROLE_HEX, FAV_COLOR, locatedNodes } from '$lib/map-util';
	import { ago, shortKey, fmtSnr, snrColor } from '$lib/format';
	import { MAP_CENTER_LONLAT } from '$lib/site';
	import { PulseEngine } from '$lib/live-pulse';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import PayloadTag from '$lib/components/PayloadTag.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';
	import LiveGroupModal from '$lib/components/LiveGroupModal.svelte';
	import MapRoleFilter from '$lib/components/MapRoleFilter.svelte';
	import BasemapSelector from '$lib/components/BasemapSelector.svelte';
	import NodeModal from '$lib/components/NodeModal.svelte';
	import FallbackMap from '$lib/components/FallbackMap.svelte';
	import ChimeControls from '$lib/components/ChimeControls.svelte';
	import { hasWebGL } from '$lib/webgl';

	let mapEl: HTMLDivElement;
	let map: maplibregl.Map | null = null;
	let webglOk = $state(true);
	let fbBanner = $state(true); // WebGL-free banner open? (offsets the Map Control panel below it)
	let ready = false;
	let nodes: Node[] = [];
	let located: Node[] = $state([]); // all located nodes (pulse resolution uses these)
	let animCount = $state(0);
	let selectedRoles = $state(new Set(['Repeater', 'RoomServer', 'ChatNode', 'Sensor']));

	// ── Low-key audio: a mellow wind-chime as each pulse reaches a node ─────
	let soundOn = $state(false);
	const SOUND_KEY = 'ridgeline-livemap-sound';
	// Overlay collapse state — both default minimized, persisted across visits.
	const MAPCTRL_KEY = 'ridgeline-livemap-mapcontrol';
	const PANEL_KEY = 'ridgeline-livemap-recent';
	let mapCtrlOpen = $state(false);
	let overlaysReady = $state(false); // gate persistence until localStorage is loaded
	let actx: AudioContext | null = null;
	let masterGain: GainNode | null = null;
	let lastTickAt = 0;

	// ── Chime tuning (user-adjustable, persisted) ──────────────────────────
	const AUDIO_KEY = 'ridgeline-livemap-audio';
	let volume = $state(0.4); // master gain, 0..0.8
	let chordId = $state('calm');
	let ringId = $state('medium');

	// Each chord is a pentatonic-style scale whose notes stay consonant however
	// they overlap, so random hits always sound musical. Lower registers read
	// mellower, the Japanese scale most wind-chime-like.
	const CHORDS: { id: string; label: string; notes: number[] }[] = [
		{ id: 'calm', label: 'Calm', notes: [261.63, 293.66, 329.63, 392.0, 440.0] }, // C major pentatonic
		{ id: 'mellow', label: 'Mellow', notes: [220.0, 261.63, 293.66, 329.63, 392.0] }, // A minor pentatonic
		{ id: 'zen', label: 'Zen', notes: [261.63, 293.66, 311.13, 392.0, 415.3] }, // Hirajoshi
		{ id: 'deep', label: 'Deep', notes: [130.81, 146.83, 164.81, 196.0, 220.0] } // C major pentatonic, low
	];
	// How long each chime rings out (fundamental decay, seconds).
	const RINGS: { id: string; label: string; decay: number }[] = [
		{ id: 'short', label: 'Short', decay: 1.0 },
		{ id: 'medium', label: 'Med', decay: 1.8 },
		{ id: 'long', label: 'Long', decay: 3.2 }
	];
	const scale = $derived(CHORDS.find((c) => c.id === chordId)?.notes ?? CHORDS[0].notes);
	const ringDecay = $derived(RINGS.find((r) => r.id === ringId)?.decay ?? 1.8);

	function ensureAudio() {
		if (!actx) {
			const AC = window.AudioContext ?? (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
			if (!AC) return;
			actx = new AC();
			masterGain = actx.createGain();
			masterGain.gain.value = volume;
			masterGain.connect(actx.destination);
		}
		if (actx.state === 'suspended') actx.resume();
	}

	// Keep the live master gain in sync with the volume control.
	$effect(() => {
		if (masterGain) masterGain.gain.value = volume;
	});

	function toggleSound() {
		soundOn = !soundOn;
		if (soundOn) ensureAudio(); // created within the click gesture so the browser allows it
		try {
			localStorage.setItem(SOUND_KEY, soundOn ? '1' : '0');
		} catch {
			/* storage unavailable */
		}
	}

	// A soft tubular-chime hit: a fundamental that rings out slowly plus a quieter
	// inharmonic partial (≈2.76×, the ratio of a real wind chime) that shimmers and
	// fades fast. Soft attack, long tail. seed picks a scale note so nodes differ.
	function playTick(seed: number) {
		if (!actx || !masterGain) return;
		const wall = performance.now();
		if (wall - lastTickAt < 120) return; // sparse, gentle trickle — chimes don't rush
		lastTickAt = wall;
		const t = actx.currentTime;
		const base = scale[Math.abs(seed) % scale.length];
		const partials = [
			{ ratio: 1, peak: 0.08, decay: ringDecay },
			{ ratio: 2.76, peak: 0.026, decay: ringDecay * 0.5 }
		];
		for (const p of partials) {
			const osc = actx.createOscillator();
			const g = actx.createGain();
			osc.type = 'sine';
			osc.frequency.value = base * p.ratio;
			osc.detune.value = (Math.random() - 0.5) * 12; // tiny drift, never mechanical
			g.gain.setValueAtTime(0, t);
			g.gain.linearRampToValueAtTime(p.peak, t + 0.012); // soft attack, no click
			g.gain.exponentialRampToValueAtTime(0.0001, t + p.decay);
			osc.connect(g).connect(masterGain);
			osc.start(t);
			osc.stop(t + p.decay + 0.05);
		}
	}

	// Play a sample chime so a tuning change is audible immediately. Bypasses the
	// trickle throttle so dragging/selecting always responds.
	function previewChime() {
		if (!soundOn) return;
		ensureAudio();
		lastTickAt = 0;
		playTick(Math.floor(Math.random() * scale.length));
	}

	// Recent packets overlay (grouped, max 15) with minimize/maximize.
	let panelOpen = $state(false);
	let selected = $state<LiveGroup | null>(null);
	let nodeKey = $state<string | null>(null);
	const recent = $derived(groupLive(live.events).slice(0, 15));

	// ── propagation pulses (shared, renderer-agnostic engine) ──────────────
	// The pulse geometry (hop resolution, comet animation, node ripples) lives in
	// $lib/live-pulse so the WebGL-free Leaflet fallback draws the exact same
	// animation onto a 2-D canvas; here we just feed its output into MapLibre.
	// Comets are coloured per message hash so each pulse matches the colour flag
	// on its Recent Packets row below.
	const engine = new PulseEngine((ev) => hashColor(ev.messageHash));

	// Pulse each newly-seen header path as it arrives.
	$effect(() => {
		void live.events.length;
		engine.ingest(live.events, located);
	});

	// ── per-frame render into GeoJSON sources ─────────────────────────────
	function frame() {
		if (!map || !ready) {
			requestAnimationFrame(frame);
			return;
		}
		const out = engine.frame(performance.now());

		(map.getSource('pulse-lines') as maplibregl.GeoJSONSource)?.setData({
			type: 'FeatureCollection',
			features: out.lines.map((l) => ({
				type: 'Feature',
				geometry: { type: 'LineString', coordinates: l.coords },
				properties: { color: l.color, opacity: l.opacity }
			})) as Feature[]
		});
		(map.getSource('pulse-dots') as maplibregl.GeoJSONSource)?.setData({
			type: 'FeatureCollection',
			features: out.dots.map((d) => ({
				type: 'Feature',
				geometry: { type: 'Point', coordinates: d.at },
				properties: { color: d.color }
			})) as Feature[]
		});
		(map.getSource('pulse-rings') as maplibregl.GeoJSONSource)?.setData({
			type: 'FeatureCollection',
			features: out.rings.map((r) => ({
				type: 'Feature',
				geometry: { type: 'Point', coordinates: r.at },
				properties: { color: r.color, r: r.r, o: r.o }
			})) as Feature[]
		});

		// Chime once as the dot reaches each node.
		if (soundOn) for (const at of out.arrivals) playTick(Math.round((at[0] + at[1]) * 100));

		animCount = out.count;
		requestAnimationFrame(frame);
	}

	// react to theme changes → swap the basemap style
	let basemapLight = false;
	let currentBasemap = basemap.id;
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

	// Re-add overlays after a basemap style swap (theme or basemap change) drops them.
	function ensureOverlays() {
		if (!map || !map.isStyleLoaded()) return;
		if (basemapHasHillshade(currentBasemap)) ensureHillshade(map, basemapLight);
		if (basemapHasLocalTerrain(currentBasemap)) ensureLocalTerrain(map, basemapLight);
		if (map.getSource('nodes')) return;
		addLayers();
		(map.getSource('nodes') as maplibregl.GeoJSONSource | undefined)?.setData(nodeFeatures());
	}

	function nodeFeatures(): FeatureCollection {
		return {
			type: 'FeatureCollection',
			features: located
				.filter((n) => selectedRoles.has(n.role))
				.map((n) => ({
					type: 'Feature',
					geometry: { type: 'Point', coordinates: [n.longitude!, n.latitude!] },
					properties: {
						role: n.role,
						color: ROLE_HEX[n.role] ?? '#8394a1',
						name: n.name,
						pubkey: n.publicKey,
						fav: favorites.has(n.publicKey)
					}
				}))
		};
	}

	// re-filter node display when the role selection or favorites change
	$effect(() => {
		void selectedRoles;
		void favorites.keys;
		if (map && ready) (map.getSource('nodes') as maplibregl.GeoJSONSource)?.setData(nodeFeatures());
	});

	async function loadNodes() {
		nodes = await api.nodes();
		located = locatedNodes(nodes);
		if (map && ready) (map.getSource('nodes') as maplibregl.GeoJSONSource)?.setData(nodeFeatures());
	}

	function addLayers() {
		if (!map) return;
		map.addSource('nodes', { type: 'geojson', data: nodeFeatures() });
		map.addSource('pulse-lines', { type: 'geojson', data: { type: 'FeatureCollection', features: [] } });
		map.addSource('pulse-dots', { type: 'geojson', data: { type: 'FeatureCollection', features: [] } });
		map.addSource('pulse-rings', { type: 'geojson', data: { type: 'FeatureCollection', features: [] } });

		map.addLayer({
			id: 'pulse-lines',
			type: 'line',
			source: 'pulse-lines',
			layout: { 'line-cap': 'round', 'line-join': 'round' },
			paint: {
				'line-color': ['get', 'color'],
				'line-width': 2,
				'line-opacity': ['get', 'opacity'],
				'line-blur': 1.5
			}
		});
		// Amber ring around favorited nodes, beneath the node dots.
		map.addLayer({
			id: 'fav-halo',
			type: 'circle',
			source: 'nodes',
			filter: ['==', ['get', 'fav'], true],
			paint: {
				// Ring hugging the dot tightly — sits right against the node radius
				// (≈ node radius + 0.5px) so it reads as the dot's own outline.
				'circle-radius': [
					'interpolate', ['linear'], ['zoom'],
					6, ['match', ['get', 'role'], 'Repeater', 2.7, 2.1],
					11, ['match', ['get', 'role'], 'Repeater', 5, 3.3],
					15, ['match', ['get', 'role'], 'Repeater', 7.5, 5.3]
				],
				'circle-color': 'rgba(0,0,0,0)',
				'circle-stroke-color': FAV_COLOR,
				'circle-stroke-width': 1.75,
				'circle-stroke-opacity': 0.95
			}
		});
		map.addLayer({
			id: 'nodes',
			type: 'circle',
			source: 'nodes',
			paint: {
				// Smaller when zoomed out, scaling up as you zoom in.
				'circle-radius': [
					'interpolate', ['linear'], ['zoom'],
					6, ['match', ['get', 'role'], 'Repeater', 2.2, 1.6],
					11, ['match', ['get', 'role'], 'Repeater', 4.5, 2.8],
					15, ['match', ['get', 'role'], 'Repeater', 7, 4.8]
				],
				'circle-color': ['get', 'color'],
				'circle-opacity': 0.85,
				'circle-stroke-width': 1,
				'circle-stroke-color': inkColor()
			}
		});
		map.addLayer({
			id: 'pulse-rings',
			type: 'circle',
			source: 'pulse-rings',
			paint: {
				'circle-radius': ['get', 'r'],
				'circle-color': 'rgba(0,0,0,0)',
				'circle-stroke-color': ['get', 'color'],
				'circle-stroke-width': 1.5,
				'circle-stroke-opacity': ['get', 'o']
			}
		});
		map.addLayer({
			id: 'pulse-glow',
			type: 'circle',
			source: 'pulse-dots',
			paint: { 'circle-radius': 11, 'circle-color': ['get', 'color'], 'circle-blur': 1, 'circle-opacity': 0.5 }
		});
		map.addLayer({
			id: 'pulse-dots',
			type: 'circle',
			source: 'pulse-dots',
			paint: {
				'circle-radius': 4.5,
				'circle-color': '#ffffff',
				'circle-stroke-width': 2,
				'circle-stroke-color': ['get', 'color']
			}
		});

	}

	// Interaction handlers — bound once. MapLibre keeps layer-id listeners across
	// removeLayer/addLayer, so they survive a basemap style swap.
	function bindEvents() {
		if (!map) return;
		// Node click → full node-detail modal.
		map.on('click', 'nodes', (e) => {
			const p = e.features![0].properties as { pubkey?: string };
			if (p?.pubkey) nodeKey = p.pubkey;
		});
		map.on('mouseenter', 'nodes', () => map && (map.getCanvas().style.cursor = 'pointer'));
		map.on('mouseleave', 'nodes', () => map && (map.getCanvas().style.cursor = ''));
	}

	// Persist overlay collapse state once loaded (default minimized on first visit).
	$effect(() => {
		const mc = mapCtrlOpen;
		const rp = panelOpen;
		const audio = { vol: volume, ch: chordId, rg: ringId };
		if (!overlaysReady) return;
		try {
			localStorage.setItem(MAPCTRL_KEY, mc ? '1' : '0');
			localStorage.setItem(PANEL_KEY, rp ? '1' : '0');
			localStorage.setItem(AUDIO_KEY, JSON.stringify(audio));
		} catch {
			/* storage unavailable */
		}
	});

	onMount(() => {
		try {
			soundOn = localStorage.getItem(SOUND_KEY) === '1';
			mapCtrlOpen = localStorage.getItem(MAPCTRL_KEY) === '1';
			panelOpen = localStorage.getItem(PANEL_KEY) === '1';
			const rawAudio = localStorage.getItem(AUDIO_KEY);
			if (rawAudio) {
				const a = JSON.parse(rawAudio);
				if (typeof a.vol === 'number') volume = Math.max(0, Math.min(0.8, a.vol));
				if (typeof a.ch === 'string' && CHORDS.some((c) => c.id === a.ch)) chordId = a.ch;
				if (typeof a.rg === 'string' && RINGS.some((r) => r.id === a.rg)) ringId = a.rg;
			}
		} catch {
			/* storage unavailable */
		}
		overlaysReady = true;
		// If sound was left on, the AudioContext can only start after a user
		// gesture — arm it on the first interaction.
		if (soundOn) {
			const unlock = () => {
				ensureAudio();
				window.removeEventListener('pointerdown', unlock);
			};
			window.addEventListener('pointerdown', unlock);
		}

		basemap.init();
		webglOk = hasWebGL();
		if (!webglOk) {
			// No WebGL → no live propagation (it needs MapLibre); show a static
			// node map via Leaflet, still refreshing the located-node set.
			loadNodes();
			const t = setInterval(loadNodes, 30000);
			return () => clearInterval(t);
		}
		currentBasemap = basemap.id;
		basemapLight = isLight();
		map = new maplibregl.Map({
			container: mapEl,
			style: basemapStyle(currentBasemap, basemapLight),
			center: MAP_CENTER_LONLAT,
			zoom: 8.4,
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
			loadNodes();
		});
		// Re-add overlays after a basemap (theme) style swap drops them.
		map.on('styledata', ensureOverlays);
		const t = setInterval(loadNodes, 30000);
		requestAnimationFrame(frame);
		return () => {
			clearInterval(t);
			map?.remove();
			map = null;
		};
	});
</script>

<Seo
	title="Live MeshCore Signal Map"
	description="Real-time signal map of the MeshCore LoRa mesh — watch packets and links light up as they happen."
	path="/live-map"
/>

<PageHeader eyebrow="Real-time Propagation" title="Live Map">
	<div class="font-mono text-fg-dim flex items-center gap-4 text-xs">
		<span><span class="text-signal tnum">{located.length}</span> <span class="text-fg-faint">located</span></span>
		<span class="flex items-center gap-1.5">
			{#if live.connected}<span class="live-dot"></span>{/if}
			<span class="text-fg-faint">{animCount} active</span>
		</span>
	</div>
</PageHeader>

<div class="px-6 py-6 md:px-10">
	<div class="panel relative overflow-hidden" style="height:calc(100vh - 220px);min-height:420px">
		{#if !webglOk}
			<FallbackMap
				nodes={located}
				roleFilter={selectedRoles}
				center={MAP_CENTER_LONLAT}
				zoom={8}
				live
				audio
				bind:bannerOpen={fbBanner}
				onselect={(k) => (nodeKey = k)}
				notice="WebGL is disabled — showing the basic live map. Enable WebGL for terrain and the full-fidelity animation."
			/>
			<MapRoleFilter
				bind:selected={selectedRoles}
				title="Map Control"
				bind:open={mapCtrlOpen}
				pos={fbBanner ? 'top-16' : 'top-3'}
			>
				<ChimeControls />
			</MapRoleFilter>
			<BasemapSelector posClass={fbBanner ? 'top-16 right-3' : 'top-3 right-3'} />
		{:else}
		<div bind:this={mapEl} class="h-full w-full"></div>
		<BasemapSelector />

		<MapRoleFilter bind:selected={selectedRoles} title="Map Control" bind:open={mapCtrlOpen}>
			<div class="label mb-1.5">Audio</div>
			<Tooltip text={soundOn ? 'Mute node chimes' : 'Play a soft wind chime as pulses reach nodes'} class="block w-full">
				<button
					onclick={toggleSound}
					aria-pressed={soundOn}
					class="flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] border px-2 py-1 text-[0.68rem] font-medium transition-colors {soundOn
						? 'border-signal/50 text-signal'
						: 'border-line text-fg-dim hover:text-fg'}"
				>
					{#if soundOn}
						<svg viewBox="0 0 24 24" class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M11 5 6 9H2v6h4l5 4z" /><path d="M15.5 8.5a5 5 0 0 1 0 7M19 5a9 9 0 0 1 0 14" /></svg>
						<span>Sound on</span>
					{:else}
						<svg viewBox="0 0 24 24" class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M11 5 6 9H2v6h4l5 4z" /><path d="M22 9l-6 6M16 9l6 6" /></svg>
						<span>Muted</span>
					{/if}
				</button>
			</Tooltip>

			{#if soundOn}
				<div class="border-line/60 mt-2 space-y-2 border-t pt-2">
					<div>
						<div class="label mb-1 flex items-center justify-between">
							<span>Volume</span><span class="text-fg-faint tnum">{Math.round((volume / 0.8) * 100)}%</span>
						</div>
						<input
							type="range"
							min="0"
							max="0.8"
							step="0.05"
							bind:value={volume}
							onchange={previewChime}
							class="accent-signal h-1 w-full cursor-pointer"
						/>
					</div>
					<div>
						<div class="label mb-1">Chord</div>
						<div class="grid grid-cols-2 gap-1">
							{#each CHORDS as c (c.id)}
								<button
									onclick={() => {
										chordId = c.id;
										previewChime();
									}}
									class="rounded-[var(--radius)] border px-1.5 py-1 text-[0.66rem] font-medium transition-colors {chordId ===
									c.id
										? 'border-signal/50 text-signal'
										: 'border-line text-fg-dim hover:text-fg'}">{c.label}</button
								>
							{/each}
						</div>
					</div>
					<div>
						<div class="label mb-1">Ring</div>
						<div class="grid grid-cols-3 gap-1">
							{#each RINGS as r (r.id)}
								<button
									onclick={() => {
										ringId = r.id;
										previewChime();
									}}
									class="rounded-[var(--radius)] border px-0.5 py-1 text-center text-[0.66rem] font-medium transition-colors {ringId ===
									r.id
										? 'border-signal/50 text-signal'
										: 'border-line text-fg-dim hover:text-fg'}">{r.label}</button
								>
							{/each}
						</div>
					</div>
				</div>
			{/if}
		</MapRoleFilter>
		{/if}

		<!-- Recent packets overlay (bottom-left) — shared by the MapLibre and
		     WebGL-free live maps. -->
		<div
			class="border-line bg-ink-2/85 absolute bottom-3 left-3 z-10 w-[280px] overflow-hidden rounded-[var(--radius)] border shadow-lg backdrop-blur-md"
		>
			<button
				onclick={() => (panelOpen = !panelOpen)}
				class="border-line/70 hover:bg-panel-2/60 flex w-full items-center gap-2 px-3 py-2 text-left transition-colors {panelOpen
					? 'border-b'
					: ''}"
			>
				{#if live.connected}<span class="live-dot"></span>{/if}
				<span class="font-display text-fg text-xs font-700 tracking-wide">RECENT PACKETS</span>
				<span class="label ml-auto tnum">{recent.length}</span>
				<svg
					viewBox="0 0 24 24"
					class="text-fg-faint h-3.5 w-3.5 transition-transform {panelOpen ? '' : 'rotate-180'}"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"><path d="M6 9l6 6 6-6" /></svg
				>
			</button>

			{#if panelOpen}
				<div class="max-h-[300px] overflow-y-auto">
					{#if recent.length === 0}
						<div class="text-fg-faint px-3 py-6 text-center text-xs">Waiting for packets…</div>
					{:else}
						<div class="divide-line/40 divide-y">
							{#each recent as g (g.key)}
								<button
									onclick={() => (selected = g)}
									class="panel-hover flex w-full items-center gap-2 px-3 py-1.5 text-left"
								>
									<Tooltip text="Matches this packet's comet on the map" class="shrink-0">
										<span
											class="h-4 w-1 rounded-full"
											style="background:{hashColor(g.messageHash)}"
										></span>
									</Tooltip>
									<PayloadTag type={g.payloadType} />
									<span class="min-w-0 flex-1 truncate text-xs">
										{#if g.node}
											<span class="text-fg">{g.node.name || shortKey(g.node.publicKey)}</span>
										{:else}
											<span class="font-mono text-fg-faint">{g.messageHash}</span>
										{/if}
									</span>
									{#if g.count > 1}
										<span class="font-mono text-signal text-[0.6rem] tnum">×{g.count}</span>
									{/if}
									<span class="font-mono w-9 text-right text-[0.62rem] tnum" style="color:{snrColor(g.bestSnr)}"
										>{fmtSnr(g.bestSnr)}</span
									>
									<span class="font-mono text-fg-faint w-6 text-right text-[0.62rem] tnum">{ago(g.latest)}</span>
								</button>
							{/each}
						</div>
					{/if}
				</div>
			{/if}
		</div>
	</div>
</div>

<LiveGroupModal group={selected} onclose={() => (selected = null)} />
<NodeModal pubkey={nodeKey} onclose={() => (nodeKey = null)} />
