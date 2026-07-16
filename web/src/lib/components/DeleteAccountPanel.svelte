<script lang="ts">
	// "Danger zone" — permanent account deletion, shared by the desktop and mobile
	// account pages. Deleting requires re-entering the password and an explicit
	// confirmation; the server releases every node the user owned and marks each
	// "previously owned by <their name>". The protected owner can't delete their
	// account, so the panel hides itself for them. `home` is where to send the
	// now-signed-out browser afterwards ('/' desktop, '/m' mobile).
	import { goto } from '$app/navigation';
	import { auth } from '$lib/auth.svelte';
	import { confirmer } from '$lib/confirm.svelte';

	let { home = '/', compact = false }: { home?: string; compact?: boolean } = $props();
	const pad = $derived(compact ? 'px-4 py-4' : 'px-5 py-5');

	let open = $state(false);
	let password = $state('');
	let busy = $state(false);
	let error = $state('');

	async function remove() {
		error = '';
		if (!password) {
			error = 'Enter your password to confirm.';
			return;
		}
		const ok = await confirmer.ask({
			title: 'Delete your account?',
			message:
				'This permanently deletes your account, notes, and private locations. Any nodes you own will be released (and shown as "previously owned by you"). This cannot be undone.',
			confirmLabel: 'Delete account',
			cancelLabel: 'Keep account',
			danger: true
		});
		if (!ok) return;
		busy = true;
		try {
			await auth.deleteAccount(password);
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
			{#if !open}
				<button
					onclick={() => {
						open = true;
						error = '';
					}}
					class="border-coral/40 text-coral hover:bg-coral/10 ml-auto rounded-[var(--radius)] border px-3 py-1.5 text-xs font-600 transition-colors"
					>Delete…</button
				>
			{/if}
		</div>
		<p class="text-fg-faint mt-1.5 text-xs leading-relaxed">
			Permanently removes your account, notes, and private locations. Nodes you own are released and
			marked <span class="text-fg-dim">“previously owned by you”</span>. This can't be undone.
		</p>

		{#if open}
			<div class="mt-3 flex flex-col gap-2">
				<input
					type="password"
					bind:value={password}
					placeholder="current password"
					autocomplete="current-password"
					class="border-line bg-ink-2 text-fg focus:border-coral w-full rounded-[var(--radius)] border px-3 py-2 text-sm outline-none"
				/>
				<div class="flex items-center gap-3">
					<button
						onclick={remove}
						disabled={busy || !password}
						class="bg-coral/15 text-coral border-coral/40 hover:bg-coral/25 rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors disabled:opacity-50"
						>{busy ? 'Deleting…' : 'Permanently delete'}</button
					>
					<button
						onclick={() => {
							open = false;
							password = '';
							error = '';
						}}
						class="text-fg-faint hover:text-fg text-xs transition-colors">Cancel</button
					>
				</div>
			</div>
		{/if}
		{#if error}<p class="text-coral mt-2 text-xs">{error}</p>{/if}
	</div>
{/if}
