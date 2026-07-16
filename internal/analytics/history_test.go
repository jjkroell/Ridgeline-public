package analytics

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/meshcore"
	"github.com/jjkroell/ridgeline/internal/store"
)

// advertFixture is the Flood/Advert packet from "WW7STR/PugetMesh Cougar" used
// across the meshcore tests; here it stands in for a node that both advertises
// and (via a synthetic packet) relays.
const advertFixture = "11007E7662676F7F0850A8A355BAAFBFC1EB7B4174C340442D7D7161C9474A2C94006CE7CF682E58408DD8FCC51906ECA98EBF94A037886BDADE7ECD09FD92B839491DF3809C9454F5286D1D3370AC31A34593D569E9A042A3B41FD331DFFB7E18599CE1E60992A076D50238C5B8F85757375354522F50756765744D65736820436F75676172"

func TestNodeHistory(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	adv, err := meshcore.DecodeHex(advertFixture)
	if err != nil || adv.Advert == nil {
		t.Fatalf("decode advert: %v", err)
	}
	pubkey := adv.Advert.PublicKey
	now := time.Now().UTC()

	// A non-advert packet whose single path hop is the node's 1-byte key prefix,
	// so it resolves (uniquely, as the only node) to this node as a relay.
	hop := strings.ToLower(pubkey[:2])
	relayHex := "0901" + hop + "AABBCCDD" // Flood TextMessage, 1-hop path, DM envelope
	relay, err := meshcore.DecodeHex(relayHex)
	if err != nil {
		t.Fatal(err)
	}
	if len(relay.Path) != 1 {
		t.Fatalf("relay path = %v, want one hop", relay.Path)
	}

	// Record chronologically (oldest first), as the daemon ingests in arrival
	// order — RawWindow's id-DESC scan then yields newest first.
	if err := st.Record(store.Observation{
		Packet: relay, RawHex: relayHex, ObserverID: "obs-C", Region: "R1",
		ReceivedAt: now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	for i, obsID := range []string{"obs-B", "obs-A"} {
		if err := st.Record(store.Observation{
			Packet: adv, RawHex: advertFixture, ObserverID: obsID, Region: "R1",
			ReceivedAt: now.Add(-time.Duration(1-i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}

	nodes, err := st.ListNodes()
	if err != nil {
		t.Fatal(err)
	}
	cutoff := now.Add(-time.Hour).Format(time.RFC3339Nano)
	entries, err := NodeHistory(st, nodes, pubkey, cutoff, 0, 100)
	if err != nil {
		t.Fatal(err)
	}

	var adverts, relays int
	for _, e := range entries {
		switch e.Kind {
		case "advert":
			adverts++
		case "relay":
			relays++
			if e.PayloadType != "TextMessage" || e.HopIndex != 0 {
				t.Errorf("relay entry = %+v, want TextMessage at hop 0", e)
			}
		}
	}
	if adverts != 2 {
		t.Errorf("advert entries = %d, want 2", adverts)
	}
	if relays != 1 {
		t.Errorf("relay entries = %d, want 1", relays)
	}
	// Newest first.
	for i := 1; i < len(entries); i++ {
		if entries[i-1].ReceivedAt < entries[i].ReceivedAt {
			t.Errorf("entries not newest-first at %d", i)
		}
	}
}
