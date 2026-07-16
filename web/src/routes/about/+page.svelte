<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import Seo from '$lib/components/Seo.svelte';
	import { SITE_NAME, SITE_DESCRIPTION } from '$lib/site';
	import { ABOUT, RADIO_PARAMS } from '$lib/site-content';

	// If this deployment opted out of an About page, send visitors home.
	onMount(() => {
		if (!ABOUT.enabled) goto('/', { replaceState: true });
	});

	const showRadio = $derived(ABOUT.showRadio && RADIO_PARAMS.length > 0);
</script>

<Seo title={`About ${SITE_NAME}`} description={SITE_DESCRIPTION} path="/about" />

{#if ABOUT.enabled}
	<article class="mx-auto max-w-3xl px-6 py-12 leading-relaxed">
		<header class="mb-10">
			{#if ABOUT.kicker}
				<p class="label text-signal mb-2">{ABOUT.kicker}</p>
			{/if}
			<h1 class="font-display text-fg text-3xl font-900 tracking-tight sm:text-4xl">
				{ABOUT.title}
			</h1>
			{#if ABOUT.intro}
				<p class="text-fg-dim mt-4 text-lg">{ABOUT.intro}</p>
			{/if}
		</header>

		{#each ABOUT.sections as section, i (section.heading + i)}
			<section class="text-fg-dim {i === 0 ? '' : 'mt-10'} space-y-4">
				<h2 class="text-fg text-xl font-700">{section.heading}</h2>
				{#each section.paragraphs as p (p)}
					<p>{p}</p>
				{/each}
			</section>
		{/each}

		{#if showRadio}
			<section class="text-fg-dim mt-10 space-y-4">
				<h2 class="text-fg text-xl font-700">Radio settings</h2>
				<p>
					Every radio on the mesh speaks the same LoRa dialect. A radio that doesn't
					match all of these won't hear a thing, so if you're setting one up, copy
					them precisely.
				</p>
				<dl
					class="border-line/70 divide-line/60 not-prose my-2 divide-y overflow-hidden rounded-[var(--radius)] border"
				>
					{#each RADIO_PARAMS as row (row.k)}
						<div class="flex items-center justify-between gap-4 px-4 py-3">
							<dt class="text-fg-dim text-sm">{row.k}</dt>
							<dd class="text-signal font-mono text-sm font-600 tabular-nums">{row.v}</dd>
						</div>
					{/each}
				</dl>
			</section>
		{/if}

		{#if ABOUT.footer}
			<footer class="border-line text-fg-faint mt-12 border-t pt-6 text-sm">
				<p>
					{ABOUT.footer}
					<a href="/" class="text-signal hover:underline">Open the live dashboard →</a>
				</p>
			</footer>
		{/if}
	</article>
{/if}
