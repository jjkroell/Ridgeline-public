<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { auth } from '$lib/auth.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';

	let token = $state('');
	let password = $state('');
	let confirm = $state('');
	let error = $state('');
	let busy = $state(false);
	let phase = $state<'form' | 'ok' | 'missing'>('form');

	onMount(() => {
		token = page.url.searchParams.get('token') ?? '';
		if (!token) phase = 'missing';
	});

	const canSubmit = $derived(
		password.length >= 8 && password === confirm && !busy
	);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		if (password.length < 8) {
			error = 'Password must be at least 8 characters.';
			return;
		}
		if (password !== confirm) {
			error = 'The passwords don’t match.';
			return;
		}
		busy = true;
		try {
			await auth.resetPassword(token, password);
			phase = 'ok';
			// Reset also signs you in — send you into the app shortly.
			setTimeout(() => goto('/account'), 1500);
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Reset password · Ridgeline</title></svelte:head>

<PageHeader eyebrow="Accounts" title="Choose a new password" />

<div class="px-6 pb-16 md:px-10">
	<div class="panel mx-auto max-w-md px-6 py-8">
		{#if phase === 'missing'}
			<div class="text-center">
				<div
					class="bg-coral/15 text-coral mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full"
				>
					<svg viewBox="0 0 24 24" class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="2"
						><path d="M6 6l12 12M18 6L6 18" stroke-linecap="round" /></svg
					>
				</div>
				<h2 class="text-fg text-lg font-700">Missing reset token</h2>
				<p class="text-fg-dim mt-2 text-sm leading-relaxed">
					This link is missing its reset token. Request a new one from the sign-in page.
				</p>
				<a href="/login" class="text-signal mt-4 inline-block text-sm hover:underline"
					>Back to sign in</a
				>
			</div>
		{:else if phase === 'ok'}
			<div class="text-center">
				<div
					class="bg-signal/15 text-signal mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full"
				>
					<svg viewBox="0 0 24 24" class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="2"
						><path d="M5 13l4 4 10-11" stroke-linecap="round" stroke-linejoin="round" /></svg
					>
				</div>
				<h2 class="text-fg text-lg font-700">Password updated</h2>
				<p class="text-fg-dim mt-2 text-sm">You're signed in with your new password. Taking you in…</p>
				<a href="/account" class="text-signal mt-4 inline-block text-sm hover:underline">Continue →</a>
			</div>
		{:else}
			<p class="text-fg-dim mb-5 text-sm leading-relaxed">
				Enter a new password for your account. You'll be signed in once it's saved.
			</p>
			<form onsubmit={submit} class="flex flex-col gap-4">
				<label class="block">
					<span class="label normal-case text-fg-dim mb-1.5 block">New password</span>
					<input
						type="password"
						bind:value={password}
						required
						minlength="8"
						autocomplete="new-password"
						placeholder="at least 8 characters"
						class="border-line bg-ink-2 text-fg focus:border-signal w-full rounded-[var(--radius)] border px-3 py-2 text-sm outline-none"
					/>
				</label>
				<label class="block">
					<span class="label normal-case text-fg-dim mb-1.5 block">Confirm new password</span>
					<input
						type="password"
						bind:value={confirm}
						required
						autocomplete="new-password"
						placeholder="re-enter it"
						class="border-line bg-ink-2 text-fg focus:border-signal w-full rounded-[var(--radius)] border px-3 py-2 text-sm outline-none"
					/>
				</label>
				{#if confirm.length > 0 && password !== confirm}
					<p class="text-coral text-xs">The passwords don't match.</p>
				{/if}
				{#if error}
					<p class="text-coral text-xs">{error}</p>
				{/if}
				<button
					type="submit"
					disabled={!canSubmit}
					class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 mt-1 w-full rounded-[var(--radius)] border px-4 py-2.5 text-sm font-600 transition-colors disabled:opacity-50"
				>
					{busy ? 'Saving…' : 'Save new password'}
				</button>
			</form>
			<p class="text-fg-faint mt-6 text-center text-xs">
				Link expired? <a href="/login" class="text-signal hover:underline">Request a new one</a>.
			</p>
		{/if}
	</div>
</div>
