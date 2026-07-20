<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api, type Observer, type ObserverAnalytics, type ObserverTelemetry, type ObserverStatus } from '$lib/api';
	import { ago, fmtNum, skewColor, fmtSkew, snrColor, roleColor, roleLabel, isFresh } from '$lib/format';
	import Sparkline from '$lib/components/Sparkline.svelte';

	const id = $derived(page.params.id ?? '');
	let windowSec = $state(86400);
	const windows = [
		{ label: '6h', sec: 21600 },
		{ label: '24h', sec: 86400 },
		{ label: '3d', sec: 259200 }
	];

	let observer = $state<Observer | null>(null);
	let data = $state<ObserverAnalytics | null>(null);
	let tel = $state<ObserverTelemetry | null>(null);
	let loading = $state(true);

	async function refresh() {
		try {
			const [obs, a, t] = await Promise.all([
				api.observers(),
				api.observerAnalytics(id, windowSec).catch(() => null),
				api.observerTelemetry(id, windowSec).catch(() => null)
			]);
			observer = obs.find((o) => o.id === id) ?? null;
			data = a;
			tel = t;
		} finally {
			loading = false;
		}
	}
	$effect(() => {
		windowSec;
		refresh();
	});
	onMount(() => {
		const t = setInterval(refresh, 15000);
		return () => clearInterval(t);
	});

	const st = $derived(observer?.status);
	function fmtDur(s?: number): string {
		if (s == null) return '—';
		const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
		return d ? `${d}d ${h}h` : h ? `${h}h ${m}m` : `${m}m`;
	}
	function noiseColor(d?: number): string {
		if (d == null) return 'var(--color-fg)';
		if (d <= -105) return 'var(--color-lime)';
		if (d <= -98) return 'var(--color-signal)';
		if (d <= -92) return 'var(--color-amber)';
		return 'var(--color-coral)';
	}
	function fmtRadio(s?: ObserverStatus): string {
		if (!s) return '—';
		const p: string[] = [];
		if (s.freqMhz != null) p.push(`${+s.freqMhz.toFixed(3)} MHz`);
		if (s.bandwidthKhz != null) p.push(`${s.bandwidthKhz}k`);
		if (s.spreadingFactor != null) p.push(`SF${s.spreadingFactor}`);
		if (s.codingRate != null) p.push(`CR${s.codingRate}`);
		return p.join(' · ') || s.radio || '—';
	}

	const kpis = $derived([
		{ label: 'Packets', value: data ? fmtNum(data.totalPackets) : '—', c: 'var(--color-signal)' },
		{ label: 'Pkts / hr', value: data ? data.packetsPerHour.toFixed(1) : '—', c: 'var(--color-signal)' },
		{ label: 'Nodes', value: data ? fmtNum(data.distinctNodes) : '—' },
		{ label: 'Direct', value: data ? fmtNum(data.directNodes) : '—', c: 'var(--color-lime)' },
		{ label: 'Avg SNR', value: data?.avgSnr != null ? data.avgSnr.toFixed(1) : '—', c: snrColor(data?.avgSnr) },
		{ label: 'Clock skew', value: fmtSkew(data?.clockSkewMs), c: skewColor(data?.clockSkewMs) }
	]);

	const batterySeries = $derived((tel?.points ?? []).map((p) => (p.batteryMv && p.batteryMv > 0 ? p.batteryMv / 1000 : null)));
	const noiseSeries = $derived((tel?.points ?? []).map((p) => p.noiseFloor ?? null));
	const hasBattery = $derived(batterySeries.some((v) => v != null));
	const hasNoise = $derived(noiseSeries.some((v) => v != null));
	const maxAct = $derived(Math.max(1, ...(data?.activity ?? [])));
	const maxPay = $derived(Math.max(1, ...(data?.payloadTypes ?? []).map((p) => p.count)));
</script>

<div class="px-4 py-4">
	{#if loading && !observer}
		<div class="text-fg-faint py-16 text-center text-sm">Loading…</div>
	{:else if !observer}
		<div class="text-fg-faint py-16 text-center text-sm">Observer not found.</div>
	{:else}
		<!-- heading -->
		<div class="mb-3 flex items-center gap-2.5">
			<span class="h-2.5 w-2.5 rounded-full" style="background:{isFresh(observer.lastSeen) ? 'var(--color-signal)' : 'var(--color-fg-faint)'}"></span>
			<h1 class="text-fg min-w-0 flex-1 truncate text-lg font-700">{observer.name ?? observer.id}</h1>
			{#if observer.region}<span class="label">{observer.region}</span>{/if}
		</div>
		<!-- Label above, actual identity here (the observer's MQTT topic key). -->
		<div class="text-fg-faint mb-3 font-mono text-[0.62rem] break-all">{observer.publicKey || observer.id}</div>

		<!-- window -->
		<div class="mb-3 flex gap-2">
			{#each windows as w (w.sec)}
				<button onclick={() => (windowSec = w.sec)} class="rounded-full border px-3 py-1 text-xs font-600 {windowSec === w.sec ? 'border-signal/50 bg-signal/15 text-signal' : 'border-line text-fg-dim'}">{w.label}</button>
			{/each}
		</div>

		<!-- KPIs -->
		<div class="border-line/60 bg-panel grid grid-cols-3 gap-px overflow-hidden rounded-2xl border">
			{#each kpis as k (k.label)}
				<div class="px-3 py-3">
					<div class="label !text-[0.55rem]">{k.label}</div>
					<div class="font-mono tnum mt-1 text-base font-700" style="color:{k.c ?? 'var(--color-fg)'}">{k.value}</div>
				</div>
			{/each}
		</div>

		<!-- radio & device -->
		<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">RADIO & DEVICE</h2>
		<div class="border-line/60 bg-panel rounded-2xl border p-4">
			{#if !st}
				<div class="text-fg-faint py-2 text-center text-sm">No status reported yet.</div>
			{:else}
				<div class="font-mono text-signal glow-signal text-base font-700">{fmtRadio(st)}</div>
				<div class="border-line/50 mt-3 grid grid-cols-2 gap-x-4 gap-y-2 border-t pt-3 text-xs">
					{#each [
						{ k: 'Battery', v: st.batteryMv && st.batteryMv > 0 ? (st.batteryMv / 1000).toFixed(2) + ' V' : '— (mains)', c: '' },
						{ k: 'Uptime', v: fmtDur(st.uptimeSecs), c: '' },
						{ k: 'Noise', v: st.noiseFloor != null ? st.noiseFloor.toFixed(0) + ' dBm' : '—', c: noiseColor(st.noiseFloor) },
						{ k: 'Recv err', v: st.recvErrors != null ? fmtNum(st.recvErrors) : '—', c: '' },
						{ k: 'TX air', v: st.txAirSecs != null ? fmtDur(st.txAirSecs) : '—', c: '' },
						{ k: 'RX air', v: st.rxAirSecs != null ? fmtDur(st.rxAirSecs) : '—', c: '' }
					] as f (f.k)}
						<div class="flex items-center justify-between">
							<span class="text-fg-faint">{f.k}</span>
							<span class="font-mono tnum" style="color:{f.c || 'var(--color-fg)'}">{f.v}</span>
						</div>
					{/each}
				</div>
				<div class="text-fg-faint mt-3 font-mono text-[0.62rem]">{st.model || '—'} · {st.firmware || ''}</div>
			{/if}
		</div>

		<!-- device trends -->
		{#if hasBattery || hasNoise}
			<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">DEVICE TRENDS</h2>
			<div class="grid grid-cols-1 gap-3">
				{#if hasBattery}
					<div class="border-line/60 bg-panel rounded-2xl border p-4">
						<div class="label">Battery</div>
						<div class="font-mono text-fg mt-1 text-lg font-700">{tel?.summary.batteryMv ? (tel.summary.batteryMv / 1000).toFixed(2) + ' V' : '—'}</div>
						<div class="mt-2"><Sparkline values={batterySeries} color="var(--color-signal)" /></div>
					</div>
				{/if}
				{#if hasNoise}
					<div class="border-line/60 bg-panel rounded-2xl border p-4">
						<div class="label">Noise floor</div>
						<div class="font-mono mt-1 text-lg font-700" style="color:{noiseColor(tel?.summary.noiseFloor)}">{tel?.summary.noiseFloor != null ? tel.summary.noiseFloor.toFixed(0) + ' dBm' : '—'}</div>
						<div class="mt-2"><Sparkline values={noiseSeries} color="var(--color-amber)" /></div>
					</div>
				{/if}
			</div>
		{/if}

		<!-- throughput -->
		{#if data && data.activity.length && data.totalPackets > 0}
			<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">FEED THROUGHPUT</h2>
			<div class="border-line/60 bg-panel rounded-2xl border p-4">
				<div class="flex h-[90px] items-end gap-px">
					{#each data.activity as c, i (i)}
						<div class="bg-signal/70 w-full rounded-t-[2px]" style="height:{Math.max(2, (c / maxAct) * 90)}px"></div>
					{/each}
				</div>
				<div class="text-fg-faint mt-2 flex justify-between font-mono text-[0.6rem]"><span>-{Math.round(data.windowHours)}h</span><span>now</span></div>
			</div>
		{/if}

		<!-- payload mix -->
		{#if data?.payloadTypes?.length}
			<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">PAYLOAD MIX</h2>
			<div class="border-line/60 bg-panel rounded-2xl border px-4 py-3">
				{#each data.payloadTypes as p (p.label)}
					<div class="flex items-center gap-3 py-1.5">
						<span class="text-fg-dim w-24 shrink-0 truncate text-xs">{p.label}</span>
						<div class="bg-line/40 h-2.5 flex-1 overflow-hidden rounded-full"><div class="bg-signal h-full" style="width:{(p.count / maxPay) * 100}%"></div></div>
						<span class="text-fg-faint w-10 text-right font-mono text-xs tnum">{p.count}</span>
					</div>
				{/each}
			</div>
		{/if}

		<!-- neighbours -->
		{#if data?.neighbors?.length}
			<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">DIRECTLY HEARS <span class="text-fg-faint font-400">· zero-hop</span></h2>
			<div class="border-line/60 bg-panel divide-line/50 divide-y overflow-hidden rounded-2xl border">
				{#each data.neighbors as l (l.nodeKey)}
					<a href="/m/nodes/{l.nodeKey}" class="active:bg-line/40 flex items-center gap-3 px-4 py-2.5">
						<span class="h-2 w-2 shrink-0 rounded-full" style="background:{roleColor(l.role)}"></span>
						<span class="text-fg min-w-0 flex-1 truncate text-sm">{l.nodeName}</span>
						<span class="label !text-[0.55rem]">{roleLabel(l.role)}</span>
						<span class="text-fg-faint font-mono text-xs tnum">×{l.count}</span>
					</a>
				{/each}
			</div>
		{/if}
	{/if}
</div>
