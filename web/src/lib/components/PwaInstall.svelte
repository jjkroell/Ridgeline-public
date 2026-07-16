<!--
  Install nudge for the mobile PWA (manifest scope is /m). Shown at most once every
  30 days while the app isn't already installed:
    • Chromium/Android — capture beforeinstallprompt and fire the native install
      dialog from our own button.
    • iOS Safari — Apple never fires beforeinstallprompt and offers no programmatic
      install, so we show Add-to-Home-Screen instructions instead.
  The 30-day clock starts when the nudge is SHOWN, so it's once-per-period
  regardless of whether the user installs, dismisses, or ignores it.
-->
<script lang="ts">
	import { onMount } from 'svelte';

	const KEY = 'ridgeline-pwa-prompt';
	const PERIOD = 30 * 24 * 60 * 60 * 1000; // 30 days

	/* eslint-disable @typescript-eslint/no-explicit-any */
	let deferred: any = null; // the captured BeforeInstallPromptEvent
	/* eslint-enable @typescript-eslint/no-explicit-any */
	let show = $state(false);
	let mode = $state<'install' | 'ios'>('install');

	const isStandalone = () =>
		window.matchMedia?.('(display-mode: standalone)').matches ||
		(navigator as unknown as { standalone?: boolean }).standalone === true;

	const isIOS = () =>
		/iphone|ipad|ipod/i.test(navigator.userAgent) ||
		// iPadOS 13+ reports as MacIntel but is touch-capable
		(navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);

	function dueAgain(): boolean {
		try {
			return Date.now() - Number(localStorage.getItem(KEY) || 0) > PERIOD;
		} catch {
			return false; // storage blocked — don't nag
		}
	}
	function stamp() {
		try {
			localStorage.setItem(KEY, String(Date.now()));
		} catch {
			/* ignore */
		}
	}
	function present(m: 'install' | 'ios') {
		mode = m;
		show = true;
		stamp();
	}

	function dismiss() {
		show = false;
	}

	async function install() {
		show = false;
		if (!deferred) return;
		deferred.prompt();
		await deferred.userChoice;
		deferred = null;
	}

	onMount(() => {
		if (isStandalone()) return; // already installed — never nudge

		const onBip = (e: Event) => {
			e.preventDefault(); // suppress Chrome's own mini-infobar; we drive it
			deferred = e;
			if (!isStandalone() && dueAgain()) present('install');
		};
		const onInstalled = () => {
			show = false;
			deferred = null;
			stamp();
		};
		window.addEventListener('beforeinstallprompt', onBip);
		window.addEventListener('appinstalled', onInstalled);

		// iOS never fires beforeinstallprompt — offer instructions on the same cadence
		// (delayed a touch so it doesn't slam the user the instant the app opens).
		let t: ReturnType<typeof setTimeout> | undefined;
		if (isIOS() && dueAgain()) t = setTimeout(() => !isStandalone() && present('ios'), 3500);

		return () => {
			window.removeEventListener('beforeinstallprompt', onBip);
			window.removeEventListener('appinstalled', onInstalled);
			clearTimeout(t);
		};
	});
</script>

{#if show}
	<div
		class="border-line bg-ink-2/95 absolute inset-x-3 z-30 flex items-center gap-3 rounded-2xl border p-3 shadow-xl backdrop-blur-md"
		style="bottom:calc(env(safe-area-inset-bottom) + 4.75rem)"
		role="dialog"
		aria-label="Install Ridgeline"
	>
		<img src="/icons/icon-192.png" alt="" class="border-line/60 h-10 w-10 shrink-0 rounded-xl border" />
		<div class="min-w-0 flex-1">
			{#if mode === 'install'}
				<div class="text-fg text-sm font-600">Install Ridgeline</div>
				<div class="text-fg-faint text-xs leading-snug">
					Add it to your home screen for a full-screen app.
				</div>
			{:else}
				<div class="text-fg text-sm font-600">Add to Home Screen</div>
				<div class="text-fg-faint text-xs leading-snug">
					Tap
					<svg viewBox="0 0 24 24" class="text-signal -mt-0.5 inline h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="M12 16V4m0 0L8 8m4-4 4 4M5 12v6a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-6" /></svg>
					then “Add to Home Screen”.
				</div>
			{/if}
		</div>
		{#if mode === 'install'}
			<button
				onclick={install}
				class="border-signal/40 bg-signal/15 text-signal shrink-0 rounded-lg border px-3 py-1.5 text-xs font-600"
				>Install</button
			>
		{/if}
		<button onclick={dismiss} aria-label="Dismiss" class="text-fg-faint hover:text-fg shrink-0 text-lg leading-none">×</button>
	</div>
{/if}
