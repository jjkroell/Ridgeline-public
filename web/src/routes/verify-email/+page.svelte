<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { auth } from '$lib/auth.svelte';
	import PageHeader from '$lib/components/PageHeader.svelte';

	let phase = $state<'working' | 'ok' | 'error'>('working');
	let message = $state('');

	onMount(async () => {
		const token = page.url.searchParams.get('token') ?? '';
		if (!token) {
			phase = 'error';
			message = 'This link is missing its verification token.';
			return;
		}
		try {
			await auth.verifyEmail(token);
			phase = 'ok';
			// Verified + logged in — send them into the app shortly.
			setTimeout(() => goto('/account'), 1500);
		} catch (err) {
			phase = 'error';
			message = String((err as Error).message ?? err);
		}
	});
</script>

<svelte:head><title>Verify email · Ridgeline</title></svelte:head>

<PageHeader eyebrow="Accounts" title="Email verification" />

<div class="px-6 pb-16 md:px-10">
	<div class="panel mx-auto max-w-md px-6 py-10 text-center">
		{#if phase === 'working'}
			<p class="text-fg-dim text-sm">Confirming your email…</p>
		{:else if phase === 'ok'}
			<div
				class="bg-signal/15 text-signal mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full"
			>
				<svg viewBox="0 0 24 24" class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="2"
					><path d="M5 13l4 4 10-11" stroke-linecap="round" stroke-linejoin="round" /></svg
				>
			</div>
			<h2 class="text-fg text-lg font-700">You're verified</h2>
			<p class="text-fg-dim mt-2 text-sm">Your account is active. Taking you in…</p>
			<a href="/account" class="text-signal mt-4 inline-block text-sm hover:underline">Continue →</a>
		{:else}
			<div
				class="bg-coral/15 text-coral mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full"
			>
				<svg viewBox="0 0 24 24" class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="2"
					><path d="M6 6l12 12M18 6L6 18" stroke-linecap="round" /></svg
				>
			</div>
			<h2 class="text-fg text-lg font-700">Verification failed</h2>
			<p class="text-fg-dim mt-2 text-sm leading-relaxed">{message}</p>
			<a href="/login" class="text-signal mt-4 inline-block text-sm hover:underline"
				>Back to sign in</a
			>
		{/if}
	</div>
</div>
