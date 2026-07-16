// The user's favorite nodes, persisted to localStorage. Like channels and
// theme, this lives only in the browser — no backend or account needed. A
// favorite is a node public key the user wants kept front and center (their own
// nodes, or ones that matter to their local mesh).
const KEY = 'ridgeline-favorites';

class Favorites {
	// Favorited node public keys, uppercase hex.
	keys = $state<string[]>([]);

	init() {
		try {
			const raw = localStorage.getItem(KEY);
			if (raw) {
				const parsed = JSON.parse(raw);
				if (Array.isArray(parsed)) {
					this.keys = parsed
						.filter((k) => typeof k === 'string' && k)
						.map((k) => k.toUpperCase());
				}
			}
		} catch {
			/* storage unavailable or malformed */
		}
	}

	has(pubkey: string): boolean {
		return this.keys.includes((pubkey ?? '').toUpperCase());
	}

	get count(): number {
		return this.keys.length;
	}

	toggle(pubkey: string) {
		const k = (pubkey ?? '').toUpperCase();
		if (!k) return;
		this.keys = this.has(k) ? this.keys.filter((x) => x !== k) : [...this.keys, k];
		this.#persist();
	}

	remove(pubkey: string) {
		const k = (pubkey ?? '').toUpperCase();
		this.keys = this.keys.filter((x) => x !== k);
		this.#persist();
	}

	#persist() {
		try {
			localStorage.setItem(KEY, JSON.stringify(this.keys));
		} catch {
			/* storage unavailable */
		}
	}
}

export const favorites = new Favorites();
