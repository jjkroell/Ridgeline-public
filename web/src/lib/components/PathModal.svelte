<script lang="ts">
	import type { LiveGroup } from '$lib/live.svelte';
	import type { Node } from '$lib/api';
	import { shortKey } from '$lib/format';
	import PayloadTag from './PayloadTag.svelte';
	import Modal from './Modal.svelte';
	import HopChips from './HopChips.svelte';

	interface Props {
		group: LiveGroup | null;
		nodes: Node[];
		showIds?: boolean;
		onclose: () => void;
	}
	let { group, nodes, showIds = false, onclose }: Props = $props();

	// The most complete observed path (longest hop list) for this transmission.
	const path = $derived.by(() => {
		if (!group) return [] as string[];
		return [...group.events].sort((a, b) => (b.path?.length ?? 0) - (a.path?.length ?? 0))[0]?.path ?? [];
	});
	const variants = $derived(
		group ? new Set(group.events.map((e) => (e.path ?? []).join('>'))).size : 0
	);
	const title = $derived(
		group?.node ? group.node.name || shortKey(group.node.publicKey, 8, 4) : (group?.messageHash ?? '')
	);
</script>

{#if group}
	<Modal {onclose} size="lg">
		<div class="border-line/70 flex items-start gap-3 border-b px-5 py-4">
			<div class="min-w-0 flex-1">
				<div class="mb-2 flex flex-wrap items-center gap-2">
					<PayloadTag type={group.payloadType} />
					<span class="label">Full Path · {path.length} hops</span>
				</div>
				<h2 class="font-display text-fg truncate text-lg font-700">{title}</h2>
				<div class="font-mono text-fg-faint text-[0.68rem]">{group.messageHash}</div>
			</div>
			<button onclick={onclose} class="text-fg-faint hover:text-fg shrink-0 text-xl leading-none" aria-label="Close">✕</button>
		</div>

		<div class="min-h-0 flex-1 overflow-y-auto px-5 py-4">
			{#if path.length === 0}
				<div class="text-fg-faint text-sm">Direct transmission — no relay hops recorded.</div>
			{:else}
				{#if variants > 1}
					<div class="label text-amber mb-3 normal-case">
						Showing the most complete route · {variants} variants seen across repeats (open the packet for per-repeat paths)
					</div>
				{/if}
				<HopChips hops={path} {nodes} {showIds} onnavigate={onclose} />
			{/if}
		</div>
	</Modal>
{/if}
