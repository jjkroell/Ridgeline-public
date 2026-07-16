package meshcore

import (
	"crypto/aes"
	"crypto/hmac"
	"crypto/sha256"
	"testing"
)

// Test vector from the meshcore-decoder reference suite: a public-channel
// GroupText from "🌲 Tree".
func TestDecodeGroupTextPublicChannel(t *testing.T) {
	const raw = "150011C3C1354D619BAE9590E4D177DB7EEAF982F5BDCF78005D75157D9535FA90178F785D"
	pkt, err := DecodeHex(raw)
	if err != nil {
		t.Fatal(err)
	}
	if pkt.PayloadType != PayloadGroupText {
		t.Fatalf("payload type = %v, want GroupText", pkt.PayloadType)
	}
	gt := pkt.GroupText
	if gt == nil {
		t.Fatal("GroupText not decoded")
	}
	if gt.ChannelHash != "11" {
		t.Errorf("channel hash = %q, want 11", gt.ChannelHash)
	}
	if gt.MAC != "C3C1" {
		t.Errorf("MAC = %q, want C3C1", gt.MAC)
	}
	if !gt.Decrypted {
		t.Fatal("message not decrypted")
	}
	if gt.Channel != "Public" {
		t.Errorf("channel = %q, want Public", gt.Channel)
	}
	if gt.Sender != "🌲 Tree" {
		t.Errorf("sender = %q, want 🌲 Tree", gt.Sender)
	}
	if gt.Message != "☁️" {
		t.Errorf("message = %q, want ☁️", gt.Message)
	}
	if gt.Timestamp != 1758484279 {
		t.Errorf("timestamp = %d, want 1758484279", gt.Timestamp)
	}
}

// A GroupText whose sender name is corrupt (invalid UTF-8) but whose message
// body is readable must still decrypt: the HMAC proves the channel key is right,
// so the whole message shouldn't be discarded just because the name is garbage.
// Regression for kod-bot channel messages vanishing from the site while showing
// fine in the MeshCore app (2026-07-04).
func TestDecodeGroupTextCorruptSenderName(t *testing.T) {
	key := mustDecodeHex("8b3387e9c5cdea6ac9e5edbaa115cd72") // Public

	// plaintext = ts(4 LE) | flags(1) | "<name>: <message>\0…", name invalid UTF-8.
	pt := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xfe}
	pt = append(pt, []byte(": hello world")...)
	for len(pt)%aes.BlockSize != 0 { // null-terminate + zero-pad to block size
		pt = append(pt, 0x00)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	ct := make([]byte, len(pt))
	for i := 0; i < len(pt); i += aes.BlockSize {
		block.Encrypt(ct[i:i+aes.BlockSize], pt[i:i+aes.BlockSize])
	}
	secret := make([]byte, 32)
	copy(secret, key)
	m := hmac.New(sha256.New, secret)
	m.Write(ct)
	sum := m.Sum(nil)

	payload := append([]byte{channelHashByte(key), sum[0], sum[1]}, ct...)
	gt := decodeGroupText(payload)
	if gt == nil || !gt.Decrypted {
		t.Fatal("message with corrupt name should still decrypt")
	}
	if gt.Message != "hello world" {
		t.Errorf("message = %q, want %q", gt.Message, "hello world")
	}
	if gt.Sender != "(corrupt name)" {
		t.Errorf("sender = %q, want %q", gt.Sender, "(corrupt name)")
	}
}
