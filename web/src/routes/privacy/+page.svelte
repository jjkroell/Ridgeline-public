<script lang="ts">
	import Seo from '$lib/components/Seo.svelte';
	import { consent } from '$lib/consent.svelte';
	import { ANALYTICS_ENABLED, ANALYTICS_PROVIDER } from '$lib/analytics';
	import { SITE_NAME, PRIVACY_CONTACT } from '$lib/site';
</script>

<Seo
	title="Privacy & Cookie Policy"
	description={`How ${SITE_NAME} uses cookies and local storage, and your choices under GDPR and PIPEDA.`}
	path="/privacy"
/>

<article class="mx-auto max-w-3xl px-6 py-12 leading-relaxed">
	<header class="mb-8">
		<p class="label text-signal mb-2">Legal</p>
		<h1 class="font-display text-fg text-3xl font-900 tracking-tight sm:text-4xl">
			Privacy &amp; Cookie Policy
		</h1>
	</header>

	<section class="text-fg-dim space-y-4">
		<p>
			{SITE_NAME} is an observatory for a MeshCore LoRa radio mesh; it shows publicly broadcast radio
			activity. This page explains what it stores on your device, what it does with any data, and the
			choices you have under the EU/UK <strong>GDPR</strong> and Canada's <strong>PIPEDA</strong>.
			The operator of this site is the data controller{#if PRIVACY_CONTACT}, reachable at
				<a href="mailto:{PRIVACY_CONTACT}" class="text-signal hover:underline">{PRIVACY_CONTACT}</a
				>{/if}.
		</p>
	</section>

	<section class="text-fg-dim mt-10 space-y-4">
		<h2 class="text-fg text-xl font-700">Cookies &amp; local storage we use</h2>
		<div class="not-prose border-line/70 overflow-x-auto rounded-[var(--radius)] border">
			<table class="w-full text-left text-sm">
				<thead class="border-line/70 text-fg-faint border-b">
					<tr>
						<th class="px-4 py-2.5 font-600">Name</th>
						<th class="px-4 py-2.5 font-600">Type</th>
						<th class="px-4 py-2.5 font-600">Purpose</th>
						<th class="px-4 py-2.5 font-600">Retention</th>
					</tr>
				</thead>
				<tbody class="divide-line/40 text-fg-dim divide-y">
					<tr>
						<td class="px-4 py-2.5 font-mono text-xs">rl_session</td>
						<td class="px-4 py-2.5">Necessary cookie</td>
						<td class="px-4 py-2.5">Keeps you signed in (only set if you create an account and log in). HttpOnly, Secure, SameSite=Lax.</td>
						<td class="px-4 py-2.5">Up to 30 days</td>
					</tr>
					<tr>
						<td class="px-4 py-2.5 font-mono text-xs">rl_csrf</td>
						<td class="px-4 py-2.5">Necessary cookie</td>
						<td class="px-4 py-2.5">Protects signed-in actions against cross-site request forgery.</td>
						<td class="px-4 py-2.5">Up to 30 days</td>
					</tr>
					<tr>
						<td class="px-4 py-2.5 font-mono text-xs">ridgeline-*</td>
						<td class="px-4 py-2.5">Functional (localStorage)</td>
						<td class="px-4 py-2.5">Remembers your preferences on this device — theme, favourites, map basemap, dashboard layout, sound and channel settings. Never sent to any server.</td>
						<td class="px-4 py-2.5">Until you clear it</td>
					</tr>
					<tr>
						<td class="px-4 py-2.5 font-mono text-xs">ridgeline-consent</td>
						<td class="px-4 py-2.5">Necessary (localStorage)</td>
						<td class="px-4 py-2.5">Remembers your cookie choice so we don't ask again.</td>
						<td class="px-4 py-2.5">Until you clear it</td>
					</tr>
					{#if ANALYTICS_ENABLED}
						<tr>
							<td class="px-4 py-2.5 font-mono text-xs">{ANALYTICS_PROVIDER}</td>
							<td class="px-4 py-2.5">Analytics (opt-in)</td>
							<td class="px-4 py-2.5">Privacy-respecting usage statistics — loaded <strong>only</strong> if you accept analytics.</td>
							<td class="px-4 py-2.5">See provider</td>
						</tr>
					{/if}
				</tbody>
			</table>
		</div>
		{#if !ANALYTICS_ENABLED}
			<p class="text-fg-faint text-sm">This deployment loads no analytics or third-party tracking of any kind.</p>
		{/if}
	</section>

	<section class="text-fg-dim mt-10 space-y-4">
		<h2 class="text-fg text-xl font-700">Accounts &amp; the data you provide</h2>
		<p>
			Accounts are optional. If you create one, we store your email address, a display name, and a
			hashed password so you can sign in, claim nodes, leave notes, and receive notifications you ask
			for. Email is used only for verification and the notifications you enable. You can edit your
			profile or <strong>delete your account</strong> at any time from your account page; deleting it
			removes your personal data and releases any nodes you had claimed.
		</p>
	</section>

	<section class="text-fg-dim mt-10 space-y-4">
		<h2 class="text-fg text-xl font-700">Legal bases &amp; your rights</h2>
		<p>
			Under the <strong>GDPR</strong>, we rely on your <strong>consent</strong> for any analytics; on
			<strong>legitimate interests</strong>/performance of a service for strictly-necessary cookies and
			local preferences; and on <strong>consent / contract</strong> for account data. Under
			<strong>PIPEDA</strong>, we collect only what's needed for the purposes above and ask for
			meaningful consent.
		</p>
		<p>
			You have the right to access, correct, or delete your personal data, to withdraw consent, and
			to complain to a supervisory authority (in Canada, the Office of the Privacy Commissioner).
			{#if PRIVACY_CONTACT}
				To exercise any of these, or with any question, contact
				<a href="mailto:{PRIVACY_CONTACT}" class="text-signal hover:underline">{PRIVACY_CONTACT}</a>.
			{:else}
				To exercise any of these, contact the operator of this site.
			{/if}
		</p>
	</section>

	<section class="text-fg-dim mt-10 space-y-4">
		<h2 class="text-fg text-xl font-700">Your choices</h2>
		<p>
			You can change your cookie choice at any time, or clear all locally-stored preferences from
			your browser's site settings.
		</p>
		<div class="not-prose flex flex-wrap gap-3 pt-1">
			<button
				onclick={() => consent.reopen()}
				class="bg-signal/15 text-signal border-signal/40 hover:bg-signal/25 rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors"
				>Manage cookie preferences</button
			>
			<a
				href="/"
				class="border-line text-fg-dim hover:border-line-bright hover:text-fg rounded-[var(--radius)] border px-4 py-2 text-sm font-600 transition-colors"
				>Back to the dashboard</a
			>
		</div>
	</section>
</article>
