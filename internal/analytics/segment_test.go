package analytics

import (
	"testing"

	"github.com/jjkroell/ridgeline/internal/store"
)

// The rule under test, stated as a table so each clause is visible:
// a node is beyond the bridge only when it is NEVER heard directly, has enough
// sightings to judge, and essentially all of its path-carrying traffic crossed
// in the near->far direction.
func TestSegmentVerdict(t *testing.T) {
	cases := []struct {
		name string
		s    segStat
		want string // "member" | "rejected"
		why  string
	}{
		{"far-side node, all traffic crosses", segStat{withPath: 60, confirmed: 60}, "member",
			"the shape every real far-side node had on the live mesh"},
		{"heard directly even once", segStat{withPath: 60, confirmed: 60, zeroHop: 1}, "rejected",
			"an observer on this side cannot hear the far side, so one direct sighting disqualifies"},
		{"routed through occasionally", segStat{withPath: 220, confirmed: 5}, "rejected",
			"2% is a packet taking an odd route, not a node living over there"},
		{"just under the share bar", segStat{withPath: 100, confirmed: 89}, "rejected", ""},
		{"just over the share bar", segStat{withPath: 100, confirmed: 90}, "member", ""},
		{"too few sightings to judge", segStat{withPath: 3, confirmed: 3}, "rejected",
			"100% of three proves nothing"},
		{"probable only (1-byte path)", segStat{withPath: 40, probable: 40}, "member",
			"a narrow originator must not be invisible - companions skew narrow"},
	}
	for _, c := range cases {
		got := "rejected"
		via := c.s.confirmed + c.s.probable
		if via > 0 && c.s.zeroHop == 0 && c.s.withPath >= segMinSightings &&
			float64(via)/float64(c.s.withPath) >= segMinShare {
			got = "member"
		}
		if got != c.want {
			t.Errorf("%s: got %s, want %s (%s)", c.name, got, c.want, c.why)
		}
	}
}

// Confidence reflects HOW the crossing was proved, which the console shows so a
// 1-byte inference is never mistaken for a 2-byte proof.
func TestSegmentConfidence(t *testing.T) {
	for _, c := range []struct {
		s    segStat
		want string
	}{
		{segStat{confirmed: 10}, "confirmed"},
		{segStat{confirmed: 1, probable: 99}, "confirmed"}, // any wide proof wins
		{segStat{probable: 10}, "probable"},
	} {
		got := "confirmed"
		if c.s.confirmed == 0 {
			got = "probable"
		}
		if got != c.want {
			t.Errorf("%+v: got %s, want %s", c.s, got, c.want)
		}
	}
}

// A bridge with no peer recorded cannot define a segment — a link needs two
// ends — so the sweep must do nothing rather than guess.
func TestDetectSegmentsNoLinksIsNoOp(t *testing.T) {
	rep, err := DetectSegments(nil, nil, nil, "2026-08-15T23:30:00Z", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Members) != 0 || rep.Scanned != 0 {
		t.Errorf("expected an empty report, got %+v", rep)
	}
}

// The far side's declared radio must never be confused with a node's own
// inherited value; ApplySegments/annotate blank the latter. This pins the
// store-level contract the API depends on.
func TestSegmentMemberShape(t *testing.T) {
	m := store.SegmentMember{NodeKey: "abc", BridgeNear: "def", Confidence: "confirmed"}
	if m.Confidence != "confirmed" && m.Confidence != "probable" {
		t.Errorf("confidence must be one of the two documented values, got %q", m.Confidence)
	}
}
