<script lang="ts">
	// Owner-only node lifecycle: retire (reversible) and scrub (permanent).
	//
	// Only rendered when the caller is the node's VERIFIED owner. The two actions
	// are deliberately unequal in weight: retire is one click, scrub demands the
	// node's name typed back and spells out both of its non-obvious consequences —
	// it releases the claim, and it deletes observations that other operators'
	// receivers recorded.
	import { nodeLifecycle, type ScrubResult } from '$lib/api';
	import { auth } from '$lib/auth.svelte';
	import { goto } from '$app/navigation';

	interface Props {
		pubkey: string;
		nodeName: string;
		/** Mobile (/m) variant — used to route the exit link correctly. */
		compact?: boolean;
		/** Whether the node is currently retired, from the node record. */
		retired?: boolean;
		/** Called after a successful action so the parent can refresh. */
		onchange?: (state: 'retired' | 'active' | 'scrubbed') => void;
	}
	let { pubkey, nodeName, compact = false, retired = false, onchange }: Props = $props();

	let busy = $state(false);
	let err = $state('');
	let confirmScrub = $state(false);
	let typed = $state('');
	let result = $state<ScrubResult | null>(null);

	// Require the node's name typed back before the destructive action unlocks.
	// Falls back to the key prefix for an unnamed node so it is never impossible.
	const expected = $derived((nodeName || pubkey.slice(0, 8)).trim());
	const canScrub = $derived(typed.trim().toLowerCase() === expected.toLowerCase());

	async function run(fn: () => Promise<unknown>, then: 'retired' | 'active' | 'scrubbed') {
		if (busy) return;
		busy = true;
		err = '';
		try {
			const r = await fn();
			if (then === 'scrubbed') result = r as ScrubResult;
			onchange?.(then);
		} catch (e) {
			err = e instanceof Error ? e.message : 'Something went wrong';
		} finally {
			busy = false;
		}
	}
</script>

<div class="space-y-4">
	{#if result}
		<div class="border-line bg-ink/50 rounded-[var(--radius)] border p-3 text-sm">
			<div class="label mb-2">Removed</div>
			<ul class="text-fg-dim tnum space-y-1 font-mono text-xs">
				<li>{result.observations} observation{result.observations === 1 ? '' : 's'}</li>
				<li>{result.notes} note{result.notes === 1 ? '' : 's'}</li>
				<li>{result.claims} claim{result.claims === 1 ? '' : 's'} (including yours)</li>
				<li>{result.locations} private location{result.locations === 1 ? '' : 's'}</li>
			</ul>
			<p class="text-fg-faint mt-2 text-xs">
				If this node advertises again it will reappear as an unclaimed node, and anyone may claim
				it.
			</p>
			<!-- The page behind this modal is showing a node that no longer exists, so
			     offer the exit rather than leaving the reader on a stale view. -->
			<button
				onclick={() => goto(compact ? '/m/nodes' : '/nodes')}
				class="border-line text-fg-dim hover:border-line-bright hover:text-fg mt-3 rounded-[var(--radius)] border px-3 py-1.5 text-sm transition-colors"
				>Back to nodes</button
			>
		</div>
	{:else}
		<!-- Retire -->
		<div class="border-line rounded-[var(--radius)] border p-3">
			<div class="mb-1 flex items-center justify-between gap-3">
				<div class="label">{retired ? 'Retired' : 'Retire'}</div>
				{#if retired}
					<span class="text-amber border-amber/40 rounded-[var(--radius)] border px-1.5 py-0.5 text-[0.6rem] tracking-wide uppercase"
						>hidden</span
					>
				{/if}
			</div>
			<p class="text-fg-dim mb-3 text-xs leading-relaxed">
				{#if retired}
					This node is hidden from the map and node lists. Everything it sent is still recorded, and
					you still own it — putting it back is one click.
				{:else}
					Hides this node from the map and node lists while keeping every packet it sent. You keep
					ownership, and it stays hidden even if the node advertises again — so this is the right
					choice for a decommissioned node that is briefly still on air.
				{/if}
			</p>
			<button
				onclick={() =>
					retired
						? run(() => nodeLifecycle.unretire(auth.csrf, pubkey), 'active')
						: run(() => nodeLifecycle.retire(auth.csrf, pubkey), 'retired')}
				disabled={busy}
				class="border-line text-fg-dim hover:border-line-bright hover:text-fg rounded-[var(--radius)] border px-3 py-1.5 text-sm transition-colors disabled:opacity-50"
			>
				{busy ? 'Working…' : retired ? 'Put back on the map' : 'Retire this node'}
			</button>
		</div>

		<!-- Scrub -->
		<div class="border-coral/40 bg-coral/5 rounded-[var(--radius)] border p-3">
			<div class="label text-coral mb-1">Delete permanently</div>
			<p class="text-fg-dim mb-2 text-xs leading-relaxed">
				Erases this node and its recorded history. Two things worth knowing before you do:
			</p>
			<ul class="text-fg-dim mb-3 space-y-1.5 text-xs leading-relaxed">
				<li class="flex gap-2">
					<span class="text-coral shrink-0">·</span>
					<span
						>It <span class="text-fg">releases your claim</span>. If the node advertises again it
						comes back unclaimed, and anyone may claim it.</span
					>
				</li>
				<li class="flex gap-2">
					<span class="text-coral shrink-0">·</span>
					<span
						>It deletes <span class="text-fg">observations other operators' receivers recorded</span>
						of this node. Retire keeps those.</span
					>
				</li>
			</ul>

			{#if !confirmScrub}
				<button
					onclick={() => (confirmScrub = true)}
					class="border-coral/50 text-coral hover:bg-coral/10 rounded-[var(--radius)] border px-3 py-1.5 text-sm transition-colors"
					>Delete this node…</button
				>
			{:else}
				<label class="text-fg-faint mb-2 block text-xs" for="scrub-confirm">
					Type <span class="text-fg font-mono">{expected}</span> to confirm
				</label>
				<div class="flex flex-wrap gap-2">
					<input
						id="scrub-confirm"
						bind:value={typed}
						autocomplete="off"
						class="border-line bg-ink text-fg min-w-0 flex-1 rounded-[var(--radius)] border px-2 py-1.5 font-mono text-sm"
					/>
					<button
						onclick={() => run(() => nodeLifecycle.scrub(auth.csrf, pubkey), 'scrubbed')}
						disabled={!canScrub || busy}
						class="bg-coral/15 text-coral border-coral/50 rounded-[var(--radius)] border px-3 py-1.5 text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-40"
						>{busy ? 'Deleting…' : 'Delete permanently'}</button
					>
					<button
						onclick={() => {
							confirmScrub = false;
							typed = '';
						}}
						class="text-fg-faint hover:text-fg px-2 py-1.5 text-sm">Cancel</button
					>
				</div>
			{/if}
		</div>
	{/if}

	{#if err}
		<div class="text-coral text-xs">{err}</div>
	{/if}
</div>
