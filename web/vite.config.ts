import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		// Set VITE_ALLOWED_HOSTS to a comma-separated list (e.g. ".example.com")
		// to reach the dev server through a reverse proxy; localhost always works.
		allowedHosts: (
			(globalThis as { process?: { env?: Record<string, string> } }).process?.env
				?.VITE_ALLOWED_HOSTS ?? ''
		)
			.split(',')
			.map((h) => h.trim())
			.filter(Boolean),
		// Proxy API + live WebSocket to the ridgelined daemon during dev.
		// Override the daemon address with RIDGELINE_API when the dev server
		// itself runs on the daemon's default port.
		proxy: {
			'/api': {
				// Read from the environment without depending on @types/node.
				target:
					(globalThis as { process?: { env?: Record<string, string> } }).process?.env
						?.RIDGELINE_API ?? 'http://localhost:8080',
				ws: true
			}
		}
	}
});
