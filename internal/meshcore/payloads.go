// Payload decode logic ported from the MIT-licensed meshcore-decoder by
// Michael Hart (https://github.com/michaelhart/meshcore-decoder).
// Original work Copyright (c) 2025 Michael Hart, MIT License.

package meshcore

import (
	"encoding/binary"
	"encoding/hex"
	"strings"
)

// upperHex renders bytes as an uppercase hex string.
func upperHex(b []byte) string {
	return strings.ToUpper(hex.EncodeToString(b))
}

// decodeDirectMessage parses the cleartext envelope shared by TextMessage,
// Request, and Response payloads: dest hash(1) + source hash(1) + MAC(2) +
// ciphertext(rest). Returns nil when too short to hold the fixed prefix.
func decodeDirectMessage(payload []byte) *DirectMessage {
	if len(payload) < 4 {
		return nil
	}
	return &DirectMessage{
		DestinationHash: upperHex(payload[0:1]),
		SourceHash:      upperHex(payload[1:2]),
		CipherMAC:       upperHex(payload[2:4]),
		Ciphertext:      upperHex(payload[4:]),
	}
}

// decodeAnonRequest parses an AnonRequest payload: dest hash(1) + sender public
// key(32) + MAC(2) + ciphertext(rest). Returns nil when too short.
func decodeAnonRequest(payload []byte) *AnonRequest {
	if len(payload) < 35 {
		return nil
	}
	return &AnonRequest{
		DestinationHash: upperHex(payload[0:1]),
		SenderPublicKey: upperHex(payload[1:33]),
		CipherMAC:       upperHex(payload[33:35]),
		Ciphertext:      upperHex(payload[35:]),
	}
}

// decodeAck parses an Ack payload: a 4-byte CRC checksum. Returns nil when too
// short.
func decodeAck(payload []byte) *Ack {
	if len(payload) < 4 {
		return nil
	}
	return &Ack{Checksum: upperHex(payload[0:4])}
}

// decodeTrace parses a Trace payload: tag(4) + auth code(4) + flags(1) +
// path hashes. The flags' low two bits select the path-hash size (1<<n). Returns
// nil when too short or the path bytes don't align to the hash size.
func decodeTrace(payload []byte) *Trace {
	if len(payload) < 9 {
		return nil
	}
	t := &Trace{
		Tag:      upperHex(payload[0:4]),
		AuthCode: binary.LittleEndian.Uint32(payload[4:8]),
		Flags:    payload[8],
	}
	t.HashSize = 1 << (t.Flags & 0x03)

	rest := payload[9:]
	if t.HashSize == 0 || len(rest)%t.HashSize != 0 {
		return nil
	}
	for off := 0; off+t.HashSize <= len(rest); off += t.HashSize {
		t.Path = append(t.Path, upperHex(rest[off:off+t.HashSize]))
	}
	return t
}

// decodeControl parses a Control payload. Only the node discovery
// request/response sub-types are interpreted; returns nil for an unknown
// sub-type or one too short for its fields.
func decodeControl(payload []byte) *Control {
	if len(payload) < 1 {
		return nil
	}
	rawFlags := payload[0]
	switch rawFlags & 0xF0 {
	case ControlNodeDiscoverReq:
		// flags(1) + type_filter(1) + tag(4), with an optional since(4).
		if len(payload) < 6 {
			return nil
		}
		c := &Control{
			SubType:    ControlNodeDiscoverReq,
			SubName:    "DiscoverReq",
			RawFlags:   rawFlags,
			PrefixOnly: rawFlags&0x01 != 0,
			TypeFilter: payload[1],
			Tag:        binary.LittleEndian.Uint32(payload[2:6]),
		}
		if len(payload) >= 10 {
			c.Since = binary.LittleEndian.Uint32(payload[6:10])
		}
		return c
	case ControlNodeDiscoverResp:
		// flags(1) + snr(1) + tag(4) + public key (8-byte prefix or 32-byte full).
		if len(payload) < 14 {
			return nil
		}
		return &Control{
			SubType:   ControlNodeDiscoverResp,
			SubName:   "DiscoverResp",
			RawFlags:  rawFlags,
			NodeRole:  DeviceRole(rawFlags & 0x0F),
			SNR:       float64(int8(payload[1])) / 4.0,
			Tag:       binary.LittleEndian.Uint32(payload[2:6]),
			PublicKey: upperHex(payload[6:]),
		}
	default:
		return nil
	}
}
