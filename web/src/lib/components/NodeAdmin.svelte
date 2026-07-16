<!--
  Node Admin — one section that consolidates a node's owner/member tools behind
  buttons, each opening its own modal: Ownership (claim), Private location, and
  Notes. The buttons show a live summary; the modals host the existing panels.
-->
<script lang="ts">
	import { claims, notes, privateLocation, type ClaimStatus } from '$lib/api';
	import Modal from './Modal.svelte';
	import ClaimPanel from './ClaimPanel.svelte';
	import PrivateLocationPanel from './PrivateLocationPanel.svelte';
	import NotesPanel from './NotesPanel.svelte';

	interface Props {
		pubkey: string;
		seedLat?: number | null;
		seedLon?: number | null;
	}
	let { pubkey, seedLat = null, seedLon = null }: Props = $props();

	type Which = '' | 'claim' | 'location' | 'notes';
	let open = $state<Which>('');

	// Summaries shown on the buttons.
	let claim = $state<ClaimStatus | null>(null);
	let noteCount = $state(0);
	let locAccess = $state(false); // caller may view a private location (owner or shared)
	let locCanEdit = $state(false);
	let locSet = $state(false);
	let locLat = $state<number | null>(null);
	let locLon = $state<number | null>(null);

	let loadedFor = $state('');
	$effect(() => {
		if (pubkey && pubkey !== loadedFor) {
			loadedFor = pubkey;
			loadSummary();
		}
	});

	async function loadSummary() {
		try {
			claim = await claims.status(pubkey);
		} catch {
			claim = null;
		}
		try {
			noteCount = (await notes.list(pubkey)).notes.length;
		} catch {
			noteCount = 0;
		}
		try {
			const r = await privateLocation.get(pubkey);
			locAccess = true;
			locCanEdit = r.canEdit ?? false;
			locSet = r.set;
			locLat = r.location?.latitude ?? null;
			locLon = r.location?.longitude ?? null;
		} catch {
			locAccess = false;
		}
	}

	const ownershipSummary = $derived.by(() => {
		if (!claim) return '—';
		if (claim.ownedByMe) return 'You own this node';
		if (claim.owner) return `Claimed by ${claim.owner.displayName}`;
		if (claim.mine?.status === 'pending') return 'Your claim is pending';
		return 'Unclaimed';
	});
	const locationSummary = $derived.by(() => {
		if (!locCanEdit) return 'Shared with you';
		if (locSet && locLat != null && locLon != null)
			return `${locLat.toFixed(5)}, ${locLon.toFixed(5)}`;
		return 'Not set';
	});

	function close() {
		open = '';
		loadSummary();
	}
	const titles: Record<Exclude<Which, ''>, string> = {
		claim: 'Ownership',
		location: 'Private location',
		notes: 'Notes'
	};
</script>

<div class="panel px-5 py-4">
	<div class="label normal-case text-fg-dim mb-3 flex items-center gap-2">
		<svg
			viewBox="0 0 24 24"
			class="text-fg-faint h-4 w-4"
			fill="none"
			stroke="currentColor"
			stroke-width="1.6"
			stroke-linecap="round"
			stroke-linejoin="round"
			><path d="M12 2 4 5v6c0 5 3.4 8.5 8 11 4.6-2.5 8-6 8-11V5l-8-3z" /><path d="m9 12 2 2 4-4" /></svg
		>
		Node Admin
	</div>

	<div class="space-y-2">
		<!-- Ownership -->
		<button
			type="button"
			onclick={() => (open = 'claim')}
			class="border-line/60 hover:border-line hover:bg-panel-2/40 flex w-full items-center gap-3 rounded-[var(--radius)] border px-3.5 py-3 text-left transition-colors"
		>
			<svg
				viewBox="0 0 24 24"
				class="text-signal h-4 w-4 shrink-0"
				fill="none"
				stroke="currentColor"
				stroke-width="1.6"
				stroke-linecap="round"
				stroke-linejoin="round"
				><path d="M12 2 4 5v6c0 5 3.4 8.5 8 11 4.6-2.5 8-6 8-11V5l-8-3z" /><path
					d="m9 12 2 2 4-4"
				/></svg
			>
			<span class="min-w-0 flex-1">
				<span class="text-fg block text-sm font-600">Ownership</span>
				<span class="text-fg-faint block truncate text-xs">{ownershipSummary}</span>
			</span>
			<svg
				viewBox="0 0 24 24"
				class="text-fg-faint h-4 w-4 shrink-0"
				fill="none"
				stroke="currentColor"
				stroke-width="1.8"
				stroke-linecap="round"
				stroke-linejoin="round"><path d="M9 6l6 6-6 6" /></svg
			>
		</button>

		<!-- Private location (only when the caller can view it) -->
		{#if locAccess}
			<button
				type="button"
				onclick={() => (open = 'location')}
				class="border-line/60 hover:border-line hover:bg-panel-2/40 flex w-full items-center gap-3 rounded-[var(--radius)] border px-3.5 py-3 text-left transition-colors"
			>
				<svg
					viewBox="0 0 24 24"
					class="text-signal h-4 w-4 shrink-0"
					fill="none"
					stroke="currentColor"
					stroke-width="1.6"
					stroke-linecap="round"
					stroke-linejoin="round"
					><rect x="5" y="11" width="14" height="9" rx="1.5" /><path
						d="M8 11V8a4 4 0 0 1 8 0v3"
					/></svg
				>
				<span class="min-w-0 flex-1">
					<span class="text-fg block text-sm font-600">Private location</span>
					<span class="text-fg-faint block truncate text-xs">{locationSummary}</span>
				</span>
				<svg
					viewBox="0 0 24 24"
					class="text-fg-faint h-4 w-4 shrink-0"
					fill="none"
					stroke="currentColor"
					stroke-width="1.8"
					stroke-linecap="round"
					stroke-linejoin="round"><path d="M9 6l6 6-6 6" /></svg
				>
			</button>
		{/if}

		<!-- Notes -->
		<button
			type="button"
			onclick={() => (open = 'notes')}
			class="border-line/60 hover:border-line hover:bg-panel-2/40 flex w-full items-center gap-3 rounded-[var(--radius)] border px-3.5 py-3 text-left transition-colors"
		>
			<svg
				viewBox="0 0 24 24"
				class="text-signal h-4 w-4 shrink-0"
				fill="none"
				stroke="currentColor"
				stroke-width="1.6"
				stroke-linecap="round"
				stroke-linejoin="round"><path d="M4 4h16v12H7l-3 3zM8 9h8M8 12h5" /></svg
			>
			<span class="min-w-0 flex-1">
				<span class="text-fg block text-sm font-600">Notes</span>
				<span class="text-fg-faint block truncate text-xs"
					>{noteCount ? `${noteCount} note${noteCount === 1 ? '' : 's'}` : 'No notes yet'}</span
				>
			</span>
			<svg
				viewBox="0 0 24 24"
				class="text-fg-faint h-4 w-4 shrink-0"
				fill="none"
				stroke="currentColor"
				stroke-width="1.8"
				stroke-linecap="round"
				stroke-linejoin="round"><path d="M9 6l6 6-6 6" /></svg
			>
		</button>
	</div>
</div>

{#if open !== ''}
	<Modal onclose={close} size="2xl">
		<div class="border-line flex shrink-0 items-center justify-between border-b px-5 py-3.5">
			<h3 class="text-fg text-base font-700">{titles[open]}</h3>
			<button
				onclick={close}
				aria-label="Close"
				class="text-fg-faint hover:text-fg -mr-1 rounded-md p-1 transition-colors"
			>
				<svg
					viewBox="0 0 24 24"
					class="h-5 w-5"
					fill="none"
					stroke="currentColor"
					stroke-width="1.8"
					stroke-linecap="round"
					stroke-linejoin="round"><path d="M6 6l12 12M18 6L6 18" /></svg
				>
			</button>
		</div>
		<div class="overflow-y-auto px-5 py-4">
			{#if open === 'claim'}
				<ClaimPanel {pubkey} onchanged={loadSummary} />
			{:else if open === 'location'}
				<PrivateLocationPanel {pubkey} {seedLat} {seedLon} />
			{:else if open === 'notes'}
				<NotesPanel {pubkey} onchanged={loadSummary} />
			{/if}
		</div>
	</Modal>
{/if}
