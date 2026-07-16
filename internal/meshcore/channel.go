// Group-channel text decryption, ported from the MIT-licensed meshcore-decoder
// by Michael Hart (https://github.com/michaelhart/meshcore-decoder).
// Original work Copyright (c) 2025 Michael Hart, MIT License.

package meshcore

import (
	"bytes"
	"crypto/aes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"unicode/utf8"
)

// Channel is a known group channel and its 16-byte AES key.
type Channel struct {
	Name string
	Key  []byte
}

// knownChannels are the channels group-text decryption will try. MeshCore's
// well-known public channel is included by default.
var knownChannels = []Channel{
	{Name: "Public", Key: mustDecodeHex("8b3387e9c5cdea6ac9e5edbaa115cd72")},
}

func mustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("meshcore: invalid channel key hex: " + err.Error())
	}
	return b
}

// channelHashByte is MeshCore's 1-byte channel identifier: the first byte of
// SHA-256 over the channel's 16-byte key.
func channelHashByte(key []byte) byte {
	h := sha256.Sum256(key)
	return h[0]
}

// decodeGroupText parses a GroupText payload — channel_hash(1) | MAC(2) |
// ciphertext — and decrypts it if a known channel key matches.
func decodeGroupText(payload []byte) *GroupText {
	if len(payload) < 3 {
		return nil
	}
	gt := &GroupText{
		ChannelHash: strings.ToUpper(hex.EncodeToString(payload[0:1])),
		MAC:         strings.ToUpper(hex.EncodeToString(payload[1:3])),
	}
	hashByte, mac, ciphertext := payload[0], payload[1:3], payload[3:]
	for _, ch := range knownChannels {
		if channelHashByte(ch.Key) != hashByte {
			continue
		}
		if ts, sender, msg, ok := decryptGroupText(ciphertext, mac, ch.Key); ok {
			gt.Decrypted = true
			gt.Channel = ch.Name
			gt.Sender = sender
			gt.Message = msg
			gt.Timestamp = ts
			break
		}
	}
	return gt
}

// decryptGroupText verifies the 2-byte HMAC (over the ciphertext, keyed by the
// 32-byte secret key||zeros) and, on success, AES-128-ECB decrypts the message,
// parsing the timestamp(4) | flags(1) | UTF-8 text layout.
func decryptGroupText(ciphertext, mac, key []byte) (ts uint32, sender, message string, ok bool) {
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return 0, "", "", false
	}

	// HMAC-SHA256 over the ciphertext, keyed by the 32-byte secret (key||zeros).
	secret := make([]byte, 32)
	copy(secret, key)
	m := hmac.New(sha256.New, secret)
	m.Write(ciphertext)
	if sum := m.Sum(nil); sum[0] != mac[0] || sum[1] != mac[1] {
		return 0, "", "", false
	}

	// AES-128-ECB decrypt (no padding): decrypt each block independently.
	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, "", "", false
	}
	out := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += aes.BlockSize {
		block.Decrypt(out[i:i+aes.BlockSize], ciphertext[i:i+aes.BlockSize])
	}
	if len(out) < 5 {
		return 0, "", "", false
	}

	ts = binary.LittleEndian.Uint32(out[0:4])
	text := out[5:]
	if i := bytes.IndexByte(text, 0); i >= 0 { // trim at null terminator
		text = text[:i]
	}
	if utf8.Valid(text) {
		sender, message = splitSender(string(text))
		return ts, sender, message, true
	}
	// The 2-byte HMAC already authenticated this payload against the channel key,
	// so invalid UTF-8 is a corrupt sender name (seen in the wild), not the wrong
	// key. Recover the message body when it's readable rather than dropping the
	// whole message; the broken name is unshowable, so use a placeholder.
	if sep := bytes.Index(text, []byte(": ")); sep > 0 && utf8.Valid(text[sep+2:]) {
		return ts, "(corrupt name)", string(text[sep+2:]), true
	}
	return 0, "", "", false
}

// splitSender separates a "sender: message" prefix when present and plausible.
func splitSender(s string) (sender, message string) {
	if i := strings.Index(s, ": "); i > 0 && i < 50 && !strings.ContainsAny(s[:i], ":[]") {
		return s[:i], s[i+2:]
	}
	return "", s
}
