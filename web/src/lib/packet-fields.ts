// Client-side byte-level breakdown of a raw MeshCore packet, for the packet
// detail view. Computes field offsets/values purely from the raw hex (+ a few
// friendly values from the decoded node), mirroring the breakdown ranges in
// internal/meshcore. No backend round-trip.
import type { LiveEvent } from './api';

export interface PacketField {
	off: number | null; // byte offset into the packet (null = derived, non-positional)
	bytes: number; // field length in bytes
	label: string;
	value: string; // display value
	desc?: string; // muted explanatory note
	key: string; // color key (see FIELD_KEYS)
}

export interface ByteRange {
	start: number;
	end: number;
	key: string;
	label: string;
}

const up = (s: string) => s.toUpperCase();
const trunc = (s: string, n: number) => (s.length > n ? s.slice(0, n) + '…' : s);

/** Little-endian uint32 from 8 hex chars. */
function le32(h: string): number {
	if (h.length < 8) return 0;
	return (
		parseInt(h.slice(0, 2), 16) +
		parseInt(h.slice(2, 4), 16) * 0x100 +
		parseInt(h.slice(4, 6), 16) * 0x10000 +
		parseInt(h.slice(6, 8), 16) * 0x1000000
	);
}

/** Signed little-endian int32 from 8 hex chars. */
function s32le(h: string): number {
	const v = le32(h);
	return v >= 0x80000000 ? v - 0x100000000 : v;
}

function advTime(h: string): string {
	const t = le32(h);
	return t ? new Date(t * 1000).toISOString().replace('T', ' ').slice(0, 19) + 'Z' : '';
}

function latLon(known: number | undefined, h: string): string {
	const v = known != null ? known : s32le(h) / 1e6;
	return v.toFixed(5);
}

function asciiName(h: string): string {
	let s = '';
	for (let i = 0; i + 1 < h.length; i += 2) {
		const c = parseInt(h.slice(i, i + 2), 16);
		if (c === 0) break;
		if (c >= 32 && c < 127) s += String.fromCharCode(c);
	}
	return s;
}

/** Signed int8 from a 2-hex-char byte. */
function s8(h: string): number {
	const v = parseInt(h, 16);
	return isNaN(v) ? 0 : v >= 128 ? v - 256 : v;
}

function roleName(role: number): string {
	return { 1: 'Chat', 2: 'Repeater', 3: 'Room', 4: 'Sensor' }[role] ?? 'Unknown';
}

function flagsDesc(fb: number, role?: string): string {
	if (isNaN(fb)) return '';
	const parts: string[] = [];
	if (role) parts.push(role);
	if (fb & 0x10) parts.push('location');
	if (fb & 0x80) parts.push('name');
	return parts.join(' · ');
}

export interface TraceInfo {
	tag: string; // 4-byte trace tag, uppercase hex
	authCode: number;
	hashSize: number; // bytes per route-hash entry (1, 2, 4, or 8)
	routeHashes: string[]; // node hashes along the traced route, uppercase hex
	hopSnr: number[]; // per-hop SNR in dB, from the header path (signed int8 / 4)
}

/**
 * Parse a Trace (0x09) packet into its traced route and per-hop SNR. The two are
 * distinct, independently-sized things: routeHashes are the node hashes recorded
 * in the trace payload (resolve via public-key prefix), while hopSnr comes from
 * the header path — for a trace, each header-path byte is a signed int8 of SNR×4,
 * the signal quality at each flood hop as this observer received it. Returns null
 * for non-trace or malformed packets.
 */
export function parseTrace(ev: LiveEvent): TraceInfo | null {
	if (ev.payloadType !== 'Trace') return null;
	const hex = (ev.raw ?? '').replace(/\s+/g, '');
	const totalBytes = Math.floor(hex.length / 2);
	const slice = (b: number, n: number) => hex.slice(b * 2, (b + n) * 2);

	let off = 1; // header byte
	if (ev.transportCodes) off += 4;
	if (off >= totalBytes) return null;
	const pb = parseInt(slice(off, 1), 16);
	const hHashSize = isNaN(pb) ? 1 : (pb >> 6) + 1;
	const hHops = isNaN(pb) ? 0 : pb & 0x3f;
	off += 1 + hHashSize * hHops; // skip the header path

	const ps = off;
	if (totalBytes - ps < 9) return null;
	const flags = parseInt(slice(ps + 8, 1), 16);
	const ths = isNaN(flags) ? 1 : 1 << (flags & 0x03);
	const routeHashes: string[] = [];
	for (let to = ps + 9; to + ths <= totalBytes; to += ths) routeHashes.push(up(slice(to, ths)));

	// Header-path SNR is only meaningful when its entries are single bytes.
	const hopSnr = hHashSize === 1 ? (ev.path ?? []).map((h) => s8(h.slice(0, 2)) / 4) : [];

	return { tag: up(slice(ps, 4)), authCode: le32(slice(ps + 4, 4)), hashSize: ths, routeHashes, hopSnr };
}

/**
 * Break a packet's raw hex into labelled fields plus the byte ranges used to
 * colour the hex dump. Transport routes (detected via the presence of transport
 * codes) carry 4 extra bytes before the path-length byte.
 */
export function buildPacketFields(ev: LiveEvent): { fields: PacketField[]; ranges: ByteRange[] } {
	const hex = (ev.raw ?? '').replace(/\s+/g, '');
	const fields: PacketField[] = [];
	const ranges: ByteRange[] = [];
	if (hex.length < 2) return { fields, ranges };

	const totalBytes = Math.floor(hex.length / 2);
	const byte = (b: number) => hex.slice(b * 2, b * 2 + 2);
	const slice = (b: number, n: number) => hex.slice(b * 2, (b + n) * 2);
	const add = (
		off: number | null,
		bytes: number,
		label: string,
		value: string,
		key: string,
		desc?: string
	) => {
		fields.push({ off, bytes, label, value, key, desc });
		if (off != null && bytes > 0) ranges.push({ start: off, end: off + bytes - 1, key, label });
	};

	// Header byte
	add(0, 1, 'Header', '0x' + up(byte(0)), 'header', `Route: ${ev.routeType} · Payload: ${ev.payloadType}`);
	let off = 1;

	// Transport codes (transport routes only) precede the path-length byte.
	if (ev.transportCodes && totalBytes >= 5) {
		add(off, 2, 'Next Hop', up(slice(off, 2)), 'transport');
		add(off + 2, 2, 'Last Hop', up(slice(off + 2, 2)), 'transport');
		off += 4;
	}
	if (off >= totalBytes) return { fields, ranges };

	// Path-length byte: top 2 bits = hash size (1-3), low 6 bits = hop count.
	const pb = parseInt(byte(off), 16);
	const hashSize = isNaN(pb) ? 1 : (pb >> 6) + 1;
	const hashCount = isNaN(pb) ? 0 : pb & 0x3f;
	add(
		off,
		1,
		'Path Length',
		'0x' + up(byte(off)),
		'pathlen',
		hashCount === 0 ? 'hash_count=0 (direct)' : `hash_size=${hashSize}B · hash_count=${hashCount}`
	);
	off += 1;

	// Path hops
	const hops = ev.path ?? [];
	if (hops.length) {
		for (let i = 0; i < hops.length; i++) {
			add(off + i * hashSize, hashSize, `Hop ${i + 1}`, up(hops[i]), 'path');
		}
		off += hashSize * hops.length;
	} else if (hashCount > 0) {
		off += hashSize * hashCount; // advance past path bytes even if not enumerated
	}
	if (off >= totalBytes) return { fields, ranges };

	// Payload — sub-fields for the types we can position from raw bytes.
	const ps = off;
	if (ev.payloadType === 'Advert' && totalBytes - ps >= 100) {
		add(ps, 32, 'Public Key', trunc(up(slice(ps, 32)), 24), 'pubkey');
		add(ps + 32, 4, 'Timestamp', up(slice(ps + 32, 4)), 'timestamp', advTime(slice(ps + 32, 4)));
		add(ps + 36, 64, 'Signature', trunc(up(slice(ps + 36, 64)), 24), 'signature');
		const appStart = ps + 100;
		if (appStart < totalBytes) {
			const fb = parseInt(byte(appStart), 16);
			add(appStart, 1, 'App Flags', '0x' + up(byte(appStart)), 'flags', flagsDesc(fb, ev.node?.role));
			let fOff = appStart + 1;
			if (!isNaN(fb)) {
				if (fb & 0x10 && fOff + 8 <= totalBytes) {
					add(fOff, 4, 'Latitude', latLon(ev.node?.latitude, slice(fOff, 4)), 'location');
					add(fOff + 4, 4, 'Longitude', latLon(ev.node?.longitude, slice(fOff + 4, 4)), 'location');
					fOff += 8;
				}
				if (fb & 0x20 && fOff + 2 <= totalBytes) fOff += 2; // feature flags
				if (fb & 0x40 && fOff + 2 <= totalBytes) fOff += 2;
				if (fb & 0x80 && fOff < totalBytes) {
					add(fOff, totalBytes - fOff, 'Name', ev.node?.name || asciiName(hex.slice(fOff * 2)), 'name');
				}
			}
		}
	} else if (ev.payloadType === 'GroupText' && totalBytes - ps >= 3) {
		add(ps, 1, 'Channel Hash', '0x' + up(byte(ps)), 'channel');
		add(ps + 1, 2, 'MAC', up(slice(ps + 1, 2)), 'mac');
		add(ps + 3, totalBytes - (ps + 3), 'Encrypted Data', trunc(up(hex.slice((ps + 3) * 2)), 32), 'encrypted');
	} else if (
		// Direct (peer-to-peer) envelope shared by TextMessage, Request, Response,
		// and the encrypted Path return. Body needs the peers' shared secret.
		(ev.payloadType === 'TextMessage' ||
			ev.payloadType === 'Request' ||
			ev.payloadType === 'Response' ||
			ev.payloadType === 'Path') &&
		totalBytes - ps >= 4
	) {
		add(ps, 1, 'Dest Hash', '0x' + up(byte(ps)), 'hash', 'first byte of destination key');
		add(ps + 1, 1, 'Source Hash', '0x' + up(byte(ps + 1)), 'hash', 'first byte of source key');
		add(ps + 2, 2, 'MAC', up(slice(ps + 2, 2)), 'mac');
		add(ps + 4, totalBytes - (ps + 4), 'Encrypted Data', trunc(up(hex.slice((ps + 4) * 2)), 32), 'encrypted');
	} else if (ev.payloadType === 'AnonRequest' && totalBytes - ps >= 35) {
		add(ps, 1, 'Dest Hash', '0x' + up(byte(ps)), 'hash', 'first byte of destination key');
		add(ps + 1, 32, 'Sender Key', trunc(up(slice(ps + 1, 32)), 24), 'pubkey', 'sender public key (clear)');
		add(ps + 33, 2, 'MAC', up(slice(ps + 33, 2)), 'mac');
		add(ps + 35, totalBytes - (ps + 35), 'Encrypted Data', trunc(up(hex.slice((ps + 35) * 2)), 32), 'encrypted');
	} else if (ev.payloadType === 'Ack' && totalBytes - ps >= 4) {
		add(ps, 4, 'Checksum', up(slice(ps, 4)), 'checksum', 'CRC of the acked message');
	} else if (ev.payloadType === 'Trace' && totalBytes - ps >= 9) {
		add(ps, 4, 'Trace Tag', up(slice(ps, 4)), 'tag');
		add(ps + 4, 4, 'Auth Code', up(slice(ps + 4, 4)), 'timestamp', String(le32(slice(ps + 4, 4))));
		const fb = parseInt(byte(ps + 8), 16);
		const ths = isNaN(fb) ? 1 : 1 << (fb & 0x03);
		add(ps + 8, 1, 'Flags', '0x' + up(byte(ps + 8)), 'flags', `hash_size=${ths}B`);
		let to = ps + 9;
		for (let i = 1; to + ths <= totalBytes; i++, to += ths) {
			add(to, ths, `Trace Hop ${i}`, up(slice(to, ths)), 'path');
		}
	} else if (ev.payloadType === 'Control' && totalBytes - ps >= 1) {
		const fb = parseInt(byte(ps), 16);
		const sub = fb & 0xf0;
		if (sub === 0x90 && totalBytes - ps >= 14) {
			add(ps, 1, 'Flags', '0x' + up(byte(ps)), 'flags', `DiscoverResp · role ${roleName(fb & 0x0f)}`);
			add(ps + 1, 1, 'SNR', s8(byte(ps + 1)) / 4 + ' dB', 'snr');
			add(ps + 2, 4, 'Tag', up(slice(ps + 2, 4)), 'tag');
			add(ps + 6, totalBytes - (ps + 6), 'Node Key', trunc(up(hex.slice((ps + 6) * 2)), 24), 'pubkey');
		} else if (sub === 0x80 && totalBytes - ps >= 6) {
			add(ps, 1, 'Flags', '0x' + up(byte(ps)), 'flags', `DiscoverReq${fb & 0x01 ? ' · prefix-only' : ''}`);
			add(ps + 1, 1, 'Type Filter', '0x' + up(byte(ps + 1)), 'flags');
			add(ps + 2, 4, 'Tag', up(slice(ps + 2, 4)), 'tag');
			if (totalBytes - ps >= 10) add(ps + 6, 4, 'Since', up(slice(ps + 6, 4)), 'timestamp', advTime(slice(ps + 6, 4)));
		} else {
			add(ps, totalBytes - ps, 'Payload', trunc(up(hex.slice(ps * 2)), 32), 'payload');
		}
	} else {
		add(ps, totalBytes - ps, 'Payload', trunc(up(hex.slice(ps * 2)), 32), 'payload');
	}

	return { fields, ranges };
}
