<!--
  The Overview "Customize" panel: reorder cards by dragging the handle (pointer
  events, so it works with mouse and touch alike) and show/hide each card with
  the eye toggle. Reflects overview.entries live; changes persist immediately.
-->
<script lang="ts">
	import { overview } from '$lib/overview.svelte';

	let draggingId = $state<string | null>(null);

	// The drag handle only STARTS the drag. Move/end are handled on the window so
	// they keep firing even as reordering moves rows around in the DOM (pointer
	// capture on the handle would be lost when its node is repositioned, leaving
	// the row stuck mid-drag). No capture, so release is always seen → it commits.
	function onPointerDown(e: PointerEvent, id: string) {
		e.preventDefault();
		draggingId = id;
	}

	function onWindowPointerMove(e: PointerEvent) {
		if (!draggingId) return;
		e.preventDefault();
		// The dragged row is pointer-events:none (see markup) so elementFromPoint
		// returns the row underneath the cursor — reorder onto it.
		const under = document.elementFromPoint(e.clientX, e.clientY);
		const row = under?.closest('[data-wid]') as HTMLElement | null;
		const targetId = row?.dataset.wid;
		if (targetId && targetId !== draggingId) overview.move(draggingId, targetId);
	}

	function endDrag() {
		draggingId = null;
	}
</script>

<svelte:window
	onpointermove={onWindowPointerMove}
	onpointerup={endDrag}
	onpointercancel={endDrag}
/>

<div class="panel rise mt-6 overflow-hidden">
	<div class="border-line/70 flex items-center gap-2.5 border-b px-5 py-3.5">
		<svg viewBox="0 0 24 24" class="text-signal h-4 w-4" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
			<path d="M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z" /><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
		</svg>
		<h2 class="font-display text-fg text-sm font-700 tracking-wide">CUSTOMIZE DASHBOARD</h2>
		<button onclick={() => overview.reset()} class="label hover:text-signal ml-auto transition-colors">Reset</button>
		<button
			onclick={() => (overview.editing = false)}
			class="border-signal/50 text-signal hover:bg-signal/10 rounded-[var(--radius)] border px-3 py-1 text-xs font-600 transition-colors"
		>
			Done
		</button>
	</div>
	<p class="text-fg-faint border-line/50 border-b px-5 py-2.5 text-xs">
		Drag <span class="text-fg-dim">⠿</span> to reorder · tap the eye to show or hide a card. The top stats row stays put.
	</p>
	<div class="divide-line/50 divide-y">
		{#each overview.entries as e (e.id)}
			{@const meta = overview.meta(e.id)}
			{#if meta}
				<div
					data-wid={e.id}
					class="flex items-center gap-3 px-4 py-2.5 transition-opacity {draggingId === e.id
						? 'pointer-events-none opacity-40'
						: ''} {e.visible ? '' : 'opacity-60'}"
				>
					<button
						class="text-fg-faint hover:text-fg-dim shrink-0 cursor-grab touch-none px-1 active:cursor-grabbing"
						aria-label="Drag to reorder {meta.title}"
						onpointerdown={(ev) => onPointerDown(ev, e.id)}
					>
						<svg viewBox="0 0 24 24" class="h-5 w-5" fill="currentColor"><circle cx="9" cy="6" r="1.6" /><circle cx="15" cy="6" r="1.6" /><circle cx="9" cy="12" r="1.6" /><circle cx="15" cy="12" r="1.6" /><circle cx="9" cy="18" r="1.6" /><circle cx="15" cy="18" r="1.6" /></svg>
					</button>
					<svg viewBox="0 0 24 24" class="text-fg-dim h-4 w-4 shrink-0 self-start mt-0.5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d={meta.icon} /></svg>
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-1.5">
							<span class="text-fg text-sm font-medium">{meta.title}</span>
							{#if meta.requiresAuth}<span class="text-fg-faint text-[0.62rem]">(sign in)</span>{/if}
						</div>
						<div class="text-fg-faint mt-0.5 text-xs">{meta.desc}</div>
					</div>
					<button
						onclick={() => overview.toggle(e.id)}
						class="shrink-0 rounded-[var(--radius)] p-1.5 transition-colors {e.visible ? 'text-signal' : 'text-fg-faint hover:text-fg-dim'}"
						aria-label={e.visible ? `Hide ${meta.title}` : `Show ${meta.title}`}
					>
						{#if e.visible}
							<svg viewBox="0 0 24 24" class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7-10-7-10-7z" /><circle cx="12" cy="12" r="3" /></svg>
						{:else}
							<svg viewBox="0 0 24 24" class="h-5 w-5" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M9.9 4.24A9.1 9.1 0 0 1 12 4c7 0 10 8 10 8a13.2 13.2 0 0 1-1.67 2.68M6.6 6.6A13.3 13.3 0 0 0 2 12s3 8 10 8a9.3 9.3 0 0 0 5.4-1.6M1 1l22 22" /></svg>
						{/if}
					</button>
				</div>
			{/if}
		{/each}
	</div>
</div>
