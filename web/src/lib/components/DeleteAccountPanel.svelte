<script lang="ts">
	// "Danger zone" — permanent account deletion, shared by the desktop and mobile
	// account pages. The "Delete…" button opens an obvious warning modal that
	// spells out exactly what is removed and what remains, and gates the delete
	// button behind BOTH re-typing the account's registered email (a deliberate
	// friction step) and re-entering the password (the server's re-auth). The
	// server releases every node the user owned and marks each "previously owned
	// by <their name>". The protected owner can't delete, so the panel hides for
	// them. `home` is where to send the now-signed-out browser afterwards.
	import { goto } from '$app/navigation';
	import { auth } from '$lib/auth.svelte';
	import { confirmer } from '$lib/confirm.svelte';
	import Modal from './Modal.svelte';

	let { home = '/', compact = false }: { home?: string; compact?: boolean } = $props();
	const pad = $derived(compact ? 'px-4 py-4' : 'px-5 py-5');

	let showModal = $state(false);
	let emailConfirm = $state('');
	let password = $state('');
	let busy = $state(false);
	let error = $state('');

	const registeredEmail = $derived((auth.user?.email ?? '').trim());
	// Case-insensitive match on the registered email (a confirmation gate, not auth
	// — the password is the real credential). Empty input never matches.
	const emailMatches = $derived(
		registeredEmail !== '' &&
			emailConfirm.trim().toLowerCase() === registeredEmail.toLowerCase()
	);
	const canDelete = $derived(emailMatches && password.length > 0 && !busy);

	function openModal() {
		emailConfirm = '';
		password = '';
		error = '';
		showModal = true;
	}

	function closeModal() {
		if (busy) return;
		showModal = false;
	}

	async function remove() {
		if (!canDelete) return;
		error = '';
		busy = true;
		try {
			await auth.deleteAccount(password);
			showModal = false;
			await confirmer.tell({ title: 'Account deleted', message: 'Your account has been removed.' });
			goto(home);
		} catch (e) {
			error = String((e as Error).message ?? e);
		} finally {
			busy = false;
		}
	}
</script>

{#if !auth.user?.isOwner}
	<div class="border-coral/30 rounded-[var(--radius)] border {pad}">
		<div class="flex flex-wrap items-center gap-2">
			<span class="font-display text-coral text-sm font-700">Delete account</span>
			<button
				onclick={openModal}
				class="border-coral/40 text-coral hover:bg-coral/10 ml-auto rounded-[var(--radius)] border px-3 py-1.5 text-xs font-600 transition-colors"
				>Delete…</button
			>
		</div>
		<p class="text-fg-faint mt-1.5 text-xs leading-relaxed">
			Permanently removes your account, notes, and private locations. Nodes you own are released and
			marked <span class="text-fg-dim">“previously owned by you”</span>. This can't be undone.
		</p>
	</div>
{/if}

{#if showModal}
	<Modal onclose={closeModal}>
		<div class="px-6 py-5">
			<div class="flex items-center gap-2.5">
				<span
					class="border-coral/40 bg-coral/15 text-coral flex h-8 w-8 shrink-0 items-center justify-center rounded-full border text-lg font-700"
					aria-hidden="true">!</span
				>
				<h2 class="font-display text-coral text-lg font-700">Delete your account</h2>
			</div>
			<p class="text-fg-dim mt-3 text-sm leading-relaxed">
				This is permanent and <strong class="text-fg">cannot be undone</strong>. Please read what
				happens before continuing.
			</p>

			<div class="border-coral/30 bg-coral/5 mt-4 rounded-[var(--radius)] border px-4 py-3">
				<div class="label text-coral mb-2 normal-case">Permanently removed</div>
				<ul class="text-fg-dim list-disc space-y-1 pl-4 text-sm leading-relaxed">
					<li>Your account and login</li>
					<li>Every note you've written</li>
					<li>Your private exact node locations</li>
					<li>Locations you've shared with other members</li>
					<li>Your active sessions — you'll be signed out everywhere</li>
				</ul>
			</div>

			<div class="border-line/70 bg-ink-2/40 mt-3 rounded-[var(--radius)] border px-4 py-3">
				<div class="label text-fg-dim mb-2 normal-case">What remains</div>
				<p class="text-fg-dim text-sm leading-relaxed">
					Nodes you own are <strong class="text-fg">released, not deleted</strong> — they stay public
					and are marked
					<span class="text-fg">“previously owned by {auth.user?.displayName || 'you'}”</span>
					until someone else claims them.
				</p>
			</div>

			<div class="mt-4 flex flex-col gap-2">
				<label for="del-email" class="text-fg-dim text-xs leading-relaxed">
					To confirm, type your email
					<span class="text-fg font-mono">{registeredEmail}</span>
				</label>
				<input
					id="del-email"
					type="email"
					bind:value={emailConfirm}
					placeholder="your email"
					autocomplete="off"
					autocapitalize="off"
					spellcheck="false"
					class="border-line bg-ink-2 text-fg focus:border-coral w-full rounded-[var(--radius)] border px-3 py-2 text-sm outline-none"
				/>
				{#if emailConfirm.trim() !== '' && !emailMatches}
					<p class="text-coral text-xs">That doesn't match your registered email.</p>
				{/if}
				<input
					type="password"
					bind:value={password}
					placeholder="current password"
					autocomplete="current-password"
					class="border-line bg-ink-2 text-fg focus:border-coral mt-1 w-full rounded-[var(--radius)] border px-3 py-2 text-sm outline-none"
				/>
			</div>

			{#if error}<p class="text-coral mt-2 text-xs">{error}</p>{/if}
		</div>
		<div class="border-line/70 flex items-center justify-end gap-3 border-t px-6 py-4">
			<button
				onclick={closeModal}
				disabled={busy}
				class="text-fg-dim hover:text-fg rounded-[var(--radius)] px-4 py-2 text-sm font-600 transition-colors disabled:opacity-50"
				>Keep account</button
			>
			<button
				onclick={remove}
				disabled={!canDelete}
				class="border-coral/40 bg-coral/15 text-coral hover:bg-coral/25 rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors disabled:opacity-50"
				>{busy ? 'Deleting…' : 'Permanently delete'}</button
			>
		</div>
	</Modal>
{/if}
