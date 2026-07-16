<!--
  Notes on a node. Body-only (no chrome) — it's rendered inside the Node Admin
  modal, which supplies the title + close. Three visibilities:
    • Public  — everyone
    • Private — only the author
    • Team    — the node's owner + shared-with users (shown only if canTeam)
-->
<script lang="ts">
	import { notes, type Note, type NoteVisibility } from '$lib/api';
	import { auth } from '$lib/auth.svelte';
	import { ago } from '$lib/format';

	interface Props {
		pubkey: string;
		/** Called after the list changes (post/edit/delete) so a parent can refresh. */
		onchanged?: () => void;
	}
	let { pubkey, onchanged }: Props = $props();

	let list = $state<Note[]>([]);
	let canTeam = $state(false);
	let loading = $state(true);
	let error = $state('');

	// New-note composer.
	let body = $state('');
	let visibility = $state<NoteVisibility>('public');
	let posting = $state(false);

	// Inline edit state.
	let editingId = $state<number | null>(null);
	let editBody = $state('');
	let editVis = $state<NoteVisibility>('public');

	const visMeta: Record<NoteVisibility, { label: string; hint: string; badge: string }> = {
		public: { label: 'Public', hint: 'Visible to everyone', badge: '' },
		private: { label: 'Private', hint: 'Only you can see this', badge: 'bg-amber/15 text-amber' },
		team: { label: 'Team', hint: 'Owner + shared-with users', badge: 'bg-signal/15 text-signal' }
	};
	const visOrder = $derived<NoteVisibility[]>(
		canTeam ? ['public', 'private', 'team'] : ['public', 'private']
	);

	let loadedFor = $state('');
	$effect(() => {
		if (pubkey && pubkey !== loadedFor) {
			loadedFor = pubkey;
			load();
		}
	});

	async function load() {
		loading = true;
		try {
			const res = await notes.list(pubkey);
			list = res.notes;
			canTeam = res.canTeam;
			error = '';
		} catch (e) {
			error = String((e as Error).message ?? e);
		} finally {
			loading = false;
		}
	}

	async function post() {
		if (!body.trim()) return;
		posting = true;
		error = '';
		try {
			await notes.create(auth.csrf, pubkey, body.trim(), visibility);
			body = '';
			visibility = 'public';
			await load();
			onchanged?.();
		} catch (e) {
			error = String((e as Error).message ?? e);
		} finally {
			posting = false;
		}
	}

	function startEdit(n: Note) {
		editingId = n.id;
		editBody = n.body;
		editVis = n.visibility;
	}

	async function saveEdit(n: Note) {
		if (!editBody.trim()) return;
		try {
			await notes.update(auth.csrf, n.id, editBody.trim(), editVis);
			editingId = null;
			await load();
			onchanged?.();
		} catch (e) {
			error = String((e as Error).message ?? e);
		}
	}

	async function remove(n: Note) {
		try {
			await notes.remove(auth.csrf, n.id);
			await load();
			onchanged?.();
		} catch (e) {
			error = String((e as Error).message ?? e);
		}
	}
</script>

<!-- Composer -->
{#if auth.loggedIn}
	<div class="border-line/70 mb-4 rounded-[var(--radius)] border p-3">
		<textarea
			bind:value={body}
			rows="3"
			maxlength="4000"
			placeholder="Add a note about this node…"
			class="bg-ink-2 text-fg focus:border-signal w-full resize-y rounded-[var(--radius)] border border-transparent px-3 py-2 text-sm outline-none"
		></textarea>
		<div class="mt-2 flex flex-wrap items-center gap-2">
			<div class="border-line/70 flex gap-0.5 rounded-full border p-0.5 text-xs">
				{#each visOrder as v (v)}
					<button
						onclick={() => (visibility = v)}
						class="rounded-full px-2.5 py-1 font-600 transition-colors {visibility === v
							? v === 'public'
								? 'bg-signal/15 text-signal'
								: v === 'private'
									? 'bg-amber/15 text-amber'
									: 'bg-signal/15 text-signal'
							: 'text-fg-dim'}">{visMeta[v].label}</button
					>
				{/each}
			</div>
			<span class="text-fg-faint text-xs">{visMeta[visibility].hint}</span>
			<button
				onclick={post}
				disabled={posting || !body.trim()}
				class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 ml-auto rounded-[var(--radius)] border px-3.5 py-1.5 text-sm font-600 transition-colors disabled:opacity-40"
				>{posting ? 'Posting…' : 'Post'}</button
			>
		</div>
	</div>
{:else}
	<p class="text-fg-dim mb-4 text-sm">
		<a href="/login" class="text-signal hover:underline">Sign in</a> to add notes.
	</p>
{/if}

{#if error}<p class="text-coral mb-3 text-xs">{error}</p>{/if}

<!-- List -->
{#if loading}
	<p class="text-fg-faint text-sm">Loading…</p>
{:else if list.length === 0}
	<p class="text-fg-faint text-sm">No notes yet.</p>
{:else}
	<div class="space-y-3">
		{#each list as n (n.id)}
			<div class="border-line/60 rounded-[var(--radius)] border px-3.5 py-3">
				<div class="mb-1.5 flex items-center gap-2">
					<span class="text-fg text-xs font-600">{n.authorName}</span>
					{#if n.visibility !== 'public'}
						<span
							class="rounded-full px-2 py-0.5 text-[0.6rem] font-600 {visMeta[n.visibility].badge}"
							>{visMeta[n.visibility].label}</span
						>
					{/if}
					<span class="text-fg-faint ml-auto text-[0.68rem]">{ago(n.createdAt)}</span>
				</div>
				{#if editingId === n.id}
					<textarea
						bind:value={editBody}
						rows="3"
						maxlength="4000"
						class="bg-ink-2 text-fg focus:border-signal w-full resize-y rounded-[var(--radius)] border border-transparent px-3 py-2 text-sm outline-none"
					></textarea>
					<div class="mt-2 flex items-center gap-2">
						<div class="border-line/70 flex gap-0.5 rounded-full border p-0.5 text-xs">
							{#each visOrder as v (v)}
								<button
									onclick={() => (editVis = v)}
									class="rounded-full px-2 py-0.5 {editVis === v
										? v === 'private'
											? 'bg-amber/15 text-amber'
											: 'bg-signal/15 text-signal'
										: 'text-fg-dim'}">{visMeta[v].label}</button
								>
							{/each}
						</div>
						<button onclick={() => saveEdit(n)} class="text-signal ml-auto text-xs font-600">Save</button
						>
						<button onclick={() => (editingId = null)} class="text-fg-faint text-xs">Cancel</button>
					</div>
				{:else}
					<p class="text-fg-dim text-sm leading-relaxed whitespace-pre-wrap">{n.body}</p>
					{#if n.mine}
						<div class="mt-2 flex gap-3">
							{#if n.userId === auth.user?.id}
								<button onclick={() => startEdit(n)} class="text-fg-faint hover:text-fg text-xs"
									>Edit</button
								>
							{/if}
							<button onclick={() => remove(n)} class="text-fg-faint hover:text-coral text-xs"
								>Delete</button
							>
						</div>
					{/if}
				{/if}
			</div>
		{/each}
	</div>
{/if}
