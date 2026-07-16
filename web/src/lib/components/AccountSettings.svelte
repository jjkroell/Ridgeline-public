<script lang="ts">
	// Self-service account settings — display name, email, and password — shared by
	// the desktop (/account) and mobile (/m/account) account pages so the two stay
	// in parity. All mutations go through the auth store; `compact` tightens the
	// padding for the mobile layout.
	import { auth } from '$lib/auth.svelte';
	import { authApi } from '$lib/api';

	let { compact = false }: { compact?: boolean } = $props();
	const pad = $derived(compact ? 'px-4 py-4' : 'px-5 py-4');

	const u = $derived(auth.user);

	// Display name
	let dn = $state(auth.user?.displayName ?? '');
	let dnMsg = $state('');
	let dnBusy = $state(false);
	async function saveName() {
		dnBusy = true;
		dnMsg = '';
		try {
			await auth.updateDisplayName(dn.trim());
			dnMsg = 'Saved.';
		} catch (e) {
			dnMsg = String((e as Error).message ?? e);
		} finally {
			dnBusy = false;
		}
	}

	// Email
	let newEmail = $state('');
	let emailPw = $state('');
	let emailMsg = $state('');
	let emailErr = $state('');
	let emailBusy = $state(false);
	async function saveEmail() {
		emailBusy = true;
		emailErr = '';
		emailMsg = '';
		try {
			await auth.changeEmail(emailPw, newEmail.trim());
			newEmail = '';
			emailPw = '';
			emailMsg = auth.user?.emailVerified
				? 'Email updated.'
				: 'Check your new inbox for a confirmation link.';
		} catch (e) {
			emailErr = String((e as Error).message ?? e);
		} finally {
			emailBusy = false;
		}
	}

	// Password
	const MIN_PW = 8;
	let curPw = $state('');
	let newPw = $state('');
	let newPw2 = $state('');
	let pwMsg = $state('');
	let pwErr = $state('');
	let pwBusy = $state(false);
	const pwLongEnough = $derived(newPw.length >= MIN_PW);
	const pwMatch = $derived(newPw2.length > 0 && newPw === newPw2);
	const pwReady = $derived(!!curPw && pwLongEnough && pwMatch);
	async function savePassword() {
		pwErr = '';
		pwMsg = '';
		if (!pwMatch) {
			pwErr = 'New passwords do not match.';
			return;
		}
		pwBusy = true;
		try {
			await auth.changePassword(curPw, newPw);
			curPw = '';
			newPw = '';
			newPw2 = '';
			pwMsg = 'Password changed.';
		} catch (e) {
			pwErr = String((e as Error).message ?? e);
		} finally {
			pwBusy = false;
		}
	}

	// Resend the verification email (shown when the address is unconfirmed).
	let resendMsg = $state('');
	async function resendVerification() {
		if (!auth.user) return;
		resendMsg = '';
		await authApi.resendVerification(auth.user.email);
		resendMsg = 'Sent — check your inbox.';
	}

	const inputCls =
		'border-line bg-ink-2 text-fg focus:border-signal w-full rounded-[var(--radius)] border px-3 py-2 text-sm outline-none';
	const btnCls =
		'bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 self-start rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors disabled:opacity-50';
</script>

<div class="divide-line/60 divide-y">
	<!-- Display name -->
	<form onsubmit={(e) => (e.preventDefault(), saveName())} class={pad}>
		<label class="label normal-case text-fg-dim mb-1.5 block" for="acct-dn">Display name</label>
		<div class="flex flex-wrap items-center gap-2">
			<input
				id="acct-dn"
				bind:value={dn}
				maxlength="64"
				placeholder="Your name or handle"
				autocomplete="nickname"
				class="min-w-0 flex-1 {inputCls}"
			/>
			<button type="submit" disabled={dnBusy || dn.trim() === (u?.displayName ?? '')} class={btnCls}
				>{dnBusy ? 'Saving…' : 'Save'}</button
			>
		</div>
		<p class="text-fg-faint mt-1.5 text-xs">Shown on your public notes and as the owner of nodes you claim.</p>
		{#if dnMsg}<p class="text-signal mt-1.5 text-xs">{dnMsg}</p>{/if}
	</form>

	<!-- Email -->
	<form onsubmit={(e) => (e.preventDefault(), saveEmail())} class={pad}>
		<label class="label normal-case text-fg-dim mb-1.5 block" for="acct-email">Email</label>
		<p class="text-fg-faint mb-2 text-xs">
			Current: <span class="text-fg-dim break-all">{u?.email}</span>. Changing it requires
			re-confirming the new address.
			{#if u && !u.emailVerified}
				<button type="button" onclick={resendVerification} class="text-signal hover:underline"
					>Resend confirmation</button
				>{#if resendMsg}<span class="text-signal"> {resendMsg}</span>{/if}
			{/if}
		</p>
		<div class="flex flex-col gap-2">
			<input
				id="acct-email"
				type="email"
				bind:value={newEmail}
				placeholder="new@email.com"
				autocomplete="email"
				class={inputCls}
			/>
			<input
				type="password"
				bind:value={emailPw}
				placeholder="current password"
				autocomplete="current-password"
				class={inputCls}
			/>
			<button type="submit" disabled={emailBusy || !newEmail.trim() || !emailPw} class={btnCls}
				>{emailBusy ? 'Updating…' : 'Change email'}</button
			>
		</div>
		{#if emailErr}<p class="text-coral mt-2 text-xs">{emailErr}</p>{/if}
		{#if emailMsg}<p class="text-signal mt-2 text-xs">{emailMsg}</p>{/if}
	</form>

	<!-- Password -->
	<form onsubmit={(e) => (e.preventDefault(), savePassword())} class={pad}>
		<label class="label normal-case text-fg-dim mb-1.5 block" for="acct-pw">Password</label>
		<div class="flex flex-col gap-2">
			<input
				id="acct-pw"
				type="password"
				bind:value={curPw}
				placeholder="current password"
				autocomplete="current-password"
				class={inputCls}
			/>
			<input
				type="password"
				bind:value={newPw}
				placeholder="new password (at least 8 characters)"
				minlength={MIN_PW}
				autocomplete="new-password"
				class={inputCls}
			/>
			{#if newPw.length > 0}
				<span class="text-fg-faint -mt-1 text-xs tnum">
					{newPw.length} character{newPw.length === 1 ? '' : 's'}{pwLongEnough
						? ''
						: ` — ${MIN_PW - newPw.length} more to reach ${MIN_PW}`}
				</span>
			{/if}
			<input
				type="password"
				bind:value={newPw2}
				placeholder="confirm new password"
				autocomplete="new-password"
				class={inputCls}
			/>
			{#if newPw2.length > 0}
				<span class="-mt-1 text-xs {pwMatch ? 'text-signal' : 'text-coral'}">
					{pwMatch ? '✓ Passwords match' : '✗ Passwords do not match'}
				</span>
			{/if}
			<button type="submit" disabled={pwBusy || !pwReady} class={btnCls}
				>{pwBusy ? 'Changing…' : 'Change password'}</button
			>
		</div>
		{#if pwErr}<p class="text-coral mt-2 text-xs">{pwErr}</p>{/if}
		{#if pwMsg}<p class="text-signal mt-2 text-xs">{pwMsg}</p>{/if}
	</form>
</div>
