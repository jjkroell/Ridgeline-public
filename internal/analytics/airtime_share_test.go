package analytics

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/store"
)

// TestTrafficShareWeightsByAirtime records two relayed transmissions of very
// different lengths, each transiting a different relay exactly once. Under the
// old packet-count model both relays scored an identical 50% share. Weighting
// by time-on-air, the relay that carried the long packet must score higher —
// it held the channel open for longer, which is what "traffic share" means.
func TestTrafficShareWeightsByAirtime(t *testing.T) {
	nodes := []store.Node{
		{PublicKey: "AA" + strings.Repeat("11", 31), Name: "relay-short", Role: "Repeater"},
		{PublicKey: "BB" + strings.Repeat("22", 31), Name: "relay-long", Role: "Repeater"},
	}

	// Flood TextMessage, one path hop = the relay's 1-byte key prefix, then a
	// DM envelope body. The bodies differ in length (and content, so the two
	// transmissions hash differently and are not deduped into one).
	shortHex := "0901aa" + "AABBCCDD"
	longHex := "0901bb" + strings.Repeat("EE", 180)

	now := time.Now().UTC()
	at := func(ago time.Duration) string {
		return now.Add(-ago).Format(time.RFC3339Nano)
	}
	raws := []store.RawObservation{
		{RawHex: shortHex, ObserverID: "obs-A", Region: "R1", ReceivedAt: at(10 * time.Minute)},
		{RawHex: longHex, ObserverID: "obs-A", Region: "R1", ReceivedAt: at(5 * time.Minute)},
	}

	radio := DefaultRadio()
	details := build(raws, nodes, 6, at(6*time.Hour), radio)

	short := details[nodes[0].PublicKey]
	long := details[nodes[1].PublicKey]
	if short == nil || long == nil {
		t.Fatalf("both relays should have details; got short=%v long=%v", short, long)
	}

	// Equal packet counts — the old model's tie.
	if short.Relay.Count24h != 1 || long.Relay.Count24h != 1 {
		t.Fatalf("relay counts = %d / %d, want 1 / 1 (the counts must stay tied for this test to mean anything)",
			short.Relay.Count24h, long.Relay.Count24h)
	}

	if !(long.TrafficShare > short.TrafficShare) {
		t.Errorf("long-packet relay share %.4f should exceed short-packet relay share %.4f",
			long.TrafficShare, short.TrafficShare)
	}

	// The shares must be exactly the airtime split, and sum to 1 when every
	// transmission has exactly one relay.
	shortAir := Airtime(len(shortHex)/2, radio)
	longAir := Airtime(len(longHex)/2, radio)
	total := shortAir + longAir
	if math.Abs(short.TrafficShare-shortAir/total) > 1e-9 {
		t.Errorf("short share = %.6f, want %.6f", short.TrafficShare, shortAir/total)
	}
	if math.Abs(long.TrafficShare-longAir/total) > 1e-9 {
		t.Errorf("long share = %.6f, want %.6f", long.TrafficShare, longAir/total)
	}
	if sum := short.TrafficShare + long.TrafficShare; math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("shares sum to %.6f, want 1.0", sum)
	}

	// Absolute airtime is reported alongside the ratio.
	if math.Abs(short.RelayAirtimeMs-shortAir) > 1e-9 {
		t.Errorf("short RelayAirtimeMs = %.3f, want %.3f", short.RelayAirtimeMs, shortAir)
	}
	if math.Abs(long.RelayAirtimeMs-longAir) > 1e-9 {
		t.Errorf("long RelayAirtimeMs = %.3f, want %.3f", long.RelayAirtimeMs, longAir)
	}
}

// TestTrafficShareCountsANodeOncePerTransmission guards the ratio's upper
// bound: a path that lists the same relay twice (a loop, or a corrupt path)
// must not let that node bill the same transmission's airtime twice and
// report a share above 100%.
func TestTrafficShareCountsANodeOncePerTransmission(t *testing.T) {
	nodes := []store.Node{
		{PublicKey: "AA" + strings.Repeat("11", 31), Name: "looping-relay", Role: "Repeater"},
	}
	// Two path hops, both resolving to the same node.
	loopHex := "0902aaaa" + "AABBCCDD"

	now := time.Now().UTC()
	raws := []store.RawObservation{
		{RawHex: loopHex, ObserverID: "obs-A", Region: "R1", ReceivedAt: now.Add(-time.Minute).Format(time.RFC3339Nano)},
	}

	d := build(raws, nodes, 6, now.Add(-6*time.Hour).Format(time.RFC3339Nano), DefaultRadio())[nodes[0].PublicKey]
	if d == nil {
		t.Fatal("relay should have details")
	}
	if d.TrafficShare > 1.0+1e-9 {
		t.Errorf("TrafficShare = %.4f, must never exceed 1.0", d.TrafficShare)
	}
	if math.Abs(d.TrafficShare-1.0) > 1e-9 {
		t.Errorf("TrafficShare = %.4f, want exactly 1.0 (sole relay of the sole transmission)", d.TrafficShare)
	}
}
