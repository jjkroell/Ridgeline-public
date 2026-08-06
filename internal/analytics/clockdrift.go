package analytics

import (
	"sort"
	"time"
)

// driftSample is one advert transmission's clock evidence: the timestamp the
// node stamped into the advert, and the earliest moment any observer received
// it. Keyed by message hash by the caller, so a re-flood of the same advert
// contributes one sample, not one per relay.
type driftSample struct {
	advertUnix uint32
	earliest   time.Time
}

// driftAcc collects clock evidence for one node across the window.
type driftAcc struct {
	byHash map[string]*driftSample
}

func newDriftAcc() *driftAcc { return &driftAcc{byHash: map[string]*driftSample{}} }

// observe records one advert observation. advertUnix is the timestamp inside
// the advert payload (the node's own clock at transmit); recv is when this
// observer's packet reached us (the SERVER clock — ingest deliberately stamps
// arrival server-side, so an observer with a bad clock cannot skew this).
//
// Only the EARLIEST reception of a given advert is kept. MeshCore re-floods an
// advert payload unchanged for a long time, so later receptions of the same
// message hash carry the original timestamp and would otherwise read as the
// node falling progressively further behind.
func (d *driftAcc) observe(hash string, advertUnix uint32, recv time.Time) {
	if hash == "" || recv.IsZero() || !plausibleClock(advertUnix, recv) {
		return
	}
	s := d.byHash[hash]
	if s == nil {
		d.byHash[hash] = &driftSample{advertUnix: advertUnix, earliest: recv}
		return
	}
	if recv.Before(s.earliest) {
		s.earliest = recv
	}
}

// The bound is on the reported timestamp, NOT on the resulting drift. A node
// whose clock never got set reports something near the epoch, which is garbage
// rather than a clock reading. But a node whose clock was set once, years ago,
// and has been wrong ever since is exactly what an operator needs to see — so
// once a timestamp looks like a real wall clock, whatever drift it implies is
// reported in full, however large.
const (
	// minPlausibleUnix is 2020-01-01. MeshCore did not exist before this, so an
	// earlier timestamp is an unset or corrupt clock, not a reading.
	minPlausibleUnix = 1577836800
	// maxFutureSec rejects timestamps absurdly far ahead of the receive time.
	// Generous: a clock a year fast is still a clock, two decades is garbage.
	maxFutureSec = 365 * 24 * 3600
)

// plausibleClock reports whether advertUnix looks like a wall-clock reading at
// all, given when the advert was received.
func plausibleClock(advertUnix uint32, recv time.Time) bool {
	if advertUnix < minPlausibleUnix {
		return false
	}
	return int64(advertUnix) <= recv.Unix()+maxFutureSec
}

// unsetThresholdSec separates a clock that has DRIFTED from one that was never
// SET. A real RTC or time-sync fault lands seconds to hours out, occasionally a
// day or two. A node whose clock was never set reports its firmware's build
// date instead — years out, and observed here across many nodes at once. They
// are different faults needing different fixes, so they are reported
// separately rather than averaged into one misleading number.
const unsetThresholdSec = 30 * 24 * 3600

// drift returns the node's median clock offset in seconds (positive = node
// ahead of the server) and whether the node looks like it is running an unset
// clock. A nil offset with unset=false means there was not enough evidence.
//
// Samples are partitioned first: adverts stamped within unsetThresholdSec are
// real clock readings, the rest are unset-clock artifacts. Whichever group the
// node mostly falls in decides its verdict, so a node that emits a few
// correctly-stamped adverts among many build-date ones is not reported as
// "812 days behind", nor is a genuinely drifting node dragged toward zero.
//
// The median (not the mean) is deliberate: one advert that sat in a relay
// queue, or one corrupt timestamp, must not move the figure. Two samples are
// required so a single reading can never stand alone.
func (d *driftAcc) drift() (*float64, bool) {
	var real, unset []float64
	for _, s := range d.byHash {
		off := float64(int64(s.advertUnix) - s.earliest.Unix())
		if off > unsetThresholdSec || off < -unsetThresholdSec {
			unset = append(unset, off)
		} else {
			real = append(real, off)
		}
	}
	if len(unset) > len(real) && len(unset) >= 2 {
		return nil, true
	}
	if len(real) < 2 {
		return nil, false
	}
	sort.Float64s(real)
	var med float64
	if n := len(real); n%2 == 1 {
		med = real[n/2]
	} else {
		med = (real[n/2-1] + real[n/2]) / 2
	}
	return &med, false
}

// samples reports how many distinct adverts backed the drift figure.
func (d *driftAcc) samples() int { return len(d.byHash) }
