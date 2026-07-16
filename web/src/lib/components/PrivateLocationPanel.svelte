<!--
  A node's PRIVATE exact location. Renders nothing unless the signed-in user is
  the node's verified owner OR someone the owner has shared the location with.
  The pin the owner drops here is never shown on the public map or in any public
  API — it's stored separately and read back only by those two roles.

  - Owner: full editor (drag/click a pin, coords, label, save/clear) + a "Shared
    with" panel to grant/revoke read access to other registered users by email.
  - Shared-with viewer: read-only map + coordinates, labelled with who shared it.

  Collapsible + minimized by default, matching the notes panel.
-->
<script lang="ts">
	import { onDestroy } from 'svelte';
	import {
		privateLocation,
		locationShares,
		type PrivateLocation,
		type LocationShare,
		type UserBrief
	} from '$lib/api';
	import { auth } from '$lib/auth.svelte';
	import { theme } from '$lib/theme.svelte';
	import { isLight } from '$lib/map-util';
	import { MAP_CENTER_LATLON } from '$lib/site';
	import UserAutocomplete from './UserAutocomplete.svelte';

	interface Props {
		pubkey: string;
		/** The node's publicly advertised coords, used only to seed the map centre. */
		seedLat?: number | null;
		seedLon?: number | null;
	}
	let { pubkey, seedLat = null, seedLon = null }: Props = $props();

	// Fallback centre if we have neither a saved nor an advertised location — the
	// configured default map centre for this deployment's region.
	const FALLBACK: [number, number] = MAP_CENTER_LATLON;

	let canView = $state(false); // owner OR shared-with — controls whether the panel shows
	let canEdit = $state(false); // owner only
	let sharedBy = $state<{ userId: number; displayName: string } | null>(null);
	let ready = $state(false); // finished the access probe

	let saved = $state<PrivateLocation | null>(null);
	let lat = $state<number | null>(null);
	let lon = $state<number | null>(null);
	let label = $state('');
	let busy = $state(false);
	let error = $state('');
	let justSaved = $state(false);

	// Sharing (owner only).
	let shares = $state<LocationShare[]>([]);
	let shareBusy = $state(false);
	let shareError = $state('');
	const sharedIds = $derived(shares.map((s) => s.granteeUserId));

	let loadedFor = $state('');
	$effect(() => {
		if (pubkey && pubkey !== loadedFor) {
			loadedFor = pubkey;
			probe();
		}
	});

	async function probe() {
		canView = false;
		ready = false;
		try {
			const res = await privateLocation.get(pubkey);
			canView = true;
			canEdit = res.canEdit ?? false;
			sharedBy = res.sharedBy ?? null;
			applyLocation(res.set ? (res.location ?? null) : null);
			if (canEdit) await loadShares();
		} catch {
			// 403 (or any error) → the caller isn't permitted; hide the panel.
			canView = false;
		} finally {
			ready = true;
		}
	}

	function applyLocation(loc: PrivateLocation | null) {
		if (loc) {
			saved = loc;
			lat = loc.latitude;
			lon = loc.longitude;
			label = loc.label;
		} else {
			saved = null;
			lat = canEdit ? (seedLat ?? null) : null;
			lon = canEdit ? (seedLon ?? null) : null;
			label = '';
		}
	}

	async function loadShares() {
		try {
			shares = await locationShares.list(pubkey);
		} catch {
			shares = [];
		}
	}

	async function save() {
		if (lat == null || lon == null) {
			error = 'Drop a pin or enter coordinates first.';
			return;
		}
		busy = true;
		error = '';
		try {
			const res = await privateLocation.set(auth.csrf, pubkey, lat, lon, label.trim());
			saved = res.location ?? null;
			justSaved = true;
			setTimeout(() => (justSaved = false), 2500);
		} catch (e) {
			error = String((e as Error).message ?? e);
		} finally {
			busy = false;
		}
	}

	async function clear() {
		busy = true;
		error = '';
		try {
			await privateLocation.remove(auth.csrf, pubkey);
			saved = null;
			lat = seedLat ?? null;
			lon = seedLon ?? null;
			label = '';
			if (map && marker) {
				map.removeLayer(marker);
				marker = null;
			}
		} catch (e) {
			error = String((e as Error).message ?? e);
		} finally {
			busy = false;
		}
	}

	async function grantUser(u: UserBrief) {
		shareBusy = true;
		shareError = '';
		try {
			shares = await locationShares.grant(auth.csrf, pubkey, { userId: u.id });
		} catch (e) {
			shareError = String((e as Error).message ?? e);
		} finally {
			shareBusy = false;
		}
	}

	async function revoke(sh: LocationShare) {
		shareError = '';
		try {
			await locationShares.revoke(auth.csrf, pubkey, sh.granteeUserId);
			shares = shares.filter((s) => s.granteeUserId !== sh.granteeUserId);
		} catch (e) {
			shareError = String((e as Error).message ?? e);
		}
	}

	// ---- Interactive Leaflet map (lazy; only mounted while expanded) ----
	let mapEl: HTMLDivElement | undefined = $state();
	/* eslint-disable @typescript-eslint/no-explicit-any */
	let L: any = null;
	let map: any = null;
	let marker: any = null;
	let tiles: any = null;
	let pinIcon: any = null;
	/* eslint-enable @typescript-eslint/no-explicit-any */
	let curLight = false;

	// A themed teal map pin as an inline divIcon — avoids Leaflet's default PNG
	// marker (whose asset path doesn't resolve under the bundler) and matches the
	// site's accent colour.
	const pinSvg =
		'<svg width="28" height="28" viewBox="0 0 24 24" fill="#34e3c4" stroke="#0b1f1a" stroke-width="1.3"><path d="M12 2c-3.87 0-7 3.13-7 7 0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7z"/><circle cx="12" cy="9" r="2.4" fill="#0b1f1a" stroke="none"/></svg>';

	// Selectable raster base layers. Only "map" follows the UI light/dark theme;
	// the rest are fixed imagery. All are free, key-less, Leaflet-friendly tiles.
	interface LayerOpt {
		id: string;
		label: string;
		desc: string;
		themed?: boolean;
	}
	const LAYERS: LayerOpt[] = [
		{ id: 'map', label: 'Map', desc: 'Clean, theme-aware base', themed: true },
		{ id: 'satellite', label: 'Satellite', desc: 'Aerial imagery' },
		{ id: 'street', label: 'Street', desc: 'Detailed streets & places' },
		{ id: 'topo', label: 'Topographic', desc: 'Contours & relief' }
	];
	let layerId = $state('map');
	let layerOpen = $state(false);
	const currentLayer = $derived(LAYERS.find((l) => l.id === layerId) ?? LAYERS[0]);

	const cartoUrl = (light: boolean) =>
		`https://{s}.basemaps.cartocdn.com/${light ? 'light_all' : 'dark_all'}/{z}/{x}/{y}{r}.png`;
	const cartoAttr =
		'© <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a> © <a href="https://carto.com/attributions" target="_blank" rel="noopener">CARTO</a>';

	// Build the Leaflet tile layer for a base-layer id at the current theme.
	function makeTiles(id: string, light: boolean) {
		switch (id) {
			case 'satellite':
				return L.tileLayer(
					'https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}',
					{
						maxZoom: 19,
						attribution:
							'Imagery © <a href="https://www.esri.com" target="_blank" rel="noopener">Esri</a>, Maxar, Earthstar Geographics'
					}
				);
			case 'street':
				return L.tileLayer('https://{s}.basemaps.cartocdn.com/rastertiles/voyager/{z}/{x}/{y}{r}.png', {
					subdomains: 'abcd',
					maxZoom: 19,
					attribution: cartoAttr
				});
			case 'topo':
				return L.tileLayer('https://{s}.tile.opentopomap.org/{z}/{x}/{y}.png', {
					subdomains: 'abc',
					maxZoom: 17,
					attribution:
						'© <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener">OpenStreetMap</a> contributors, SRTM | © <a href="https://opentopomap.org" target="_blank" rel="noopener">OpenTopoMap</a> (CC-BY-SA)'
				});
			case 'map':
			default:
				return L.tileLayer(cartoUrl(light), { subdomains: 'abcd', maxZoom: 19, attribution: cartoAttr });
		}
	}

	// Swap the active base layer, preserving the marker + view.
	function selectLayer(id: string) {
		layerId = id;
		layerOpen = false;
		if (!map || !L) return;
		if (tiles) map.removeLayer(tiles);
		tiles = makeTiles(id, curLight).addTo(map);
	}

	// Initialise the map once the panel has access and the container is in the DOM.
	// The panel is mounted inside the Node Admin modal, so this runs when it opens;
	// onDestroy tears the map down when the modal closes.
	$effect(() => {
		if (ready && canView && mapEl && !map) {
			initMap(mapEl);
		}
	});

	async function initMap(el: HTMLDivElement) {
		await import('leaflet/dist/leaflet.css');
		L = (await import('leaflet')).default ?? (await import('leaflet'));
		if (!el || map) return;
		curLight = isLight();
		pinIcon = L.divIcon({
			className: 'rl-private-pin',
			html: pinSvg,
			iconSize: [28, 28],
			iconAnchor: [14, 26]
		});
		const center: [number, number] = [lat ?? FALLBACK[0], lon ?? FALLBACK[1]];
		map = L.map(el, { center, zoom: lat != null ? 14 : 9, attributionControl: true });
		tiles = makeTiles(layerId, curLight).addTo(map);
		if (lat != null && lon != null) placeMarker(lat, lon);
		// Owner may click anywhere to drop / move the pin; viewers can only pan/zoom.
		if (canEdit) {
			map.on('click', (e: { latlng: { lat: number; lng: number } }) => {
				setCoords(e.latlng.lat, e.latlng.lng);
			});
		}
		setTimeout(() => map?.invalidateSize(), 80);
	}

	function placeMarker(la: number, lo: number) {
		if (!L || !map) return;
		if (marker) {
			marker.setLatLng([la, lo]);
			return;
		}
		marker = L.marker([la, lo], { draggable: canEdit, icon: pinIcon }).addTo(map);
		if (canEdit) {
			marker.on('dragend', () => {
				const p = marker.getLatLng();
				setCoords(p.lat, p.lng);
			});
		}
	}

	// Round to ~1m precision so the inputs stay tidy.
	function setCoords(la: number, lo: number) {
		lat = Math.round(la * 1e5) / 1e5;
		lon = Math.round(lo * 1e5) / 1e5;
		placeMarker(lat, lon);
	}

	// Keep the marker in sync when the number inputs change.
	function onInput() {
		if (lat != null && lon != null && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180) {
			placeMarker(lat, lon);
			map?.panTo([lat, lon]);
		}
	}

	// Swap tile theme with the UI — only the themed "map" layer changes URL.
	$effect(() => {
		void theme.mode;
		const light = isLight();
		if (!map || !tiles || light === curLight) return;
		curLight = light;
		if (currentLayer.themed) tiles.setUrl(cartoUrl(light));
	});

	onDestroy(() => {
		map?.remove();
		map = null;
	});
</script>

{#if ready && canView}
	{#if canEdit}
		<p class="text-fg-faint mb-3 text-xs leading-relaxed">
			This exact location is <span class="text-fg-dim font-600">private</span> — only you and people
			you share it with can see it. It never appears on the public map or in any public data. Drag the
			pin (or click the map) to set your node's true position.
		</p>
	{:else}
		<p class="text-fg-faint mb-3 text-xs leading-relaxed">
			<span class="text-fg-dim font-600">{sharedBy?.displayName ?? 'The owner'}</span> shared this node's
			private exact location with you. It's read-only and never appears on the public map.
		</p>
	{/if}

	{#if canEdit || saved}
		<div class="border-line/70 relative mb-3 h-64 overflow-hidden rounded-[var(--radius)] border">
			<div bind:this={mapEl} class="h-full w-full"></div>

			<!-- Base-layer selector (top-right, above the Leaflet panes) -->
			<div class="absolute top-2 right-2 z-[1000]">
				<button
					type="button"
					onclick={() => (layerOpen = !layerOpen)}
					class="border-line bg-ink-2/85 hover:bg-panel-2/70 flex items-center gap-1.5 rounded-[var(--radius)] border px-2.5 py-1.5 backdrop-blur-md transition-colors"
					aria-label="Base map: {currentLayer.label}"
				>
					<svg
						viewBox="0 0 24 24"
						class="text-fg-dim h-3.5 w-3.5 shrink-0"
						fill="none"
						stroke="currentColor"
						stroke-width="1.7"
						stroke-linecap="round"
						stroke-linejoin="round"
						><path d="M12 2 2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" /></svg
					>
					<span class="text-fg text-xs font-600">{currentLayer.label}</span>
					<svg
						viewBox="0 0 24 24"
						class="text-fg-faint h-3 w-3 shrink-0 transition-transform {layerOpen ? 'rotate-180' : ''}"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"><path d="M6 9l6 6 6-6" /></svg
					>
				</button>
				{#if layerOpen}
					<div
						class="border-line bg-ink-2/95 mt-1 w-44 overflow-hidden rounded-[var(--radius)] border backdrop-blur-md"
					>
						{#each LAYERS as l (l.id)}
							{@const on = l.id === layerId}
							<button
								type="button"
								onclick={() => selectLayer(l.id)}
								class="hover:bg-panel-2/60 flex w-full items-center gap-2 px-3 py-2 text-left transition-colors {on
									? 'bg-signal/10'
									: ''}"
							>
								<span
									class="h-1.5 w-1.5 shrink-0 rounded-full"
									style="background:{on ? 'var(--color-signal)' : 'var(--color-fg-faint)'}"
								></span>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-xs font-600 {on ? 'text-signal' : 'text-fg'}"
										>{l.label}</span
									>
									<span class="text-fg-faint mt-0.5 block truncate text-[0.62rem] leading-tight"
										>{l.desc}</span
									>
								</span>
							</button>
						{/each}
					</div>
				{/if}
			</div>
		</div>
	{/if}

	<div class="mb-3 flex flex-wrap items-end gap-3">
		<label class="flex flex-col gap-1">
			<span class="text-fg-faint text-[0.68rem]">Latitude</span>
			<input
				type="number"
				step="0.00001"
				bind:value={lat}
				oninput={onInput}
				readonly={!canEdit}
				class="bg-ink-2 text-fg focus:border-signal w-32 rounded-[var(--radius)] border border-transparent px-2.5 py-1.5 text-sm outline-none read-only:opacity-70"
			/>
		</label>
		<label class="flex flex-col gap-1">
			<span class="text-fg-faint text-[0.68rem]">Longitude</span>
			<input
				type="number"
				step="0.00001"
				bind:value={lon}
				oninput={onInput}
				readonly={!canEdit}
				class="bg-ink-2 text-fg focus:border-signal w-32 rounded-[var(--radius)] border border-transparent px-2.5 py-1.5 text-sm outline-none read-only:opacity-70"
			/>
		</label>
		<label class="flex flex-1 flex-col gap-1">
			<span class="text-fg-faint text-[0.68rem]">Label {canEdit ? '(optional)' : ''}</span>
			<input
				type="text"
				bind:value={label}
				maxlength="120"
				readonly={!canEdit}
				placeholder={canEdit ? 'rooftop, repeater site…' : ''}
				class="bg-ink-2 text-fg focus:border-signal min-w-32 rounded-[var(--radius)] border border-transparent px-2.5 py-1.5 text-sm outline-none read-only:opacity-70"
			/>
		</label>
	</div>

	{#if error}<p class="text-coral mb-2 text-xs">{error}</p>{/if}

	{#if canEdit}
		<div class="flex items-center gap-2">
			<button
				onclick={save}
				disabled={busy || lat == null || lon == null}
				class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 rounded-[var(--radius)] border px-3.5 py-1.5 text-sm font-600 transition-colors disabled:opacity-40"
				>{busy ? 'Saving…' : saved ? 'Update location' : 'Save location'}</button
			>
			{#if saved}
				<button
					onclick={clear}
					disabled={busy}
					class="text-fg-faint hover:text-coral rounded-[var(--radius)] px-3 py-1.5 text-sm transition-colors"
					>Clear</button
				>
			{/if}
			{#if justSaved}<span class="text-signal text-xs font-600">Saved ✓</span>{/if}
		</div>

		<!-- Shared with — grant/revoke read access to other registered users -->
		<div class="border-line/60 mt-4 border-t pt-3">
			<div class="mb-2 flex items-center gap-2">
				<span class="label normal-case text-fg-dim">Shared with</span>
				{#if shares.length}
					<span class="bg-line/60 text-fg-dim rounded-full px-2 py-0.5 text-[0.62rem] font-600"
						>{shares.length}</span
					>
				{/if}
			</div>
			{#if shares.length}
				<ul class="mb-2.5 space-y-1.5">
					{#each shares as sh (sh.granteeUserId)}
						<li class="flex items-center gap-2">
							<span
								class="bg-signal/15 text-signal grid h-6 w-6 place-items-center rounded-full text-[0.6rem] font-700"
								>{(sh.displayName || sh.email).slice(0, 2).toUpperCase()}</span
							>
							<span class="text-fg min-w-0 truncate text-sm font-600">{sh.displayName}</span>
							<span class="text-fg-faint min-w-0 flex-1 truncate text-xs">{sh.email}</span>
							<button onclick={() => revoke(sh)} class="text-fg-faint hover:text-coral shrink-0 text-xs"
								>Revoke</button
							>
						</li>
					{/each}
				</ul>
			{:else}
				<p class="text-fg-faint mb-2.5 text-xs">Not shared with anyone yet.</p>
			{/if}
			<UserAutocomplete
				onselect={grantUser}
				exclude={sharedIds}
				placeholder={shareBusy ? 'Sharing…' : 'Share with a user by username…'}
			/>
			{#if shareError}<p class="text-coral mt-1.5 text-xs">{shareError}</p>{/if}
			<p class="text-fg-faint mt-1.5 text-[0.68rem] leading-relaxed">
				Start typing a registered Ridgeline user's username to give them read-only access to this exact
				location. They can't edit it or share it further.
			</p>
		</div>
	{/if}
{/if}
