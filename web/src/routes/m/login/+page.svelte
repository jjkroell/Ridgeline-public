<script lang="ts">
	import { goto } from '$app/navigation';
	import { auth } from '$lib/auth.svelte';

	let mode = $state<'login' | 'register'>('login');
	let email = $state('');
	let password = $state('');
	let displayName = $state('');
	let error = $state('');
	let busy = $state(false);

	$effect(() => {
		if (auth.ready && auth.loggedIn) goto('/m/account');
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		error = '';
		busy = true;
		try {
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
	<!-- Mode toggle -->
	<div class="border-line/70 mb-5 flex gap-1 rounded-xl border p-1">
		<button
			onclick={() => ((mode = 'login'), (error = ''))}
			class="flex-1 rounded-lg px-3 py-2 text-sm font-600 transition-colors
				{mode === 'login' ? 'bg-signal/15 text-signal' : 'text-fg-dim'}">Sign in</button
		>
		<button
			onclick={() => ((mode = 'register'), (error = ''))}
			class="flex-1 rounded-lg px-3 py-2 text-sm font-600 transition-colors
				{mode === 'register' ? 'bg-signal/15 text-signal' : 'text-fg-dim'}">Create account</button
		>
	</div>

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
		<label class="block">
			<span class="label normal-case text-fg-dim mb-1.5 block">Password</span>
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

		{#if error}<p class="text-coral text-sm">{error}</p>{/if}

		<button
			type="submit"
			disabled={busy}
			class="bg-signal/15 text-signal border-signal/40 active:bg-signal/25 mt-1 w-full rounded-xl border px-4 py-3.5 text-sm font-700 transition-colors disabled:opacity-50"
		>
			{busy ? 'Working…' : mode === 'login' ? 'Sign in' : 'Create account'}
		</button>
	</form>

	<p class="text-fg-faint mt-6 text-center text-xs leading-relaxed">
		Anyone can register. Claiming nodes and setting private locations require admin approval.
	</p>
</div>
