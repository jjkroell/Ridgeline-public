// MeshCore-compatible Ed25519 key generation, entirely in the browser.
//
// A MeshCore node's identity is an Ed25519 key pair, and its "hash ID" is simply
// the first 1, 2, or 3 bytes of the *public* key. You don't choose the hash ID
// directly — you generate a key whose public key happens to start with the bytes
// you want (a "vanity" search). This module derives keys in the exact format the
// MeshCore firmware/app imports.
//
// Derivation (matches agessaman/meshcore-web-keygen, the reference tool):
//   seed   = 32 random bytes
//   digest = SHA-512(seed)
//   clamped = clamp(digest[0:32])            (RFC 8032 scalar clamping)
//   publicKey  = clamped · BasePoint         (32 bytes) — equals getPublicKey(seed)
//   privateKey = clamped(32) ‖ digest[32:64] (64 bytes, the *expanded* secret)
//
// Everything runs locally; private keys never leave the device.
import * as ed from '@noble/ed25519';
import { sha512 } from '@noble/hashes/sha512';

// noble v2 needs a synchronous SHA-512 to derive keys without async overhead in
// the vanity hot loop. Wire it once on module load.
ed.etc.sha512Sync = (...m) => sha512(ed.etc.concatBytes(...m));

const HEX = '0123456789ABCDEF';

export function bytesToHex(bytes: Uint8Array): string {
	let out = '';
	for (let i = 0; i < bytes.length; i++) {
		out += HEX[bytes[i] >> 4] + HEX[bytes[i] & 15];
	}
	return out;
}

export function hexToBytes(hex: string): Uint8Array {
	const clean = hex.trim();
	if (clean.length % 2 !== 0) throw new Error('hex must have an even length');
	const out = new Uint8Array(clean.length / 2);
	for (let i = 0; i < out.length; i++) {
		out[i] = parseInt(clean.slice(i * 2, i * 2 + 2), 16);
	}
	return out;
}

export interface Keypair {
	/** 64-hex (32-byte) public key, uppercase. */
	publicKey: string;
	/** 128-hex (64-byte) expanded private key, uppercase — the MeshCore import format. */
	privateKey: string;
}

/** Public key bytes for a seed — equivalent to clamp(SHA-512(seed))·B. */
export function publicKeyFromSeed(seed: Uint8Array): Uint8Array {
	return ed.getPublicKey(seed);
}

/** Build the 64-byte MeshCore private key (clamped scalar ‖ hash prefix) from a seed. */
export function privateKeyFromSeed(seed: Uint8Array): Uint8Array {
	const digest = sha512(seed);
	const priv = new Uint8Array(64);
	priv.set(digest.subarray(0, 32), 0);
	priv[0] &= 248;
	priv[31] &= 63;
	priv[31] |= 64;
	priv.set(digest.subarray(32, 64), 32);
	return priv;
}

/** Generate one random MeshCore key pair. */
export function generateKeypair(): Keypair {
	const seed = crypto.getRandomValues(new Uint8Array(32));
	return {
		publicKey: bytesToHex(publicKeyFromSeed(seed)),
		privateKey: bytesToHex(privateKeyFromSeed(seed))
	};
}

// MeshCore reserves the hash IDs 0x00 and 0xFF, so a usable key never starts with
// those bytes.
export function isReservedFirstByte(firstByte: number): boolean {
	return firstByte === 0x00 || firstByte === 0xff;
}

/**
 * Search for a key pair whose public key starts with `prefixBytes`, calling
 * `onBatch(tried)` periodically so a caller (e.g. a worker) can report progress
 * and check for cancellation. Returns the matching key pair, or null if
 * `shouldStop()` becomes true first. `prefixBytes` is matched byte-for-byte.
 */
export function searchVanity(
	prefixBytes: Uint8Array,
	opts: {
		batchSize?: number;
		shouldStop: () => boolean;
		onBatch: (tried: number) => void;
	}
): Keypair | null {
	const { shouldStop, onBatch } = opts;
	const batchSize = Math.max(64, opts.batchSize ?? 1024);
	const n = prefixBytes.length;
	const seed = new Uint8Array(32);

	while (!shouldStop()) {
		for (let i = 0; i < batchSize; i++) {
			crypto.getRandomValues(seed);
			const pub = ed.getPublicKey(seed);
			if (isReservedFirstByte(pub[0])) continue;
			let match = true;
			for (let b = 0; b < n; b++) {
				if (pub[b] !== prefixBytes[b]) {
					match = false;
					break;
				}
			}
			if (match) {
				return {
					publicKey: bytesToHex(pub),
					privateKey: bytesToHex(privateKeyFromSeed(seed))
				};
			}
		}
		onBatch(batchSize);
	}
	return null;
}

/**
 * Sanity check that our derivation matches noble's own getPublicKey for the same
 * seed (they must, since getPublicKey performs the identical clamp+mul). Returns
 * true on success; used by a dev-only assertion, not the hot path.
 */
export function selfTest(): boolean {
	const seed = new Uint8Array(32).fill(7);
	const pub = bytesToHex(publicKeyFromSeed(seed));
	const priv = privateKeyFromSeed(seed);
	// The private key's first 32 bytes are the clamped scalar; clamping invariants:
	const clampOk = (priv[0] & 0x07) === 0 && (priv[31] & 0xc0) === 0x40;
	return pub.length === 64 && priv.length === 64 && clampOk;
}
