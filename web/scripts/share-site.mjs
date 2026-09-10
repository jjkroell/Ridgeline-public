// Non-secret build-time branding for the Go server's public link previews.
import { writeFile } from 'node:fs/promises';
import { loadEnv } from 'vite';
const env = loadEnv(process.env.NODE_ENV || 'production', process.cwd(), 'VITE_');
await writeFile('build/share-site.json', JSON.stringify({
  name: env.VITE_SITE_NAME || 'Ridgeline',
  url: (env.VITE_SITE_URL || '').replace(/\/+$/, ''),
}) + '\n');
