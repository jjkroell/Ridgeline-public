package analytics

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestLinkScore(t *testing.T) {
	tests := []struct {
		name   string
		snr    float64
		length int
		sf     int
		want   float64
	}{
		// snr below the SF7 threshold (-7.5) → no chance.
		{"below threshold", -9, 32, 7, 0},
		// exactly at threshold → margin 0 → 0.
		{"at threshold", -7.5, 32, 7, 0},
		// snr 0, len 32, SF7: success=(0+7.5)/10=0.75, penalty=1-32/256=0.875.
		{"mid", 0, 32, 7, 0.65625},
		// strong snr clamps to 1.0 (1.25*0.875 > 1).
		{"clamps high", 5, 32, 7, 1.0},
		// unsupported SF returns 0.
		{"bad sf", 5, 32, 6, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := LinkScore(tc.snr, tc.length, tc.sf)
			if !approx(got, tc.want) {
				t.Errorf("LinkScore(%v,%d,%d) = %v, want %v", tc.snr, tc.length, tc.sf, got, tc.want)
			}
		})
	}
}

func TestAirtime(t *testing.T) {
	// SF7 / BW62.5k / CR5 / preamble17, 32-byte packet.
	// t_sym = 128/62.5 = 2.048ms; t_preamble = 21.25*2.048 = 43.52ms
	// numerator = max(256-28+28+16,0)=272; denom = 28; ceil(272/28)=10
	// n_payload = 8 + 10*5 = 58; t_payload = 58*2.048 = 118.784
	// total = 162.304ms
	got := Airtime(32, DefaultRadio())
	if !approx(got, 162.304) {
		t.Errorf("Airtime(32, default) = %v, want 162.304", got)
	}

	// Longer packet must take longer on air.
	if Airtime(64, DefaultRadio()) <= got {
		t.Errorf("airtime should increase with length")
	}
}
