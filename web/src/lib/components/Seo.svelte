<script lang="ts">
	// Per-page SEO. Go renders the initial public share metadata; this component
	// takes over once the SPA mounts and updates it during client navigation.
	import { SITE_NAME, SITE_URL } from '$lib/site';
	import { onMount } from 'svelte';

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

	// The Go server owns the initial no-JS head. Once this component mounts,
	// Svelte owns navigation metadata, so a previous node's image cannot linger.
	onMount(() => {
		for (const tag of document.head.querySelectorAll('[data-ridgeline-share]')) {
			// Svelte updates document.title in place; keep that title element.
			if (tag.tagName === 'TITLE') tag.removeAttribute('data-ridgeline-share');
			else tag.remove();
		}
	});
	const sharePath = $derived(path === '/m' ? '/' : path.replace(/^\/m\//, '/').replace(/\/$/, '') || '/');
	const hasCard = $derived(
		['/', '/nodes', '/live', '/channels', '/map', '/live-map', '/analytics', '/topology', '/identity', '/observers'].includes(sharePath)
		|| /^\/nodes\/[a-fA-F0-9]{64}$/.test(sharePath)
	);
	const image = $derived(hasCard
		? `${SITE_URL}/share/card.png?path=${encodeURIComponent(sharePath)}`
		: `${SITE_URL}/icons/icon-512.png`);
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
	<meta property="og:type" content="website" />
	<meta property="og:site_name" content={SITE_NAME} />
	<meta property="og:image" content={image} />
	<meta property="og:image:alt" content={`${title} — ${description}`} />
	<meta name="twitter:card" content={hasCard ? 'summary_large_image' : 'summary'} />
	<meta name="twitter:image" content={image} />
	<meta name="twitter:image:alt" content={`${title} — ${description}`} />
</svelte:head>
