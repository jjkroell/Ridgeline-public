<script lang="ts">
	import '../app.css';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { live } from '$lib/live.svelte';
	import { theme } from '$lib/theme.svelte';
	import { channels } from '$lib/channels.svelte';
	import { favorites } from '$lib/favorites.svelte';
	import { basemap } from '$lib/basemap.svelte';
	import { overview } from '$lib/overview.svelte';
	import { auth } from '$lib/auth.svelte';
	import { announce } from '$lib/announce.svelte';
	import { consent } from '$lib/consent.svelte';
	import AnnouncementModal from '$lib/components/AnnouncementModal.svelte';
	import CookieConsent from '$lib/components/CookieConsent.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import Tooltip from '$lib/components/Tooltip.svelte';
	import { SITE_NAME, SITE_TAGLINE } from '$lib/site';
	import { ABOUT } from '$lib/site-content';

	let { children } = $props();

	onMount(() => {
		theme.init();
		channels.init();
		favorites.init();
		basemap.init();
		overview.init();
		auth.init();
		live.start();
		consent.init();
		announce.init();
	});

	type NavItem = {
		href: string;
		label: string;
		icon: string;
		exact?: boolean;
		adminOnly?: boolean;
		aboutOnly?: boolean;
	};
	const navItems: NavItem[] = [
		{ href: '/', label: 'Overview', exact: true, icon: 'grid' },
		{ href: '/nodes', label: 'Nodes', icon: 'node' },
		{ href: '/live', label: 'Feed', icon: 'pulse' },
		{ href: '/map', label: 'Map', icon: 'map' },
		{ href: '/live-map', label: 'Live Map', icon: 'signal' },
		{ href: '/analytics', label: 'Analytics', icon: 'chart' },
		{ href: '/topology', label: 'Topology', icon: 'graph' },
		{ href: '/channels', label: 'Channels', icon: 'hash' },
		{ href: '/identity', label: 'Identity', icon: 'key' },
		{ href: '/observers', label: 'Observers', icon: 'eye' },
		{ href: '/about', label: 'About', icon: 'info', aboutOnly: true },
		{ href: '/admin', label: 'Admin', icon: 'shield', adminOnly: true }
	];
	// The Admin console is only reachable by admin accounts, so hide its nav link
	// from everyone else. The About link is dropped when this deployment opted out
	// of an About page (see $lib/site-content).
	const nav = $derived(
		navItems.filter(
			(item) => (!item.adminOnly || auth.isAdmin) && (!item.aboutOnly || ABOUT.enabled)
		)
	);

	function active(href: string, exact = false): boolean {
		const p = page.url.pathname;
		return exact ? p === href : p === href || p.startsWith(href + '/');
	}

	// The mobile app (/m/*) supplies its own chrome — skip the desktop sidebar/topbar
	// but keep the shared init above (theme, channels, favorites, live store).
	const isMobileApp = $derived(page.url.pathname === '/m' || page.url.pathname.startsWith('/m/'));

	const icons: Record<string, string> = {
		grid: 'M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z',
		node: 'M12 2v6m0 8v6M2 12h6m8 0h6M12 8a4 4 0 100 8 4 4 0 000-8z',
		pulse: 'M2 12h4l3 8 4-16 3 8h6',
		map: 'M9 4 3 6v14l6-2 6 2 6-2V4l-6 2-6-2zM9 4v14M15 6v14',
		signal: 'M12 12h.01M8.5 8.5a5 5 0 000 7M15.5 8.5a5 5 0 010 7M5.6 5.6a9 9 0 000 12.8M18.4 5.6a9 9 0 010 12.8',
		hash: 'M10 3 8 21M16 3l-2 18M4 9h16M3 15h16',
		chart: 'M3 3v18h18M7 14v4M12 9v9M17 5v13',
		graph: 'M6 8a2.5 2.5 0 100-5 2.5 2.5 0 000 5zM18 8a2.5 2.5 0 100-5 2.5 2.5 0 000 5zM12 21a2.5 2.5 0 100-5 2.5 2.5 0 000 5zM7 7l4 9M17 7l-4 9',
		eye: 'M2 12s4-7 10-7 10 7 10 7-4 7-10 7-10-7-10-7zM12 9a3 3 0 100 6 3 3 0 000-6z',
		key: 'M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.78 7.78 5.5 5.5 0 0 1 7.78-7.78zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3',
		shield: 'M12 2 4 5v6c0 5 3.4 8.5 8 11 4.6-2.5 8-6 8-11V5l-8-3z',
		info: 'M12 2a10 10 0 100 20 10 10 0 000-20zM12 8h.01M11 12h1v5h1'
	};
</script>

<!-- App-wide "what's new" modal (self-guards on announce.open). -->
<AnnouncementModal />
<!-- Cookie-consent banner (self-guards on consent.open); shown on desktop + /m. -->
<CookieConsent />
<!-- The /m app layout mounts its own ConfirmDialog; only render one here on the
     desktop app so the confirmer singleton doesn't drive two stacked dialogs. -->
{#if !isMobileApp}
	<ConfirmDialog />
{/if}

{#if isMobileApp}
	{@render children()}
{:else}
<div class="flex min-h-screen">
	<!-- Sidebar -->
	<aside
		class="border-line/80 bg-ink-2/70 sticky top-0 hidden h-screen w-[220px] shrink-0 flex-col border-r backdrop-blur-sm md:flex"
	>
		<a href="/" class="group flex items-center gap-3 px-5 pt-6 pb-5">
			<svg viewBox="0 0 32 32" class="h-8 w-8" aria-hidden="true">
				<path
					d="M3 23 L11 11 L16 17 L22 7 L29 23"
					fill="none"
					stroke="var(--color-signal)"
					stroke-width="2.2"
					stroke-linejoin="round"
					stroke-linecap="round"
				/>
				<path
					d="M3 27 L11 17 L16 22 L22 14 L29 27"
					fill="none"
					stroke="var(--color-amber)"
					stroke-width="1.4"
					stroke-linejoin="round"
					stroke-linecap="round"
					opacity="0.6"
				/>
				<circle cx="22" cy="7" r="2.4" fill="var(--color-signal)" />
			</svg>
			<div class="leading-none">
				<div class="font-display text-fg text-[1.05rem] font-900 tracking-tight">
					{SITE_NAME.toUpperCase()}
				</div>
				<div class="label mt-1">{SITE_TAGLINE}</div>
			</div>
		</a>

		<nav class="mt-3 flex flex-col gap-0.5 px-3">
			{#each nav as item (item.href)}
				{@const on = active(item.href, item.exact)}
				<a
					href={item.href}
					class="group relative flex items-center gap-3 rounded-[var(--radius)] px-3 py-2.5 text-sm transition-colors
						{on ? 'text-fg' : 'text-fg-dim hover:text-fg'}"
				>
					{#if on}
						<span class="bg-signal absolute top-1/2 left-0 h-5 w-[2px] -translate-y-1/2 rounded-full"
						></span>
					{/if}
					<svg
						viewBox="0 0 24 24"
						class="h-[18px] w-[18px] shrink-0 transition-colors {on
							? 'text-signal'
							: 'text-fg-faint group-hover:text-fg-dim'}"
						fill="none"
						stroke="currentColor"
						stroke-width="1.6"
						stroke-linecap="round"
						stroke-linejoin="round"
					>
						<path d={icons[item.icon]} />
					</svg>
					<span class="font-medium">{item.label}</span>
				</a>
			{/each}
		</nav>

		<div class="mt-auto px-5 pb-5">
			<!-- Account -->
			<div class="mb-3">
				{#if auth.loggedIn}
					<a
						href="/account"
						class="group border-line hover:border-line-bright flex items-center gap-2.5 rounded-[var(--radius)] border px-3 py-2 transition-colors"
					>
						<span
							class="bg-signal/15 text-signal flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-700"
						>
							{(auth.user?.displayName || auth.user?.email || '?').charAt(0).toUpperCase()}
						</span>
						<span class="min-w-0 flex-1">
							<span class="text-fg block truncate text-xs font-600"
								>{auth.user?.displayName || auth.user?.email}</span
							>
							<span class="label !text-fg-faint block">{auth.isAdmin ? 'Admin' : 'Member'}</span>
						</span>
						{#if auth.unseenShares > 0}
							<Tooltip
								text="{auth.unseenShares} node{auth.unseenShares === 1 ? '' : 's'} newly shared with you"
							>
								<span
									class="bg-signal text-ink grid h-5 min-w-5 shrink-0 place-items-center rounded-full px-1.5 text-[0.65rem] font-700"
									>{auth.unseenShares}</span
								>
							</Tooltip>
						{/if}
					</a>
				{:else}
					<a
						href="/login"
						class="text-fg-dim hover:border-line-bright hover:text-fg border-line flex w-full items-center gap-2.5 rounded-[var(--radius)] border px-3 py-2 text-xs transition-colors"
					>
						<svg
							viewBox="0 0 24 24"
							class="text-fg-faint h-4 w-4"
							fill="none"
							stroke="currentColor"
							stroke-width="1.6"
							stroke-linecap="round"
							stroke-linejoin="round"
							><path d="M15 3h4a2 2 0 012 2v14a2 2 0 01-2 2h-4M10 17l5-5-5-5M15 12H3" /></svg
						>
						<span class="font-medium">Sign in</span>
					</a>
				{/if}
			</div>
			<button
				onclick={() => theme.toggle()}
				class="text-fg-dim hover:border-line-bright hover:text-fg border-line flex w-full items-center gap-2.5 rounded-[var(--radius)] border px-3 py-2 text-xs transition-colors"
				aria-label="Toggle light and dark theme"
			>
				{#if theme.mode === 'dark'}
					<svg
						viewBox="0 0 24 24"
						class="h-4 w-4 text-amber"
						fill="none"
						stroke="currentColor"
						stroke-width="1.6"
						stroke-linecap="round"
						stroke-linejoin="round"
						><circle cx="12" cy="12" r="4" /><path
							d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"
						/></svg
					>
					<span class="font-medium">Light mode</span>
				{:else}
					<svg
						viewBox="0 0 24 24"
						class="text-signal h-4 w-4"
						fill="none"
						stroke="currentColor"
						stroke-width="1.6"
						stroke-linecap="round"
						stroke-linejoin="round"><path d="M21 12.8A9 9 0 1111.2 3a7 7 0 009.8 9.8z" /></svg
					>
					<span class="font-medium">Dark mode</span>
				{/if}
			</button>
			<div class="border-line mt-3 flex items-center gap-2 border-t pt-3">
				{#if live.connected}
					<span class="live-dot"></span>
					<span class="label !text-signal">Live</span>
				{:else}
					<span class="bg-coral/70 inline-block h-2 w-2 rounded-full"></span>
					<span class="label !text-coral">Offline</span>
				{/if}
				<span class="label ml-auto tnum">{live.total}</span>
			</div>
			<a href="/privacy" class="label !text-fg-faint hover:!text-fg-dim mt-2 block transition-colors">Privacy &amp; cookies</a>
		</div>
	</aside>

	<!-- Main -->
	<div class="flex min-w-0 flex-1 flex-col">
		<!-- Mobile top bar -->
		<header
			class="border-line bg-ink-2/80 flex items-center gap-4 border-b px-4 py-3 backdrop-blur-sm md:hidden"
		>
			<a href="/" class="font-display font-900 tracking-tight">{SITE_NAME.toUpperCase()}</a>
			<nav class="ml-auto flex gap-3 text-xs">
				{#each nav as item (item.href)}
					<a
						href={item.href}
						class={active(item.href, item.exact) ? 'text-signal' : 'text-fg-dim'}>{item.label}</a
					>
				{/each}
			</nav>
		</header>

		<main class="flex-1">
			{@render children()}
		</main>
	</div>
</div>
{/if}
