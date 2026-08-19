<script lang="ts">
	import Seo from '$lib/components/Seo.svelte';
	import { onMount } from 'svelte';
	import { api, type Observer } from '$lib/api';
	import { ago, fmtNum, isFresh } from '$lib/format';
	import StandbyBadge from '$lib/components/StandbyBadge.svelte';
	import ObserverSetupModal from '$lib/components/ObserverSetupModal.svelte';
	import { MQTT } from '$lib/site-content';

	let observers = $state<Observer[]>([]);
	let showSetup = $state(false);

	async function refresh() {
		try {
			observers = await api.observers();
		} catch {
			/* keep last */
		}
	}
	onMount(() => {
		refresh();
		const t = setInterval(refresh, 15000);
		return () => clearInterval(t);
	});

	function radioLine(o: Observer): string {
		const s = o.status;
		if (!s) return '';
		const p: string[] = [];
		if (s.freqMhz != null) p.push(`${+s.freqMhz.toFixed(3)}`);
		if (s.spreadingFactor != null) p.push(`SF${s.spreadingFactor}`);
		return p.join(' · ');
	}
</script>

<Seo title="Observers" description="Listening posts feeding this observatory." path="/m/observers" />

{#if showSetup && MQTT.broker}
	<ObserverSetupModal onclose={() => (showSetup = false)} />
{/if}

<div class="px-4 py-4">
	<div class="mb-2 flex items-center gap-2 px-1">
		<div class="text-fg-faint font-mono text-[0.62rem]">{observers.length} listening posts</div>
		{#if MQTT.broker}
		<button
			onclick={() => (showSetup = true)}
			class="border-signal/50 bg-signal/10 text-signal active:bg-signal/20 ml-auto rounded-full border px-3 py-1 font-mono text-[0.62rem]"
			>Add an observer</button
		>
		{/if}
	</div>
	<div class="flex flex-col gap-3">
		{#each observers as o (o.id)}
			{@const reporting = isFresh(o.lastSeen)}
			<a href="/m/observers/{encodeURIComponent(o.id)}" class="border-line/60 bg-panel active:bg-line/40 rounded-2xl border px-4 py-3.5">
				<div class="flex items-center gap-2.5">
					<span class="h-2.5 w-2.5 shrink-0 rounded-full" style="background:{reporting ? 'var(--color-signal)' : 'var(--color-fg-faint)'}"></span>
					<span class="text-fg min-w-0 flex-1 truncate text-sm font-600">{o.name ?? o.id}</span>
					{#if o.jwtAuthAt}
						<span class="text-lime shrink-0 font-mono text-[0.55rem] font-600 tracking-wider">JWT AUTH</span>
					{/if}
					{#if o.standbySince}
						<StandbyBadge observer={o} compact />
					{:else if o.region}<span class="label !text-[0.55rem] shrink-0">{o.region}</span>{/if}
				</div>
				<div class="text-fg-faint mt-2 flex items-center gap-2 font-mono text-[0.62rem]">
					<!-- Standby wins over Reporting/Silent: the receiver is still reporting,
					     its packets are just being discarded. -->
					<span class={o.standbySince ? 'text-amber' : reporting ? 'text-signal' : ''}
						>{o.standbySince ? 'Standby' : reporting ? 'Reporting' : 'Silent'}</span
					>
					<span>· {fmtNum(o.packetCount)} pkts</span>
					<span>· {ago(o.lastSeen)}</span>
				</div>
				{#if radioLine(o)}
					<div class="text-fg-faint mt-1 font-mono text-[0.62rem]">{radioLine(o)}{#if o.status?.noiseFloor != null} · noise {o.status.noiseFloor.toFixed(0)}dBm{/if}</div>
				{/if}
			</a>
		{/each}
	</div>
</div>
