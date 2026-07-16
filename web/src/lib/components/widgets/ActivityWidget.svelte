<script lang="ts">
	import { onMount } from 'svelte';
	import WidgetShell from './WidgetShell.svelte';
	import { overview } from '$lib/overview.svelte';
	import { api, type MeshAnalytics } from '$lib/api';
	import { fmtNum } from '$lib/format';
	import Sparkline from '$lib/components/Sparkline.svelte';

	let { base = '' }: { base?: string } = $props();
	const m = overview.meta('activity')!;

	let data = $state<MeshAnalytics | null>(null);

	async function load() {
		try {
			data = await api.meshAnalytics(21600, 10); // 6h, 10-min buckets
		} catch {
			/* keep last */
		}
	}
	onMount(() => {
		load();
		const t = setInterval(load, 30000);
		return () => clearInterval(t);
	});

	const util = $derived(data?.airtime?.map((b) => b.utilPct) ?? []);
	const tierColor = $derived(
		{ idle: 'var(--color-fg-dim)', light: 'var(--color-signal)', moderate: 'var(--color-amber)', heavy: 'var(--color-coral)', congested: 'var(--color-coral)' }[
			data?.kpis?.congestionTier ?? ''
		] ?? 'var(--color-signal)'
	);
</script>

<WidgetShell title={m.title} icon={m.icon} href="{base}/analytics" linkLabel="Analytics →">
	{#if !data}
		<div class="text-fg-faint px-5 py-10 text-center text-sm">Loading…</div>
	{:else}
		<div class="px-5 py-4">
			<div class="grid grid-cols-3 gap-3">
				<div>
					<div class="label">Active</div>
					<div class="font-display text-fg tnum mt-1 text-2xl font-700">{fmtNum(data.kpis.activeNodes)}</div>
				</div>
				<div>
					<div class="label">Channel Use</div>
					<div class="font-display tnum mt-1 text-2xl font-700" style="color:{tierColor}">{data.kpis.channelUtilPct.toFixed(1)}%</div>
				</div>
				<div>
					<div class="label">Load</div>
					<div class="font-display mt-1 text-sm font-700 capitalize" style="color:{tierColor}">{data.kpis.congestionTier}</div>
				</div>
			</div>
			{#if util.length > 1}
				<div class="mt-4">
					<div class="label mb-1">Channel utilisation · 6h</div>
					<Sparkline values={util} height={44} color={tierColor} />
				</div>
			{/if}
		</div>
	{/if}
</WidgetShell>
