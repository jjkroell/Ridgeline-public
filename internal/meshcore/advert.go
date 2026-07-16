// Advert decode logic ported from the MIT-licensed meshcore-decoder by
// Michael Hart (https://github.com/michaelhart/meshcore-decoder).
// Original work Copyright (c) 2025 Michael Hart, MIT License.

package meshcore

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"unicode/utf8"
)

// advertMinLen is the fixed prefix: public_key(32) + timestamp(4) +
// signature(64) + flags(1).
const advertMinLen = 32 + 4 + 64 + 1

// advertSigPrefix is public_key(32) + timestamp(4) — the start of the signed
// message; the rest is app_data (flags + optional location/features + name).
const advertSigPrefix = 32 + 4

// maxAdvertDataSize mirrors the firmware's MAX_ADVERT_DATA_SIZE: the app_data
// signed (and transmitted) is capped at 32 bytes. Signature verification must
// use the same cap or valid adverts with trailing bytes would fail.
const maxAdvertDataSize = 32

// decodeAdvert parses an Advert payload. It returns nil only when the payload
// is too short to contain the fixed prefix.
func decodeAdvert(payload []byte) *Advert {
	if len(payload) < advertMinLen {
		return nil
	}

	a := &Advert{
		PublicKey:      strings.ToUpper(hex.EncodeToString(payload[0:32])),
		Timestamp:      binary.LittleEndian.Uint32(payload[32:36]),
		Signature:      strings.ToUpper(hex.EncodeToString(payload[36:100])),
		Flags:          payload[100],
		SignatureValid: verifyAdvertSignature(payload),
	}
	a.DeviceRole = parseDeviceRole(a.Flags)
	a.HasLocation = a.Flags&advertFlagHasLocation != 0
	a.HasName = a.Flags&advertFlagHasName != 0

	offset := advertMinLen

	if a.HasLocation && len(payload) >= offset+8 {
		a.Latitude = float64(int32(binary.LittleEndian.Uint32(payload[offset:offset+4]))) / 1e6
		a.Longitude = float64(int32(binary.LittleEndian.Uint32(payload[offset+4:offset+8]))) / 1e6
		offset += 8
	}

	// Feature fields are not yet interpreted, but their presence shifts the
	// name offset.
	if a.Flags&advertFlagHasFeature1 != 0 {
		offset += 2
	}
	if a.Flags&advertFlagHasFeature2 != 0 {
		offset += 2
	}

	if a.HasName && len(payload) > offset {
		a.Name = decodeNodeName(payload[offset:])
	}

	return a
}

// verifyAdvertSignature checks the advert's Ed25519 signature the same way the
// firmware does: the signed message is pub_key(32) || timestamp(4) || app_data,
// where app_data is the bytes after the signature, capped at maxAdvertDataSize.
// Returns false for any malformed/short payload.
func verifyAdvertSignature(payload []byte) bool {
	if len(payload) < advertMinLen { // need at least through flags/app_data start
		return false
	}
	pub := payload[0:32]
	sig := payload[36:100]
	appData := payload[advertSigPrefix+64:] // from byte 100
	if len(appData) > maxAdvertDataSize {
		appData = appData[:maxAdvertDataSize]
	}
	msg := make([]byte, 0, advertSigPrefix+len(appData))
	msg = append(msg, payload[0:advertSigPrefix]...) // pub_key || timestamp
	msg = append(msg, appData...)
	return ed25519.Verify(ed25519.PublicKey(pub), msg, sig)
}

func parseDeviceRole(flags uint8) DeviceRole {
	switch flags & 0x0F {
	case 0x01:
		return RoleChatNode
	case 0x02:
		return RoleRepeater
	case 0x03:
		return RoleRoomServer
	case 0x04:
		return RoleSensor
	default:
		return RoleChatNode
	}
}

// decodeNodeName interprets the trailing name bytes as UTF-8 truncated at the
// first NUL, with control characters stripped and surrounding space trimmed.
func decodeNodeName(b []byte) string {
	if i := indexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	s := string(b)
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}
