package store

import (
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
)

// A signature-invalid advert must never create or mutate a node row — it can only
// be recorded as a raw observation. This guards against RF-corrupted (or forged)
// advert copies defacing a node's name/location or spawning phantom nodes.
func TestSignatureInvalidAdvertDoesNotTouchNode(t *testing.T) {
	st := testStore(t)
	const pk = "F44DD6E89E6FA4BAB9C0CE2DFD07C574FB52BC9A7EE4CC23D199DE4AA617A369"

	rec := func(name string, sigValid bool, hash string) {
		obs := Observation{
			Packet: &meshcore.Packet{
				MessageHash: hash,
				Advert:      &meshcore.Advert{PublicKey: pk, HasName: true, Name: name, SignatureValid: sigValid},
			},
			RawHex:     "00",
			ReceivedAt: time.Now(),
		}
		if err := st.Record(obs); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	// A corrupt advert alone creates no node.
	rec("P#e//jnitm", false, "aaaa0001")
	if ok, _ := st.NodeExists(pk); ok {
		t.Fatal("signature-invalid advert must not create a node")
	}

	// A genuine advert establishes the node and its name.
	rec("UBCV//Zenith", true, "aaaa0002")
	nodes, _ := st.ListNodes()
	var got string
	for _, n := range nodes {
		if n.PublicKey == pk {
			got = n.Name
		}
	}
	if got != "UBCV//Zenith" {
		t.Fatalf("expected valid advert name, got %q", got)
	}

	// A later corrupt copy must NOT overwrite the good name.
	rec("P#e//jnitm", false, "aaaa0003")
	nodes, _ = st.ListNodes()
	for _, n := range nodes {
		if n.PublicKey == pk && n.Name != "UBCV//Zenith" {
			t.Fatalf("corrupt advert defaced the name: %q", n.Name)
		}
	}
}
