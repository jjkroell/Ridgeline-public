<script lang="ts">
	import type { Snippet } from 'svelte';

	// `class` is applied to the wrapper so it can carry the layout classes of the
	// element it replaces (widths, shrink-0, etc.) and not disturb flex rows.
	let {
		text,
		class: cls = '',
		children
	}: { text: string; class?: string; children: Snippet } = $props();

	let show = $state(false);
	let pos = $state({ x: 0, y: 0 });

	// Render the bubble straight under <body>. Its `position: fixed` is otherwise
	// resolved against any ancestor with a transform (e.g. PageHeader's `rise`
	// animation), which mis-anchors it and overflows the viewport.
	function portal(node: HTMLElement) {
		document.body.appendChild(node);
		return { destroy: () => node.remove() };
	}

	function enter(e: MouseEvent) {
		const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
		// Clamp the (centered, max-250px) bubble inside the viewport so it never
		// overflows the right/left edge — an overflowing fixed element adds a
		// scrollbar and reflows the page (~10px shift) near edge-anchored controls.
		const half = 125; // half of max-w-[250px]
		const margin = 8;
		const center = r.left + r.width / 2;
		const x = Math.min(Math.max(center, half + margin), window.innerWidth - half - margin);
		pos = { x, y: r.top };
		show = true;
	}
</script>

<span class="inline-flex {cls}" onmouseenter={enter} onmouseleave={() => (show = false)} role="note">
	{@render children()}
</span>

{#if show}
	<div
		use:portal
		class="border-line-bright bg-ink-2 text-fg-dim pointer-events-none fixed z-[100] max-w-[250px] rounded-[var(--radius)] border px-2.5 py-1.5 text-xs leading-snug shadow-xl"
		style="left:{pos.x}px;top:{pos.y}px;transform:translate(-50%,calc(-100% - 9px))"
	>
		{text}
		<span
			class="bg-ink-2 border-line-bright absolute top-full left-1/2 -mt-[5px] h-2 w-2 -translate-x-1/2 rotate-45 border-r border-b"
		></span>
	</div>
{/if}
