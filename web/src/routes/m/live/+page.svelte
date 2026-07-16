<script lang="ts">
	import { live, groupLive, type LiveGroup } from '$lib/live.svelte';
	import { ago, shortKey, fmtSnr, snrColor, roleColor } from '$lib/format';
	import LiveGroupModal from '$lib/components/LiveGroupModal.svelte';

	let selected = $state<LiveGroup | null>(null);
	let paused = $state(false);
	let frozen = $state<LiveGroup[]>([]);

	const liveGroups = $derived(groupLive(live.events));
	const groups = $derived(paused ? frozen : liveGroups);

	function togglePause() {
		if (!paused) frozen = liveGroups;
		paused = !paused;
	}
</script>

<div class="px-4 py-4">
	<div class="mb-3 flex items-center gap-2">
		<div class="text-fg-faint flex items-center gap-1.5 font-mono text-[0.68rem]">
			{#if live.connected}<span class="live-dot"></span><span class="text-signal">STREAMING</span>{:else}<span class="bg-coral/70 h-2 w-2 rounded-full"></span><span class="text-coral">OFFLINE</span>{/if}
		</div>
		<span class="text-fg-faint ml-auto font-mono text-[0.62rem] tnum">{live.total} this session</span>
		<button onclick={togglePause} class="rounded-full border px-3 py-1 text-xs font-600 {paused ? 'border-amber/50 text-amber bg-amber/10' : 'border-line text-fg-dim'}">{paused ? 'Paused' : 'Pause'}</button>
	</div>

	<div class="border-line/60 bg-panel divide-line/50 divide-y overflow-hidden rounded-2xl border">
		{#if groups.length === 0}
			<div class="text-fg-faint px-4 py-12 text-center text-sm">Waiting for packets…</div>
		{:else}
			{#each groups as g (g.key)}
				{@const col = roleColor(g.node?.role ?? '')}
				<button onclick={() => (selected = g)} class="active:bg-line/40 flex w-full items-start gap-2.5 px-4 py-3 text-left">
					<span class="mt-0.5 shrink-0 rounded-md px-1.5 py-0.5 text-[0.6rem] font-700" style="color:{col};background:color-mix(in srgb, {col} 12%, transparent)">{g.payloadType.slice(0, 4)}</span>
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-1.5">
							<span class="text-fg min-w-0 truncate text-sm font-medium">
								{#if g.node}{g.node.name || shortKey(g.node.publicKey)}{:else}<span class="font-mono text-fg-dim text-xs">{g.messageHash}</span>{/if}
							</span>
							{#if g.count > 1}<span class="text-signal bg-signal/10 shrink-0 rounded px-1 py-0.5 font-mono text-[0.58rem] tnum">×{g.count}</span>{/if}
						</div>
						<div class="text-fg-faint mt-0.5 font-mono text-[0.62rem]">{g.payloadType}</div>
					</div>
					<div class="shrink-0 text-right">
						<div class="font-mono text-xs tnum" style="color:{snrColor(g.bestSnr)}">{fmtSnr(g.bestSnr)} dB</div>
						<div class="text-fg-faint mt-0.5 font-mono text-[0.62rem]">{ago(g.latest)}</div>
					</div>
				</button>
			{/each}
		{/if}
	</div>
</div>

<LiveGroupModal group={selected} onclose={() => (selected = null)} />
