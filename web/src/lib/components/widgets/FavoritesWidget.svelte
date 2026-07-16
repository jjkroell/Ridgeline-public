<script lang="ts">
	import WidgetShell from './WidgetShell.svelte';
	import { overview } from '$lib/overview.svelte';
	import { favorites } from '$lib/favorites.svelte';
	import { ago, shortKey, nodeStatus } from '$lib/format';
	import RoleBadge from '$lib/components/RoleBadge.svelte';
	import FavoriteStar from '$lib/components/FavoriteStar.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';
	import type { Node } from '$lib/api';

	let { base = '', nodes = [] }: { base?: string; nodes?: Node[] } = $props();
	const m = overview.meta('favorites')!;

	const favNodes = $derived(
		favorites.keys
			.map((k) => nodes.find((n) => n.publicKey.toUpperCase() === k))
			.filter((n): n is Node => !!n)
	);
</script>

<WidgetShell title={m.title} icon={m.icon} color="amber" fill href="{base}/nodes" linkLabel="Manage →">
	{#if favNodes.length}
		<div class="divide-line/50 divide-y">
			{#each favNodes as n (n.publicKey)}
				{@const st = nodeStatus(n)}
				<a href="{base}/nodes/{n.publicKey}" class="panel-hover flex items-center gap-3 px-5 py-2.5">
					<Tooltip text={st.label} class="shrink-0"><span class="h-2 w-2 rounded-full" style="background:{st.color}"></span></Tooltip>
					<div class="min-w-0 flex-1">
						<div class="text-fg truncate text-sm font-medium">{n.name || shortKey(n.publicKey)}</div>
						<div class="font-mono text-fg-faint mt-0.5 text-[0.68rem]">{st.label} · {ago(n.lastSeen)}</div>
					</div>
					<RoleBadge role={n.role} />
					<FavoriteStar pubkey={n.publicKey} size="sm" />
				</a>
			{/each}
		</div>
	{:else}
		<div class="text-fg-faint px-5 py-8 text-center text-sm">
			No favorites yet — tap the ☆ on a node to pin it here.
		</div>
	{/if}
</WidgetShell>
