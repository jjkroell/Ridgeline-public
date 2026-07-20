package meshcore

import "testing"

// TestRelayPathExcludesTrace pins the reason RelayPath exists: a trace's header
// path is per-hop SNR, not relay hashes, and those bytes are indistinguishable
// from 1-byte hop hashes by inspection. Walking them as a path invents relays
// that never carried the packet.
func TestRelayPathExcludesTrace(t *testing.T) {
	hops := []string{"A3", "F1", "0C"}

	relayed := &Packet{PayloadType: PayloadTextMessage, Path: hops}
	if got := relayed.RelayPath(); len(got) != 3 {
		t.Errorf("RelayPath on a non-trace = %v, want the header path %v", got, hops)
	}

	trace := &Packet{PayloadType: PayloadTrace, Path: hops}
	if got := trace.RelayPath(); got != nil {
		t.Errorf("RelayPath on a trace = %v, want nil — those bytes are SNR readings", got)
	}
	// The raw header path stays readable: the packet inspector renders a trace's
	// per-hop SNR from exactly these bytes.
	if len(trace.Path) != 3 {
		t.Errorf("trace.Path = %v, want the raw header bytes preserved", trace.Path)
	}
}

// TestTracePayloadCarriesTheRealRoute documents where a trace's actual route
// lives, and that it is sized independently of the header path.
func TestTracePayloadCarriesTheRealRoute(t *testing.T) {
	// tag(4) + authCode(4) + flags(1) + route hashes. flags&0x03 == 1 → 2-byte
	// hashes, regardless of what the header path's hash size is.
	payload := []byte{
		0xDE, 0xAD, 0xBE, 0xEF, // tag
		0x01, 0x00, 0x00, 0x00, // auth code
		0x01,                   // flags: hash size = 1<<1 = 2 bytes
		0xAA, 0xBB, 0xCC, 0xDD, // two 2-byte route hashes
	}
	tr := decodeTrace(payload)
	if tr == nil {
		t.Fatal("decodeTrace returned nil")
	}
	if tr.HashSize != 2 {
		t.Errorf("HashSize = %d, want 2 (from flags&0x03)", tr.HashSize)
	}
	want := []string{"AABB", "CCDD"}
	if len(tr.Path) != len(want) || tr.Path[0] != want[0] || tr.Path[1] != want[1] {
		t.Errorf("Trace.Path = %v, want %v", tr.Path, want)
	}
}
