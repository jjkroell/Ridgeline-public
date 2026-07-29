// How the live feed labels a packet's time: elapsed ("2m") or wall clock
// ("16:47:12"). Persisted to localStorage, same pattern as theme/favorites.
//
// The store also owns a ticking clock. ago() is computed during render, so a
// relative label only refreshed when something else re-rendered the list —
// which incoming packets happened to do. Pause the feed, or let a quiet mesh
// go idle, and every "2m" silently went stale. Reading `now` here makes the
// relative labels update on their own.
import { agoFrom, clockTime } from '$lib/format';

export type TimeMode = 'relative' | 'clock';

const KEY = 'ridgeline-time-mode';
const TICK_MS = 10_000;

class TimeModeStore {
	mode = $state<TimeMode>('relative');
	#now = $state(Date.now());
	#timer: ReturnType<typeof setInterval> | null = null;

	init() {
		let saved: string | null = null;
		try {
			saved = localStorage.getItem(KEY);
		} catch {
			/* storage unavailable */
		}
		if (saved === 'clock' || saved === 'relative') this.mode = saved;
		this.#start();
	}

	set(mode: TimeMode) {
		if (mode === this.mode) return;
		this.mode = mode;
		try {
			localStorage.setItem(KEY, mode);
		} catch {
			/* storage unavailable */
		}
	}

	toggle() {
		this.set(this.mode === 'relative' ? 'clock' : 'relative');
	}

	/** The label for a feed row's timestamp, in whichever mode is active. */
	label(iso?: string): string {
		return this.mode === 'clock' ? clockTime(iso) : agoFrom(iso, this.#now);
	}

	#start() {
		// Only the relative mode needs the tick, but keeping it running means
		// switching modes doesn't have to start/stop anything.
		if (this.#timer === null) {
			this.#timer = setInterval(() => (this.#now = Date.now()), TICK_MS);
		}
	}
}

export const timeMode = new TimeModeStore();
