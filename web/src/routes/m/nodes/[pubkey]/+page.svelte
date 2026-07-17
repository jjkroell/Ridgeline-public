<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { page } from '$app/state';
	import maplibregl from 'maplibre-gl';
	import 'maplibre-gl/dist/maplibre-gl.css';
	import QRCode from 'qrcode';
	import { api, type Node, type NodeAnalytics, type NodeObserverStat, type BlockEntry } from '$lib/api';
	import { ago, shortKey, fmtNum, fmtSnr, snrColor, roleColor, roleLabel, nodeStatus, fmtRadio, fmtCoord } from '$lib/format';
	import { nodeHashId } from '$lib/hash-ids';
	import { favorites } from '$lib/favorites.svelte';
	import { basemapStyleUrl, collapseAttribution } from '$lib/map-basemap';
	import { theme } from '$lib/theme.svelte';
	import { hasWebGL } from '$lib/webgl';
	import LeafletInset from '$lib/components/LeafletInset.svelte';
	import NodeAdmin from '$lib/components/NodeAdmin.svelte';

	const pubkey = $derived((page.params.pubkey ?? '').toUpperCase());

	let node = $state<Node | null>(null);
	let detail = $state<NodeAnalytics | null>(null);
	let nodesList = $state<Node[]>([]);
	let quarantined = $state(false);
	let block = $state<BlockEntry | null>(null);
	let loaded = $state(false);
	let qrSvg = $state('');
	let copied = $state(false);

	async function refresh() {
		try {
			const [resp, list] = await Promise.all([api.nodeDetail(pubkey), api.nodes()]);
			node = resp.node;
			detail = resp.detail;
			quarantined = !!resp.quarantined;
			block = resp.block ?? null;
			nodesList = list;
		} finally {
			loaded = true;
		}
	}
	// "Heard by" observers over a selectable range. Queried on demand so the range
	// can span a node's advert cadence (~30h) — the fixed snapshot window can't.
	const obsRanges = [
		{ label: '6h', sec: 21600 },
		{ label: '24h', sec: 86400 },
		{ label: '3d', sec: 259200 },
		{ label: '7d', sec: 604800 }
	];
	let observers = $state<NodeObserverStat[]>([]);
	let obsRange = $state(259200); // 3d
	let obsLoading = $state(false);
	async function loadObservers() {
		obsLoading = true;
		try {
			observers = await api.nodeObservers(pubkey, obsRange);
		} catch {
			observers = [];
		} finally {
			obsLoading = false;
		}
	}
	function setObsRange(sec: number) {
		if (sec === obsRange) return;
		obsRange = sec;
		loadObservers();
	}

	onMount(() => {
		refresh();
		loadObservers();
		const t = setInterval(refresh, 15000);
		return () => clearInterval(t);
	});

	const status = $derived(nodeStatus({ lastSeen: node?.lastSeen, lastRelayed: detail?.relay.lastRelayed }));
	const isRelay = $derived(node?.role === 'Repeater' || node?.role === 'RoomServer');
	const hasLoc = $derived(!!node && node.latitude != null && node.longitude != null);

	const hashId = $derived(nodeHashId(nodesList, node, pubkey));

	function cadence(sec?: number): string {
		if (sec == null) return '—';
		if (sec < 3600) return `every ~${Math.round(sec / 60)}m`;
		return `every ~${(sec / 3600).toFixed(1)}h`;
	}
	const facts = $derived(
		node
			? [
					{ k: 'Status', v: status.label, c: status.color },
					{ k: 'Radio', v: fmtRadio(node.radio) },
					{ k: 'Location', v: fmtCoord(node.latitude, node.longitude) },
					{ k: 'Last advert', v: ago(node.lastAdvert || node.lastSeen) + ' ago' },
					{ k: 'Last relay', v: detail?.relay.lastRelayed ? ago(detail.relay.lastRelayed) + ' ago' : '—' },
					{ k: 'Packets', v: detail ? `${fmtNum(detail.totalPackets)} · seen ${fmtNum(detail.totalObservations)}×` : '—' },
					{ k: 'Adverts (all-time)', v: fmtNum(node.advertTxCount) },
					{ k: 'Avg SNR', v: detail?.avgSnr != null ? detail.avgSnr.toFixed(1) + ' dB' : '—', c: snrColor(detail?.avgSnr) },
					{ k: 'Avg hops', v: detail?.avgHops != null ? detail.avgHops.toFixed(1) : '—' },
					{ k: 'Advert cadence', v: cadence(detail?.advertIntervalSec) }
				]
			: []
	);
	const activityMax = $derived(Math.max(1, ...(detail?.activity ?? [])));

	// QR — meshcore://contact/add per spec; lowercase 64-hex key (canonical),
	// type 1=Companion 2=Repeater 3=RoomServer 4=Sensor. EC 'L' keeps the code
	// low-density so it scans easily even for long / emoji node names.
	$effect(() => {
		if (!node) return;
		const n = node;
		const role = n.role === 'Repeater' ? 2 : n.role === 'RoomServer' ? 3 : n.role === 'Sensor' ? 4 : 1;
		const url = `meshcore://contact/add?name=${encodeURIComponent(n.name || 'Unknown')}&public_key=${n.publicKey.toLowerCase()}&type=${role}`;
		QRCode.toString(url, { type: 'svg', margin: 1, errorCorrectionLevel: 'L' }).then((s) => (qrSvg = s)).catch(() => (qrSvg = ''));
	});

	async function copyKey() {
		try {
			await navigator.clipboard.writeText(pubkey);
			copied = true;
			setTimeout(() => (copied = false), 1200);
		} catch {
			/* clipboard unavailable */
		}
	}

	// Inset map — created once when a location is known.
	let mapEl = $state<HTMLDivElement>();
	let map: maplibregl.Map | null = null;
	let marker: maplibregl.Marker | null = null;
	const webglOk = hasWebGL();
	$effect(() => {
		if (!hasLoc || !mapEl || map || !webglOk) return;
		const lat = untrack(() => node!.latitude!);
		const lng = untrack(() => node!.longitude!);
		const light = theme.mode === 'light';
		map = new maplibregl.Map({ container: mapEl, style: basemapStyleUrl(light), center: [lng, lat], zoom: 11, attributionControl: { compact: true }, interactive: false });
		marker = new maplibregl.Marker({ color: '#34e3c4' }).setLngLat([lng, lat]).addTo(map);
		map.on('load', () => { map?.resize(); if (map) collapseAttribution(map); });
	});
	$effect(() => {
		if (map && marker && node?.latitude != null && node?.longitude != null) marker.setLngLat([node.longitude, node.latitude]);
	});
	onMount(() => () => { map?.remove(); map = null; });
</script>

<div class="px-4 py-4">
	{#if !loaded}
		<div class="text-fg-faint py-16 text-center text-sm">Loading…</div>
	{:else if quarantined}
		<div class="border-coral/40 bg-coral/5 rounded-2xl border p-5">
			<div class="text-coral flex items-center gap-2 text-sm font-700"><span class="text-base">⚠</span> QUARANTINED NODE</div>
			<p class="text-fg mt-3 text-sm leading-relaxed">Suspected of acting as an RF bridge or MQTT injector, feeding traffic from another mesh. Hidden from the maps, list, and feed pending review.</p>
			<div class="text-fg-faint mt-3 font-mono text-xs">{block?.name || shortKey(pubkey)} · {block?.reason || ''}</div>
			<a href="/m/admin" class="text-signal mt-3 inline-block text-xs">Manage in Admin →</a>
		</div>
	{:else if !node}
		<div class="text-fg-faint py-16 text-center text-sm">Node not found — it may not have advertised yet.</div>
	{:else}
		<!-- heading -->
		<div class="mb-3 flex items-center gap-2.5">
			<button onclick={() => favorites.toggle(pubkey)} aria-label="Favorite" class="-ml-1 shrink-0 p-1">
				<svg viewBox="0 0 24 24" class="h-6 w-6 {favorites.has(pubkey) ? 'text-amber' : 'text-fg-faint'}" fill={favorites.has(pubkey) ? 'currentColor' : 'none'} stroke="currentColor" stroke-width="1.5"><path d="M12 2.5l2.9 5.9 6.5.95-4.7 4.6 1.1 6.45L12 17.9l-5.8 3.05 1.1-6.45-4.7-4.6 6.5-.95z" /></svg>
			</button>
			<h1 class="text-fg min-w-0 flex-1 truncate text-lg font-700">{node.name || shortKey(pubkey)}</h1>
			<span class="rounded-full px-2 py-0.5 text-[0.6rem] font-700" style="color:{roleColor(node.role)};background:color-mix(in srgb, {roleColor(node.role)} 14%, transparent)">{roleLabel(node.role)}</span>
		</div>
		<div class="mb-4 flex items-center gap-2">
			<span class="h-2 w-2 rounded-full" style="background:{status.color}"></span>
			<span class="font-mono text-xs" style="color:{status.color}">{status.label}</span>
		</div>

		<!-- pubkey -->
		<div class="border-line/60 bg-panel mb-3 rounded-2xl border px-4 py-3">
			<div class="flex items-center justify-between">
				<span class="label">Public key</span>
				<button onclick={copyKey} class="text-xs {copied ? 'text-signal' : 'text-fg-faint active:text-fg'}">{copied ? 'copied' : 'copy'}</button>
			</div>
			<div class="text-fg-dim mt-1.5 font-mono text-[0.7rem] break-all">{pubkey}</div>
		</div>

		<!-- Node Admin: claim, private location, notes — each in its own modal -->
		<div class="mb-3">
			<NodeAdmin {pubkey} seedLat={node?.latitude} seedLon={node?.longitude} />
		</div>

		<!-- hash id -->
		<div class="border-line/60 bg-panel mb-3 rounded-2xl border px-4 py-3">
			<div class="label mb-1.5">Hash ID {#if hashId}· {hashId.bytes}-byte{/if}</div>
			{#if hashId}
				<div class="flex items-baseline gap-2.5">
					<span class="font-mono text-signal glow-signal text-2xl font-700 tracking-[0.12em]">{hashId.hex}</span>
					{#if hashId.artifactOf}
						<span class="text-coral bg-coral/10 rounded px-1.5 py-0.5 font-mono text-[0.6rem]">corrupted copy</span>
					{:else if hashId.shared === 0}
						<span class="text-signal bg-signal/10 rounded px-1.5 py-0.5 font-mono text-[0.6rem]">unique</span>
					{:else}
						<a
							href="/m/identity?len={hashId.bytes}&id={hashId.hex}"
							class="text-amber bg-amber/10 active:bg-amber/20 rounded px-1.5 py-0.5 font-mono text-[0.6rem] tnum"
							>+{hashId.shared} collision{hashId.shared > 1 ? 's' : ''} ›</a
						>
					{/if}
				</div>
				{#if hashId.artifactOf}
					<div class="text-coral/90 mt-2 text-xs leading-relaxed">
						⚠ Likely a packet-corruption artifact of <span class="text-signal"
							>{hashId.artifactOf.name || 'a real node'}</span
						> — {hashId.reason}.
					</div>
				{/if}
			{:else}
				<div class="text-fg-faint text-sm">Unknown — not seen advertising yet.</div>
			{/if}
		</div>

		<!-- location -->
		{#if hasLoc}
			<div class="border-line/60 mb-3 h-48 w-full overflow-hidden rounded-2xl border">
				{#if webglOk}
					<div bind:this={mapEl} class="h-full w-full"></div>
				{:else}
					<LeafletInset lat={node!.latitude!} lon={node!.longitude!} />
				{/if}
			</div>
		{/if}

		<!-- QR -->
		{#if qrSvg}
			<div class="border-line/60 bg-panel mb-3 flex flex-col items-center gap-2 rounded-2xl border px-4 py-4">
				<span class="label self-start">MeshCore Contact</span>
				<div class="w-48 rounded-lg bg-white p-2.5">{@html qrSvg}</div>
				<span class="text-fg-faint text-center text-[0.62rem]">Scan from the MeshCore app: Contacts → + → Scan QR</span>
			</div>
		{/if}

		<!-- facts -->
		<div class="border-line/60 bg-panel divide-line/50 mb-3 divide-y overflow-hidden rounded-2xl border">
			{#each facts as f (f.k)}
				<div class="flex items-center justify-between gap-3 px-4 py-2.5">
					<span class="label normal-case shrink-0">{f.k}</span>
					<span class="font-mono text-right text-sm tnum break-all" style="color:{f.c ?? 'var(--color-fg)'}">{f.v}</span>
				</div>
			{/each}
		</div>

		<!-- relay activity -->
		{#if isRelay && detail}
			<div class="border-line/60 bg-panel mb-3 rounded-2xl border px-4 py-3.5">
				<div class="label mb-3">Relay Activity</div>
				{#each [{ k: 'Traffic share', v: detail.trafficShare }, { k: 'Bridge centrality', v: detail.bridge }] as r (r.k)}
					<div class="mb-2 flex items-center gap-3">
						<span class="text-fg-dim w-28 shrink-0 text-xs">{r.k}</span>
						<div class="bg-line/40 h-2.5 flex-1 overflow-hidden rounded-full"><div class="bg-signal h-full" style="width:{Math.min(100, r.v * 100)}%"></div></div>
						<span class="text-fg-faint w-10 text-right font-mono text-xs tnum">{(r.v * 100).toFixed(0)}%</span>
					</div>
				{/each}
				<div class="text-fg-faint mt-1 font-mono text-[0.62rem]">relayed {fmtNum(detail.relay.count24h)}× / 24h{#if detail.relay.count1h} · {detail.relay.count1h}× last hr{/if}</div>
			</div>
		{/if}

		<!-- advert activity sparkline -->
		{#if detail?.activity?.length}
			<div class="border-line/60 bg-panel mb-3 rounded-2xl border px-4 py-3.5">
				<div class="label mb-3 flex items-center justify-between"><span>Advert Activity</span><span class="text-fg-faint font-mono normal-case">last {detail.windowHours}h</span></div>
				<div class="flex h-12 items-end gap-1">
					{#each detail.activity as count, i (i)}
						<div class="bg-signal/80 min-w-0 flex-1 rounded-sm" style="height:{count === 0 ? 2 : Math.max(8, (count / activityMax) * 100)}%;{count === 0 ? 'opacity:0.25' : ''}"></div>
					{/each}
				</div>
			</div>
		{/if}

		<!-- heard by (adverts received, over a selectable range — ~30h cadence
		     means a short window often shows none) -->
		<div class="mt-5 mb-2 flex items-center gap-2 px-1">
			<h2 class="font-display text-fg text-xs font-700 tracking-wide">HEARD BY · {observers.length}</h2>
			<div class="ml-auto flex items-center gap-1">
				{#each obsRanges as r (r.sec)}
					<button
						onclick={() => setObsRange(r.sec)}
						class="rounded-full border px-2 py-0.5 text-[0.6rem] font-600 transition-colors {obsRange === r.sec ? 'border-signal text-signal' : 'border-line text-fg-faint'}"
						>{r.label}</button
					>
				{/each}
			</div>
		</div>
		{#if obsLoading && observers.length === 0}
			<div class="text-fg-faint px-1 py-3 text-xs">Loading…</div>
		{:else if observers.length === 0}
			<div class="border-line/60 bg-panel text-fg-faint rounded-2xl border px-4 py-4 text-center text-xs">
				No adverts heard in this range.
			</div>
		{:else}
			<div class="border-line/60 bg-panel divide-line/50 divide-y overflow-hidden rounded-2xl border">
				{#each observers as o (o.id)}
					<a href="/m/observers/{encodeURIComponent(o.id)}" class="active:bg-line/40 flex items-center gap-3 px-4 py-2.5">
						<span class="text-fg min-w-0 flex-1 truncate text-sm">{o.id}</span>
						<span class="font-mono text-xs tnum" style="color:{snrColor(o.avgSnr)}">{fmtSnr(o.avgSnr)} dB</span>
						<span class="text-fg-faint font-mono text-xs tnum">×{o.count}</span>
					</a>
				{/each}
			</div>
		{/if}

		<!-- neighbours -->
		{#if detail?.neighbors?.length}
			<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">NEIGHBOURS · {detail.neighbors.length}</h2>
			<div class="border-line/60 bg-panel divide-line/50 divide-y overflow-hidden rounded-2xl border">
				{#each detail.neighbors as n (n.publicKey)}
					<a href="/m/nodes/{n.publicKey}" class="active:bg-line/40 flex items-center gap-3 px-4 py-2.5">
						<span class="h-2 w-2 shrink-0 rounded-full" style="background:{roleColor(n.role)}"></span>
						<span class="text-fg min-w-0 flex-1 truncate text-sm">{n.name || shortKey(n.publicKey)}</span>
						<span class="text-fg-faint font-mono text-xs tnum">×{n.count}</span>
					</a>
				{/each}
			</div>
		{/if}

	{/if}
</div>
