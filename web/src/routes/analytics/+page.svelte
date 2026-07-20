<script lang="ts">
	import { onMount } from 'svelte';
	import Seo from '$lib/components/Seo.svelte';
	import { api, type MeshAnalytics } from '$lib/api';
	import { fmtNum, roleColor, shortKey, skewColor, fmtSkew } from '$lib/format';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import RoleBadge from '$lib/components/RoleBadge.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';
	import BarRow from '$lib/components/BarRow.svelte';
	import KpiStrip from '$lib/components/KpiStrip.svelte';
	import WindowToggle from '$lib/components/WindowToggle.svelte';

	interface DirectLinkRef {
		key: string;
		name: string;
		role: string;
		obs: string[];
	}

	const windows = [
		{ label: '1h', sec: 3600, bucket: 2 },
		{ label: '6h', sec: 21600, bucket: 10 },
		{ label: '24h', sec: 86400, bucket: 30 }
	];

	let windowSec = $state(21600);
	let bucketMin = $derived(windows.find((w) => w.sec === windowSec)?.bucket ?? 10);
	let data = $state<MeshAnalytics | null>(null);
	let error = $state<string | null>(null);
	let loading = $state(true);

	async function refresh() {
		try {
			data = await api.meshAnalytics(windowSec, bucketMin);
			error = null;
		} catch (e) {
			error = (e as Error).message;
		} finally {
			loading = false;
		}
	}

	// Reload when the window changes, and poll while mounted.
	$effect(() => {
		windowSec; // track
		refresh();
	});
	onMount(() => {
		const t = setInterval(refresh, 30000);
		return () => clearInterval(t);
	});

	function pct(n?: number): string {
		return n == null ? '—' : `${n.toFixed(n < 1 ? 2 : 1)}%`;
	}
	function score(n?: number): string {
		return n == null ? '—' : n.toFixed(2);
	}

	const k = $derived(data?.kpis);
	const kpis = $derived([
		{ label: 'Active Nodes', value: k ? fmtNum(k.activeNodes) : '—', accent: true },
		{ label: 'Transmissions', value: k ? fmtNum(k.transmissions) : '—' },
		{ label: 'Observations', value: k ? fmtNum(k.observations) : '—' },
		{ label: 'Avg Link Score', value: score(k?.avgLinkScore), accent: true, hint: 'Mean per-reception decode probability (SNR vs SF threshold × length penalty).' },
		{ label: 'Flood Redundancy', value: k?.floodRedundancy ? `×${k.floodRedundancy.toFixed(1)}` : '—', hint: 'Average observers hearing each transmission.' },
		{ label: 'Channel Util', value: pct(k?.channelUtilPct), accent: true, hint: 'Estimated logical airtime ÷ window. Lower bound — counts each packet once, not physical flood retransmissions.' }
	]);

	const maxUtil = $derived(Math.max(0.0001, ...(data?.airtime ?? []).map((b) => b.utilPct)));
	const maxPayload = $derived(Math.max(1, ...(data?.payloadTypes ?? []).map((p) => p.count)));
	const maxRoute = $derived(Math.max(1, ...(data?.routeTypes ?? []).map((p) => p.count)));
	const maxLink = $derived(Math.max(1, ...(data?.linkScoreHist ?? []).map((b) => b.count)));
	const maxSnr = $derived(Math.max(1, ...(data?.snrHist ?? []).map((b) => b.count)));
	const maxRelay = $derived(Math.max(1, ...(data?.topRelays ?? []).map((r) => r.relayed)));

	const maxObs = $derived(Math.max(1, ...(data?.observers ?? []).map((o) => o.observations)));
	const maxReach = $derived(Math.max(1, ...(data?.directReach ?? []).map((b) => b.count)));
	const maxRelayAir = $derived(Math.max(1, ...(data?.topRelays ?? []).map((r) => r.airtimeMs)));
	const maxHash = $derived(Math.max(1, ...(data?.hashSizes ?? []).map((b) => b.count)));

	const tierColors: Record<string, string> = {
		Quiet: 'var(--color-lime)',
		Normal: 'var(--color-signal)',
		Busy: 'var(--color-amber)',
		Congested: 'var(--color-coral)'
	};
	function tierColor(t?: string): string {
		return (t && tierColors[t]) || 'var(--color-fg-dim)';
	}
	function fmtAir(ms: number): string {
		return ms >= 1000 ? (ms / 1000).toFixed(1) + ' s' : Math.round(ms) + ' ms';
	}

	// RF adjacency layout: observers on a ring, each directly-heard node placed at
	// the resultant direction of the observers that hear it — so nodes heard by
	// several observers (bridges) drift toward the centre, single-observer nodes
	// sit out near their observer. Deterministic; no physics sim.
	const CX = 500,
		CY = 215,
		R = 165;
	function jhash(s: string): number {
		let h = 2166136261;
		for (let i = 0; i < s.length; i++) h = (h ^ s.charCodeAt(i)) * 16777619;
		return (h >>> 0) / 4294967295;
	}
	const graph = $derived.by(() => {
		const links = data?.directLinks ?? [];
		if (!links.length) return null;
		// Observers actually present as direct-link endpoints.
		const obsIds = [...new Set(links.map((l) => l.observer))];
		// Observers are keyed by public key; show the friendly label.
		const obsLabel = new Map(links.map((l) => [l.observer, l.observerName ?? l.observer]));
		const obsPos = new Map<string, { x: number; y: number; a: number }>();
		obsIds.forEach((id, i) => {
			const a = (i / obsIds.length) * Math.PI * 2 - Math.PI / 2;
			obsPos.set(id, { x: CX + Math.cos(a) * R, y: CY + Math.sin(a) * R, a });
		});
		// Group links by node.
		const byNode = new Map<string, DirectLinkRef>();
		for (const l of links) {
			let n = byNode.get(l.nodeKey);
			if (!n) {
				n = { key: l.nodeKey, name: l.nodeName, role: l.role, obs: [] };
				byNode.set(l.nodeKey, n);
			}
			n.obs.push(l.observer);
		}
		const nodes = [...byNode.values()].map((n) => {
			let vx = 0,
				vy = 0;
			for (const o of n.obs) {
				const p = obsPos.get(o);
				if (p) {
					vx += Math.cos(p.a);
					vy += Math.sin(p.a);
				}
			}
			const mag = Math.min(1, Math.hypot(vx, vy) / n.obs.length); // 1=single dir, 0=spread
			const ang = Math.atan2(vy, vx) + (jhash(n.key) - 0.5) * 0.5;
			const rad = R * (0.18 + 0.46 * mag) + (jhash(n.key + 'r') - 0.5) * 22;
			return { ...n, x: CX + Math.cos(ang) * rad, y: CY + Math.sin(ang) * rad };
		});
		const nodePos = new Map(nodes.map((n) => [n.key, n]));
		const edges = links
			.map((l) => {
				const o = obsPos.get(l.observer);
				const n = nodePos.get(l.nodeKey);
				return o && n ? { x1: o.x, y1: o.y, x2: n.x, y2: n.y } : null;
			})
			.filter((e): e is { x1: number; y1: number; x2: number; y2: number } => !!e);
		const observers = obsIds.map((id) => ({ id, label: obsLabel.get(id) ?? id, ...obsPos.get(id)! }));
		return { observers, nodes, edges };
	});

	// Mesh topology (relay backbone) force-directed layout. Deterministic: nodes
	// seed from a hash of their key and the simulation is a pure function of the
	// data, so identical snapshots lay out identically (no jitter between polls).
	const TW = 1000,
		TH = 540;
	const topo = $derived.by(() => {
		const allNodes = data?.topology?.nodes ?? [];
		const edges = data?.topology?.edges ?? [];
		// Show only the connected backbone: nodes that actually hand off to/from
		// another relay. Nodes with no edge carry no topology and would just scatter
		// to the boundary under repulsion.
		const connected = new Set<string>();
		for (const e of edges) {
			connected.add(e.a);
			connected.add(e.b);
		}
		const nodes = allNodes.filter((n) => connected.has(n.publicKey));
		if (nodes.length < 2) return null;
		const pos = new Map<string, { x: number; y: number }>();
		for (const n of nodes) {
			const a = jhash(n.publicKey) * Math.PI * 2;
			const r = 0.25 + 0.55 * jhash(n.publicKey + 'r');
			pos.set(n.publicKey, { x: TW / 2 + Math.cos(a) * r * TW * 0.42, y: TH / 2 + Math.sin(a) * r * TH * 0.42 });
		}
		const present = edges.filter((e) => pos.has(e.a) && pos.has(e.b));
		const k = Math.sqrt((TW * TH) / nodes.length) * 0.55; // ideal edge length
		let temp = TW * 0.09;
		for (let it = 0; it < 220; it++) {
			const disp = new Map(nodes.map((n) => [n.publicKey, { x: 0, y: 0 }]));
			for (let i = 0; i < nodes.length; i++) {
				for (let j = i + 1; j < nodes.length; j++) {
					const pi = pos.get(nodes[i].publicKey)!,
						pj = pos.get(nodes[j].publicKey)!;
					const dx = pi.x - pj.x,
						dy = pi.y - pj.y;
					const d = Math.hypot(dx, dy) || 0.01;
					const f = (k * k) / d / d;
					const di = disp.get(nodes[i].publicKey)!,
						dj = disp.get(nodes[j].publicKey)!;
					di.x += dx * f;
					di.y += dy * f;
					dj.x -= dx * f;
					dj.y -= dy * f;
				}
			}
			for (const e of present) {
				const pa = pos.get(e.a)!,
					pb = pos.get(e.b)!;
				const dx = pa.x - pb.x,
					dy = pa.y - pb.y;
				const d = Math.hypot(dx, dy) || 0.01;
				// Fruchterman–Reingold attraction (d²/k), scaled up a little by edge
				// weight so repeatedly-used hand-offs sit closer together.
				const f = ((d * d) / k) * (1 + Math.min(1.5, e.weight / 20));
				const ux = dx / d,
					uy = dy / d;
				const da = disp.get(e.a)!,
					db = disp.get(e.b)!;
				da.x -= ux * f;
				da.y -= uy * f;
				db.x += ux * f;
				db.y += uy * f;
			}
			for (const n of nodes) {
				const dp = disp.get(n.publicKey)!,
					p = pos.get(n.publicKey)!;
				const d = Math.hypot(dp.x, dp.y) || 0.01;
				p.x += (dp.x / d) * Math.min(d, temp);
				p.y += (dp.y / d) * Math.min(d, temp);
				p.x = Math.max(24, Math.min(TW - 24, p.x));
				p.y = Math.max(24, Math.min(TH - 24, p.y));
			}
			temp *= 0.972;
		}
		const maxRelayed = Math.max(1, ...nodes.map((n) => n.relayed));
		const maxW = Math.max(1, ...present.map((e) => e.weight));
		const degree = new Map<string, number>();
		for (const e of present) {
			degree.set(e.a, (degree.get(e.a) ?? 0) + 1);
			degree.set(e.b, (degree.get(e.b) ?? 0) + 1);
		}
		return {
			maxW,
			nodes: nodes.map((n) => ({
				...n,
				...pos.get(n.publicKey)!,
				r: 3.5 + 7 * Math.sqrt(n.relayed / maxRelayed),
				degree: degree.get(n.publicKey) ?? 0
			})),
			edges: present.map((e) => ({
				...e,
				...{ p1: pos.get(e.a)!, p2: pos.get(e.b)! },
				op: 0.2 + 0.55 * Math.sqrt(e.weight / maxW)
			}))
		};
	});

	// Relay-health trend: link-score line over the channel-utilisation bars (0–1
	// score mapped onto the 150px chart height), drawn as an overlaid polyline.
	const linkTrend = $derived.by(() => {
		const a = data?.airtime ?? [];
		const pts = a
			.map((b, i) => ({ b, i }))
			.filter(({ b }) => b.avgLinkScore != null)
			.map(({ b, i }) => `${((i + 0.5) / a.length) * 100},${(1 - (b.avgLinkScore as number)) * 150}`);
		return pts.length >= 2 ? pts.join(' ') : '';
	});

	function fmtTime(iso: string): string {
		return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}
	const radioLabel = $derived(
		data ? `SF${data.radio.SpreadingFactor} · BW${(data.radio.BandwidthHz / 1000).toFixed(1)}k · CR4/${data.radio.CodingRate}` : ''
	);
</script>

<Seo
	title="MeshCore Mesh Analytics"
	description="Network health, link quality and activity analytics for the MeshCore LoRa mesh."
	path="/analytics"
/>

<PageHeader eyebrow="Network Observatory" title="Mesh Analytics">
	<div class="flex items-center gap-3">
		{#if data}
			<Tooltip text="Mesh load tier from estimated channel utilisation">
				<span
					class="label rounded-full border px-2.5 py-1"
					style="color:{tierColor(data.kpis.congestionTier)};border-color:{tierColor(data.kpis.congestionTier)}55"
					>{data.kpis.congestionTier}</span
				>
			</Tooltip>
		{/if}
		<WindowToggle options={windows} bind:value={windowSec} />
	</div>
</PageHeader>

<div class="px-6 py-6 md:px-10">
	{#if error}
		<div class="panel border-coral/40 text-coral mb-6 px-4 py-3 text-sm">
			Can't reach the daemon — {error}
		</div>
	{/if}

	{#if loading && !data}
		<div class="text-fg-faint px-5 py-16 text-center text-sm">Computing mesh analytics…</div>
	{:else if data}
		<!-- KPI strip -->
		<KpiStrip items={kpis} />

		<!-- Channel utilisation timeline -->
		<section class="panel rise mt-6" style="animation-delay:120ms">
			<div class="border-line/70 flex items-center justify-between border-b px-5 py-3.5">
				<h2 class="font-display text-fg text-sm font-700 tracking-wide">CHANNEL UTILISATION & RELAY HEALTH</h2>
				<span class="font-mono text-fg-faint text-[0.68rem]">{bucketMin}-min buckets · airtime + link score</span>
			</div>
			<div class="px-5 py-5">
				{#if data.airtime.length === 0}
					<div class="text-fg-faint py-8 text-center text-sm">No traffic in window.</div>
				{:else}
					<div class="relative h-[150px]">
						<div class="flex h-full items-end gap-px">
							{#each data.airtime as b (b.timestamp)}
								<Tooltip
									text="{fmtTime(b.timestamp)} · {pct(b.utilPct)} util · {b.transmissions} tx · {b.relayTx} relayed{b.avgLinkScore != null ? ` · link ${score(b.avgLinkScore)}` : ''}"
									class="flex-1 items-end"
								>
									<div
										class="bg-signal/70 hover:bg-signal w-full rounded-t-[2px] transition-colors"
										style="height:{Math.max(2, (b.utilPct / maxUtil) * 150)}px"
									></div>
								</Tooltip>
							{/each}
						</div>
						{#if linkTrend}
							<!-- Relay-health overlay: mean link score (decode probability) per slice. -->
							<svg class="pointer-events-none absolute inset-0 h-full w-full" viewBox="0 0 100 150" preserveAspectRatio="none">
								<polyline points={linkTrend} fill="none" stroke="var(--color-lime)" stroke-width="1.4" stroke-linejoin="round" vector-effect="non-scaling-stroke" opacity="0.9" />
							</svg>
						{/if}
					</div>
					<div class="text-fg-faint mt-2 flex justify-between font-mono text-[0.62rem]">
						<span>{fmtTime(data.airtime[0].timestamp)}</span>
						<span class="flex items-center gap-3">
							<span>peak {pct(maxUtil)}</span>
							<span class="flex items-center gap-1"><span class="inline-block h-0.5 w-3" style="background:var(--color-lime)"></span>link score</span>
						</span>
						<span>{fmtTime(data.airtime[data.airtime.length - 1].timestamp)}</span>
					</div>
				{/if}
			</div>
		</section>

		<!-- Traffic mix -->
		<div class="mt-6 grid gap-6 lg:grid-cols-2">
			<section class="panel rise" style="animation-delay:180ms">
				<div class="border-line/70 border-b px-5 py-3.5">
					<h2 class="font-display text-fg text-sm font-700 tracking-wide">PAYLOAD TYPES</h2>
				</div>
				<div class="py-3">
					{#each data.payloadTypes as p (p.label)}
						<BarRow label={p.label} count={p.count} max={maxPayload} color="var(--color-signal)" />
					{/each}
				</div>
			</section>
			<section class="panel rise" style="animation-delay:220ms">
				<div class="border-line/70 border-b px-5 py-3.5">
					<h2 class="font-display text-fg text-sm font-700 tracking-wide">ROUTE TYPES</h2>
				</div>
				<div class="py-3">
					{#each data.routeTypes as p (p.label)}
						<BarRow label={p.label} count={p.count} max={maxRoute} color="var(--color-sky)" />
					{/each}
				</div>
			</section>
		</div>

		<!-- RF / link health -->
		<div class="mt-6 grid gap-6 lg:grid-cols-2">
			<section class="panel rise" style="animation-delay:260ms">
				<div class="border-line/70 flex items-center justify-between border-b px-5 py-3.5">
					<h2 class="font-display text-fg text-sm font-700 tracking-wide">LINK-SCORE DISTRIBUTION</h2>
					<span class="font-mono text-fg-faint text-[0.68rem]">decode probability</span>
				</div>
				<div class="py-3">
					{#each data.linkScoreHist as b (b.label)}
						<BarRow label={b.label} count={b.count} max={maxLink} color="var(--color-lime)" />
					{/each}
				</div>
			</section>
			<section class="panel rise" style="animation-delay:300ms">
				<div class="border-line/70 flex items-center justify-between border-b px-5 py-3.5">
					<h2 class="font-display text-fg text-sm font-700 tracking-wide">SNR DISTRIBUTION</h2>
					<span class="font-mono text-fg-faint text-[0.68rem]">dB</span>
				</div>
				<div class="py-3">
					{#each data.snrHist as b (b.label)}
						<BarRow label={b.label} count={b.count} max={maxSnr} color="var(--color-amber)" />
					{/each}
				</div>
			</section>
		</div>

		<!-- Top relays -->
		<section class="panel rise mt-6" style="animation-delay:340ms">
			<div class="border-line/70 flex items-center justify-between border-b px-5 py-3.5">
				<h2 class="font-display text-fg text-sm font-700 tracking-wide">BUSIEST RELAYS</h2>
				<span class="font-mono text-fg-faint text-[0.68rem]">forwarded tx · est. airtime contributed</span>
			</div>
			<div class="divide-line/50 divide-y">
				{#each data.topRelays as r, i (r.publicKey)}
					<a href="/nodes/{r.publicKey}" class="panel-hover flex items-center gap-3 px-5 py-2.5">
						<span class="font-mono text-fg-faint w-5 shrink-0 text-right text-xs tnum">{i + 1}</span>
						<div class="min-w-0 flex-1">
							<div class="text-fg truncate text-sm font-medium">{r.name || shortKey(r.publicKey)}</div>
							<!-- count bar (role colour) over a fainter airtime bar -->
							<div class="mt-1.5 space-y-1">
								<div class="bg-line/40 h-1.5 overflow-hidden rounded-[var(--radius)]">
									<div class="h-full rounded-[var(--radius)]" style="width:{(r.relayed / maxRelay) * 100}%;background:{roleColor(r.role)}"></div>
								</div>
								<Tooltip text="estimated channel airtime this relay contributed" class="block">
									<div class="bg-line/40 h-1 overflow-hidden rounded-[var(--radius)]">
										<div class="bg-amber/70 h-full rounded-[var(--radius)]" style="width:{(r.airtimeMs / maxRelayAir) * 100}%"></div>
									</div>
								</Tooltip>
							</div>
						</div>
						<RoleBadge role={r.role} />
						<div class="w-16 shrink-0 text-right">
							<div class="font-mono tnum text-fg-dim text-sm">{fmtNum(r.relayed)} tx</div>
							<div class="font-mono tnum text-amber text-[0.68rem]">{fmtAir(r.airtimeMs)}</div>
						</div>
					</a>
				{/each}
			</div>
		</section>

		<!-- RF adjacency graph (zero-hop direct links) -->
		<section class="panel rise mt-6" style="animation-delay:380ms">
			<div class="border-line/70 flex items-center justify-between border-b px-5 py-3.5">
				<h2 class="font-display text-fg text-sm font-700 tracking-wide">RF ADJACENCY — DIRECT LINKS</h2>
				<span class="font-mono text-fg-faint text-[0.68rem]">zero-hop adverts · observer ↔ node</span>
			</div>
			<div class="px-3 py-3">
				{#if !graph}
					<div class="text-fg-faint py-10 text-center text-sm">No direct (zero-hop) adverts in window.</div>
				{:else}
					<svg viewBox="0 0 1000 430" class="h-[430px] w-full">
						{#each graph.edges as e, i (i)}
							<line x1={e.x1} y1={e.y1} x2={e.x2} y2={e.y2} stroke="var(--color-line-bright)" stroke-width="0.7" opacity="0.5" />
						{/each}
						{#each graph.nodes as n (n.key)}
							<a href="/nodes/{n.key}">
								<circle cx={n.x} cy={n.y} r="4.5" fill={roleColor(n.role)} opacity="0.9" />
								<title>{n.name} · heard directly by {n.obs.length} observer{n.obs.length > 1 ? 's' : ''}</title>
							</a>
						{/each}
						{#each graph.observers as o (o.id)}
							<g>
								<circle cx={o.x} cy={o.y} r="7" fill="var(--color-violet)" stroke="var(--color-ink)" stroke-width="1.5" />
								<text
									x={o.x}
									y={o.y + (o.y < CY ? -12 : 18)}
									text-anchor="middle"
									class="font-mono"
									fill="var(--color-fg-dim)"
									font-size="11">{o.label.length > 18 ? o.label.slice(0, 17) + '…' : o.label}</text
								>
								<title>{o.label}</title>
							</g>
						{/each}
					</svg>
					<div class="text-fg-faint flex items-center justify-center gap-5 pb-1 font-mono text-[0.62rem]">
						<span class="flex items-center gap-1.5"><span class="inline-block h-2.5 w-2.5 rounded-full" style="background:var(--color-violet)"></span>observer</span>
						<span class="flex items-center gap-1.5"><span class="inline-block h-2 w-2 rounded-full" style="background:var(--color-role-repeater)"></span>node (by role)</span>
						<span>bridge nodes drift centre · click a node to open it</span>
					</div>
				{/if}
			</div>
		</section>

		<!-- Mesh topology (relay backbone — node↔node hand-offs) -->
		<section class="panel rise mt-6" style="animation-delay:400ms">
			<div class="border-line/70 flex items-center justify-between border-b px-5 py-3.5">
				<h2 class="font-display text-fg text-sm font-700 tracking-wide">MESH TOPOLOGY — RELAY BACKBONE</h2>
				<span class="font-mono text-fg-faint text-[0.68rem]">node ↔ node hand-offs · size = tx forwarded</span>
			</div>
			<div class="px-3 py-3">
				{#if !topo}
					<div class="text-fg-faint py-10 text-center text-sm">Not enough multi-hop relaying in window to map the backbone.</div>
				{:else}
					<svg viewBox="0 0 {TW} {TH}" class="h-[540px] w-full">
						{#each topo.edges as e (e.a + e.b)}
							<line x1={e.p1.x} y1={e.p1.y} x2={e.p2.x} y2={e.p2.y} stroke="var(--color-line-bright)" stroke-width={0.5 + 1.6 * (e.weight / topo.maxW)} opacity={e.op} />
						{/each}
						{#each topo.nodes as n (n.publicKey)}
							<a href="/nodes/{n.publicKey}">
								<circle cx={n.x} cy={n.y} r={n.r} fill={roleColor(n.role)} opacity="0.9" stroke="var(--color-ink)" stroke-width="1" />
								<title>{n.name} · forwarded {fmtNum(n.relayed)} tx · {n.degree} neighbour{n.degree === 1 ? '' : 's'}</title>
							</a>
						{/each}
					</svg>
					<div class="text-fg-faint flex flex-wrap items-center justify-center gap-x-5 gap-y-1 pb-1 font-mono text-[0.62rem]">
						<span>{topo.nodes.length} relays · {topo.edges.length} links</span>
						<span>line weight = times two relays handed off · click a node to open it</span>
					</div>
				{/if}
			</div>
		</section>

		<!-- Observer coverage (full width — includes clock skew) -->
		<section class="panel rise mt-6" style="animation-delay:420ms">
			<div class="border-line/70 flex items-center justify-between border-b px-5 py-3.5">
				<h2 class="font-display text-fg text-sm font-700 tracking-wide">OBSERVER COVERAGE</h2>
				<span class="font-mono text-fg-faint text-[0.68rem]">distinct · direct · clock skew · receptions</span>
			</div>
			<div class="divide-line/50 divide-y">
				{#each data.observers as o (o.id)}
					<div class="px-5 py-2.5">
						<div class="flex items-center gap-3">
							<div class="min-w-0 flex-1">
								<div class="text-fg truncate text-sm font-medium">{o.name ?? o.id}</div>
								{#if o.region}<div class="font-mono text-fg-faint mt-0.5 text-[0.62rem]">{o.region}</div>{/if}
							</div>
							<Tooltip text="distinct nodes whose adverts reached it">
								<span class="font-mono tnum text-fg-dim w-16 text-right text-xs">{fmtNum(o.distinctNodes)} <span class="text-fg-faint">nodes</span></span>
							</Tooltip>
							<Tooltip text="nodes heard directly (zero-hop RF neighbours)">
								<span class="font-mono tnum text-lime w-16 text-right text-xs">{fmtNum(o.directNodes)} <span class="text-fg-faint">direct</span></span>
							</Tooltip>
							<Tooltip text="median receive-time deviation from consensus on shared packets — large = drifting clock">
								<span class="font-mono tnum w-16 text-right text-xs" style="color:{skewColor(o.clockSkewMs)}">{fmtSkew(o.clockSkewMs)}</span>
							</Tooltip>
							<span class="font-mono tnum text-fg-dim w-12 shrink-0 text-right text-sm">{fmtNum(o.observations)}</span>
						</div>
						<div class="bg-line/40 mt-1.5 h-1.5 overflow-hidden rounded-[var(--radius)]">
							<div class="bg-violet h-full rounded-[var(--radius)]" style="width:{(o.observations / maxObs) * 100}%"></div>
						</div>
					</div>
				{/each}
			</div>
		</section>

		<!-- Identity (hash size) + direct-reach redundancy -->
		<div class="mt-6 grid gap-6 lg:grid-cols-2">
			<section class="panel rise" style="animation-delay:460ms">
				<div class="border-line/70 flex items-center justify-between border-b px-5 py-3.5">
					<h2 class="font-display text-fg text-sm font-700 tracking-wide">IDENTITY — HASH SIZE</h2>
					<span class="font-mono text-fg-faint text-[0.68rem]">nodes by path-hash length</span>
				</div>
				<div class="py-3">
					{#each data.hashSizes as b (b.label)}
						<BarRow label={b.label} count={b.count} max={maxHash} color={b.label === 'unknown' ? 'var(--color-fg-faint)' : 'var(--color-sky)'} />
					{/each}
				</div>
				<p class="text-fg-faint border-line/50 mt-1 border-t px-5 py-3 text-[0.7rem] leading-relaxed">
					Each node advertises a 1-, 2- or 3-byte hash by which it's identified in packet paths. Shorter
					hashes collide more; "unknown" nodes haven't advertised yet.
				</p>
			</section>
			<section class="panel rise" style="animation-delay:500ms">
				<div class="border-line/70 flex items-center justify-between border-b px-5 py-3.5">
					<h2 class="font-display text-fg text-sm font-700 tracking-wide">DIRECT-REACH REDUNDANCY</h2>
					<span class="font-mono text-fg-faint text-[0.68rem]">observers per node</span>
				</div>
				<div class="py-3">
					{#each data.directReach as b (b.label)}
						<BarRow label={b.label} count={b.count} max={maxReach} color="var(--color-violet)" />
					{/each}
				</div>
				<p class="text-fg-faint border-line/50 mt-1 border-t px-5 py-3 text-[0.7rem] leading-relaxed">
					How many observers hear each node directly. Nodes on only one observer are single points of RF
					failure; higher counts mean redundant coverage.
				</p>
			</section>
		</div>

		<div class="text-fg-faint mt-6 text-center font-mono text-[0.62rem]">
			radio {radioLabel} · window {data.windowHours.toFixed(1)}h · generated {fmtTime(data.generatedAt)}
		</div>
	{/if}
</div>
