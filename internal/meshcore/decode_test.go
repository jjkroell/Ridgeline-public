package meshcore

import "testing"

// advertFixture and its expectations come from the meshcore-decoder test suite
// (tests/packet-structure.test.ts): a Flood/Advert packet from node
// "WW7STR/PugetMesh Cougar".
const advertFixture = "11007E7662676F7F0850A8A355BAAFBFC1EB7B4174C340442D7D7161C9474A2C94006CE7CF682E58408DD8FCC51906ECA98EBF94A037886BDADE7ECD09FD92B839491DF3809C9454F5286D1D3370AC31A34593D569E9A042A3B41FD331DFFB7E18599CE1E60992A076D50238C5B8F85757375354522F50756765744D65736820436F75676172"

func TestDecodeAdvertFixture(t *testing.T) {
	p, err := DecodeHex(advertFixture)
	if err != nil {
		t.Fatalf("DecodeHex error: %v", err)
	}
	if !p.Valid {
		t.Fatalf("packet not valid: %v", p.Errors)
	}
	if p.RouteType != RouteFlood {
		t.Errorf("RouteType = %v, want Flood", p.RouteType)
	}
	if p.PayloadType != PayloadAdvert {
		t.Errorf("PayloadType = %v, want Advert", p.PayloadType)
	}
	if p.PathHopCount != 0 {
		t.Errorf("PathHopCount = %d, want 0", p.PathHopCount)
	}
	if p.Advert == nil {
		t.Fatal("Advert is nil")
	}
	if got, want := p.Advert.Name, "WW7STR/PugetMesh Cougar"; got != want {
		t.Errorf("Name = %q, want %q", got, want)
	}
	if !p.Advert.HasLocation {
		t.Error("HasLocation = false, want true")
	}
	if len(p.Advert.PublicKey) != 64 {
		t.Errorf("PublicKey length = %d, want 64 hex chars", len(p.Advert.PublicKey))
	}
}

func TestDecodeTooShort(t *testing.T) {
	p, err := Decode([]byte{0x11})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Valid {
		t.Error("expected Valid=false for 1-byte packet")
	}
}

// messageHash must be stable across path differences for the same payload,
// since flooded packets accumulate path hops at each observer.
func TestMessageHashRouteInvariant(t *testing.T) {
	// Same Advert payload, one with a 1-hop path, one with none.
	base := advertFixture
	p1, _ := DecodeHex(base)
	// Inject a single 1-byte hop: path-len byte 0x01 + one hop byte 0xAB,
	// keeping the header and payload identical.
	withHop := base[:2] + "01AB" + base[4:]
	p2, _ := DecodeHex(withHop)
	if p1.MessageHash != p2.MessageHash {
		t.Errorf("hash differs across path: %s vs %s", p1.MessageHash, p2.MessageHash)
	}
}
