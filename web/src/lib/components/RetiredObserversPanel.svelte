<script lang="ts">
	// Retired (decommissioned) observers — withdrawn from the observers page while
	// every packet they reported is kept and stays attributed to them. Shared by
	// the desktop (/admin) and mobile (/m/admin) consoles so the two stay in
	// parity. Self-loading, like MembersPanel; `compact` switches to mobile chrome.
	//
	// The panel renders nothing at all when no observer is retired, so it stays out
	// of the way on a healthy install.
	import { auth } from '$lib/auth.svelte';
	import { admin, type Observer } from '$lib/api';
	import { ago } from '$lib/format';

	let { compact = false }: { compact?: boolean } = $props();

	const shell = $derived(
		compact
			? 'border-line/60 bg-panel mb-3 rounded-2xl border overflow-hidden'
			: 'panel rise mt-6 overflow-hidden'
	);
	const head = $derived(compact ? 'px-4 py-3' : 'px-5 py-3.5');
	const row = $derived(compact ? 'px-4 py-3' : 'px-5 py-3');

	let retired = $state<Observer[]>([]);
	let loading = $state(false);
	let error = $state('');
	let busyId = $state<string | null>(null);

	async function load() {
		if (!auth.isAdmin) return;
		loading = true;
		error = '';
		try {
			retired = await admin.retiredObservers();
		} catch (e) {
			error = String((e as Error).message ?? e);
		} finally {
			loading = false;
		}
	}

	async function unretire(o: Observer) {
		busyId = o.id;
		error = '';
		try {
			await admin.unretireObserver(auth.csrf, o.id);
			await load();
		} catch (e) {
			error = String((e as Error).message ?? e);
		} finally {
			busyId = null;
		}
	}

	// Load once when admin status is confirmed — same `loaded` guard as
	// MembersPanel: gating on retired.length would loop forever when empty.
	let loaded = $state(false);
	$effect(() => {
		if (auth.isAdmin && !loaded) {
			loaded = true;
			load();
		}
	});
</script>

{#if retired.length > 0 || error}
	<section class={shell}>
		<div class="border-line/70 flex items-center gap-2.5 border-b {head}">
			<h2 class="font-display text-fg font-700 tracking-wide {compact ? 'text-xs' : 'text-sm'}">
				RETIRED OBSERVERS
			</h2>
			<span class="label normal-case text-fg-faint">{retired.length} hidden</span>
			<button
				onclick={load}
				class="label hover:text-signal ml-auto transition-colors"
				disabled={loading}>{loading ? 'Loading…' : 'Refresh'}</button
			>
		</div>
		<p class="text-fg-dim border-line/70 border-b py-2.5 text-xs {compact ? 'px-4' : 'px-5'}">
			Hidden from the observers page. Every packet they reported is kept and still counts.
		</p>
		{#if error}
			<div class="text-coral py-3 text-xs {compact ? 'px-4' : 'px-5'}">{error}</div>
		{/if}
		<div class="divide-line/60 divide-y">
			{#each retired as o (o.id)}
				<div class="flex items-center gap-3 {row}">
					<div class="min-w-0 flex-1">
						<div class="text-fg truncate text-sm font-600">{o.id}</div>
						<div class="label normal-case text-fg-faint mt-0.5 truncate">
							{o.packetCount.toLocaleString()} packets · last heard {ago(o.lastSeen)}{o.retiredAt
								? ` · retired ${ago(o.retiredAt)}`
								: ''}
						</div>
					</div>
					<button
						onclick={() => unretire(o)}
						disabled={busyId === o.id}
						class="border-signal/40 text-signal hover:bg-signal/15 shrink-0 rounded-[var(--radius)] border px-3 py-1 text-xs font-600 transition-colors disabled:opacity-50"
						>{busyId === o.id ? 'Restoring…' : 'Un-retire'}</button
					>
				</div>
			{/each}
		</div>
	</section>
{/if}
