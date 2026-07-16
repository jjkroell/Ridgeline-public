import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	compilerOptions: { runes: true },
	kit: {
		// Single-page app: the Go daemon serves the built static files and
		// falls back to index.html so client routing handles every path.
		adapter: adapter({ fallback: 'index.html' }),
		// The /about page is prerendered (see its +page.ts). Its crawl follows links
		// in the shared app.html head/nav — don't fail the whole build on the
		// pre-existing /favicon.svg reference or on links to SPA-only (non-prerendered)
		// routes; those are served fine at runtime by the Go daemon's index.html fallback.
		prerender: {
			handleHttpError: 'warn',
			handleMissingId: 'warn'
		},
		alias: {
			$lib: 'src/lib'
		}
	}
};

export default config;
