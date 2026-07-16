<script lang="ts">
	// Per-page SEO. Overrides the crawl-time defaults baked into app.html once the
	// SPA renders (Googlebot picks these up on its JavaScript pass, and they set the
	// browser tab title). Keep titles under ~60 chars and descriptions ~150–160.
	import { SITE_NAME, SITE_URL } from '$lib/site';

	let {
		title,
		description,
		path = ''
	}: { title: string; description: string; path?: string } = $props();

	// Suffix the brand unless the title already carries it.
	const fullTitle = $derived(
		title.includes(SITE_NAME) ? title : `${title} · ${SITE_NAME}`
	);
	// Absolute when a site URL is configured, otherwise path-relative.
	const canonical = $derived(`${SITE_URL}${path}`);
</script>

<svelte:head>
	<title>{fullTitle}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href={canonical} />
	<meta property="og:title" content={fullTitle} />
	<meta property="og:description" content={description} />
	<meta property="og:url" content={canonical} />
	<meta name="twitter:title" content={fullTitle} />
	<meta name="twitter:description" content={description} />
</svelte:head>
