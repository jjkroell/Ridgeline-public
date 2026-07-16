package store

import "time"

// telemetryMinInterval is the rate floor: at most one telemetry sample is kept
// per observer per interval. /status is a retained MQTT message republished
// periodically (and re-delivered in a burst on reconnect), so without this a
// reconnect or chatty observer would flood the log. An evenly-spaced series
// also charts more cleanly than a change-triggered one.
const telemetryMinInterval = 5 * time.Minute

// TelemetryPoint is one sample of an observer's device telemetry at a point in
// time. Fields mirror the device stats of ObserverStatus; nil means the
// observer didn't report that field in this sample.
type TelemetryPoint struct {
	RecordedAt string   `json:"recordedAt"`
	BatteryMV  *int     `json:"batteryMv,omitempty"`
	UptimeSecs *int64   `json:"uptimeSecs,omitempty"`
	NoiseFloor *float64 `json:"noiseFloor,omitempty"`
	TxAirSecs  *float64 `json:"txAirSecs,omitempty"`
	RxAirSecs  *float64 `json:"rxAirSecs,omitempty"`
	RecvErrors *int     `json:"recvErrors,omitempty"`
	QueueLen   *int     `json:"queueLen,omitempty"`
}

// RecordObserverTelemetry appends a telemetry sample for an observer, subject to
// the rate floor (telemetryMinInterval since the last sample). recordedAt is an
// RFC3339 server-receipt time. Samples carrying no device fields at all are
// dropped — an all-NULL row adds nothing to a trend.
func (s *Store) RecordObserverTelemetry(id, recordedAt string, st ObserverStatus) error {
	if id == "" {
		return nil
	}
	if st.BatteryMV == nil && st.UptimeSecs == nil && st.NoiseFloor == nil &&
		st.TxAirSecs == nil && st.RxAirSecs == nil && st.RecvErrors == nil && st.QueueLen == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Rate floor: skip if the last sample for this observer is too recent.
	var lastAt string
	s.db.QueryRow(`SELECT recorded_at FROM observer_telemetry WHERE observer_id = ? ORDER BY id DESC LIMIT 1`, id).Scan(&lastAt)
	if lastAt != "" {
		prev, err1 := time.Parse(time.RFC3339, lastAt)
		cur, err2 := time.Parse(time.RFC3339, recordedAt)
		if err1 == nil && err2 == nil && cur.Sub(prev) < telemetryMinInterval {
			return nil
		}
	}

	_, err := s.db.Exec(`
		INSERT INTO observer_telemetry
			(observer_id, recorded_at, battery_mv, uptime_secs, noise_floor,
			 tx_air_secs, rx_air_secs, recv_errors, queue_len)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		id, recordedAt, st.BatteryMV, st.UptimeSecs, st.NoiseFloor,
		st.TxAirSecs, st.RxAirSecs, st.RecvErrors, st.QueueLen)
	return err
}

// ObserverTelemetry returns an observer's telemetry samples received at or after
// sinceISO, oldest first (so callers can chart left-to-right), capped at limit.
func (s *Store) ObserverTelemetry(id, sinceISO string, limit int) ([]TelemetryPoint, error) {
	if limit <= 0 || limit > 20000 {
		limit = 5000
	}
	rows, err := s.db.Query(`
		SELECT recorded_at, battery_mv, uptime_secs, noise_floor,
		       tx_air_secs, rx_air_secs, recv_errors, queue_len
		FROM observer_telemetry
		WHERE observer_id = ? AND recorded_at >= ?
		ORDER BY recorded_at ASC
		LIMIT ?`, id, sinceISO, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TelemetryPoint{}
	for rows.Next() {
		var p TelemetryPoint
		if err := rows.Scan(&p.RecordedAt, &p.BatteryMV, &p.UptimeSecs, &p.NoiseFloor,
			&p.TxAirSecs, &p.RxAirSecs, &p.RecvErrors, &p.QueueLen); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PruneTelemetry deletes telemetry samples recorded before beforeISO, returning
// the number removed. Backs the daemon's retention pass.
func (s *Store) PruneTelemetry(beforeISO string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.Exec(`DELETE FROM observer_telemetry WHERE recorded_at < ?`, beforeISO)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
