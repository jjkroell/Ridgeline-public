// Sign a server-issued challenge with a MeshCore node's Ed25519 private key,
// entirely in the browser — the key never leaves the device. Used by the
// private-key node-ownership proof: the server sends a random challenge, we sign
// it here, and only the signature is returned for the server to verify against
// the node's public key.
//
// MeshCore stores the *expanded* secret key (clamped scalar ‖ SHA-512 prefix),
// not a 32-byte seed, so we can't use noble's seed-based `sign()`. We implement
// RFC 8032 Ed25519 signing directly from the expanded key. The result is an
// ordinary Ed25519 signature that Go's crypto/ed25519.Verify accepts (verified
// end-to-end against the server). We also accept a 32-byte seed for convenience
// by expanding it the same way the key generator does.
import * as ed from '@noble/ed25519';
import { sha512 } from '@noble/hashes/sha512';
import { hexToBytes, bytesToHex, privateKeyFromSeed } from './keygen';

// noble needs a synchronous SHA-512 for the sync point ops below.
ed.etc.sha512Sync = (...m) => sha512(ed.etc.concatBytes(...m));

const L = ed.CURVE.n; // curve order

function bytesToNumberLE(b: Uint8Array): bigint {
	let n = 0n;
	for (let i = b.length - 1; i >= 0; i--) n = (n << 8n) | BigInt(b[i]);
	return n;
}

function numberToBytesLE(n: bigint, len: number): Uint8Array {
	const out = new Uint8Array(len);
	for (let i = 0; i < len; i++) {
		out[i] = Number(n & 255n);
		n >>= 8n;
	}
	return out;
}

const mod = (a: bigint, m = L): bigint => ((a % m) + m) % m;

/**
 * Normalise pasted key input into a 64-byte expanded MeshCore secret. Accepts a
 * 128-hex expanded key (the MeshCore export format) or a 64-hex 32-byte seed
 * (expanded here). Tolerates whitespace and an optional 0x prefix.
 */
function toExpandedKey(input: string): Uint8Array {
	let hex = input.trim().replace(/^0x/i, '').replace(/\s+/g, '');
	if (!/^[0-9a-fA-F]*$/.test(hex) || hex.length === 0) {
		throw new Error('Private key must be hexadecimal.');
	}
	const bytes = hexToBytes(hex);
	if (bytes.length === 64) return bytes;
	if (bytes.length === 32) return privateKeyFromSeed(bytes);
	throw new Error('Expected a 64-byte (128-hex) MeshCore private key.');
}

/**
 * Sign `message` (UTF-8) with the given MeshCore private key and return the
 * 128-hex Ed25519 signature. Throws on malformed key input.
 */
export function signChallenge(privInput: string, message: string): string {
	const priv = toExpandedKey(privInput);
	const a = mod(bytesToNumberLE(priv.slice(0, 32))); // clamped scalar, reduced mod L
	const prefix = priv.slice(32, 64);
	const A = ed.Point.BASE.multiply(a).toRawBytes(); // this key's public key
	const M = new TextEncoder().encode(message);

	const r = mod(bytesToNumberLE(sha512(ed.etc.concatBytes(prefix, M))));
	if (r === 0n) throw new Error('Could not sign — try again.'); // astronomically unlikely
	const R = ed.Point.BASE.multiply(r).toRawBytes();
	const k = mod(bytesToNumberLE(sha512(ed.etc.concatBytes(R, A, M))));
	const S = mod(r + k * a);
	return bytesToHex(ed.etc.concatBytes(R, numberToBytesLE(S, 32)));
}

/** The public key (64-hex, uppercase) a given private key belongs to. */
export function publicKeyFor(privInput: string): string {
	const priv = toExpandedKey(privInput);
	const a = mod(bytesToNumberLE(priv.slice(0, 32)));
	return bytesToHex(ed.Point.BASE.multiply(a).toRawBytes());
}
