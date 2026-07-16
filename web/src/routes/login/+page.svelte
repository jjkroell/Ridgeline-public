<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/auth.svelte';
	import { authApi, AuthError } from '$lib/api';
	import PageHeader from '$lib/components/PageHeader.svelte';

	let mode = $state<'login' | 'register'>('login');
	let email = $state('');
	let password = $state('');
	let displayName = $state('');
	let error = $state('');
	let busy = $state(false);
	// Set to the address a confirmation link was sent to — shows the "check your
	// email" panel in place of the form.
	let sentTo = $state('');
	// True when a sign-in was refused because the address isn't confirmed yet.
	let unverified = $state(false);
	let resent = $state(false);

	// Already signed in → send them to their account.
	$effect(() => {
		if (auth.ready && auth.loggedIn) goto('/account');
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		unverified = false;
		busy = true;
		try {
			if (mode === 'register') {
				const r = await auth.register(email.trim(), password, displayName.trim());
				if (r.verificationSent) {
					sentTo = r.email ?? email.trim();
					return;
				}
				goto('/account'); // owner / email-disabled: logged straight in
			} else {
				await auth.login(email.trim(), password);
				goto('/account');
			}
		} catch (err) {
			if (err instanceof AuthError && err.unverified) {
				unverified = true;
			} else {
				error = String((err as Error).message ?? err);
			}
		} finally {
			busy = false;
		}
	}

	async function resend() {
		busy = true;
		try {
			await authApi.resendVerification(email.trim());
			resent = true;
			if (!sentTo) sentTo = email.trim();
		} finally {
			busy = false;
		}
	}

	function swap(to: 'login' | 'register') {
		mode = to;
		error = '';
		unverified = false;
	}
</script>

<svelte:head><title>Sign in · Ridgeline</title></svelte:head>

<PageHeader eyebrow="Accounts" title={mode === 'login' ? 'Sign in' : 'Create account'} />

<div class="px-6 pb-16 md:px-10">
	{#if sentTo}
		<!-- Verification email sent -->
		<div class="panel mx-auto max-w-md px-6 py-8 text-center">
			<div
				class="bg-signal/15 text-signal mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full"
			>
				<svg viewBox="0 0 24 24" class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="1.8"
					><path
						d="M4 6h16v12H4zM4 7l8 6 8-6"
						stroke-linecap="round"
						stroke-linejoin="round"
					/></svg
				>
			</div>
			<h2 class="text-fg text-lg font-700">Check your email</h2>
			<p class="text-fg-dim mt-2 text-sm leading-relaxed">
				We sent a confirmation link to <strong class="text-fg break-all">{sentTo}</strong>. Click it
				to activate your account, then sign in. The link expires in 24 hours.
			</p>
			<p class="text-fg-faint mt-4 text-xs">
				Didn't get it? Check spam, or
				<button onclick={resend} disabled={busy} class="text-signal hover:underline disabled:opacity-50"
					>resend it</button
				>{#if resent}<span class="text-signal"> — sent.</span>{/if}
			</p>
			<button
				onclick={() => {
					sentTo = '';
					resent = false;
					swap('login');
				}}
				class="text-fg-faint hover:text-fg mt-6 text-xs">← Back to sign in</button
			>
		</div>
	{:else}
	<div class="panel mx-auto max-w-md px-6 py-8">
		<!-- Mode toggle -->
		<div class="border-line/70 mb-6 flex gap-1 rounded-[var(--radius)] border p-1">
			<button
				onclick={() => swap('login')}
				class="flex-1 rounded-[calc(var(--radius)-2px)] px-3 py-1.5 text-sm font-600 transition-colors
					{mode === 'login' ? 'bg-signal/15 text-signal' : 'text-fg-dim hover:text-fg'}"
			>
				Sign in
			</button>
			<button
				onclick={() => swap('register')}
				class="flex-1 rounded-[calc(var(--radius)-2px)] px-3 py-1.5 text-sm font-600 transition-colors
					{mode === 'register' ? 'bg-signal/15 text-signal' : 'text-fg-dim hover:text-fg'}"
			>
				Create account
			</button>
		</div>

		<form onsubmit={submit} class="flex flex-col gap-4">
			{#if mode === 'register'}
				<label class="block">
					<span class="label normal-case text-fg-dim mb-1.5 block">Display name</span>
					<input
						type="text"
						bind:value={displayName}
						placeholder="Your name or handle"
						maxlength="64"
						autocomplete="nickname"
						class="border-line bg-ink-2 text-fg focus:border-signal w-full rounded-[var(--radius)] border px-3 py-2 text-sm outline-none"
					/>
					<span class="text-fg-faint mt-1 block text-xs">Shown on your public notes. Optional.</span>
				</label>
			{/if}
			<label class="block">
				<span class="label normal-case text-fg-dim mb-1.5 block">Email</span>
				<input
					type="email"
					bind:value={email}
					required
					autocomplete="email"
					placeholder="you@example.com"
					class="border-line bg-ink-2 text-fg focus:border-signal w-full rounded-[var(--radius)] border px-3 py-2 text-sm outline-none"
				/>
			</label>
			<label class="block">
				<span class="label normal-case text-fg-dim mb-1.5 block">Password</span>
				<input
					type="password"
					bind:value={password}
					required
					minlength={mode === 'register' ? 8 : undefined}
					autocomplete={mode === 'register' ? 'new-password' : 'current-password'}
					placeholder={mode === 'register' ? 'at least 8 characters' : '••••••••'}
					class="border-line bg-ink-2 text-fg focus:border-signal w-full rounded-[var(--radius)] border px-3 py-2 text-sm outline-none"
				/>
			</label>

			{#if error}
				<p class="text-coral text-xs">{error}</p>
			{/if}
			{#if unverified}
				<div class="border-amber/40 bg-amber/10 rounded-[var(--radius)] border px-3 py-2.5">
					<p class="text-fg-dim text-xs leading-relaxed">
						<strong class="text-amber">Confirm your email first.</strong> We need you to click the
						verification link we emailed you before you can sign in.
						<button
							type="button"
							onclick={resend}
							disabled={busy}
							class="text-signal hover:underline disabled:opacity-50">Resend the link</button
						>{#if resent}<span class="text-signal"> — sent.</span>{/if}
					</p>
				</div>
			{/if}

			<button
				type="submit"
				disabled={busy}
				class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 mt-1 w-full rounded-[var(--radius)] border px-4 py-2.5 text-sm font-600 transition-colors disabled:opacity-50"
			>
				{busy ? 'Working…' : mode === 'login' ? 'Sign in' : 'Create account'}
			</button>
		</form>

		<p class="text-fg-faint mt-6 text-center text-xs leading-relaxed">
			{#if mode === 'register'}
				Anyone can register. We'll email you a link to confirm your address before your first
				sign-in.
			{:else}
				New here?
				<button onclick={() => swap('register')} class="text-signal hover:underline"
					>Create an account</button
				>.
			{/if}
		</p>
	</div>
	{/if}
</div>
