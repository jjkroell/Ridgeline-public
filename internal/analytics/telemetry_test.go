package analytics

import (
	"testing"
	"time"

	"github.com/jjkroell/ridgeline/internal/store"
)

func ptrI(v int) *int         { return &v }
func ptrI64(v int64) *int64   { return &v }
func ptrF(v float64) *float64 { return &v }

func TestSummarizeTelemetry(t *testing.T) {
	base := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	at := func(h int) string { return base.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }

	// 5 hourly samples: battery draining 100 mV/hr, noise rising 1 dB/hr, one reboot.
	points := []store.TelemetryPoint{
		{RecordedAt: at(0), BatteryMV: ptrI(4000), UptimeSecs: ptrI64(1000), NoiseFloor: ptrF(-100)},
		{RecordedAt: at(1), BatteryMV: ptrI(3900), UptimeSecs: ptrI64(4600), NoiseFloor: ptrF(-99)},
		{RecordedAt: at(2), BatteryMV: ptrI(3800), UptimeSecs: ptrI64(50), NoiseFloor: ptrF(-98)}, // reboot
		{RecordedAt: at(3), BatteryMV: ptrI(3700), UptimeSecs: ptrI64(3650), NoiseFloor: ptrF(-97)},
		{RecordedAt: at(4), BatteryMV: ptrI(3600), UptimeSecs: ptrI64(7250), NoiseFloor: ptrF(-96)},
	}

	s := SummarizeTelemetry(points)

	if s.Samples != 5 {
		t.Errorf("Samples = %d, want 5", s.Samples)
	}
	if s.SpanHours != 4 {
		t.Errorf("SpanHours = %v, want 4", s.SpanHours)
	}
	if s.BatteryMv == nil || *s.BatteryMv != 3600 {
		t.Errorf("BatteryMv = %v, want 3600 (latest)", s.BatteryMv)
	}
	if s.BatteryTrendMvHr == nil || *s.BatteryTrendMvHr > -99 || *s.BatteryTrendMvHr < -101 {
		t.Errorf("BatteryTrendMvHr = %v, want ~-100", s.BatteryTrendMvHr)
	}
	if s.BatteryDir != "discharging" {
		t.Errorf("BatteryDir = %q, want discharging", s.BatteryDir)
	}
	if s.Reboots != 1 {
		t.Errorf("Reboots = %d, want 1", s.Reboots)
	}
	if s.NoiseFloor == nil || *s.NoiseFloor != -96 {
		t.Errorf("NoiseFloor = %v, want -96 (latest)", s.NoiseFloor)
	}
	if s.NoiseMin == nil || *s.NoiseMin != -100 {
		t.Errorf("NoiseMin = %v, want -100", s.NoiseMin)
	}
	if s.NoiseMax == nil || *s.NoiseMax != -96 {
		t.Errorf("NoiseMax = %v, want -96", s.NoiseMax)
	}
	if s.NoiseTrendDbHr == nil || *s.NoiseTrendDbHr < 0.99 || *s.NoiseTrendDbHr > 1.01 {
		t.Errorf("NoiseTrendDbHr = %v, want ~1.0", s.NoiseTrendDbHr)
	}
}

func TestSummarizeTelemetryMainsAndEmpty(t *testing.T) {
	if got := SummarizeTelemetry(nil); got.Samples != 0 {
		t.Errorf("empty: Samples = %d, want 0", got.Samples)
	}
	// Mains-powered observer reports battery 0 → excluded from battery trend.
	base := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	points := []store.TelemetryPoint{
		{RecordedAt: base.Format(time.RFC3339), BatteryMV: ptrI(0), NoiseFloor: ptrF(-102)},
		{RecordedAt: base.Add(time.Hour).Format(time.RFC3339), BatteryMV: ptrI(0), NoiseFloor: ptrF(-101)},
	}
	s := SummarizeTelemetry(points)
	if s.BatteryMv != nil || s.BatteryTrendMvHr != nil || s.BatteryDir != "" {
		t.Errorf("mains observer should have no battery trend, got mv=%v trend=%v dir=%q", s.BatteryMv, s.BatteryTrendMvHr, s.BatteryDir)
	}
	if s.NoiseAvg == nil || *s.NoiseAvg != -101.5 {
		t.Errorf("NoiseAvg = %v, want -101.5", s.NoiseAvg)
	}
}
