package analytics

import "math"

// RadioParams are the LoRa PHY settings the mesh runs on. They drive the airtime
// estimate and (via SF) the link-score SNR threshold. Defaults match the MeshCore
// "USA/Canada (Recommended)" preset this mesh uses — SF7 / BW62.5kHz / CR5,
// preamble 17. Frequency is irrelevant to both formulas so it is not modelled.
type RadioParams struct {
	SpreadingFactor int // SF7..SF12
	BandwidthHz     int // e.g. 62500
	CodingRate      int // CR denominator: 5=4/5 .. 8=4/8
	PreambleSymbols int // preamble length in symbols
}

// DefaultRadio is the default mesh radio config (MeshCore USA/Canada recommended).
func DefaultRadio() RadioParams {
	return RadioParams{SpreadingFactor: 7, BandwidthHz: 62500, CodingRate: 5, PreambleSymbols: 17}
}

// snrThresholds is the minimum demodulation SNR (dB) per spreading factor, from
// MeshCore's RadioLibWrappers.cpp. Below the threshold a packet has ~no chance of
// decoding; above it, success rises with the margin.
var snrThresholds = map[int]float64{7: -7.5, 8: -10.0, 9: -12.5, 10: -15.0, 11: -17.5, 12: -20.0}

// LinkScore estimates the probability (0..1) that a packet of the given length was
// successfully received, from its observed SNR and the spreading factor. Ported
// from pyMC_Repeater's calculate_packet_score: a success rate driven by the SNR
// margin above the per-SF demod threshold, scaled by a collision penalty that
// grows with packet length (longer packets are likelier to collide). It is a
// per-observation link-quality measure — more meaningful than raw SNR because it
// folds in the SF-dependent sensitivity floor and airtime exposure.
func LinkScore(snr float64, lengthBytes, sf int) float64 {
	if sf < 7 {
		return 0
	}
	threshold, ok := snrThresholds[sf]
	if !ok {
		threshold = -10.0
	}
	if snr < threshold {
		return 0
	}
	successFromSNR := (snr - threshold) / 10.0
	collisionPenalty := 1.0 - float64(lengthBytes)/256.0
	score := successFromSNR * collisionPenalty
	return math.Max(0.0, math.Min(1.0, score))
}

// Airtime returns the time-on-air in milliseconds for a LoRa packet of the given
// PHY-payload length, using the Semtech reference formula. Assumes CRC on and
// explicit header (the MeshCore defaults). Used to estimate channel utilisation:
// summed over a window and divided by the window length it yields band busy-ness.
func Airtime(lengthBytes int, rp RadioParams) float64 {
	sf := rp.SpreadingFactor
	bwHz := rp.BandwidthHz
	cr := rp.CodingRate
	preamble := rp.PreambleSymbols

	const crc = 1 // CRC enabled
	const h = 0   // explicit header

	// Low-data-rate optimisation: required at SF11/SF12 on <=125kHz bandwidth.
	de := 0
	if sf >= 11 && bwHz <= 125000 {
		de = 1
	}

	tSym := math.Pow(2, float64(sf)) / (float64(bwHz) / 1000.0) // ms per symbol
	tPreamble := (float64(preamble) + 4.25) * tSym

	numerator := math.Max(float64(8*lengthBytes-4*sf+28+16*crc-20*h), 0)
	denominator := float64(4 * (sf - 2*de))
	nPayload := 8 + math.Ceil(numerator/denominator)*float64(cr)
	tPayload := nPayload * tSym

	return tPreamble + tPayload
}
