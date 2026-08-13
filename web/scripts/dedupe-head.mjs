// Post-build: strip the generic fallback metadata from PRERENDERED pages.
//
// app.html carries a generic title/description/og/twitter block after
// %sveltekit.head% so the SPA fallback shell (rendered with ssr=false, hence no
// route metadata of its own) still unfurls sensibly. Prerendered routes DO emit
// their own set via Seo.svelte, so on those files the generic block is a
// duplicate — and link unfurlers that take the first match, or the last, can
// end up showing the wrong card either way.
//
// This removes the generic copy from any built HTML that already has a
// route-specific equivalent, leaving exactly one authoritative set.
// See issue #1 on jjkroell/Ridgeline-public.
import { readdir, readFile, writeFile } from 'node:fs/promises';
import { join } from 'node:path';

const BUILD = 'build';
const KEYS = [
	{ re: /<title>[\s\S]*?<\/title>/g, name: 'title' },
	{ re: /<meta\s+name="description"[\s\S]*?\/>/g, name: 'description' },
	{ re: /<meta\s+property="og:title"[\s\S]*?\/>/g, name: 'og:title' },
	{ re: /<meta\s+property="og:description"[\s\S]*?\/>/g, name: 'og:description' },
	{ re: /<meta\s+name="twitter:title"[\s\S]*?\/>/g, name: 'twitter:title' },
	{ re: /<meta\s+name="twitter:description"[\s\S]*?\/>/g, name: 'twitter:description' }
];

async function* htmlFiles(dir) {
	for (const e of await readdir(dir, { withFileTypes: true })) {
		const p = join(dir, e.name);
		if (e.isDirectory()) yield* htmlFiles(p);
		else if (e.name.endsWith('.html')) yield p;
	}
}

let changed = 0;
for await (const file of htmlFiles(BUILD)) {
	let html = await readFile(file, 'utf8');
	const headEnd = html.indexOf('</head>');
	if (headEnd === -1) continue;
	let head = html.slice(0, headEnd);
	const rest = html.slice(headEnd);
	let touched = false;
	for (const { re } of KEYS) {
		const hits = head.match(re);
		// Only act when duplicated. The LAST occurrence is the app.html fallback,
		// because it sits after %sveltekit.head%; drop it and keep the route's own.
		if (!hits || hits.length < 2) continue;
		const last = hits[hits.length - 1];
		head = head.slice(0, head.lastIndexOf(last)) + head.slice(head.lastIndexOf(last) + last.length);
		touched = true;
	}
	if (touched) {
		await writeFile(file, head + rest);
		changed++;
		console.log(`  deduped ${file}`);
	}
}
console.log(`dedupe-head: ${changed} prerendered page(s) cleaned`);
