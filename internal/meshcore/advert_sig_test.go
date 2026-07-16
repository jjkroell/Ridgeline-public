package meshcore

import "testing"

// A real flood advert captured live ("🍒 Cherry Hill 🍒"); its signature must verify.
const realAdvertHex = "10B76000008A654F144C4A43D07F024E8E0A59120F9A3C2E825453F88F861C48C2F2BA245CD672C56C5BE3F52CB4470337904147543724983C2978DCCE234CE41898674714704C6A642E674B47350417E441B17CF5CC4BF444792266EDC3F4FE7970F4DF977CA9344CE16A45FEA7FCD25A85E9653FD2F1DB63666B0A792290A37F398341E70B61099279FFED021081B5F8F09F8D924368657272792048696C6C20F09F8D92"

func TestAdvertSignatureValid(t *testing.T) {
	pkt, err := DecodeHex(realAdvertHex)
	if err != nil || pkt == nil || pkt.Advert == nil {
		t.Fatalf("decode: %v", err)
	}
	if !pkt.Advert.SignatureValid {
		t.Error("a genuine captured advert should have a valid signature")
	}
}

func TestAdvertSignatureTamperedName(t *testing.T) {
	// Flip the last byte of the name — the signature must no longer verify,
	// proving an attacker can't inject a chosen name under a victim's pubkey.
	b := []byte(realAdvertHex)
	if b[len(b)-1] == '2' {
		b[len(b)-1] = '3'
	} else {
		b[len(b)-1] = '2'
	}
	pkt, err := DecodeHex(string(b))
	if err != nil || pkt == nil || pkt.Advert == nil {
		t.Fatalf("decode tampered: %v", err)
	}
	if pkt.Advert.SignatureValid {
		t.Error("an advert with a tampered name must fail signature verification")
	}
}
