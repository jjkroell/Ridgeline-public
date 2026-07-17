<script lang="ts">
	// Cookie-consent banner. Shows once until the visitor decides; re-openable from
	// /privacy via consent.reopen(). When this build ships analytics it offers a
	// granular opt-in (Necessary always on, Analytics optional); otherwise it's a
	// slim "essential cookies only" disclosure. Reject is exactly as easy as Accept.
	import { consent } from '$lib/consent.svelte';
	import { ANALYTICS_ENABLED, ANALYTICS_PROVIDER } from '$lib/analytics';

	let showPrefs = $state(false);
	let analyticsPref = $state(false);

	function openPrefs() {
		analyticsPref = consent.analytics; // seed toggle from the current choice
		showPrefs = true;
	}
	function close() {
		showPrefs = false;
	}
</script>

{#if consent.open}
	<div
		class="fixed inset-x-0 bottom-0 z-[1000] flex justify-center px-3 pb-3"
		role="region"
		aria-label="Cookie consent"
	>
		<div
			class="panel border-line/80 bg-ink-2/95 w-full max-w-3xl rounded-[var(--radius)] border p-4 shadow-2xl backdrop-blur-sm sm:p-5"
		>
			{#if !ANALYTICS_ENABLED}
				<!-- No non-essential cookies: a plain disclosure with a single ack. -->
				<div class="flex flex-col gap-3 sm:flex-row sm:items-center">
					<p class="text-fg-dim min-w-0 flex-1 text-sm leading-relaxed">
						This site uses only <strong class="text-fg">essential cookies</strong> (to keep you signed
						in) and stores your preferences — like theme and favourites — locally in your browser. No
						tracking, no third parties.
						<a href="/privacy" class="text-signal hover:underline">Privacy &amp; cookies</a>.
					</p>
					<button
						onclick={() => consent.acceptAll()}
						class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 shrink-0 rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors"
						>Got it</button
					>
				</div>
			{:else if !showPrefs}
				<!-- Summary view: reject / customize / accept, all one click. -->
				<div class="flex flex-col gap-3">
					<p class="text-fg-dim text-sm leading-relaxed">
						We use <strong class="text-fg">essential cookies</strong> to keep you signed in and store
						your preferences locally. With your permission we also use
						<strong class="text-fg">{ANALYTICS_PROVIDER}</strong> analytics to understand how the site
						is used. See our
						<a href="/privacy" class="text-signal hover:underline">Privacy &amp; Cookie Policy</a>.
					</p>
					<div class="flex flex-wrap items-center gap-2">
						<button
							onclick={() => consent.rejectNonEssential()}
							class="border-line text-fg-dim hover:border-line-bright hover:text-fg rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors"
							>Reject non-essential</button
						>
						<button
							onclick={openPrefs}
							class="border-line text-fg-dim hover:border-line-bright hover:text-fg rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors"
							>Customize</button
						>
						<button
							onclick={() => consent.acceptAll()}
							class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 ml-auto rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors"
							>Accept all</button
						>
					</div>
				</div>
			{:else}
				<!-- Customize view: granular toggles. -->
				<div class="flex flex-col gap-3">
					<h2 class="font-display text-fg text-sm font-700 tracking-wide">COOKIE PREFERENCES</h2>
					<label class="border-line/60 flex items-start gap-3 rounded-[var(--radius)] border px-3 py-2.5 opacity-80">
						<input type="checkbox" checked disabled class="mt-0.5 accent-[var(--color-signal)]" />
						<span class="min-w-0">
							<span class="text-fg block text-sm font-600">Necessary <span class="text-fg-faint font-400">· always on</span></span>
							<span class="text-fg-dim block text-xs leading-relaxed">Sign-in session &amp; security (CSRF), plus your local UI preferences. Required for the site to work.</span>
						</span>
					</label>
					<label class="border-line/60 hover:border-line-bright flex cursor-pointer items-start gap-3 rounded-[var(--radius)] border px-3 py-2.5 transition-colors">
						<input type="checkbox" bind:checked={analyticsPref} class="mt-0.5 accent-[var(--color-signal)]" />
						<span class="min-w-0">
							<span class="text-fg block text-sm font-600">Analytics <span class="text-fg-faint font-400">· {ANALYTICS_PROVIDER}</span></span>
							<span class="text-fg-dim block text-xs leading-relaxed">Cookieless, privacy-respecting usage stats (page views, referrer, rough region). Helps improve the site. Off by default.</span>
						</span>
					</label>
					<div class="flex flex-wrap items-center gap-2">
						<button
							onclick={close}
							class="border-line text-fg-dim hover:border-line-bright hover:text-fg rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors"
							>Back</button
						>
						<button
							onclick={() => consent.save(analyticsPref)}
							class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 ml-auto rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors"
							>Save preferences</button
						>
					</div>
				</div>
			{/if}
		</div>
	</div>
{/if}
