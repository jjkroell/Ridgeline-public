<!--
  Shared frame for Overview dashboard widgets: a panel with an icon + title
  header (optional "view all" link) and a body. Used by every widget so the
  customizable cards read as one system on both desktop and mobile.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		title,
		icon,
		href,
		linkLabel = 'View →',
		color = 'dim',
		fill = false,
		children
	}: {
		title: string;
		icon: string;
		href?: string;
		linkLabel?: string;
		color?: 'signal' | 'amber' | 'dim';
		fill?: boolean;
		children: Snippet;
	} = $props();

	const colorClass = $derived({ signal: 'text-signal', amber: 'text-amber', dim: 'text-fg-dim' }[color]);
</script>

<section class="panel flex h-full flex-col overflow-hidden">
	<div class="border-line/70 flex items-center gap-2.5 border-b px-5 py-3.5">
		<svg
			viewBox="0 0 24 24"
			class="h-4 w-4 shrink-0 {colorClass}"
			fill={fill ? 'currentColor' : 'none'}
			stroke="currentColor"
			stroke-width="1.6"
			stroke-linecap="round"
			stroke-linejoin="round"><path d={icon} /></svg
		>
		<h2 class="font-display text-fg truncate text-sm font-700 tracking-wide">{title.toUpperCase()}</h2>
		{#if href}
			<a {href} class="label hover:text-signal ml-auto shrink-0 transition-colors">{linkLabel}</a>
		{/if}
	</div>
	<div class="min-h-0 flex-1 overflow-y-auto" style="max-height:20rem">
		{@render children()}
	</div>
</section>
