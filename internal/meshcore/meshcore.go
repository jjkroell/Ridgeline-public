// Package meshcore decodes MeshCore LoRa mesh packets from their raw wire
// form into structured Go values.
//
// The wire format and decode logic are ported from the MIT-licensed
// meshcore-decoder by Michael Hart (https://github.com/michaelhart/meshcore-decoder),
// which itself references the MeshCore firmware packet documentation
// (https://github.com/meshcore-dev/MeshCore/blob/main/docs/packet_structure.md).
//
// Original work Copyright (c) 2025 Michael Hart, MIT License.
package meshcore

// RouteType is the 2-bit routing mode in the packet header (bits 0-1).
type RouteType uint8

const (
	RouteTransportFlood  RouteType = 0x00
	RouteFlood           RouteType = 0x01
	RouteDirect          RouteType = 0x02
	RouteTransportDirect RouteType = 0x03
)

// IsFlood reports whether the route is a flood, where the originator broadcasts
// to the whole mesh and relays accumulate their hashes into the path. Only flood
// adverts carry the originator's configured path-hash size; a zero-hop (direct)
// advert is sent with path_len=0, which always decodes as hash size 1 regardless
// of the node's setting — so it carries no usable hash-size signal.
func (r RouteType) IsFlood() bool {
	return r == RouteFlood || r == RouteTransportFlood
}

func (r RouteType) String() string {
	switch r {
	case RouteTransportFlood:
		return "TransportFlood"
	case RouteFlood:
		return "Flood"
	case RouteDirect:
		return "Direct"
	case RouteTransportDirect:
		return "TransportDirect"
	default:
		return "Unknown"
	}
}

// PayloadType is the 4-bit payload classifier in the packet header (bits 2-5).
type PayloadType uint8

const (
	PayloadRequest     PayloadType = 0x00
	PayloadResponse    PayloadType = 0x01
	PayloadTextMessage PayloadType = 0x02
	PayloadAck         PayloadType = 0x03
	PayloadAdvert      PayloadType = 0x04
	PayloadGroupText   PayloadType = 0x05
	PayloadGroupData   PayloadType = 0x06
	PayloadAnonRequest PayloadType = 0x07
	PayloadPath        PayloadType = 0x08
	PayloadTrace       PayloadType = 0x09
	PayloadMultipart   PayloadType = 0x0A
	PayloadControl     PayloadType = 0x0B
	PayloadRawCustom   PayloadType = 0x0F
)

func (p PayloadType) String() string {
	switch p {
	case PayloadRequest:
		return "Request"
	case PayloadResponse:
		return "Response"
	case PayloadTextMessage:
		return "TextMessage"
	case PayloadAck:
		return "Ack"
	case PayloadAdvert:
		return "Advert"
	case PayloadGroupText:
		return "GroupText"
	case PayloadGroupData:
		return "GroupData"
	case PayloadAnonRequest:
		return "AnonRequest"
	case PayloadPath:
		return "Path"
	case PayloadTrace:
		return "Trace"
	case PayloadMultipart:
		return "Multipart"
	case PayloadControl:
		return "Control"
	case PayloadRawCustom:
		return "RawCustom"
	default:
		return "Unknown"
	}
}

// DeviceRole is the node role advertised in the lower 4 bits of an Advert's
// app-flags byte.
type DeviceRole uint8

const (
	RoleUnknown    DeviceRole = 0x00
	RoleChatNode   DeviceRole = 0x01
	RoleRepeater   DeviceRole = 0x02
	RoleRoomServer DeviceRole = 0x03
	RoleSensor     DeviceRole = 0x04
)

func (d DeviceRole) String() string {
	switch d {
	case RoleChatNode:
		return "ChatNode"
	case RoleRepeater:
		return "Repeater"
	case RoleRoomServer:
		return "RoomServer"
	case RoleSensor:
		return "Sensor"
	default:
		return "Unknown"
	}
}

// Advert app-flag bits.
const (
	advertFlagHasLocation = 0x10
	advertFlagHasFeature1 = 0x20
	advertFlagHasFeature2 = 0x40
	advertFlagHasName     = 0x80
)

// Packet is a decoded MeshCore packet.
type Packet struct {
	// MessageHash is an 8-hex-digit identifier derived from the packet's
	// route-invariant content, used to deduplicate observations of the same
	// transmission seen by multiple observers.
	MessageHash string

	RouteType      RouteType
	PayloadType    PayloadType
	PayloadVersion uint8

	// TransportCodes holds the two 16-bit region transport codes present only
	// on TransportFlood/TransportDirect packets. Nil otherwise.
	TransportCodes *[2]uint16

	// PathHopCount is the number of hops recorded in Path.
	PathHopCount int
	// PathHashSize is the number of bytes per hop entry (1, 2, or 3).
	PathHashSize int
	// Path holds one uppercase-hex string per hop, or nil when empty.
	Path []string

	// PayloadRaw is the uppercase-hex of the undecoded payload bytes.
	PayloadRaw string

	// Advert is populated when PayloadType == PayloadAdvert.
	Advert *Advert

	// GroupText is populated when PayloadType == PayloadGroupText.
	GroupText *GroupText

	// TextMessage, Request, and Response are direct (peer-to-peer) payloads
	// that share the same envelope; the body is sealed with a shared secret we
	// don't hold, so only the routing hashes and MAC are decoded. Exactly one
	// is populated, matching PayloadType.
	TextMessage *DirectMessage
	Request     *DirectMessage
	Response    *DirectMessage

	// AnonRequest is populated when PayloadType == PayloadAnonRequest.
	AnonRequest *AnonRequest

	// Ack is populated when PayloadType == PayloadAck.
	Ack *Ack

	// ReturnPath is populated when PayloadType == PayloadPath. On real traffic a
	// PATH packet carries the discovered return path encrypted; only the routing
	// envelope (the same dest/src/MAC shape as a direct message) is in the clear,
	// so the path list itself is inside Ciphertext and not decoded here. Distinct
	// from the packet header's Path.
	ReturnPath *DirectMessage

	// Trace is populated when PayloadType == PayloadTrace.
	Trace *Trace

	// Control is populated when PayloadType == PayloadControl.
	Control *Control

	TotalBytes int
	Valid      bool
	Errors     []string
}

// DirectMessage is the cleartext envelope shared by TextMessage (0x02),
// Request (0x00), and Response (0x01) payloads: a 1-byte destination hash, a
// 1-byte source hash, a 2-byte cipher MAC, and the encrypted body. The body
// (timestamp, message/request text) needs the peers' shared secret to decrypt,
// which a passive observer doesn't have, so it's left as Ciphertext.
type DirectMessage struct {
	DestinationHash string // first byte of the destination public key, uppercase hex
	SourceHash      string // first byte of the source public key, uppercase hex
	CipherMAC       string // 2-byte MAC, uppercase hex
	Ciphertext      string // encrypted remainder, uppercase hex
}

// AnonRequest is the decoded body of an AnonRequest (0x07) payload: a login or
// request from a sender not yet known to the destination, so it carries the
// sender's full public key in the clear. The body remains encrypted.
type AnonRequest struct {
	DestinationHash string // first byte of the destination public key, uppercase hex
	SenderPublicKey string // 32-byte Ed25519 key, uppercase hex
	CipherMAC       string // 2-byte MAC, uppercase hex
	Ciphertext      string // encrypted remainder, uppercase hex
}

// Ack is the decoded body of an Ack (0x03) payload: a 4-byte CRC checksum over
// the acknowledged message's timestamp, text, and sender public key.
type Ack struct {
	Checksum string // 4-byte CRC, uppercase hex
}

// Trace is the decoded body of a Trace (0x09) payload: a path-tracing probe.
// The per-hop path hashes and any SNR values from the header path are not
// decrypted (the trace carries them in the clear).
type Trace struct {
	Tag      string   // 4-byte trace tag, uppercase hex
	AuthCode uint32   // authentication/verification code
	Flags    uint8    // application-defined control flags
	HashSize int      // bytes per path-hash entry (1, 2, 4, or 8)
	Path     []string // node hashes along the trace path, uppercase hex
}

// Control sub-types occupy the upper 4 bits of the control flags byte.
const (
	ControlNodeDiscoverReq  uint8 = 0x80
	ControlNodeDiscoverResp uint8 = 0x90
)

// Control is the decoded body of a Control (0x0B) payload. Only the node
// discovery request/response sub-types are interpreted; the populated fields
// depend on SubType.
type Control struct {
	SubType  uint8  // upper 4 bits of the flags byte
	SubName  string // human-readable sub-type name
	RawFlags uint8

	// NodeDiscoverReq fields.
	PrefixOnly bool
	TypeFilter uint8
	Tag        uint32
	Since      uint32

	// NodeDiscoverResp fields.
	NodeRole  DeviceRole
	SNR       float64
	PublicKey string // responder key (prefix or full), uppercase hex
}

// GroupText is the decoded body of a GroupText (0x05) channel message. The
// ciphertext is decrypted when its channel hash matches a known channel key
// (currently the public channel); otherwise only the channel hash is known.
type GroupText struct {
	ChannelHash string // 1-byte channel identifier, uppercase hex
	MAC         string // 2-byte cipher MAC, uppercase hex

	// Set when decryption succeeds.
	Decrypted bool
	Channel   string // matched channel name, e.g. "Public"
	Sender    string // sender name parsed from the message, if present
	Message   string // decrypted message text
	Timestamp uint32 // message timestamp, unix seconds
}

// Advert is the decoded body of an Advert (0x04) payload: a node announcing
// its identity, optional location, and optional name.
type Advert struct {
	PublicKey  string // 32-byte Ed25519 key, uppercase hex
	Timestamp  uint32 // unix seconds
	Signature  string // 64-byte Ed25519 signature, uppercase hex
	Flags      uint8
	DeviceRole DeviceRole

	HasLocation bool
	Latitude    float64
	Longitude   float64

	HasName bool
	Name    string

	// SignatureValid reports whether the advert's Ed25519 signature verifies
	// against its own public key over pubkey||timestamp||app_data (capped at
	// MAX_ADVERT_DATA_SIZE, matching the MeshCore firmware). A valid signature
	// proves the advert was produced by the holder of the node's private key —
	// it cannot be forged or replayed with altered fields by a rogue observer.
	// Used to authenticate node-ownership claims (a temporary code in the name).
	SignatureValid bool
}
