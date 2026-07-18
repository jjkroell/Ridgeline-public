<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { live } from '$lib/live.svelte';
	import { theme } from '$lib/theme.svelte';
	import { auth } from '$lib/auth.svelte';
	import PwaInstall from '$lib/components/PwaInstall.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import DevBanner from '$lib/components/DevBanner.svelte';

	let { children } = $props();

	// Detail routes (node/observer) show a back button instead of the logo, since
	// an installed PWA has no browser back chrome.
	const isDetail = $derived(/^\/m\/(nodes|observers)\/.+/.test(page.url.pathname));
	function goBack() {
		if (history.length > 1) history.back();
		else goto('/m/' + (page.url.pathname.split('/')[2] ?? ''));
	}

	// Primary bottom-tab destinations (5). Secondary live in the "More" sheet.
	const tabs = [
		{ href: '/m', label: 'Home', exact: true, icon: 'home' },
		{ href: '/m/nodes', label: 'Nodes', icon: 'nodes' },
		{ href: '/m/live', label: 'Feed', icon: 'live' },
		{ href: '/m/live-map', label: 'Live Map', icon: 'livemap' },
		{ href: '/m/more', label: 'More', icon: 'more', sheet: true }
	];
	const more = [
		{ href: '/m/account', label: 'Account', icon: 'account', desc: 'Sign in or manage your account' },
		{ href: '/m/map', label: 'Map', icon: 'map', desc: 'Node locations & coverage' },
		{ href: '/m/analytics', label: 'Analytics', icon: 'analytics', desc: 'Mesh-wide health & traffic' },
		{ href: '/m/topology', label: 'Topology', icon: 'topology', desc: 'Relay backbone graph' },
		{ href: '/m/channels', label: 'Channels', icon: 'channels', desc: 'Decrypted group chat' },
		{ href: '/m/identity', label: 'Identity', icon: 'keys', desc: 'Collisions & key generator' },
		{ href: '/m/observers', label: 'Observers', icon: 'observers', desc: 'Listening posts & telemetry' },
		{ href: '/m/admin', label: 'Admin', icon: 'admin', desc: 'Injection control (restricted)' }
	];

	const icons: Record<string, string> = {
		home: 'M3 11l9-8 9 8M5 10v10a1 1 0 0 0 1 1h4v-6h4v6h4a1 1 0 0 0 1-1V10',
		nodes: 'M12 4.5a1.8 1.8 0 1 0 0-3.6 1.8 1.8 0 0 0 0 3.6zM4.5 22a1.8 1.8 0 1 0 0-3.6 1.8 1.8 0 0 0 0 3.6zM19.5 22a1.8 1.8 0 1 0 0-3.6 1.8 1.8 0 0 0 0 3.6zM12 6v3.5m0 0-5.2 7m5.2-7 5.2 7',
		live: 'M2 12h3l2-7 3.5 15L16 8l2 4h4',
		map: 'M12 21s7-6.6 7-12a7 7 0 1 0-14 0c0 5.4 7 12 7 12zM12 9.2a2.6 2.6 0 1 0 0 5.2 2.6 2.6 0 0 0 0-5.2z',
		more: 'M5 12h.01M12 12h.01M19 12h.01',
		livemap: 'M4.9 4.9a10 10 0 0 0 0 14.2M19.1 4.9a10 10 0 0 1 0 14.2M8 8a5 5 0 0 0 0 8M16 8a5 5 0 0 1 0 8M12 11.2a1 1 0 1 0 0 1.6 1 1 0 0 0 0-1.6z',
		analytics: 'M5 21V11M12 21V4M19 21v-7M3 21h18',
		topology: 'M6 8a2.5 2.5 0 100-5 2.5 2.5 0 000 5zM18 8a2.5 2.5 0 100-5 2.5 2.5 0 000 5zM12 21a2.5 2.5 0 100-5 2.5 2.5 0 000 5zM7 7l4 9M17 7l-4 9',
		channels: 'M21 11.5a7.5 7.5 0 0 1-10.8 6.7L3.5 20l1.3-5A7.5 7.5 0 1 1 21 11.5z',
		observers: 'M2 12s4-7 10-7 10 7 10 7-4 7-10 7-10-7-10-7zM12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z',
		admin: 'M12 2 4 5v6c0 5 3.4 8.5 8 11 4.6-2.5 8-6 8-11V5l-8-3z',
		keys: 'M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.78 7.78 5.5 5.5 0 0 1 7.78-7.78zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3',
		account: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0'
	};

	function activeTab(href: string, exact = false): boolean {
		const p = page.url.pathname;
		if (exact) return p === href;
		return p === href || p.startsWith(href + '/');
	}

	// App-bar title derived from the route.
	const title = $derived.by(() => {
		const p = page.url.pathname;
		if (p === '/m') return 'Overview';
		if (p.startsWith('/m/nodes')) return p === '/m/nodes' ? 'Nodes' : 'Node';
		if (p.startsWith('/m/live-map')) return 'Live Map';
		if (p.startsWith('/m/live')) return 'Feed';
		if (p.startsWith('/m/map')) return 'Map';
		if (p.startsWith('/m/analytics')) return 'Analytics';
		if (p.startsWith('/m/topology')) return 'Topology';
		if (p.startsWith('/m/channels')) return 'Channels';
		if (p.startsWith('/m/identity')) return 'Node Identity';
		if (p.startsWith('/m/observers')) return p === '/m/observers' ? 'Observers' : 'Observer';
		if (p.startsWith('/m/admin')) return 'Admin';
		if (p.startsWith('/m/account')) return 'Account';
		if (p.startsWith('/m/login')) return auth.loggedIn ? 'Account' : 'Sign in';
		return 'Ridgeline';
	});

	let moreOpen = $state(false);
	const anyMoreActive = $derived(more.some((m) => activeTab(m.href)));
	// Close the sheet on navigation.
	$effect(() => {
		page.url.pathname;
		moreOpen = false;
	});
</script>

<div class="bg-ink text-fg fixed inset-0 flex flex-col overflow-hidden">
	<!-- Non-production banner (self-guards; only on a dev/staging instance). -->
	<DevBanner />
	<!-- App bar -->
	<header
		class="border-line/70 bg-ink-2/85 z-20 flex shrink-0 items-center gap-3 border-b px-4 backdrop-blur-md"
		style="padding-top:calc(env(safe-area-inset-top) + 0.6rem);padding-bottom:0.6rem"
	>
		{#if isDetail}
			<button onclick={goBack} aria-label="Back" class="text-fg-dim active:text-fg -ml-1 p-1">
				<svg viewBox="0 0 24 24" class="h-6 w-6" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 18l-6-6 6-6" /></svg>
			</button>
		{:else}
			<svg viewBox="0 0 32 32" class="h-6 w-6 shrink-0" aria-hidden="true">
				<path d="M3 23 11 11 16 17 22 7 29 23" fill="none" stroke="var(--color-signal)" stroke-width="2.4" stroke-linejoin="round" stroke-linecap="round" />
				<path d="M3 27 11 17 16 22 22 14 29 27" fill="none" stroke="var(--color-amber)" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round" opacity="0.6" />
			</svg>
		{/if}
		<h1 class="font-display text-fg text-[1.15rem] font-900 tracking-tight">{title}</h1>
		<div class="ml-auto flex items-center gap-3">
			<div class="flex items-center gap-1.5">
				{#if live.connected}
					<span class="live-dot"></span>
					<span class="label !text-signal !text-[0.6rem]">Live</span>
				{:else}
					<span class="bg-coral/70 inline-block h-2 w-2 rounded-full"></span>
					<span class="label !text-coral !text-[0.6rem]">Off</span>
				{/if}
			</div>
			<button onclick={() => theme.toggle()} aria-label="Toggle theme" class="text-fg-faint active:text-fg p-1">
				{#if theme.mode === 'dark'}
					<svg viewBox="0 0 24 24" class="text-amber h-5 w-5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4" /><path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></svg>
				{:else}
					<svg viewBox="0 0 24 24" class="text-signal h-5 w-5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" /></svg>
				{/if}
			</button>
		</div>
	</header>

	<!-- Scrollable content (min-h-0 so flex-1 can shrink + scroll internally) -->
	<main class="min-h-0 flex-1 overflow-y-auto overscroll-contain">
		{@render children()}
	</main>

	<!-- Bottom tab bar -->
	<nav
		class="border-line/70 bg-ink-2/90 z-20 grid shrink-0 grid-cols-5 border-t backdrop-blur-md"
		style="padding-bottom:env(safe-area-inset-bottom)"
	>
		{#each tabs as t (t.href)}
			{@const on = t.sheet ? moreOpen || anyMoreActive : activeTab(t.href, t.exact)}
			{#if t.sheet}
				<button onclick={() => (moreOpen = !moreOpen)} class="relative flex flex-col items-center gap-1 py-2.5">
					{#if on}<span class="bg-signal absolute top-0 h-[2px] w-7 rounded-full"></span>{/if}
					<svg viewBox="0 0 24 24" class="h-[22px] w-[22px] {on ? 'text-signal' : 'text-fg-faint'}" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d={icons[t.icon]} /></svg>
					<span class="text-[0.62rem] font-medium whitespace-nowrap {on ? 'text-signal' : 'text-fg-faint'}">{t.label}</span>
				</button>
			{:else}
				<a href={t.href} class="relative flex flex-col items-center gap-1 py-2.5">
					{#if on}<span class="bg-signal absolute top-0 h-[2px] w-7 rounded-full"></span>{/if}
					<svg viewBox="0 0 24 24" class="h-[22px] w-[22px] {on ? 'text-signal' : 'text-fg-faint'}" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d={icons[t.icon]} /></svg>
					<span class="text-[0.62rem] font-medium whitespace-nowrap {on ? 'text-signal' : 'text-fg-faint'}">{t.label}</span>
				</a>
			{/if}
		{/each}
	</nav>

	<!-- Install nudge (once / 30 days while not installed); floats above the tab bar -->
	<PwaInstall />

	<!-- Styled confirm()/alert() replacement -->
	<ConfirmDialog />

	<!-- More sheet (absolute within the fixed viewport root → unambiguous anchor) -->
	{#if moreOpen}
		<button class="absolute inset-0 z-30 bg-black/50" aria-label="Close menu" onclick={() => (moreOpen = false)}></button>
		<div
			class="border-line bg-ink-2 animate-sheet absolute inset-x-0 bottom-0 z-40 rounded-t-2xl border-t px-3 pt-2"
			style="padding-bottom:calc(env(safe-area-inset-bottom) + 0.75rem)"
		>
		<div class="bg-line mx-auto mb-3 mt-1 h-1 w-10 rounded-full"></div>
		<div class="grid grid-cols-1 gap-1">
			{#each more as m (m.href)}
				{@const on = activeTab(m.href)}
				<a href={m.href} class="flex items-center gap-3 rounded-xl px-3 py-3 {on ? 'bg-signal/10' : 'active:bg-line/40'}">
					<span class="border-line/60 grid h-10 w-10 shrink-0 place-items-center rounded-xl border {on ? 'bg-signal/15' : 'bg-panel'}">
						<svg viewBox="0 0 24 24" class="h-[20px] w-[20px] {on ? 'text-signal' : 'text-fg-dim'}" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d={icons[m.icon]} /></svg>
					</span>
					<span class="min-w-0">
						<span class="text-fg block text-sm font-600 {on ? '!text-signal' : ''}">{m.label}</span>
						<span class="text-fg-faint block text-xs">{m.desc}</span>
					</span>
				</a>
			{/each}
		</div>
	</div>
{/if}
</div>

<style>
	.animate-sheet {
		animation: sheet-up 0.22s cubic-bezier(0.2, 0.8, 0.2, 1);
	}
	@keyframes sheet-up {
		from {
			transform: translateY(100%);
		}
	}
</style>
