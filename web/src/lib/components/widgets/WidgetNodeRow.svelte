<!--
  One node row inside a list widget: name (+ optional claimed badge), a mono
  sub-line, and a trailing slot (defaults to role badge + last-heard). Links to
  the node detail page under the given base ('' desktop, '/m' mobile).
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { Node } from '$lib/api';
	import { ago, shortKey } from '$lib/format';
	import RoleBadge from '$lib/components/RoleBadge.svelte';
	import ClaimedBadge from '$lib/components/ClaimedBadge.svelte';

	let {
		node,
		base = '',
		sub,
		trailing
	}: {
		node: Node;
		base?: string;
		/** Mono sub-line under the name; defaults to the short public key. */
		sub?: string;
		trailing?: Snippet<[Node]>;
	} = $props();
</script>

<a href="{base}/nodes/{node.publicKey}" class="panel-hover flex items-center gap-3 px-5 py-2.5">
	<div class="min-w-0 flex-1">
		<div class="flex items-center gap-1.5">
			<span class="text-fg truncate text-sm font-medium">{node.name || shortKey(node.publicKey)}</span>
			{#if node.claimed}<ClaimedBadge pubkey={node.publicKey} />{/if}
		</div>
		<div class="font-mono text-fg-faint mt-0.5 text-[0.68rem] truncate">{sub ?? shortKey(node.publicKey, 8, 4)}</div>
	</div>
	{#if trailing}
		{@render trailing(node)}
	{:else}
		<RoleBadge role={node.role} />
		<span class="font-mono text-fg-faint w-8 shrink-0 text-right text-xs tnum">{ago(node.lastSeen)}</span>
	{/if}
</a>
