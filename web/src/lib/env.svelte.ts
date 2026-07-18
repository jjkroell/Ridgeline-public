// Instance role, read once from /api/health. The server reports `environment`
// (e.g. "dev") only when its config sets it — production and self-hosted
// instances leave it unset, so isDev is false there and no banner shows. This
// signal is config-driven, not build-driven, so the same web bundle is safe to
// ship everywhere; only a box whose config opts in ever flags itself.
class Env {
	/** Raw environment string from the server; '' means a normal/production box. */
	environment = $state('');
	#started = false;

	async init() {
		if (this.#started) return;
		this.#started = true;
		try {
			const r = await fetch('/api/health');
			if (r.ok) {
				const j = await r.json();
				this.environment = typeof j.environment === 'string' ? j.environment : '';
			}
		} catch {
			// Health unreachable — assume a normal instance; never show the banner
			// on a transient error.
		}
	}

	/** A non-production instance that should carry a visible banner. */
	get isDev(): boolean {
		return this.environment === 'dev' || this.environment === 'staging';
	}

	/** Short uppercase label for the banner, e.g. "DEV". */
	get label(): string {
		return this.environment.toUpperCase();
	}
}

export const env = new Env();
