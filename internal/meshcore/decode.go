// Decode logic ported from the MIT-licensed meshcore-decoder by Michael Hart
// (https://github.com/michaelhart/meshcore-decoder).
// Original work Copyright (c) 2025 Michael Hart, MIT License.

package meshcore

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

// DecodeHex decodes a packet from its uppercase-or-lowercase hex string form.
func DecodeHex(s string) (*Packet, error) {
	s = strings.ReplaceAll(s, " ", "")
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("meshcore: invalid hex: %w", err)
	}
	return Decode(b)
}

// Decode parses raw MeshCore packet bytes. It returns a *Packet even for
// malformed input, with Valid=false and Errors populated; the error return is
// reserved for nil/empty input.
func Decode(b []byte) (*Packet, error) {
	if len(b) < 2 {
		return &Packet{
			TotalBytes: len(b),
			Valid:      false,
			Errors:     []string{"packet too short (minimum 2 bytes required)"},
		}, nil
	}

	p := &Packet{TotalBytes: len(b)}

	header := b[0]
	p.RouteType = RouteType(header & 0x03)
	p.PayloadType = PayloadType((header >> 2) & 0x0F)
	p.PayloadVersion = (header >> 6) & 0x03
	offset := 1

	// Transport codes: present only on transport route types.
	if p.RouteType == RouteTransportFlood || p.RouteType == RouteTransportDirect {
		if len(b) < offset+4 {
			return p.fail("packet too short for transport codes"), nil
		}
		codes := [2]uint16{
			binary.LittleEndian.Uint16(b[offset : offset+2]),
			binary.LittleEndian.Uint16(b[offset+2 : offset+4]),
		}
		p.TransportCodes = &codes
		offset += 4
	}

	// Path-length byte: bits 7:6 select hash size (value+1 => 1,2,3 bytes per
	// hop); bits 5:0 are the hop count (0-63).
	if len(b) < offset+1 {
		return p.fail("packet too short for path length"), nil
	}
	hashSize, hopCount, pathBytes := decodePathLenByte(b[offset])
	if hashSize == 4 {
		return p.fail("invalid path length byte: reserved hash size (bits 7:6 = 11)"), nil
	}
	p.PathHashSize = hashSize
	p.PathHopCount = hopCount
	offset++

	if len(b) < offset+pathBytes {
		return p.fail("packet too short for path data"), nil
	}
	if hopCount > 0 {
		p.Path = make([]string, hopCount)
		for i := 0; i < hopCount; i++ {
			hop := b[offset+i*hashSize : offset+(i+1)*hashSize]
			p.Path[i] = strings.ToUpper(hex.EncodeToString(hop))
		}
	}
	offset += pathBytes

	payload := b[offset:]
	p.PayloadRaw = strings.ToUpper(hex.EncodeToString(payload))
	p.MessageHash = messageHash(b, p.RouteType, p.PayloadType, p.PayloadVersion)

	switch p.PayloadType {
	case PayloadAdvert:
		p.Advert = decodeAdvert(payload)
	case PayloadGroupText:
		p.GroupText = decodeGroupText(payload)
	case PayloadTextMessage:
		p.TextMessage = decodeDirectMessage(payload)
	case PayloadRequest:
		p.Request = decodeDirectMessage(payload)
	case PayloadResponse:
		p.Response = decodeDirectMessage(payload)
	case PayloadAnonRequest:
		p.AnonRequest = decodeAnonRequest(payload)
	case PayloadAck:
		p.Ack = decodeAck(payload)
	case PayloadPath:
		p.ReturnPath = decodeDirectMessage(payload)
	case PayloadTrace:
		p.Trace = decodeTrace(payload)
	case PayloadControl:
		p.Control = decodeControl(payload)
	}

	p.Valid = true
	return p, nil
}

func (p *Packet) fail(msg string) *Packet {
	p.Valid = false
	p.Errors = append(p.Errors, msg)
	return p
}

// decodePathLenByte returns the per-hop hash size, hop count, and total path
// byte length encoded in a path-length byte.
func decodePathLenByte(b byte) (hashSize, hopCount, byteLength int) {
	hashSize = int(b>>6) + 1
	hopCount = int(b & 0x3F)
	return hashSize, hopCount, hopCount * hashSize
}

// messageHash reproduces the decoder's route-invariant packet hash: an
// 8-hex-digit value over a synthetic header byte plus the payload, so the same
// transmission observed via different routes hashes identically. TRACE packets
// instead use their 32-bit trace tag.
func messageHash(b []byte, rt RouteType, pt PayloadType, ver uint8) string {
	offset := 1
	if rt == RouteTransportFlood || rt == RouteTransportDirect {
		offset += 4
	}

	if pt == PayloadTrace && len(b) >= 13 {
		o := offset
		if len(b) > o {
			_, _, pathBytes := decodePathLenByte(b[o])
			o += 1 + pathBytes
		}
		if len(b) >= o+4 {
			tag := binary.LittleEndian.Uint32(b[o : o+4])
			return fmt.Sprintf("%08X", tag)
		}
	}

	if len(b) > offset {
		_, _, pathBytes := decodePathLenByte(b[offset])
		offset += 1 + pathBytes
	}
	if offset > len(b) {
		offset = len(b)
	}

	// Reproduce the reference fold: hash starts at 0 and folds a synthetic
	// header byte first, then each payload byte, via hash = hash*31 + v
	// (mod 2^32, matching the reference's &0xffffffff masking).
	constantHeader := (uint32(pt) << 2) | (uint32(ver) << 6)
	var hash uint32
	hash = hash<<5 - hash + constantHeader
	for _, v := range b[offset:] {
		hash = hash<<5 - hash + uint32(v)
	}
	return fmt.Sprintf("%08X", hash)
}
