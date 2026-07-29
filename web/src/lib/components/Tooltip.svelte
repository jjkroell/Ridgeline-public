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
	// The anchor's centre, unclamped — the edge clamp is applied at render, once
	// the bubble's real width is known.
	let pos = $state({ x: 0, y: 0 });
	let bubbleW = $state(0);

	const MARGIN = 8;
	// Clamping against half of max-w (125px) rather than the actual width pushed
	// every tooltip in the left sidebar to x >= 133 — so a row of small controls
	// got one shared position instead of one per control. Measure, then clamp.
	const half = $derived(bubbleW ? bubbleW / 2 : 0);
	const left = $derived(
		bubbleW
			? Math.min(Math.max(pos.x, half + MARGIN), window.innerWidth - half - MARGIN)
			: pos.x
	);

	// Render the bubble straight under <body>. Its `position: fixed` is otherwise
	// resolved against any ancestor with a transform (e.g. PageHeader's `rise`
	// animation), which mis-anchors it and overflows the viewport.
	function portal(node: HTMLElement) {
		document.body.appendChild(node);
		return { destroy: () => node.remove() };
	}

	function enter(e: MouseEvent) {
		const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
		// Anchor on the trigger's centre. Keeping the viewport clamp out of here
		// matters because an overflowing fixed element adds a scrollbar and
		// reflows the page (~10px shift) near edge-anchored controls — but the
		// clamp can only be correct once the bubble has been measured.
		bubbleW = 0;
		pos = { x: r.left + r.width / 2, y: r.top };
		show = true;
	}
</script>

<span class="inline-flex {cls}" onmouseenter={enter} onmouseleave={() => (show = false)} role="note">
	{@render children()}
</span>

{#if show}
	<div
		use:portal
		bind:clientWidth={bubbleW}
		class="border-line-bright bg-ink-2 text-fg-dim pointer-events-none fixed z-[100] max-w-[250px] rounded-[var(--radius)] border px-2.5 py-1.5 text-xs leading-snug shadow-xl"
		style="left:{left}px;top:{pos.y}px;transform:translate(-50%,calc(-100% - 9px));visibility:{bubbleW
			? 'visible'
			: 'hidden'}"
	>
		{text}
		<span
			class="bg-ink-2 border-line-bright absolute top-full left-1/2 -mt-[5px] h-2 w-2 -translate-x-1/2 rotate-45 border-r border-b"
		></span>
	</div>
{/if}
