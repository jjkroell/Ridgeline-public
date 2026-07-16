// Web Worker that brute-forces MeshCore vanity keys off the main thread. The
// controller spawns one of these per CPU core and terminates them all once any
// worker reports a match (terminate is also how cancellation works — the search
// loop is synchronous and can't poll for a stop message mid-run).
import { searchVanity, hexToBytes } from './keygen';

interface StartMessage {
	prefixHex: string;
	batchSize?: number;
}

self.onmessage = (e: MessageEvent<StartMessage>) => {
	const { prefixHex, batchSize } = e.data;
	let prefixBytes: Uint8Array;
	try {
		prefixBytes = hexToBytes(prefixHex);
	} catch (err) {
		self.postMessage({ type: 'error', message: (err as Error).message });
		return;
	}

	const keypair = searchVanity(prefixBytes, {
		batchSize,
		shouldStop: () => false, // cancellation happens via worker.terminate()
		onBatch: (tried) => self.postMessage({ type: 'progress', tried })
	});

	if (keypair) self.postMessage({ type: 'found', keypair });
};
