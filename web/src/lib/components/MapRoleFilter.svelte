<script lang="ts">
	import type { Snippet } from 'svelte';
	import { roleColor } from '$lib/format';

	// `selected` is the set of role keys currently shown. When `title` is set the
	// component renders as a labelled card (with an optional `children` slot for
	// extra controls, e.g. the live map's sound toggle); otherwise it's the bare
	// toggle row used by the static map.
	let {
		selected = $bindable(),
		title = '',
		open = $bindable(true),
		// Vertical offset class for the absolute panel. Overridable so the WebGL-free
		// maps can drop it below the "WebGL disabled" banner; defaults flush to the top.
		pos = 'top-3',
		children
	}: { selected: Set<string>; title?: string; open?: boolean; pos?: string; children?: Snippet } = $props();

	const ROLES: [string, string][] = [
		['Repeater', 'Repeaters'],
		['RoomServer', 'Rooms'],
		['ChatNode', 'Companions'],
		['Sensor', 'Sensors']
	];

	function toggle(key: string) {
		const s = new Set(selected);
		if (s.has(key)) s.delete(key);
		else s.add(key);
		selected = s;
	}
</script>

{#snippet roleBtn(key: string, label: string)}
	{@const on = selected.has(key)}
	{@const c = roleColor(key)}
	<button
		onclick={() => toggle(key)}
		class="flex items-center gap-1.5 rounded-[var(--radius)] px-2 py-1 text-[0.68rem] font-medium transition-colors {on
			? ''
			: 'text-fg-faint hover:text-fg-dim'}"
		style={on ? `color:${c};background:color-mix(in srgb, ${c} 14%, transparent)` : ''}
	>
		<span class="inline-block h-1.5 w-1.5 rounded-full" style="background:{on ? c : 'var(--color-fg-faint)'}"></span>
		{label}
	</button>
{/snippet}

{#if title}
	<div class="border-line bg-ink-2/85 absolute {pos} left-3 z-10 w-[132px] overflow-hidden rounded-[var(--radius)] border backdrop-blur-md transition-[top] duration-200">
		<button
			onclick={() => (open = !open)}
			class="hover:bg-panel-2/60 flex w-full items-center gap-2 px-3 py-2 text-left transition-colors {open ? 'border-line/70 border-b' : ''}"
		>
			<span class="font-display text-fg text-xs font-700 tracking-wide">{title}</span>
			<svg viewBox="0 0 24 24" class="text-fg-faint ml-auto h-3.5 w-3.5 transition-transform {open ? '' : 'rotate-180'}" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9l6 6 6-6" /></svg>
		</button>
		{#if open}
			<div class="p-2">
				<div class="label mb-1.5">Node types</div>
				<div class="flex flex-col items-stretch gap-1">
					{#each ROLES as [key, label] (key)}
						{@render roleBtn(key, label)}
					{/each}
				</div>
				{#if children}
					<div class="border-line/60 mt-2 border-t pt-2">
						{@render children()}
					</div>
				{/if}
			</div>
		{/if}
	</div>
{:else}
	<div class="border-line bg-ink-2/85 absolute {pos} left-3 z-10 flex flex-wrap gap-1 rounded-[var(--radius)] border p-1 backdrop-blur-md transition-[top] duration-200">
		{#each ROLES as [key, label] (key)}
			{@render roleBtn(key, label)}
		{/each}
	</div>
{/if}
