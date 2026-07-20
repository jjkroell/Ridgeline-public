<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { auth } from '$lib/auth.svelte';
	import { confirmer } from '$lib/confirm.svelte';
	import { api, admin, type Observer, type ObserverAnalytics, type ObserverStatus, type ObserverTelemetry } from '$lib/api';
	import { ago, fmtNum, skewColor, fmtSkew, snrColor, roleColor, roleLabel, isFresh } from '$lib/format';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';
	import BarRow from '$lib/components/BarRow.svelte';
	import KpiStrip from '$lib/components/KpiStrip.svelte';
	import WindowToggle from '$lib/components/WindowToggle.svelte';
	import Sparkline from '$lib/components/Sparkline.svelte';

	const id = $derived(page.params.id ?? '');

	const windows = [
		{ label: '6h', sec: 21600 },
		{ label: '24h', sec: 86400 },
		{ label: '3d', sec: 259200 }
	];
	let windowSec = $state(86400);

	let observer = $state<Observer | null>(null);
	let data = $state<ObserverAnalytics | null>(null);
	let telemetry = $state<ObserverTelemetry | null>(null);
	let loading = $state(true);

	async function refresh() {
		try {
			const [obs, a, tel] = await Promise.all([
				api.observers(),
				api.observerAnalytics(id, windowSec).catch(() => null),
				api.observerTelemetry(id, windowSec).catch(() => null)
			]);
			observer = obs.find((o) => o.id === id) ?? null;
			data = a;
			telemetry = tel;
		} finally {
			loading = false;
		}
	}
	// Reload on window change + poll.
	$effect(() => {
		windowSec;
		refresh();
	});
	onMount(() => {
		const t = setInterval(refresh, 15000);
		return () => clearInterval(t);
	});

	// Admin-only: retire a decommissioned observer. Withdraws it from the
	// observers page and keeps every packet it reported, so its history stays
	// attributable. Reversible, and it survives the broker replaying the
	// observer's retained /status — which is what used to resurrect observers
	// that had merely been deleted.
	let retiring = $state(false);
	async function retireObserver() {
		if (
			!(await confirmer.ask({
				title: `Retire observer "${id}"?`,
				message:
					'Removes it from the observers page. Every packet it reported is kept, and stays attributed to it in history. You can un-retire it later.',
				confirmLabel: 'Retire observer'
			}))
		)
			return;
		retiring = true;
		try {
			await admin.retireObserver(auth.csrf, id);
			goto('/observers');
		} catch (e) {
			retiring = false;
			await confirmer.tell({ title: 'Retire failed', message: (e as Error).message });
		}
	}

	// Admin-only: permanently delete the observer AND every packet it reported.
	let deleting = $state(false);
	async function deleteObserver() {
		if (
			!(await confirmer.ask({
				title: `Delete observer "${id}"?`,
				message: 'This permanently removes the observer and all of its stored packets. This cannot be undone.',
				confirmLabel: 'Delete observer',
				danger: true
			}))
		)
			return;
		deleting = true;
		try {
			await admin.deleteObservers(auth.csrf, [id]);
			goto('/observers');
		} catch (e) {
			deleting = false;
			await confirmer.tell({ title: 'Delete failed', message: (e as Error).message });
		}
	}

	function fmtDuration(secs?: number): string {
		if (secs == null) return '—';
		const d = Math.floor(secs / 86400);
		const h = Math.floor((secs % 86400) / 3600);
		const m = Math.floor((secs % 3600) / 60);
		if (d) return `${d}d ${h}h`;
		if (h) return `${h}h ${m}m`;
		return `${m}m`;
	}
	// Noise floor: lower (more negative) is quieter/better → greener.
	function noiseColor(dbm?: number): string {
		if (dbm == null) return 'var(--color-fg)';
		if (dbm <= -105) return 'var(--color-lime)';
		if (dbm <= -98) return 'var(--color-signal)';
		if (dbm <= -92) return 'var(--color-amber)';
		return 'var(--color-coral)';
	}
	function fmtRadio(s?: ObserverStatus): string {
		if (!s) return '—';
		const parts: string[] = [];
		if (s.freqMhz != null) parts.push(`${(+s.freqMhz.toFixed(3)).toString()} MHz`);
		if (s.bandwidthKhz != null) parts.push(`${s.bandwidthKhz}k`);
		if (s.spreadingFactor != null) parts.push(`SF${s.spreadingFactor}`);
		if (s.codingRate != null) parts.push(`CR${s.codingRate}`);
		return parts.join(' · ') || s.radio || '—';
	}

	const st = $derived(observer?.status);
	// Short numeric metrics → grid; long device strings → full-width rows.
	const metricFacts = $derived(
		st
			? [
					{ k: 'State', v: st.state ? st.state[0].toUpperCase() + st.state.slice(1) : '—', c: st.state === 'online' ? 'var(--color-signal)' : 'var(--color-fg-faint)' },
					{ k: 'Battery', v: st.batteryMv && st.batteryMv > 0 ? (st.batteryMv / 1000).toFixed(2) + ' V' : '— (mains)' },
					{ k: 'Uptime', v: fmtDuration(st.uptimeSecs) },
					{ k: 'Noise floor', v: st.noiseFloor != null ? st.noiseFloor.toFixed(0) + ' dBm' : '—', c: st.noiseFloor != null ? noiseColor(st.noiseFloor) : undefined },
					{ k: 'TX airtime', v: st.txAirSecs != null ? fmtDuration(st.txAirSecs) : '—' },
					{ k: 'RX airtime', v: st.rxAirSecs != null ? fmtDuration(st.rxAirSecs) : '—' },
					{ k: 'Recv errors', v: st.recvErrors != null ? fmtNum(st.recvErrors) : '—' }
				]
			: []
	);
	const deviceFacts = $derived(
		st
			? [
					{ k: 'Model', v: st.model || '—' },
					{ k: 'Firmware', v: st.firmware || '—' },
					{ k: 'Client', v: st.clientVersion || '—' }
				]
			: []
	);

	const kpis = $derived([
		{ label: 'Packets', value: data ? fmtNum(data.totalPackets) : '—', accent: true },
		{ label: 'Packets / hr', value: data ? data.packetsPerHour.toFixed(1) : '—', accent: true },
		{ label: 'Distinct nodes', value: data ? fmtNum(data.distinctNodes) : '—', hint: 'distinct nodes whose adverts reached this observer' },
		{ label: 'Direct (0-hop)', value: data ? fmtNum(data.directNodes) : '—', color: 'var(--color-lime)', hint: 'nodes heard directly — RF neighbours' },
		{ label: 'Avg SNR', value: data?.avgSnr != null ? data.avgSnr.toFixed(1) + ' dB' : '—', color: snrColor(data?.avgSnr) },
		{ label: 'Clock skew', value: fmtSkew(data?.clockSkewMs), color: skewColor(data?.clockSkewMs), hint: 'median receive-time deviation from consensus on shared packets — large = drifting clock' }
	]);

	// Device trends (from the telemetry time series).
	const sum = $derived(telemetry?.summary);
	const batterySeries = $derived((telemetry?.points ?? []).map((p) => (p.batteryMv && p.batteryMv > 0 ? p.batteryMv / 1000 : null)));
	const noiseSeries = $derived((telemetry?.points ?? []).map((p) => p.noiseFloor ?? null));
	const hasBattery = $derived(batterySeries.some((v) => v != null));
	const hasNoise = $derived(noiseSeries.some((v) => v != null));
	const hasTelemetry = $derived((telemetry?.summary.samples ?? 0) > 0 && (hasBattery || hasNoise));

	function batteryDirLabel(s?: typeof sum): { text: string; color: string } {
		if (!s?.batteryDir || s.batteryTrendMvHr == null) return { text: '—', color: 'var(--color-fg-faint)' };
		const mvhr = Math.abs(s.batteryTrendMvHr).toFixed(0);
		if (s.batteryDir === 'charging') return { text: `▲ charging +${mvhr} mV/h`, color: 'var(--color-lime)' };
		if (s.batteryDir === 'discharging') return { text: `▼ draining −${mvhr} mV/h`, color: 'var(--color-coral)' };
		return { text: 'stable', color: 'var(--color-signal)' };
	}
	// Noise trend: rising (positive) is bad (getting noisier).
	function noiseTrendLabel(s?: typeof sum): { text: string; color: string } {
		if (!s || s.noiseTrendDbHr == null) return { text: '—', color: 'var(--color-fg-faint)' };
		const v = s.noiseTrendDbHr;
		if (v > 0.1) return { text: `▲ rising +${v.toFixed(2)} dB/h`, color: 'var(--color-coral)' };
		if (v < -0.1) return { text: `▼ falling ${v.toFixed(2)} dB/h`, color: 'var(--color-lime)' };
		return { text: 'steady', color: 'var(--color-signal)' };
	}
	const batDir = $derived(batteryDirLabel(sum));
	const noiseDir = $derived(noiseTrendLabel(sum));

	const maxAct = $derived(Math.max(1, ...(data?.activity ?? []).map((c) => c)));
	const maxPayload = $derived(Math.max(1, ...(data?.payloadTypes ?? []).map((p) => p.count)));
	const maxSnr = $derived(Math.max(1, ...(data?.snrHist ?? []).map((b) => b.count)));
	const maxNbr = $derived(Math.max(1, ...(data?.neighbors ?? []).map((n) => n.count)));
</script>

<PageHeader eyebrow="Listening Post" title={id}>
	<div class="flex items-center gap-3">
		{#if observer}
			<Tooltip text={isFresh(observer.lastSeen) ? 'reporting' : 'silent'}>
				<span class="h-2 w-2 rounded-full" style="background:{isFresh(observer.lastSeen) ? 'var(--color-signal)' : 'var(--color-fg-faint)'}"></span>
			</Tooltip>
		{/if}
		<WindowToggle options={windows} bind:value={windowSec} />
		{#if auth.isAdmin}
			<Tooltip text="Withdraw this observer from the observers page but keep every packet it reported. Reversible.">
				<button
					onclick={retireObserver}
					disabled={retiring || deleting}
					class="border-fg-faint/40 text-fg-dim hover:bg-fg-faint/10 hover:text-fg rounded-[var(--radius)] border px-3 py-1 text-xs font-600 transition-colors disabled:opacity-50"
				>{retiring ? 'Retiring…' : 'Retire observer'}</button>
			</Tooltip>
			<Tooltip text="Permanently delete this observer AND every packet it reported. Prefer Retire for a receiver that has simply left the network.">
				<button
					onclick={deleteObserver}
					disabled={retiring || deleting}
					class="border-coral/40 text-coral hover:bg-coral/15 rounded-[var(--radius)] border px-3 py-1 text-xs font-600 transition-colors disabled:opacity-50"
				>{deleting ? 'Deleting…' : 'Delete observer'}</button>
			</Tooltip>
		{/if}
		<a href="/observers" class="label hover:text-signal transition-colors">← All</a>
	</div>
</PageHeader>

<div class="px-6 py-6 md:px-10">
	{#if loading && !data}
		<div class="panel text-fg-faint px-5 py-12 text-center text-sm">Loading…</div>
	{:else if !observer}
		<div class="panel text-fg-faint px-5 py-12 text-center text-sm">
			Observer not found. It may not have reported recently.
		</div>
	{:else}
		<!-- KPI strip -->
		<KpiStrip items={kpis} />

		<!-- Radio & device telemetry (from the observer's /status message) -->
		<section class="panel rise mt-6" style="animation-delay:100ms">
			<div class="border-line/70 flex items-center justify-between border-b px-5 py-3.5">
				<h2 class="font-display text-fg text-sm font-700 tracking-wide">RADIO & DEVICE</h2>
				<span class="font-mono text-fg-faint text-[0.68rem]">
					{observer.lastStatusAt ? `status ${ago(observer.lastStatusAt)} ago` : 'no status reported'}
				</span>
			</div>
			{#if !st}
				<div class="text-fg-faint px-5 py-8 text-center text-sm">
					This observer hasn't published a status message yet.
				</div>
			{:else}
				<div class="px-5 py-4">
					<div class="label mb-1">Radio config</div>
					<div class="font-mono text-signal glow-signal text-lg font-700 tracking-tight">{fmtRadio(st)}</div>
				</div>
				<div class="border-line/50 grid grid-cols-2 gap-x-6 border-t px-5 py-1 md:grid-cols-3">
					{#each metricFacts as f (f.k)}
						<div class="border-line/30 flex items-center justify-between gap-3 border-b py-2">
							<span class="label normal-case shrink-0">{f.k}</span>
							<span class="font-mono text-right text-xs tnum" style="color:{f.c ?? 'var(--color-fg)'}">{f.v}</span>
						</div>
					{/each}
				</div>
				<div class="border-line/50 divide-line/30 divide-y border-t">
					{#each deviceFacts as f (f.k)}
						<div class="flex items-center justify-between gap-4 px-5 py-2">
							<span class="label normal-case shrink-0">{f.k}</span>
							<span class="font-mono text-fg-dim min-w-0 truncate text-right text-xs">{f.v}</span>
						</div>
					{/each}
				</div>
			{/if}
		</section>

		<!-- Device trends (battery / noise floor over time, from the telemetry log) -->
		<section class="panel rise mt-6" style="animation-delay:110ms">
			<div class="border-line/70 flex items-center justify-between border-b px-5 py-3.5">
				<h2 class="font-display text-fg text-sm font-700 tracking-wide">DEVICE TRENDS</h2>
				<span class="font-mono text-fg-faint text-[0.68rem]">
					{#if sum && sum.samples > 0}
						{sum.samples} samples · {sum.spanHours.toFixed(1)}h{#if sum.reboots > 0} · {sum.reboots} reboot{sum.reboots === 1 ? '' : 's'}{/if}
					{:else}
						from /status over time
					{/if}
				</span>
			</div>
			{#if !hasTelemetry}
				<div class="text-fg-faint px-5 py-8 text-center text-sm">
					No telemetry history yet — samples accumulate from each /status report (one per ~5&nbsp;min). Check back shortly.
				</div>
			{:else}
				<div class="grid gap-px md:grid-cols-2">
					<!-- Battery -->
					<div class="px-5 py-4">
						<div class="mb-1 flex items-baseline justify-between">
							<span class="label normal-case">Battery</span>
							<span class="font-mono text-xs tnum" style="color:{batDir.color}">{batDir.text}</span>
						</div>
						{#if hasBattery}
							<div class="font-mono text-fg text-lg font-700 tracking-tight">
								{sum?.batteryMv ? (sum.batteryMv / 1000).toFixed(2) + ' V' : '—'}
							</div>
							<div class="mt-2"><Sparkline values={batterySeries} color="var(--color-signal)" /></div>
						{:else}
							<div class="text-fg-faint py-3 text-sm">Mains-powered (no battery reported).</div>
						{/if}
					</div>
					<!-- Noise floor -->
					<div class="border-line/40 px-5 py-4 md:border-l">
						<div class="mb-1 flex items-baseline justify-between">
							<span class="label normal-case">Noise floor</span>
							<span class="font-mono text-xs tnum" style="color:{noiseDir.color}">{noiseDir.text}</span>
						</div>
						{#if hasNoise}
							<div class="font-mono text-lg font-700 tracking-tight" style="color:{noiseColor(sum?.noiseFloor)}">
								{sum?.noiseFloor != null ? sum.noiseFloor.toFixed(0) + ' dBm' : '—'}
							</div>
							<div class="mt-2"><Sparkline values={noiseSeries} color="var(--color-amber)" /></div>
							<div class="text-fg-faint mt-2 flex justify-between font-mono text-[0.62rem] tnum">
								<span>min {sum?.noiseMin?.toFixed(0) ?? '—'}</span>
								<span>avg {sum?.noiseAvg?.toFixed(1) ?? '—'}</span>
								<span>max {sum?.noiseMax?.toFixed(0) ?? '—'}</span>
							</div>
						{:else}
							<div class="text-fg-faint py-3 text-sm">No noise-floor readings reported.</div>
						{/if}
					</div>
				</div>
			{/if}
		</section>

		<!-- Feed throughput timeline -->
		<section class="panel rise mt-6" style="animation-delay:120ms">
			<div class="border-line/70 flex items-center justify-between border-b px-5 py-3.5">
				<h2 class="font-display text-fg text-sm font-700 tracking-wide">FEED THROUGHPUT</h2>
				<span class="font-mono text-fg-faint text-[0.68rem]">packets / hour</span>
			</div>
			<div class="px-5 py-5">
				{#if !data || data.activity.length === 0 || data.totalPackets === 0}
					<div class="text-fg-faint py-8 text-center text-sm">No packets in window.</div>
				{:else}
					<div class="flex h-[140px] items-end gap-px">
						{#each data.activity as count, i (i)}
							<Tooltip text="{data.activity.length - 1 - i === 0 ? 'this hour' : `${data.activity.length - 1 - i}h ago`} · {count} pkt" class="flex-1 items-end">
								<div class="bg-signal/70 hover:bg-signal w-full rounded-t-[2px] transition-colors" style="height:{Math.max(2, (count / maxAct) * 140)}px"></div>
							</Tooltip>
						{/each}
					</div>
					<div class="text-fg-faint mt-2 flex justify-between font-mono text-[0.62rem]">
						<span>-{Math.round(data.windowHours)}h</span><span>peak {maxAct} pkt/h</span><span>now</span>
					</div>
				{/if}
			</div>
		</section>

		<!-- Payload mix + SNR distribution -->
		<div class="mt-6 grid gap-6 lg:grid-cols-2">
			<section class="panel rise" style="animation-delay:180ms">
				<div class="border-line/70 border-b px-5 py-3.5">
					<h2 class="font-display text-fg text-sm font-700 tracking-wide">PAYLOAD MIX</h2>
				</div>
				<div class="py-3">
					{#each data?.payloadTypes ?? [] as p (p.label)}
						<BarRow label={p.label} count={p.count} max={maxPayload} color="var(--color-signal)" />
					{/each}
				</div>
			</section>
			<section class="panel rise" style="animation-delay:220ms">
				<div class="border-line/70 flex items-center justify-between border-b px-5 py-3.5">
					<h2 class="font-display text-fg text-sm font-700 tracking-wide">SNR DISTRIBUTION</h2>
					<span class="font-mono text-fg-faint text-[0.68rem]">dB</span>
				</div>
				<div class="py-3">
					{#each data?.snrHist ?? [] as b (b.label)}
						<BarRow label={b.label} count={b.count} max={maxSnr} color="var(--color-amber)" />
					{/each}
				</div>
			</section>
		</div>

		<!-- Direct RF neighbours -->
		<section class="panel rise mt-6" style="animation-delay:260ms">
			<div class="border-line/70 flex items-center gap-2.5 border-b px-5 py-3">
				<h3 class="font-display text-fg text-sm font-700 tracking-wide">DIRECTLY HEARS</h3>
				<span class="label normal-case ml-2 text-fg-faint">zero-hop RF neighbours</span>
				<span class="label ml-auto tnum">{data?.neighbors?.length ?? 0} node{(data?.neighbors?.length ?? 0) === 1 ? '' : 's'}</span>
			</div>
			{#if !data?.neighbors?.length}
				<div class="text-fg-faint px-5 py-8 text-center text-sm">No zero-hop adverts heard in window.</div>
			{:else}
				<div class="divide-line/40 divide-y">
					{#each data.neighbors as l (l.nodeKey)}
						<a href="/nodes/{l.nodeKey}" class="panel-hover flex items-center gap-3 px-5 py-2 text-sm">
							<span class="h-2 w-2 shrink-0 rounded-full" style="background:{roleColor(l.role)}"></span>
							<span class="text-fg min-w-0 flex-1 truncate">{l.nodeName}</span>
							<span class="label !text-[0.58rem]">{roleLabel(l.role)}</span>
							<Tooltip text="times heard at zero hops in window" class="shrink-0">
								<span class="font-mono text-fg-faint text-xs tnum">×{l.count}</span>
							</Tooltip>
						</a>
					{/each}
				</div>
			{/if}
		</section>

		<!-- Seen facts -->
		<div class="panel rise divide-line/40 mt-6 divide-y" style="animation-delay:320ms">
			{#each [{ k: 'Status', v: isFresh(observer.lastSeen) ? 'Reporting' : 'Silent', c: isFresh(observer.lastSeen) ? 'var(--color-signal)' : 'var(--color-fg-faint)' }, { k: 'Region', v: observer.region || data?.region || '—' }, { k: 'First seen', v: ago(observer.firstSeen) + ' ago' }, { k: 'Last heard', v: ago(observer.lastSeen) + ' ago' }, { k: 'Packets (all-time)', v: fmtNum(observer.packetCount) }] as f (f.k)}
				<div class="flex items-center justify-between px-5 py-2.5">
					<span class="label normal-case">{f.k}</span>
					<span class="font-mono text-sm tnum" style="color:{f.c ?? 'var(--color-fg)'}">{f.v}</span>
				</div>
			{/each}
		</div>
	{/if}
</div>
