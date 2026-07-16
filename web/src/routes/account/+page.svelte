<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { auth } from '$lib/auth.svelte';
	import { authApi, claims, shares, type ClaimWithNode, type SharedWithMe } from '$lib/api';
	import { ago, shortKey } from '$lib/format';
	import PageHeader from '$lib/components/PageHeader.svelte';
	import OwnershipIcon from '$lib/components/OwnershipIcon.svelte';
	import AccountSettings from '$lib/components/AccountSettings.svelte';
	import DeleteAccountPanel from '$lib/components/DeleteAccountPanel.svelte';

	// Resend state for the "confirm your email" banner. The settings forms
	// themselves live in the shared <AccountSettings /> component.
	let resendMsg = $state('');

	// Collapsible sections — persisted per browser so each section remembers
	// whether it was left open. Default collapsed on first visit.
	const openKeys = {
		settings: 'ridgeline-acct-open-settings',
		nodes: 'ridgeline-acct-open-nodes',
		shared: 'ridgeline-acct-open-shared'
	};
	function readOpen(key: string): boolean {
		try {
			return localStorage.getItem(key) === '1';
		} catch {
			return false;
		}
	}
	let openSettings = $state(readOpen(openKeys.settings));
	let openNodes = $state(readOpen(openKeys.nodes));
	let openShared = $state(readOpen(openKeys.shared));
	// Persist whenever any section is toggled.
	$effect(() => {
		try {
			localStorage.setItem(openKeys.settings, openSettings ? '1' : '0');
			localStorage.setItem(openKeys.nodes, openNodes ? '1' : '0');
			localStorage.setItem(openKeys.shared, openShared ? '1' : '0');
		} catch {
			/* storage unavailable — sections just won't persist */
		}
	});

	async function resendVerification() {
		if (!auth.user) return;
		resendMsg = '';
		await authApi.resendVerification(auth.user.email);
		resendMsg = 'Sent — check your inbox.';
	}

	// The caller's node claims (pending + owned).
	let myNodes = $state<ClaimWithNode[]>([]);
	async function loadMyNodes() {
		try {
			myNodes = await claims.mine();
		} catch {
			/* leave empty */
		}
	}

	// Nodes shared WITH me (private locations others have granted me access to).
	let sharedWithMe = $state<SharedWithMe[]>([]);
	async function loadSharedWithMe() {
		try {
			sharedWithMe = await shares.mine();
			// Viewing the list clears the "new shares" badge.
			await auth.markSharesSeen();
		} catch {
			/* leave empty */
		}
	}

	// Redirect to sign-in if not authenticated (once the /me probe resolves).
	$effect(() => {
		if (auth.ready && !auth.loggedIn) goto('/login');
	});

	onMount(() => {
		loadMyNodes();
		loadSharedWithMe();
	});

	async function signOut() {
		await auth.logout();
		goto('/');
	}
</script>

<svelte:head><title>Account · Ridgeline</title></svelte:head>

<PageHeader eyebrow="Accounts" title="Your account">
	<button
		onclick={signOut}
		class="border-line text-fg-dim hover:border-coral/50 hover:text-coral rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors"
	>
		Sign out
	</button>
</PageHeader>

{#snippet chevron(open: boolean)}
	<svg
		viewBox="0 0 24 24"
		class="text-fg-faint ml-auto h-4 w-4 shrink-0 transition-transform {open ? 'rotate-180' : ''}"
		fill="none"
		stroke="currentColor"
		stroke-width="1.8"
		stroke-linecap="round"
		stroke-linejoin="round"><path d="M6 9l6 6 6-6" /></svg
	>
{/snippet}

{#if auth.loggedIn}
	{@const u = auth.user}
	<div class="flex flex-col gap-6 px-6 pb-16 md:px-10">
		{#if u && !u.emailVerified}
			<div class="border-amber/40 bg-amber/10 rounded-[var(--radius)] border px-4 py-3">
				<p class="text-fg-dim text-sm leading-relaxed">
					<strong class="text-amber">Confirm your email.</strong>
					We sent a link to <strong class="text-fg break-all">{u.email}</strong>. Until it's confirmed
					you won't be able to sign in again.
					<button onclick={resendVerification} class="text-signal hover:underline">Resend</button
					>{#if resendMsg}<span class="text-signal"> {resendMsg}</span>{/if}
				</p>
			</div>
		{/if}

		<!-- Profile -->
		<div class="panel px-6 py-6">
			<div class="flex items-center gap-4">
				<span
					class="bg-signal/15 text-signal flex h-14 w-14 shrink-0 items-center justify-center rounded-full text-xl font-700"
				>
					{(u?.displayName || u?.email || '?').charAt(0).toUpperCase()}
				</span>
				<div class="min-w-0">
					<div class="font-display text-fg truncate text-xl font-700">
						{u?.displayName || u?.email}
					</div>
					<div class="text-fg-dim truncate text-sm">{u?.email}</div>
				</div>
				<div class="ml-auto flex flex-wrap justify-end gap-2">
					{#if u?.isOwner}
						<span class="bg-signal/15 text-signal rounded-full px-2.5 py-1 text-xs font-600">Owner</span>
					{:else if u?.isAdmin}
						<span class="bg-signal/15 text-signal rounded-full px-2.5 py-1 text-xs font-600"
							>Admin</span
						>
					{/if}
				</div>
			</div>
			<div class="border-line/70 mt-5 grid grid-cols-2 gap-4 border-t pt-5 text-sm sm:grid-cols-3">
				<div>
					<div class="label mb-1">Joined</div>
					<div class="text-fg-dim">{ago(u?.createdAt)}</div>
				</div>
				<div>
					<div class="label mb-1">Last login</div>
					<div class="text-fg-dim">{u?.lastLogin ? ago(u.lastLogin) : '—'}</div>
				</div>
			</div>
		</div>

		<!-- Account settings -->
		<div class="panel overflow-hidden">
			<button
				onclick={() => (openSettings = !openSettings)}
				aria-expanded={openSettings}
				class="panel-hover flex w-full items-center gap-2.5 px-5 py-3.5 text-left {openSettings
					? 'border-line/70 border-b'
					: ''}"
			>
				<span class="font-display text-fg text-sm font-700">Account settings</span>
				{#if u && !u.emailVerified}
					<span class="bg-amber/15 text-amber rounded-full px-2 py-0.5 text-xs font-600"
						>Email unconfirmed</span
					>
				{/if}
				{@render chevron(openSettings)}
			</button>
			{#if openSettings}
			<AccountSettings />
			{/if}
		</div>

		<!-- My nodes -->
		{#if myNodes.length}
			<div class="panel overflow-hidden">
				<button
					onclick={() => (openNodes = !openNodes)}
					aria-expanded={openNodes}
					class="panel-hover flex w-full items-center gap-2.5 px-5 py-3.5 text-left {openNodes
						? 'border-line/70 border-b'
						: ''}"
				>
					<span class="font-display text-fg text-sm font-700">My nodes</span>
					<span class="text-fg-faint text-xs">{myNodes.length}</span>
					{@render chevron(openNodes)}
				</button>
				{#if openNodes}
				<div class="divide-line/60 divide-y">
					{#each myNodes as c (c.id)}
						<a
							href="/nodes/{c.nodePubkey}"
							class="panel-hover flex items-center gap-3 px-5 py-3"
						>
							<span class="min-w-0 flex-1">
								<span class="flex items-center gap-1.5">
									<span class="text-fg truncate text-sm font-600">{c.nodeName || shortKey(c.nodePubkey)}</span>
									<OwnershipIcon kind={c.status === 'verified' ? 'owned' : 'pending'} />
								</span>
								<span class="text-fg-faint block truncate font-mono text-xs">{shortKey(c.nodePubkey, 6, 4)}</span>
							</span>
							{#if c.status === 'verified'}
								<span class="bg-signal/15 text-signal rounded-full px-2.5 py-1 text-xs font-600"
									>Owned</span
								>
							{:else}
								<span class="bg-amber/15 text-amber rounded-full px-2.5 py-1 text-xs font-600"
									>Pending</span
								>
							{/if}
						</a>
					{/each}
				</div>
				{/if}
			</div>
		{/if}

		<!-- Shared with me -->
		{#if sharedWithMe.length}
			<div class="panel overflow-hidden">
				<button
					onclick={() => (openShared = !openShared)}
					aria-expanded={openShared}
					class="panel-hover flex w-full items-center gap-2.5 px-5 py-3.5 text-left {openShared
						? 'border-line/70 border-b'
						: ''}"
				>
					<span class="font-display text-fg text-sm font-700">Shared with me</span>
					<span class="text-fg-faint text-xs">{sharedWithMe.length}</span>
					{#if sharedWithMe.some((s) => !s.seen)}
						<span class="bg-signal text-ink rounded-full px-2 py-0.5 text-xs font-700">New</span>
					{/if}
					{@render chevron(openShared)}
				</button>
				{#if openShared}
				<div class="divide-line/60 divide-y">
					{#each sharedWithMe as sh (sh.nodePubkey)}
						<a href="/nodes/{sh.nodePubkey}" class="panel-hover flex items-center gap-3 px-5 py-3">
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
				{/if}
			</div>
		{/if}

		<!-- Danger zone -->
		<DeleteAccountPanel home="/" />
	</div>
{/if}
