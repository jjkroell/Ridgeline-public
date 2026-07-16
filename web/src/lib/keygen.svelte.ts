// Drives the vanity key search across every CPU core and exposes live progress
// as runes. The actual Ed25519 work runs in keygen-worker.ts instances; this
// just fans out the job, aggregates attempt counts, and stops everything the
// moment one worker finds a match.
import type { Keypair } from './keygen';

type WorkerMsg =
	| { type: 'progress'; tried: number }
	| { type: 'found'; keypair: Keypair }
	| { type: 'error'; message: string };

class KeygenController {
	running = $state(false);
	attempts = $state(0);
	elapsedMs = $state(0);
	result = $state<Keypair | null>(null);
	error = $state<string | null>(null);
	/** The prefix currently being / last searched for, uppercase hex. */
	prefixHex = $state('');
	workerCount = $state(0);

	#workers: Worker[] = [];
	#timer: ReturnType<typeof setInterval> | null = null;
	#startedAt = 0;

	/** Estimated keys/second across all workers. */
	get rate(): number {
		return this.elapsedMs > 0 ? (this.attempts / this.elapsedMs) * 1000 : 0;
	}

	start(prefixHex: string) {
		this.cancel();
		this.prefixHex = prefixHex.toUpperCase();
		this.attempts = 0;
		this.elapsedMs = 0;
		this.result = null;
		this.error = null;
		this.running = true;
		this.#startedAt = performance.now();

		const cores = navigator.hardwareConcurrency || 4;
		const count = Math.max(1, Math.min(cores, 16));
		this.workerCount = count;
		for (let i = 0; i < count; i++) {
			const w = new Worker(new URL('./keygen-worker.ts', import.meta.url), { type: 'module' });
			w.onmessage = (e: MessageEvent<WorkerMsg>) => this.#onMessage(e.data);
			w.onerror = (e) => {
				this.error = e.message || 'key generation worker failed';
				this.cancel();
			};
			w.postMessage({ prefixHex: this.prefixHex, batchSize: 1024 });
			this.#workers.push(w);
		}

		this.#timer = setInterval(() => {
			if (this.running) this.elapsedMs = performance.now() - this.#startedAt;
		}, 200);
	}

	#onMessage(msg: WorkerMsg) {
		if (!this.running) return;
		if (msg.type === 'progress') {
			this.attempts += msg.tried;
		} else if (msg.type === 'found') {
			this.elapsedMs = performance.now() - this.#startedAt;
			this.result = msg.keypair;
			this.running = false;
			this.#teardown();
		} else if (msg.type === 'error') {
			this.error = msg.message;
			this.cancel();
		}
	}

	cancel() {
		this.running = false;
		this.#teardown();
	}

	#teardown() {
		for (const w of this.#workers) w.terminate();
		this.#workers = [];
		if (this.#timer !== null) {
			clearInterval(this.#timer);
			this.#timer = null;
		}
	}

	/** Clear a finished result so the form returns to its idle state. */
	reset() {
		this.cancel();
		this.result = null;
		this.error = null;
		this.attempts = 0;
		this.elapsedMs = 0;
		this.prefixHex = '';
	}
}

export const keygen = new KeygenController();
