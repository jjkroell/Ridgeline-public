package analytics

import (
	"github.com/jjkroell/ridgeline/internal/store"
)

// TelemetrySummary distills an observer's telemetry time series into the trends
// that matter for ops: is the battery draining, is the noise floor creeping up,
// has the device rebooted. Computed from the raw points so the UI doesn't have
// to. Pointers are nil when there isn't enough data to judge.
type TelemetrySummary struct {
	Samples   int     `json:"samples"`
	SpanHours float64 `json:"spanHours"`

	// Battery (mV). Trend is the least-squares slope in mV/hour over samples
	// that report a positive battery voltage (mains-powered observers report 0
	// and are excluded). Direction is "charging"/"discharging"/"stable"/"" .
	BatteryMv        *int     `json:"batteryMv,omitempty"` // latest
	BatteryTrendMvHr *float64 `json:"batteryTrendMvHr,omitempty"`
	BatteryDir       string   `json:"batteryDir,omitempty"`
	// Reboots = count of uptime resets (uptime_secs dropping between samples).
	Reboots int `json:"reboots"`

	// Noise floor (dBm). Lower is quieter/better; a positive trend means the RF
	// environment is getting noisier.
	NoiseFloor     *float64 `json:"noiseFloor,omitempty"` // latest
	NoiseTrendDbHr *float64 `json:"noiseTrendDbHr,omitempty"`
	NoiseMin       *float64 `json:"noiseMin,omitempty"`
	NoiseMax       *float64 `json:"noiseMax,omitempty"`
	NoiseAvg       *float64 `json:"noiseAvg,omitempty"`
}

// ObserverTelemetryReport is the telemetry endpoint payload: the raw series plus
// the derived health summary.
type ObserverTelemetryReport struct {
	ID      string                 `json:"id"`
	Points  []store.TelemetryPoint `json:"points"`
	Summary TelemetrySummary       `json:"summary"`
}

// SummarizeTelemetry derives battery/noise/reboot trends from an observer's
// telemetry points (assumed oldest-first, as ObserverTelemetry returns them).
func SummarizeTelemetry(points []store.TelemetryPoint) TelemetrySummary {
	s := TelemetrySummary{Samples: len(points)}
	if len(points) == 0 {
		return s
	}

	t0 := parseTime(points[0].RecordedAt)
	tEnd := parseTime(points[len(points)-1].RecordedAt)
	if !t0.IsZero() && !tEnd.IsZero() {
		s.SpanHours = tEnd.Sub(t0).Hours()
	}
	hoursOf := func(p store.TelemetryPoint) float64 {
		return parseTime(p.RecordedAt).Sub(t0).Hours()
	}

	// Battery: regression over positive readings only (0 = mains-powered).
	var bx, by []float64
	for _, p := range points {
		if p.BatteryMV != nil && *p.BatteryMV > 0 {
			bx = append(bx, hoursOf(p))
			by = append(by, float64(*p.BatteryMV))
			mv := *p.BatteryMV
			s.BatteryMv = &mv
		}
	}
	if slope, ok := lsqSlope(bx, by); ok {
		s.BatteryTrendMvHr = &slope
		switch {
		case slope > 2:
			s.BatteryDir = "charging"
		case slope < -2:
			s.BatteryDir = "discharging"
		default:
			s.BatteryDir = "stable"
		}
	}

	// Reboots: uptime resets between consecutive reporting samples.
	var prevUp *int64
	for _, p := range points {
		if p.UptimeSecs == nil {
			continue
		}
		if prevUp != nil && *p.UptimeSecs < *prevUp {
			s.Reboots++
		}
		u := *p.UptimeSecs
		prevUp = &u
	}

	// Noise floor: latest, min/max/avg, regression slope.
	var nx, ny []float64
	var nSum float64
	for _, p := range points {
		if p.NoiseFloor == nil {
			continue
		}
		v := *p.NoiseFloor
		nx = append(nx, hoursOf(p))
		ny = append(ny, v)
		nSum += v
		nf := v
		s.NoiseFloor = &nf
		if s.NoiseMin == nil || v < *s.NoiseMin {
			lo := v
			s.NoiseMin = &lo
		}
		if s.NoiseMax == nil || v > *s.NoiseMax {
			hi := v
			s.NoiseMax = &hi
		}
	}
	if len(ny) > 0 {
		avg := nSum / float64(len(ny))
		s.NoiseAvg = &avg
	}
	if slope, ok := lsqSlope(nx, ny); ok {
		s.NoiseTrendDbHr = &slope
	}

	return s
}

// lsqSlope returns the least-squares slope of y over x. ok is false when there
// are fewer than two distinct x values (slope undefined).
func lsqSlope(x, y []float64) (float64, bool) {
	n := float64(len(x))
	if len(x) < 2 {
		return 0, false
	}
	var sx, sy, sxx, sxy float64
	for i := range x {
		sx += x[i]
		sy += y[i]
		sxx += x[i] * x[i]
		sxy += x[i] * y[i]
	}
	denom := n*sxx - sx*sx
	if denom == 0 {
		return 0, false
	}
	return (n*sxy - sx*sy) / denom, true
}
