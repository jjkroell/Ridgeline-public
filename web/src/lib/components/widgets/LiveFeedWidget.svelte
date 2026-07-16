<script lang="ts">
	import WidgetShell from './WidgetShell.svelte';
	import { overview } from '$lib/overview.svelte';
	import { live, groupLive, type LiveGroup } from '$lib/live.svelte';
	import { ago, shortKey, snrColor, fmtSnr } from '$lib/format';
	import PayloadTag from '$lib/components/PayloadTag.svelte';
	import LiveGroupModal from '$lib/components/LiveGroupModal.svelte';

	let { base = '' }: { base?: string } = $props();
	const m = overview.meta('livefeed')!;

	const groups = $derived(groupLive(live.events));
	let selected = $state<LiveGroup | null>(null);
</script>

<WidgetShell title={m.title} icon={m.icon} href="{base}/live" linkLabel="View all →">
	<div class="divide-line/50 divide-y">
		{#if groups.length === 0}
			<div class="text-fg-faint px-5 py-10 text-center text-sm">Waiting for packets…</div>
		{:else}
			{#each groups.slice(0, 8) as g (g.key)}
				<button
					onclick={() => (selected = g)}
					class="panel-hover flex w-full items-center gap-3 px-5 py-2.5 text-left text-sm"
				>
					<PayloadTag type={g.payloadType} />
					<span class="text-fg-dim flex min-w-0 flex-1 items-center gap-2 truncate">
						<span class="truncate">
							{#if g.node}
								<span class="text-fg">{g.node.name || shortKey(g.node.publicKey)}</span>
							{:else}
								<span class="font-mono text-fg-faint">{g.messageHash}</span>
							{/if}
						</span>
						{#if g.count > 1}
							<span class="font-mono text-signal bg-signal/10 shrink-0 rounded-[var(--radius)] px-1.5 py-0.5 text-[0.62rem] tnum">×{g.count}</span>
						{/if}
					</span>
					<span class="font-mono tnum text-xs" style="color:{snrColor(g.bestSnr)}">{fmtSnr(g.bestSnr)} dB</span>
					<span class="font-mono text-fg-faint w-10 shrink-0 text-right text-xs">{ago(g.latest)}</span>
				</button>
			{/each}
		{/if}
	</div>
</WidgetShell>

<LiveGroupModal group={selected} onclose={() => (selected = null)} />
