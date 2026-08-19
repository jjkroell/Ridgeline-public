<script lang="ts">
	import Modal from './Modal.svelte';
	import { announce } from '$lib/announce.svelte';

	// Each entry is one new capability, in user-facing terms.
	const items: { icon: string; title: string; body: string }[] = [
		{
			icon: 'M4.9 19.1a10 10 0 0 1 0-14.2M19.1 4.9a10 10 0 0 1 0 14.2M7.8 16.2a6 6 0 0 1 0-8.4M16.2 7.8a6 6 0 0 1 0 8.4M12 13a1 1 0 100-2 1 1 0 000 2z',
			title: 'Nodes on another frequency are now marked',
			body: 'If part of your mesh runs on a second frequency joined by a bridge, those nodes now show in a distinct colour with their frequency under the name, on the node list and every map. They are worked out from the traffic crossing the bridge — a receiver on one side cannot hear the other directly.'
		},
		{
			icon: 'M12 21s-7-6.2-7-11a7 7 0 1114 0c0 4.8-7 11-7 11z M12 10a2 2 0 100-4 2 2 0 000 4z',
			title: 'Missing nodes are back on the maps',
			body: 'Nodes in a distant part of a mesh could be dropped from every map by the check that filters out broken GPS: a tight cluster of nodes elsewhere made anything far away look like an error. Remote nodes now stay on the map, and genuinely bad coordinates are still caught.'
		},
		{
			icon: 'M3 20h18L14 7l-4 6-2-2z',
			title: 'Node maps show the terrain',
			body: 'The small location map on a node\'s page now uses the same shaded-relief basemap as the full maps, so you can see at a glance whether a node is sitting on a ridge or down in a valley.'
		},
		{
			icon: 'M9 4 3 6v14l6-2 6 2 6-2V4l-6 2-6-2zM9 4v14M15 6v14',
			title: 'Observers can be put on standby',
			body: 'An observer on standby stays connected and listed, but every packet it reports is discarded instead of stored — for a receiver you are moving, bench-testing, or running firmware you do not yet trust.'
		}
	];
</script>

{#if announce.open}
	<Modal onclose={() => announce.close()} size="2xl">
		<div class="border-line/70 flex items-center gap-3 border-b px-6 py-4">
			<span class="bg-signal/15 text-signal rounded-full px-2.5 py-1 text-xs font-700">New</span>
			<h2 class="font-display text-fg text-lg font-700">What's new on Ridgeline</h2>
			<button
				onclick={() => announce.close()}
				class="text-fg-faint hover:text-fg ml-auto text-xl leading-none"
				aria-label="Close">✕</button
			>
		</div>

		<div class="min-h-0 flex-1 overflow-y-auto px-6 py-5">
			<ul class="flex flex-col gap-4">
				{#each items as item (item.title)}
					<li class="flex items-start gap-3">
						<span
							class="bg-signal/10 text-signal mt-0.5 grid h-9 w-9 shrink-0 place-items-center rounded-full"
						>
							<svg
								viewBox="0 0 24 24"
								class="h-[18px] w-[18px]"
								fill="none"
								stroke="currentColor"
								stroke-width="1.6"
								stroke-linecap="round"
								stroke-linejoin="round"><path d={item.icon} /></svg
							>
						</span>
						<div class="min-w-0">
							<div class="text-fg text-sm font-600">{item.title}</div>
							<div class="text-fg-dim mt-0.5 text-sm leading-relaxed">{item.body}</div>
						</div>
					</li>
				{/each}
			</ul>
		</div>

		<div class="border-line/70 flex items-center justify-end gap-3 border-t px-6 py-4">
			<a
				href="/about"
				onclick={() => announce.close()}
				class="text-fg-dim hover:text-fg text-sm transition-colors">Learn more</a
			>
			<button
				onclick={() => announce.close()}
				class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors"
				>Got it</button
			>
		</div>
	</Modal>
{/if}
