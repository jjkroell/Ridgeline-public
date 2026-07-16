<!--
  Renders the user's chosen Overview widgets, in their saved order, into a
  responsive grid (2 columns on desktop, 1 on mobile). Shared by the desktop (/)
  and mobile (/m) Overview pages; `base` is '' or '/m' so widget links route to
  the right app. The top stat row lives on the pages, not here.
-->
<script lang="ts">
	import { overview } from '$lib/overview.svelte';
	import type { Node, Stats } from '$lib/api';

	import FavoritesWidget from './FavoritesWidget.svelte';
	import LiveFeedWidget from './LiveFeedWidget.svelte';
	import RecentNodesWidget from './RecentNodesWidget.svelte';
	import ClaimedNodesWidget from './ClaimedNodesWidget.svelte';
	import MyNodesWidget from './MyNodesWidget.svelte';
	import NewNodesWidget from './NewNodesWidget.svelte';
	import BackboneWidget from './BackboneWidget.svelte';
	import MiniMapWidget from './MiniMapWidget.svelte';
	import ActivityWidget from './ActivityWidget.svelte';
	import ObserversWidget from './ObserversWidget.svelte';
	import ChannelsWidget from './ChannelsWidget.svelte';

	let { base = '', nodes = [], stats = null }: { base?: string; nodes?: Node[]; stats?: Stats | null } =
		$props();

	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const COMPONENTS: Record<string, any> = {
		favorites: FavoritesWidget,
		livefeed: LiveFeedWidget,
		recentnodes: RecentNodesWidget,
		claimed: ClaimedNodesWidget,
		mynodes: MyNodesWidget,
		newnodes: NewNodesWidget,
		backbone: BackboneWidget,
		minimap: MiniMapWidget,
		activity: ActivityWidget,
		observers: ObserversWidget,
		channels: ChannelsWidget
	};
</script>

{#if overview.visibleIds.length === 0}
	<div class="panel text-fg-faint mt-6 px-5 py-10 text-center text-sm">
		No cards shown. Use <span class="text-fg-dim">Customize</span> to add some.
	</div>
{:else}
	<div class="mt-6 grid grid-cols-1 gap-5 lg:grid-cols-2">
		{#each overview.entries as e (e.id)}
			{#if e.visible}
				{@const meta = overview.meta(e.id)}
				{@const Comp = COMPONENTS[e.id]}
				{#if Comp}
					<div class={meta?.size === 'full' ? 'lg:col-span-2' : ''}>
						<Comp {base} {nodes} {stats} />
					</div>
				{/if}
			{/if}
		{/each}
	</div>
{/if}
