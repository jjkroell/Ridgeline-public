<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/auth.svelte';
	import { authApi } from '$lib/api';

	let mode = $state<'login' | 'register' | 'forgot'>('login');
	let email = $state('');
	let password = $state('');
	let displayName = $state('');
	let error = $state('');
	let busy = $state(false);
	let resetSent = $state(false);

	$effect(() => {
		if (auth.ready && auth.loggedIn) goto('/m/account');
	});

	function swap(to: 'login' | 'register' | 'forgot') {
		mode = to;
		error = '';
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		busy = true;
		try {
			if (mode === 'forgot') {
				await authApi.forgotPassword(email.trim());
				resetSent = true;
				return;
			}
			if (mode === 'register') await auth.register(email.trim(), password, displayName.trim());
			else await auth.login(email.trim(), password);
			goto('/m/account');
		} catch (err) {
			error = String((err as Error).message ?? err);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>Sign in · Ridgeline</title></svelte:head>

<div class="px-4 py-6">
	{#if resetSent}
		<!-- Password-reset email requested -->
		<div class="panel px-5 py-8 text-center">
			<h2 class="text-fg text-lg font-700">Check your email</h2>
			<p class="text-fg-dim mt-2 text-sm leading-relaxed">
				If an account exists for <strong class="text-fg break-all">{email.trim()}</strong>, we've sent
				a link to reset its password. It expires in 1 hour.
			</p>
			<button
				onclick={() => {
					resetSent = false;
					password = '';
					swap('login');
				}}
				class="text-fg-faint active:text-fg mt-6 text-xs">← Back to sign in</button
			>
		</div>
	{:else}
	<!-- Mode toggle -->
	{#if mode !== 'forgot'}
	<div class="border-line/70 mb-5 flex gap-1 rounded-xl border p-1">
		<button
			onclick={() => swap('login')}
			class="flex-1 rounded-lg px-3 py-2 text-sm font-600 transition-colors
				{mode === 'login' ? 'bg-signal/15 text-signal' : 'text-fg-dim'}">Sign in</button
		>
		<button
			onclick={() => swap('register')}
			class="flex-1 rounded-lg px-3 py-2 text-sm font-600 transition-colors
				{mode === 'register' ? 'bg-signal/15 text-signal' : 'text-fg-dim'}">Create account</button
		>
	</div>
	{:else}
		<p class="text-fg-dim mb-5 text-sm leading-relaxed">
			Enter your account email and we'll send you a link to set a new password.
		</p>
	{/if}

	<form onsubmit={submit} class="flex flex-col gap-4">
		{#if mode === 'register'}
			<label class="block">
				<span class="label normal-case text-fg-dim mb-1.5 block">Display name</span>
				<input
					type="text"
					bind:value={displayName}
					maxlength="64"
					placeholder="Your name or handle"
					autocomplete="nickname"
					class="border-line bg-ink-2 text-fg focus:border-signal w-full rounded-xl border px-3.5 py-3 text-base outline-none"
				/>
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
				class="border-line bg-ink-2 text-fg focus:border-signal w-full rounded-xl border px-3.5 py-3 text-base outline-none"
			/>
		</label>
		{#if mode !== 'forgot'}
			<label class="block">
				<span class="label normal-case text-fg-dim mb-1.5 flex items-baseline justify-between">
					<span>Password</span>
					{#if mode === 'login'}
						<button
							type="button"
							onclick={() => swap('forgot')}
							class="text-signal text-[0.7rem] font-500 normal-case active:underline"
							>Forgot password?</button
						>
					{/if}
				</span>
				<input
					type="password"
					bind:value={password}
					required
					minlength={mode === 'register' ? 8 : undefined}
					autocomplete={mode === 'register' ? 'new-password' : 'current-password'}
					placeholder={mode === 'register' ? 'at least 8 characters' : '••••••••'}
					class="border-line bg-ink-2 text-fg focus:border-signal w-full rounded-xl border px-3.5 py-3 text-base outline-none"
				/>
			</label>
		{/if}

		{#if error}<p class="text-coral text-sm">{error}</p>{/if}

		<button
			type="submit"
			disabled={busy}
			class="bg-signal/15 text-signal border-signal/40 active:bg-signal/25 mt-1 w-full rounded-xl border px-4 py-3.5 text-sm font-700 transition-colors disabled:opacity-50"
		>
			{busy
				? 'Working…'
				: mode === 'login'
					? 'Sign in'
					: mode === 'forgot'
						? 'Send reset link'
						: 'Create account'}
		</button>
	</form>

	<p class="text-fg-faint mt-6 text-center text-xs leading-relaxed">
		{#if mode === 'forgot'}
			Remembered it?
			<button onclick={() => swap('login')} class="text-signal active:underline">Back to sign in</button
			>.
		{:else}
			Anyone can register. We'll email a link to confirm your address before your first sign-in.
		{/if}
	</p>
	{/if}
</div>
