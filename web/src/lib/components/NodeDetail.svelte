<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import maplibregl from 'maplibre-gl';
	// Needed for marker/control styling when this component is reached directly
	// (e.g. /nodes/[pubkey]) without having visited a full map route first.
	import 'maplibre-gl/dist/maplibre-gl.css';
	import QRCode from 'qrcode';
	import { api, type Node, type NodeAnalytics, type NodeHistoryEntry, type NodeObserverStat, type NodeActivity, type BlockEntry } from '$lib/api';
	import { basemapStyleUrl, collapseAttribution } from '$lib/map-basemap';
	import { isLight } from '$lib/map-util';
	import { hasWebGL } from '$lib/webgl';
	import LeafletInset from './LeafletInset.svelte';
	import { ago, shortKey, fmtCoord, fmtSnr, snrColor, roleColor, roleLabel, nodeStatus, fmtRadio } from '$lib/format';
	import { nodeHashId } from '$lib/hash-ids';
	import PayloadTag from './PayloadTag.svelte';
	import RoleBadge from './RoleBadge.svelte';
	import FavoriteStar from './FavoriteStar.svelte';
	import Tooltip from './Tooltip.svelte';
	import NodeAdmin from './NodeAdmin.svelte';

	interface Props {
		pubkey: string;
		/** Full node list, when the parent already has it (for collision counts). */
		nodes?: Node[];
		/** Render a name + role heading (used in the modal; the page has its own). */
		heading?: boolean;
		/** Barebones snapshot: identity, location and key facts only — no QR,
		 * sparkline, relay scores or the observers/packets/history/neighbour lists.
		 * Used by the modal; the full page links from it for depth. */
		compact?: boolean;
	}
	let { pubkey, nodes: nodesProp = undefined, heading = false, compact = false }: Props = $props();

	let node = $state<Node | null>(null);
	let detail = $state<NodeAnalytics | null>(null);
	let nodesList = $state<Node[]>(nodesProp ?? []);
	let loaded = $state(false);
	let quarantined = $state(false);
	let block = $state<BlockEntry | null>(null);

	// On-demand stored history (own adverts + relayed packets) over a chosen range.
	const ranges = [
		{ label: '6h', sec: 21600 },
		{ label: '24h', sec: 86400 },
		{ label: '3d', sec: 259200 }
	];
	let history = $state<NodeHistoryEntry[]>([]);
	let histRange = $state(86400);
	let histLoading = $state(false);

	async function loadHistory() {
		histLoading = true;
		try {
			history = await api.nodeHistory(pubkey, histRange, 300);
		} catch {
			history = [];
		} finally {
			histLoading = false;
		}
	}
	function setRange(sec: number) {
		if (sec === histRange) return;
		histRange = sec;
		loadHistory();
	}

	// "Heard by" observers over a selectable range. The fixed analytics snapshot
	// only spans a few hours; since nodes advert roughly every ~30h, that window
	// rarely catches an advert — so this queries on demand over a wider range.
	const obsRanges = [
		{ label: '6h', sec: 21600 },
		{ label: '24h', sec: 86400 },
		{ label: '3d', sec: 259200 },
		{ label: '7d', sec: 604800 }
	];
	let observers = $state<NodeObserverStat[]>([]);
	let obsRange = $state(259200); // 3d — wide enough to span a typical advert cadence
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

	// Weekday×hour activity heatmap (full page only).
	let heatmap = $state<NodeActivity | null>(null);
	async function loadHeatmap() {
		try {
			heatmap = await api.nodeHeatmap(pubkey, 7);
		} catch {
			heatmap = null;
		}
	}
	const DAY = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
	function cellStyle(count: number, max: number): string {
		if (count === 0) return 'background:var(--color-line);opacity:0.4';
		return `background:var(--color-signal);opacity:${(0.2 + 0.8 * (count / Math.max(1, max))).toFixed(3)}`;
	}

	async function refresh() {
		try {
			const [resp, list] = await Promise.all([
				api.nodeDetail(pubkey),
				nodesProp ? Promise.resolve(nodesProp) : api.nodes()
			]);
			node = resp.node;
			detail = resp.detail;
			quarantined = !!resp.quarantined;
			block = resp.block ?? null;
			nodesList = list;
		} finally {
			loaded = true;
		}
	}
	onMount(() => {
		refresh();
		if (!compact) {
			loadObservers(); // "Heard by" over a wider, selectable range
			loadHistory(); // snapshot view skips the history list + heatmap
			loadHeatmap();
		}
		const t = setInterval(refresh, 15000);
		return () => clearInterval(t);
	});

	const hasLoc = $derived(!!node && node.latitude != null && node.longitude != null);

	// Hash ID + collision count. Corruption artifacts are filtered out so the
	// count reflects only genuinely distinct nodes — and if THIS record is itself
	// a corrupted copy of a real node, we surface that instead.
	const hashId = $derived(nodeHashId(nodesList, node, pubkey));

	// Liveness from the most recent of advert or relay activity (see nodeStatus).
	const status = $derived(
		nodeStatus({ lastSeen: node?.lastSeen, lastRelayed: detail?.relay.lastRelayed })
	);
	const isRelay = $derived(node?.role === 'Repeater' || node?.role === 'RoomServer');

	// Score → label/colour classification bands.
	function trafficLabel(s: number) {
		if (s >= 0.8) return { label: 'Critical', color: 'var(--color-signal)' };
		if (s >= 0.6) return { label: 'Valuable', color: 'var(--color-signal)' };
		if (s >= 0.3) return { label: 'Moderate', color: 'var(--color-amber)' };
		if (s >= 0.1) return { label: 'Marginal', color: 'var(--color-amber)' };
		return { label: 'Redundant', color: 'var(--color-fg-faint)' };
	}
	function bridgeLabel(s: number) {
		if (s >= 0.5) return { label: 'Critical bridge', color: 'var(--color-signal)' };
		if (s >= 0.2) return { label: 'Important', color: 'var(--color-signal)' };
		if (s >= 0.05) return { label: 'Some role', color: 'var(--color-amber)' };
		if (s > 0) return { label: 'Marginal', color: 'var(--color-amber)' };
		return { label: 'No bridge role', color: 'var(--color-fg-faint)' };
	}
	const fmtAbs = (iso?: string) => (iso ? new Date(iso).toLocaleString() : '—');

	// Advert cadence (heartbeat) → friendly "every ~N" string.
	function cadence(sec?: number): string {
		if (sec == null) return '—';
		if (sec < 90) return `every ~${Math.round(sec)}s`;
		if (sec < 5400) return `every ~${Math.round(sec / 60)}m`;
		return `every ~${(sec / 3600).toFixed(1)}h`;
	}
	const activityMax = $derived(Math.max(1, ...(detail?.activity ?? [1])));
	const tl = $derived(trafficLabel(detail?.trafficShare ?? 0));
	const bl = $derived(bridgeLabel(detail?.bridge ?? 0));

	// Overview facts. Compact (modal snapshot) shows a key subset; the full page
	// shows everything. `c` is an optional value colour.
	const facts = $derived.by((): { k: string; v: string; c?: string }[] => {
		if (!node) return [];
		const lastRelay = {
			k: 'Last relay',
			v: detail?.relay.lastRelayed
				? ago(detail.relay.lastRelayed) + ' ago' + (detail.relay.count1h ? ` · ${detail.relay.count1h}× last hr` : '')
				: 'none in 24h',
			c: detail?.relay.lastRelayed ? 'var(--color-fg)' : 'var(--color-fg-faint)'
		};
		const packets6h = {
			k: 'Packets (6h)',
			v: detail ? `${detail.totalPackets}` + (detail.totalObservations !== detail.totalPackets ? ` (seen ${detail.totalObservations}×)` : '') : '—'
		};
		const avgSnr = { k: 'Avg SNR', v: detail?.avgSnr != null ? detail.avgSnr.toFixed(1) + ' dB' : '—' };
		const location = { k: 'Location', v: fmtCoord(node.latitude, node.longitude) };
		const radio = { k: 'Radio', v: fmtRadio(node.radio) };
		if (compact) {
			return [
				{ k: 'Status', v: status.label, c: status.color },
				radio,
				location,
				{ k: 'Last advert', v: ago(node.lastSeen) + ' ago' },
				lastRelay,
				packets6h,
				avgSnr
			];
		}
		return [
			{ k: 'Status', v: status.label, c: status.color },
			radio,
			location,
			{ k: 'Last advert', v: ago(node.lastSeen) + ' ago' },
			lastRelay,
			{ k: 'First seen', v: ago(node.firstSeen) + ' ago' },
			packets6h,
			{ k: 'Packets today', v: detail ? String(detail.packetsToday) : '—' },
			{ k: 'Adverts (all-time)', v: String(node.advertTxCount) },
			{ k: 'Advert cadence', v: cadence(detail?.advertIntervalSec) },
			avgSnr,
			{ k: 'Avg hops', v: detail?.avgHops != null ? detail.avgHops.toFixed(1) : '—' }
		];
	});

	// Format "freq,bw,sf,cr" → "915.0 MHz · 62.5k · SF7 · CR5".
	let copied = $state(false);
	async function copyKey() {
		await navigator.clipboard.writeText(pubkey);
		copied = true;
		setTimeout(() => (copied = false), 1200);
	}

	// --- Inset map of the node's location ---
	let mapEl = $state<HTMLDivElement>();
	let map: maplibregl.Map | null = null;
	let marker: maplibregl.Marker | null = null;
	// Create the map ONCE. Gate on the memoized `hasLoc` boolean (not `node`,
	// which refresh() reassigns to a fresh object every 15s — tracking it here
	// would tear down and recreate the whole map on every poll, flickering the
	// attribution back open and never letting the marker settle). Coords are
	// read untracked so a node refresh can't retrigger this effect.
	const webglOk = hasWebGL();
	$effect(() => {
		if (!mapEl || !hasLoc || map || !webglOk) return;
		untrack(() => {
			const lng = node!.longitude!;
			const lat = node!.latitude!;
			const light = isLight();
			map = new maplibregl.Map({
				container: mapEl!,
				style: basemapStyleUrl(light),
				center: [lng, lat],
				zoom: 11,
				attributionControl: { compact: true },
				// Locked thumbnail: no scroll-zoom / drag / dbl-click — wheel events
				// pass through so the page scrolls past it instead of the map moving.
				interactive: false
			});
			marker = new maplibregl.Marker({ color: '#34e3c4' }).setLngLat([lng, lat]).addTo(map);
			const m = map;
			m.on('load', () => collapseAttribution(m));
			setTimeout(() => m.resize(), 80);
		});
		return () => {
			map?.remove();
			map = null;
			marker = null;
		};
	});
	// Keep the marker on the node's current location without recreating the map.
	$effect(() => {
		const lat = node?.latitude;
		const lng = node?.longitude;
		if (map && marker && lat != null && lng != null) marker.setLngLat([lng, lat]);
	});

	// --- QR contact code (scanned by the MeshCore app's in-app contact scanner) ---
	// URI per the MeshCore spec: meshcore://contact/add?name=&public_key=&type=
	// public_key is the canonical lowercase 64-hex key (the spec's example is
	// lowercase; some clients parse it strictly), name is URL-encoded, type is
	// 1=Companion 2=Repeater 3=RoomServer 4=Sensor. errorCorrectionLevel 'L'
	// keeps the module count (and density) lowest so the on-screen code stays
	// easy to scan even for nodes with long / emoji names.
	let qrSvg = $state('');
	$effect(() => {
		if (!node) return;
		const typeMap: Record<string, number> = { ChatNode: 1, Repeater: 2, RoomServer: 3, Sensor: 4 };
		const t = typeMap[node.role] ?? 2;
		const url = `meshcore://contact/add?name=${encodeURIComponent(node.name || 'Unknown')}&public_key=${node.publicKey.toLowerCase()}&type=${t}`;
		QRCode.toString(url, { type: 'svg', margin: 1, errorCorrectionLevel: 'L' })
			.then((s) => (qrSvg = s))
			.catch(() => (qrSvg = ''));
	});
</script>

{#if !loaded}
	<div class="panel text-fg-faint px-5 py-12 text-center text-sm">Loading…</div>
{:else if quarantined}
	<div class="panel rise border-coral/40 overflow-hidden">
		<div class="bg-coral/10 border-coral/30 flex items-center gap-2.5 border-b px-5 py-3.5">
			<svg viewBox="0 0 24 24" class="text-coral h-5 w-5 shrink-0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
				<path d="M12 9v4M12 17h.01M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
			</svg>
			<h2 class="font-display text-coral text-sm font-700 tracking-wide">QUARANTINED NODE</h2>
		</div>
		<div class="px-5 py-6">
			<p class="text-fg text-sm leading-relaxed">
				This node is currently <span class="text-coral font-600">quarantined</span> — it's suspected
				of acting as an <span class="font-600">RF bridge</span> or
				<span class="font-600">MQTT injector</span>, feeding traffic from another mesh into this network.
				Its data has been hidden from the maps, node list, and live feed pending review.
			</p>

			<div class="border-line/50 mt-5 grid gap-x-6 gap-y-2 border-t pt-4 text-sm sm:grid-cols-2">
				<div class="flex items-center justify-between gap-3">
					<span class="label normal-case">Identity</span>
					<span class="font-mono text-fg-dim text-xs">{block?.name || shortKey(pubkey, 8, 4)}</span>
				</div>
				<div class="flex items-center justify-between gap-3">
					<span class="label normal-case">Classified as</span>
					<span class="font-mono text-coral text-xs">{block?.kind === 'bridge' ? 'RF bridge' : 'injected node'}</span>
				</div>
				<div class="flex items-center justify-between gap-3">
					<span class="label normal-case">Reason</span>
					<span class="font-mono text-fg-dim text-xs">{block?.reason || '—'}</span>
				</div>
				<div class="flex items-center justify-between gap-3">
					<span class="label normal-case">Since</span>
					<span class="font-mono text-fg-dim text-xs">{block?.createdAt ? ago(block.createdAt) + ' ago' : '—'}</span>
				</div>
				<div class="flex items-center justify-between gap-3 sm:col-span-2">
					<span class="label normal-case shrink-0">Public key</span>
					<span class="font-mono text-fg-faint min-w-0 truncate text-[0.7rem]">{pubkey}</span>
				</div>
			</div>

			<p class="text-fg-faint mt-5 text-xs leading-relaxed">
				If this is a legitimate node on your mesh, an administrator can release it from the
				<a href="/admin" class="text-signal hover:underline">Admin panel</a> — releasing restores it
				everywhere; the detection list also offers <span class="text-fg-dim">Dismiss</span> to mark a
				node as known-good without quarantining it.
			</p>
		</div>
	</div>
{:else if !node}
	<div class="panel text-fg-faint px-5 py-12 text-center text-sm">
		Node not found. It may not have advertised yet.
	</div>
{:else}
	{#if heading}
		<div class="mb-5 flex flex-wrap items-center gap-3">
			<FavoriteStar {pubkey} />
			<h2 class="font-display text-fg text-2xl font-700 tracking-tight">
				{node.name || shortKey(pubkey, 8, 4)}
			</h2>
			<RoleBadge role={node.role} />
			<span class="label rounded-full border px-2 py-0.5" style="color:{status.color};border-color:{status.color}55">{status.label}</span>
		</div>
	{/if}

	<!-- Public key strip -->
	<div class="mb-5 flex items-center gap-2">
		<button onclick={copyKey} class="panel panel-hover flex flex-1 items-center gap-3 px-5 py-3 text-left">
			<span class="label shrink-0">PUBKEY</span>
			<span class="font-mono text-fg whitespace-nowrap text-[0.72rem] tracking-tight">{pubkey}</span>
			<span class="label ml-auto shrink-0 {copied ? '!text-signal' : ''}">{copied ? 'COPIED' : 'COPY'}</span>
		</button>
	</div>

	<!-- Node Admin: claim, private location, notes — each in its own modal -->
	<div class="mb-5">
		<NodeAdmin {pubkey} seedLat={node?.latitude} seedLon={node?.longitude} />
	</div>

	<div class="grid gap-5 {compact ? '' : 'lg:grid-cols-3'}">
		<!-- LEFT: hash, map, QR, overview, scores -->
		<div class="space-y-4 lg:col-span-1">
			<!-- Hash ID -->
			<div class="panel px-5 py-4">
				<div class="label mb-3 flex items-center justify-between">
					Hash ID
					{#if hashId}<span class="font-mono text-fg-faint normal-case">{hashId.bytes}-byte</span>{/if}
				</div>
				{#if hashId}
					<div class="flex items-baseline gap-3">
						<span class="font-mono text-signal glow-signal text-2xl font-700 tracking-[0.15em]">{hashId.hex}</span>
						{#if hashId.artifactOf}
							<span class="font-mono text-coral bg-coral/10 rounded-[var(--radius)] px-1.5 py-0.5 text-[0.62rem]">corrupted copy</span>
						{:else if hashId.shared === 0}
							<span class="font-mono text-signal bg-signal/10 rounded-[var(--radius)] px-1.5 py-0.5 text-[0.62rem]">unique</span>
						{:else}
							<Tooltip
								text="{hashId.shared} other {hashId.bytes}-byte node{hashId.shared > 1 ? 's' : ''} share this hash ID. Open the Identity page to compare them."
							>
								<a
									href="/identity?len={hashId.bytes}&id={hashId.hex}"
									class="font-mono text-amber bg-amber/10 hover:bg-amber/20 rounded-[var(--radius)] px-1.5 py-0.5 text-[0.62rem] tnum underline-offset-2 transition-colors hover:underline"
									>+{hashId.shared} collision{hashId.shared > 1 ? 's' : ''}</a
								>
							</Tooltip>
						{/if}
					</div>
					{#if hashId.artifactOf}
						<div class="text-coral/90 mt-2 text-xs leading-relaxed">
							⚠ Likely a packet-corruption artifact of
							<span class="text-signal">{hashId.artifactOf.name || 'a real node'}</span> — {hashId.reason}.
							This isn't a real node; consider scrubbing it.
						</div>
					{/if}
				{:else}
					<div class="text-fg-faint text-sm">Unknown — not seen advertising yet.</div>
				{/if}
			</div>

			{#if hasLoc}
				{#if webglOk}
					<div bind:this={mapEl} class="border-line h-44 w-full overflow-hidden rounded-[var(--radius)] border"></div>
				{:else}
					<div class="border-line h-44 w-full overflow-hidden rounded-[var(--radius)] border">
						<LeafletInset lat={node!.latitude!} lon={node!.longitude!} />
					</div>
				{/if}
			{/if}

			{#if qrSvg && !compact}
				<div class="panel flex flex-col items-center gap-2 px-5 py-4">
					<div class="label self-start">MeshCore Contact</div>
					<div class="qr w-44 rounded-[var(--radius)] bg-white p-2.5">{@html qrSvg}</div>
					<div class="text-fg-faint text-center text-[0.62rem]">Scan from the MeshCore app: Contacts → + → Scan QR</div>
				</div>
			{/if}

			<!-- Overview -->
			<div class="panel divide-line/40 divide-y">
				{#each facts as f (f.k)}
					<div class="flex items-center justify-between gap-3 px-5 py-2.5">
						<span class="label normal-case shrink-0">{f.k}</span>
						<span
							class="label normal-case tnum text-right"
							style="color:{f.c ?? 'var(--color-fg)'}{f.k === 'Radio' ? ';letter-spacing:0' : ''}"
							>{f.v}</span
						>
					</div>
				{/each}
			</div>

			<!-- Advert activity sparkline (per-hour over the window) -->
			{#if detail?.activity?.length && !compact}
				<div class="panel px-5 py-4">
					<div class="label mb-3 flex items-center justify-between">
						<span>Advert Activity</span>
						<span class="font-mono text-fg-faint normal-case">last {detail.windowHours}h</span>
					</div>
					<div class="flex h-12 items-end gap-1">
						{#each detail.activity as count, i (i)}
							<Tooltip
								text="{count} advert{count === 1 ? '' : 's'} · {detail.windowHours - 1 - i === 0 ? 'this hour' : `${detail.windowHours - 1 - i}h ago`}"
								class="h-full min-w-0 flex-1 items-end"
							>
								<div
									class="bg-signal/80 hover:bg-signal w-full rounded-sm transition-all"
									style="height:{count === 0 ? 2 : Math.max(8, (count / activityMax) * 100)}%;{count === 0 ? 'opacity:0.25' : ''}"
								></div>
							</Tooltip>
						{/each}
					</div>
					<div class="text-fg-faint mt-1.5 flex justify-between text-[0.58rem]">
						<span>-{detail.windowHours}h</span><span>now</span>
					</div>
				</div>
			{/if}

			<!-- Relay role + scores (repeaters / room servers) -->
			{#if isRelay && detail && !compact}
				<div class="panel px-5 py-4">
					<div class="label mb-3">Relay Activity</div>
					<div class="divide-line/40 divide-y text-sm">
						<div class="flex items-center justify-between py-2">
							<span class="label normal-case">Last relayed</span>
							<span class="font-mono text-xs tnum" style="color:{detail.relay.active ? 'var(--color-signal)' : 'var(--color-fg-dim)'}">
								{detail.relay.lastRelayed ? ago(detail.relay.lastRelayed) + ' ago' : 'never'}
							</span>
						</div>
						<div class="flex items-center justify-between py-2">
							<span class="label normal-case">Relays 1h / 24h</span>
							<span class="font-mono text-fg text-xs tnum">{detail.relay.count1h} / {detail.relay.count24h}</span>
						</div>
							<div class="py-2">
							<div class="mb-1 flex items-center justify-between">
								<Tooltip text="Fraction of relayed traffic that transited this node (24h window)"><span class="label normal-case">Traffic share</span></Tooltip>
								<span class="font-mono text-xs" style="color:{tl.color}">{(detail.trafficShare * 100).toFixed(1)}% · {tl.label}</span>
							</div>
							<div class="bg-panel-2 h-1.5 w-full overflow-hidden rounded-full">
								<div class="h-full rounded-full" style="width:{Math.max(2, detail.trafficShare * 100)}%;background:{tl.color}"></div>
							</div>
						</div>
							<div class="py-2">
							<div class="mb-1 flex items-center justify-between">
								<Tooltip text="Betweenness centrality — how often this node sits on shortest paths (1.0 = most structurally critical)"><span class="label normal-case">Bridge score</span></Tooltip>
								<span class="font-mono text-xs" style="color:{bl.color}">{(detail.bridge * 100).toFixed(1)}% · {bl.label}</span>
							</div>
							<div class="bg-panel-2 h-1.5 w-full overflow-hidden rounded-full">
								<div class="h-full rounded-full" style="width:{Math.max(2, detail.bridge * 100)}%;background:{bl.color}"></div>
							</div>
						</div>
					</div>
				</div>
			{/if}

			{#if compact}
				<a href="/nodes/{pubkey}" class="panel panel-hover block px-5 py-3 text-center">
					<span class="label">Heard by {detail?.observers.length ?? 0} · {detail?.neighbors.length ?? 0} neighbour{(detail?.neighbors.length ?? 0) === 1 ? '' : 's'}</span>
					<span class="text-signal mt-1 block text-sm">Open full detail ↗</span>
				</a>
			{/if}
		</div>

		{#if !compact}
		<!-- RIGHT: observers, recent packets, neighbors -->
		<div class="space-y-4 lg:col-span-2">
			<!-- Activity heatmap (weekday × hour) -->
			<section class="panel">
				<div class="border-line/70 flex items-center gap-2.5 border-b px-5 py-3">
					<h3 class="font-display text-fg text-sm font-700 tracking-wide">ACTIVITY HEATMAP</h3>
					<span class="label ml-auto normal-case">last 7d · UTC{heatmap ? ` · ${heatmap.total} pkt` : ''}</span>
				</div>
				{#if !heatmap || heatmap.total === 0}
					<div class="text-fg-faint px-5 py-8 text-center text-sm">No activity in the last 7 days.</div>
				{:else}
					<div class="px-5 py-4">
						<div class="flex flex-col gap-[3px]">
							{#each heatmap.grid as row, d (d)}
								<div class="flex items-center gap-2">
									<span class="font-mono text-fg-faint w-7 shrink-0 text-[0.58rem]">{DAY[d]}</span>
									<div class="grid flex-1 gap-[2px]" style="grid-template-columns:repeat(24,1fr)">
										{#each row as count, h (h)}
											<Tooltip text="{DAY[d]} {h.toString().padStart(2, '0')}:00 UTC · {count} packet{count === 1 ? '' : 's'}" class="block w-full">
												<div class="h-[13px] w-full rounded-[1px]" style={cellStyle(count, heatmap.max)}></div>
											</Tooltip>
										{/each}
									</div>
								</div>
							{/each}
							<div class="text-fg-faint mt-0.5 flex gap-2 pl-9 font-mono text-[0.55rem]">
								<div class="grid flex-1" style="grid-template-columns:repeat(24,1fr)">
									{#each Array(24) as _, h (h)}
										<span class="text-center">{h % 6 === 0 ? h : ''}</span>
									{/each}
								</div>
							</div>
						</div>
					</div>
				{/if}
			</section>

			<!-- Heard By (observers that received this node's adverts, over a
			     selectable range — adverts are ~30h apart, so 6h often shows none) -->
			<section class="panel">
				<div class="border-line/70 flex items-center gap-2.5 border-b px-5 py-3">
					<h3 class="font-display text-fg text-sm font-700 tracking-wide">HEARD BY</h3>
					<span class="label tnum">{observers.length} observer{observers.length === 1 ? '' : 's'}</span>
					<div class="ml-auto flex items-center gap-1">
						{#each obsRanges as r (r.sec)}
							<button
								onclick={() => setObsRange(r.sec)}
								class="label rounded-[var(--radius)] border px-2 py-0.5 transition-colors {obsRange === r.sec ? 'border-signal text-signal' : 'border-line text-fg-faint hover:text-fg'}"
								>{r.label}</button
							>
						{/each}
					</div>
				</div>
				{#if obsLoading && observers.length === 0}
					<div class="text-fg-faint px-5 py-8 text-center text-sm">Loading…</div>
				{:else if observers.length === 0}
					<div class="text-fg-faint px-5 py-8 text-center text-sm">No adverts heard in this range.</div>
				{:else}
					<div class="divide-line/40 divide-y">
						{#each observers as o (o.id)}
							<div class="flex items-center gap-3 px-5 py-2 text-sm">
								<span class="text-fg min-w-0 flex-1 truncate font-mono text-xs">{o.id}</span>
								{#if o.region}<span class="label !text-[0.58rem]">{o.region}</span>{/if}
								<span class="font-mono text-fg-faint w-14 text-right text-xs tnum">{o.count} advert{o.count === 1 ? '' : 's'}</span>
								<span class="font-mono w-14 text-right text-xs tnum" style="color:{snrColor(o.avgSnr)}">{o.avgSnr != null ? o.avgSnr.toFixed(1) + ' dB' : '—'}</span>
								<span class="font-mono text-fg-faint w-16 text-right text-xs tnum">{o.avgRssi != null ? o.avgRssi.toFixed(0) + ' dBm' : '—'}</span>
							</div>
						{/each}
					</div>
				{/if}
			</section>

			<!-- Activity history (stored observations, on demand) -->
			<section class="panel">
				<div class="border-line/70 flex items-center gap-2.5 border-b px-5 py-3">
					<h3 class="font-display text-fg text-sm font-700 tracking-wide">ACTIVITY HISTORY</h3>
					<div class="ml-auto flex items-center gap-1">
						{#each ranges as r (r.sec)}
							<button
								onclick={() => setRange(r.sec)}
								class="label rounded-[var(--radius)] border px-2 py-0.5 transition-colors {histRange === r.sec ? 'border-signal text-signal' : 'border-line text-fg-faint hover:text-fg'}"
								>{r.label}</button
							>
						{/each}
					</div>
				</div>
				{#if histLoading && history.length === 0}
					<div class="text-fg-faint px-5 py-8 text-center text-sm">Loading…</div>
				{:else if history.length === 0}
					<div class="text-fg-faint px-5 py-8 text-center text-sm">No observations in this range.</div>
				{:else}
					<div class="divide-line/40 max-h-96 divide-y overflow-y-auto">
						{#each history as h (h.messageHash + h.receivedAt + h.kind + h.hopIndex)}
							<div class="flex items-center gap-3 px-5 py-2 text-sm">
								<Tooltip text={fmtAbs(h.receivedAt)} class="w-10 shrink-0"><span class="font-mono text-fg-faint text-xs tnum">{ago(h.receivedAt)}</span></Tooltip>
								<span class="label shrink-0 rounded px-1.5 py-0.5 text-[0.56rem] {h.kind === 'advert' ? 'text-signal bg-signal/10' : 'text-sky bg-sky/10'}">{h.kind === 'advert' ? 'SENT' : 'RELAY'}</span>
								<PayloadTag type={h.payloadType} />
								<span class="font-mono text-fg-dim min-w-0 flex-1 truncate text-xs">via {h.observerName ?? h.observerId ?? '—'}</span>
								{#if h.kind === 'relay'}<Tooltip text="this node's position in the packet's path" class="shrink-0"><span class="font-mono text-fg-dim text-xs tnum">hop {h.hopIndex + 1}/{h.pathHops}</span></Tooltip>{/if}
								<span class="font-mono w-14 shrink-0 text-right text-xs tnum" style="color:{snrColor(h.snr)}">{fmtSnr(h.snr)} dB</span>
							</div>
						{/each}
					</div>
					<div class="text-fg-faint border-line/50 border-t px-5 py-2 text-center text-[0.62rem]">
						{history.length} observation{history.length === 1 ? '' : 's'} · adverts sent + packets relayed{history.length >= 300 ? ' (capped)' : ''}
					</div>
				{/if}
			</section>

			<!-- Neighbors -->
			<section class="panel">
				<div class="border-line/70 flex items-center gap-2.5 border-b px-5 py-3">
					<h3 class="font-display text-fg text-sm font-700 tracking-wide">NEIGHBORS</h3>
					<span class="label ml-auto tnum">{detail?.neighbors.length ?? 0}</span>
				</div>
				{#if !detail || detail.neighbors.length === 0}
					<div class="text-fg-faint px-5 py-8 text-center text-sm">No path-adjacent nodes observed in the window.</div>
				{:else}
					<div class="divide-line/40 divide-y">
						{#each detail.neighbors as nb (nb.publicKey)}
							<a href="/nodes/{nb.publicKey}" class="panel-hover flex items-center gap-3 px-5 py-2 text-sm">
								<span class="h-2 w-2 shrink-0 rounded-full" style="background:{roleColor(nb.role)}"></span>
								<span class="text-fg min-w-0 flex-1 truncate">{nb.name}</span>
								<span class="label !text-[0.58rem]">{roleLabel(nb.role)}</span>
								<span class="font-mono text-fg-faint text-xs tnum">×{nb.count}</span>
							</a>
						{/each}
					</div>
				{/if}
			</section>
		</div>
		{/if}
	</div>
{/if}

<style>
	.qr :global(svg) {
		display: block;
		width: 100%;
		height: auto;
	}
</style>
