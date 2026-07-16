<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { auth } from '$lib/auth.svelte';
	import { authApi, claims, shares, type ClaimWithNode, type SharedWithMe } from '$lib/api';
	import { ago, shortKey } from '$lib/format';
	import OwnershipIcon from '$lib/components/OwnershipIcon.svelte';
	import AccountSettings from '$lib/components/AccountSettings.svelte';
	import DeleteAccountPanel from '$lib/components/DeleteAccountPanel.svelte';

	let myNodes = $state<ClaimWithNode[]>([]);
	async function loadMyNodes() {
		try {
			myNodes = await claims.mine();
		} catch {
			/* leave empty */
		}
	}

	let sharedWithMe = $state<SharedWithMe[]>([]);
	async function loadSharedWithMe() {
		try {
			sharedWithMe = await shares.mine();
			await auth.markSharesSeen();
		} catch {
			/* leave empty */
		}
	}

	onMount(() => {
		loadMyNodes();
		loadSharedWithMe();
	});

	async function signOut() {
		await auth.logout();
		goto('/m');
	}

	let settingsOpen = $state(false);

	let resendMsg = $state('');
	async function resendVerification() {
		if (!auth.user) return;
		resendMsg = '';
		await authApi.resendVerification(auth.user.email);
		resendMsg = 'Sent — check your inbox.';
	}
</script>

<svelte:head><title>Account · Ridgeline</title></svelte:head>

{#if !auth.ready}
	<div class="text-fg-faint px-4 py-16 text-center text-sm">Loading…</div>
{:else if !auth.loggedIn}
	<!-- Signed-out prompt -->
	<div class="px-4 py-10 text-center">
		<span
			class="bg-signal/15 text-signal mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-full"
		>
			<svg
				viewBox="0 0 24 24"
				class="h-7 w-7"
				fill="none"
				stroke="currentColor"
				stroke-width="1.6"
				stroke-linecap="round"
				stroke-linejoin="round"><path d="M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0" /></svg
			>
		</span>
		<h2 class="font-display text-fg text-lg font-700">You're signed out</h2>
		<p class="text-fg-dim mx-auto mt-2 max-w-xs text-sm leading-relaxed">
			Sign in to claim your nodes, add notes, and manage private locations.
		</p>
		<a
			href="/m/login"
			class="bg-signal/15 text-signal border-signal/40 active:bg-signal/25 mt-6 inline-block rounded-xl border px-6 py-3 text-sm font-700"
			>Sign in or register</a
		>
	</div>
{:else}
	{@const u = auth.user}
	<div class="flex flex-col gap-4 px-4 py-5">
		{#if u && !u.emailVerified}
			<div class="border-amber/40 bg-amber/10 rounded-xl border px-4 py-3">
				<p class="text-fg-dim text-sm leading-relaxed">
					<strong class="text-amber">Confirm your email.</strong>
					We sent a link to <strong class="text-fg break-all">{u.email}</strong>. Until it's
					confirmed you won't be able to sign in again.
					<button onclick={resendVerification} class="text-signal underline">Resend</button
					>{#if resendMsg}<span class="text-signal"> {resendMsg}</span>{/if}
				</p>
			</div>
		{/if}

		<!-- Profile -->
		<div class="panel px-4 py-4">
			<div class="flex items-center gap-3">
				<span
					class="bg-signal/15 text-signal flex h-12 w-12 shrink-0 items-center justify-center rounded-full text-lg font-700"
					>{(u?.displayName || u?.email || '?').charAt(0).toUpperCase()}</span
				>
				<div class="min-w-0">
					<div class="font-display text-fg truncate text-base font-700">
						{u?.displayName || u?.email}
					</div>
					<div class="text-fg-dim truncate text-xs">{u?.email}</div>
				</div>
			</div>
			<div class="mt-3 flex flex-wrap gap-2">
				{#if u?.isOwner}
					<span class="bg-signal/15 text-signal rounded-full px-2.5 py-1 text-xs font-600">Owner</span>
				{:else if u?.isAdmin}
					<span class="bg-signal/15 text-signal rounded-full px-2.5 py-1 text-xs font-600">Admin</span>
				{/if}
				<span class="text-fg-faint self-center text-xs">joined {ago(u?.createdAt)}</span>
			</div>
		</div>

		<!-- Account settings -->
		<div class="panel overflow-hidden">
			<button
				onclick={() => (settingsOpen = !settingsOpen)}
				aria-expanded={settingsOpen}
				class="active:bg-line/40 flex w-full items-center gap-2 px-4 py-3 text-left {settingsOpen
					? 'border-line/70 border-b'
					: ''}"
			>
				<span class="font-display text-fg text-sm font-700">Account settings</span>
				{#if u && !u.emailVerified}
					<span class="bg-amber/15 text-amber rounded-full px-2 py-0.5 text-xs font-600"
						>Email unconfirmed</span
					>
				{/if}
				<svg
					viewBox="0 0 24 24"
					class="text-fg-faint ml-auto h-4 w-4 shrink-0 transition-transform {settingsOpen
						? 'rotate-180'
						: ''}"
					fill="none"
					stroke="currentColor"
					stroke-width="1.8"
					stroke-linecap="round"
					stroke-linejoin="round"><path d="M6 9l6 6 6-6" /></svg
				>
			</button>
			{#if settingsOpen}
				<AccountSettings compact />
			{/if}
		</div>

		<!-- My nodes -->
		{#if myNodes.length}
			<div class="panel overflow-hidden">
				<div class="border-line/70 flex items-center gap-2 border-b px-4 py-3">
					<span class="font-display text-fg text-sm font-700">My nodes</span>
					<span class="text-fg-faint text-xs">{myNodes.length}</span>
				</div>
				<div class="divide-line/60 divide-y">
					{#each myNodes as c (c.id)}
						<a href="/m/nodes/{c.nodePubkey}" class="active:bg-line/40 flex items-center gap-3 px-4 py-3">
							<span class="min-w-0 flex-1">
								<span class="flex items-center gap-1.5">
									<span class="text-fg truncate text-sm font-600">{c.nodeName || shortKey(c.nodePubkey)}</span>
									<OwnershipIcon kind={c.status === 'verified' ? 'owned' : 'pending'} />
								</span>
								<span class="text-fg-faint block truncate font-mono text-xs"
									>{shortKey(c.nodePubkey, 6, 4)}</span
								>
							</span>
							{#if c.status === 'verified'}
								<span class="bg-signal/15 text-signal rounded-full px-2.5 py-1 text-xs font-600">Owned</span>
							{:else}
								<span class="bg-amber/15 text-amber rounded-full px-2.5 py-1 text-xs font-600">Pending</span>
							{/if}
						</a>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Shared with me -->
		{#if sharedWithMe.length}
			<div class="panel overflow-hidden">
				<div class="border-line/70 flex items-center gap-2 border-b px-4 py-3">
					<span class="font-display text-fg text-sm font-700">Shared with me</span>
					<span class="text-fg-faint text-xs">{sharedWithMe.length}</span>
				</div>
				<div class="divide-line/60 divide-y">
					{#each sharedWithMe as sh (sh.nodePubkey)}
						<a
							href="/m/nodes/{sh.nodePubkey}"
							class="active:bg-line/40 flex items-center gap-3 px-4 py-3"
						>
							<span class="min-w-0 flex-1">
								<span class="flex items-center gap-1.5">
									<span class="text-fg truncate text-sm font-600">{sh.nodeName || shortKey(sh.nodePubkey)}</span>
									<OwnershipIcon kind="shared" sharedBy={sh.sharedByName} />
								</span>
								<span class="text-fg-faint block truncate text-xs"
									>Shared by {sh.sharedByName} · {ago(sh.createdAt)}</span
								>
							</span>
							{#if !sh.seen}
								<span class="bg-signal text-ink rounded-full px-2.5 py-1 text-xs font-700">New</span>
							{/if}
						</a>
					{/each}
				</div>
			</div>
		{/if}

		{#if auth.isAdmin}
			<a
				href="/m/admin"
				class="panel active:bg-line/40 flex items-center gap-3 px-4 py-3.5 text-sm font-600"
			>
				<span class="bg-signal/15 text-signal grid h-8 w-8 shrink-0 place-items-center rounded-full">
					<svg
						viewBox="0 0 24 24"
						class="h-4 w-4"
						fill="none"
						stroke="currentColor"
						stroke-width="1.6"
						stroke-linecap="round"
						stroke-linejoin="round"><path d="M12 3l7 4v5c0 4.5-3 7.5-7 9-4-1.5-7-4.5-7-9V7z" /></svg
					>
				</span>
				<span class="text-fg flex-1">Admin console</span>
				<span class="text-fg-faint">›</span>
			</a>
		{/if}

		<!-- Danger zone -->
		<DeleteAccountPanel home="/m" compact />

		<button
			onclick={signOut}
			class="border-line text-fg-dim active:border-coral/50 active:text-coral w-full rounded-xl border px-4 py-3 text-sm font-600"
			>Sign out</button
		>
	</div>
{/if}
