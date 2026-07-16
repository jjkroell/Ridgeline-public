<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type MeshAnalytics } from '$lib/api';
	import { fmtNum, shortKey, roleColor } from '$lib/format';

	let windowSec = $state(21600);
	const windows = [
		{ label: '1h', sec: 3600 },
		{ label: '6h', sec: 21600 },
		{ label: '24h', sec: 86400 }
	];
	let data = $state<MeshAnalytics | null>(null);
	let loading = $state(true);

	async function refresh() {
		try {
			data = await api.meshAnalytics(windowSec);
		} catch {
			/* keep */
		} finally {
			loading = false;
		}
	}
	$effect(() => {
		windowSec;
		refresh();
	});
	onMount(() => {
		const t = setInterval(refresh, 30000);
		return () => clearInterval(t);
	});

	const tierColor: Record<string, string> = {
		Quiet: 'var(--color-lime)',
		Normal: 'var(--color-signal)',
		Busy: 'var(--color-amber)',
		Congested: 'var(--color-coral)'
	};
	const kpis = $derived(
		data
			? [
					{ label: 'Active nodes', value: fmtNum(data.kpis.activeNodes) },
					{ label: 'Transmissions', value: fmtNum(data.kpis.transmissions) },
					{ label: 'Observations', value: fmtNum(data.kpis.observations) },
					{ label: 'Avg link', value: data.kpis.avgLinkScore != null ? (data.kpis.avgLinkScore * 100).toFixed(0) + '%' : '—' },
					{ label: 'Redundancy', value: data.kpis.floodRedundancy != null ? data.kpis.floodRedundancy.toFixed(1) + '×' : '—' },
					{ label: 'Channel util', value: data.kpis.channelUtilPct.toFixed(2) + '%' }
				]
			: []
	);
	const maxPay = $derived(Math.max(1, ...(data?.payloadTypes ?? []).map((p) => p.count)));
	const maxRelay = $derived(Math.max(1, ...(data?.topRelays ?? []).map((r) => r.relayed)));
</script>

<div class="px-4 py-4">
	<!-- window -->
	<div class="mb-3 flex items-center gap-2">
		{#each windows as w (w.sec)}
			<button onclick={() => (windowSec = w.sec)} class="rounded-full border px-3 py-1 text-xs font-600 {windowSec === w.sec ? 'border-signal/50 bg-signal/15 text-signal' : 'border-line text-fg-dim'}">{w.label}</button>
		{/each}
		{#if data}
			<span class="ml-auto rounded-full px-2.5 py-1 text-xs font-700" style="color:{tierColor[data.kpis.congestionTier] ?? 'var(--color-fg)'};background:color-mix(in srgb, {tierColor[data.kpis.congestionTier] ?? 'var(--color-fg)'} 14%, transparent)">{data.kpis.congestionTier}</span>
		{/if}
	</div>

	{#if loading && !data}
		<div class="text-fg-faint py-16 text-center text-sm">Loading…</div>
	{:else if data}
		<!-- KPIs -->
		<div class="border-line/60 bg-panel grid grid-cols-3 gap-px overflow-hidden rounded-2xl border">
			{#each kpis as k (k.label)}
				<div class="px-3 py-3">
					<div class="label !text-[0.55rem]">{k.label}</div>
					<div class="font-mono tnum text-fg mt-1 text-base font-700">{k.value}</div>
				</div>
			{/each}
		</div>

		<!-- traffic mix -->
		<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">TRAFFIC MIX</h2>
		<div class="border-line/60 bg-panel rounded-2xl border px-4 py-3">
			{#each data.payloadTypes as p (p.label)}
				<div class="flex items-center gap-3 py-1.5">
					<span class="text-fg-dim w-24 shrink-0 truncate text-xs">{p.label}</span>
					<div class="bg-line/40 h-2.5 flex-1 overflow-hidden rounded-full"><div class="bg-signal h-full" style="width:{(p.count / maxPay) * 100}%"></div></div>
					<span class="text-fg-faint w-12 text-right font-mono text-xs tnum">{fmtNum(p.count)}</span>
				</div>
			{/each}
		</div>

		<!-- busiest relays -->
		<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">BUSIEST RELAYS</h2>
		<div class="border-line/60 bg-panel divide-line/50 divide-y overflow-hidden rounded-2xl border">
			{#each data.topRelays.slice(0, 10) as r (r.publicKey)}
				<a href="/m/nodes/{r.publicKey}" class="active:bg-line/40 flex items-center gap-3 px-4 py-2.5">
					<span class="h-2 w-2 shrink-0 rounded-full" style="background:{roleColor(r.role)}"></span>
					<span class="text-fg min-w-0 flex-1 truncate text-sm">{r.name || shortKey(r.publicKey)}</span>
					<div class="bg-line/40 h-1.5 w-16 shrink-0 overflow-hidden rounded-full"><div class="bg-amber h-full" style="width:{(r.relayed / maxRelay) * 100}%"></div></div>
					<span class="text-fg-faint w-8 text-right font-mono text-xs tnum">{r.relayed}</span>
				</a>
			{/each}
		</div>

		<!-- observer coverage -->
		<h2 class="font-display text-fg mt-5 mb-2 px-1 text-xs font-700 tracking-wide">OBSERVER COVERAGE</h2>
		<div class="border-line/60 bg-panel divide-line/50 divide-y overflow-hidden rounded-2xl border">
			{#each data.observers as o (o.id)}
				<a href="/m/observers/{encodeURIComponent(o.id)}" class="active:bg-line/40 flex items-center gap-3 px-4 py-2.5">
					<span class="text-fg min-w-0 flex-1 truncate text-sm">{o.id}</span>
					<span class="text-fg-faint font-mono text-[0.62rem] tnum">{fmtNum(o.distinctNodes)} nodes · {fmtNum(o.directNodes)} direct</span>
				</a>
			{/each}
		</div>
		<div class="text-fg-faint mt-3 mb-1 px-1 text-center font-mono text-[0.58rem]">SF{data.radio.SpreadingFactor} · BW{(data.radio.BandwidthHz / 1000).toFixed(1)}k · {Math.round(data.windowHours)}h window</div>
	{/if}
</div>
