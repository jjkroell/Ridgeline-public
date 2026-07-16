package meshcore

import "testing"

// Each fixture is a synthetic packet: header byte + path-length byte (0x00, no
// path) + payload, exercising one payload-type decoder. Header byte =
// route(Flood=0x01) | payloadType<<2.

func TestDecodeAck(t *testing.T) {
	// header 0x0D = Flood | Ack(0x03)<<2; payload = checksum DEADBEEF.
	p, err := DecodeHex("0D00DEADBEEF")
	if err != nil {
		t.Fatal(err)
	}
	if p.PayloadType != PayloadAck {
		t.Fatalf("PayloadType = %v, want Ack", p.PayloadType)
	}
	if p.Ack == nil {
		t.Fatal("Ack is nil")
	}
	if p.Ack.Checksum != "DEADBEEF" {
		t.Errorf("Checksum = %q, want DEADBEEF", p.Ack.Checksum)
	}
}

func TestDecodeTextMessage(t *testing.T) {
	// header 0x09 = Flood | TextMessage(0x02)<<2; dest AA, src BB, MAC CCDD,
	// ciphertext 11223344.
	p, err := DecodeHex("0900AABBCCDD11223344")
	if err != nil {
		t.Fatal(err)
	}
	if p.TextMessage == nil {
		t.Fatal("TextMessage is nil")
	}
	dm := p.TextMessage
	if dm.DestinationHash != "AA" || dm.SourceHash != "BB" {
		t.Errorf("dest/src = %q/%q, want AA/BB", dm.DestinationHash, dm.SourceHash)
	}
	if dm.CipherMAC != "CCDD" {
		t.Errorf("CipherMAC = %q, want CCDD", dm.CipherMAC)
	}
	if dm.Ciphertext != "11223344" {
		t.Errorf("Ciphertext = %q, want 11223344", dm.Ciphertext)
	}
}

func TestDecodeAnonRequest(t *testing.T) {
	// header 0x1D = Flood | AnonRequest(0x07)<<2; dest AA, 32-byte key of 0x11,
	// MAC BBCC, ciphertext DDEE.
	key := ""
	for i := 0; i < 32; i++ {
		key += "11"
	}
	p, err := DecodeHex("1D00AA" + key + "BBCCDDEE")
	if err != nil {
		t.Fatal(err)
	}
	if p.AnonRequest == nil {
		t.Fatal("AnonRequest is nil")
	}
	ar := p.AnonRequest
	if ar.DestinationHash != "AA" {
		t.Errorf("DestinationHash = %q, want AA", ar.DestinationHash)
	}
	if len(ar.SenderPublicKey) != 64 {
		t.Errorf("SenderPublicKey len = %d, want 64", len(ar.SenderPublicKey))
	}
	if ar.CipherMAC != "BBCC" || ar.Ciphertext != "DDEE" {
		t.Errorf("MAC/cipher = %q/%q, want BBCC/DDEE", ar.CipherMAC, ar.Ciphertext)
	}
}

func TestDecodePath(t *testing.T) {
	// A real PATH (0x08) packet captured live from the mesh. Header 0x22 =
	// Direct | Path<<2, then a 3-byte header hop (9A3C2E), then the encrypted
	// return-path envelope: dest 6E, src F4, MAC 46FB, ciphertext rest. The path
	// list itself is inside the ciphertext, so only the envelope is decoded.
	const pathFixture = "22819A3C2E6EF446FB7B685AF8ACF89CDEDEA026ADC56F6A89DE965E8EE72953920FEE991948DD5067"
	p, err := DecodeHex(pathFixture)
	if err != nil {
		t.Fatal(err)
	}
	if p.PayloadType != PayloadPath {
		t.Fatalf("PayloadType = %v, want Path", p.PayloadType)
	}
	if p.ReturnPath == nil {
		t.Fatal("ReturnPath is nil")
	}
	rp := p.ReturnPath
	if rp.DestinationHash != "6E" || rp.SourceHash != "F4" {
		t.Errorf("dest/src = %q/%q, want 6E/F4", rp.DestinationHash, rp.SourceHash)
	}
	if rp.CipherMAC != "46FB" {
		t.Errorf("CipherMAC = %q, want 46FB", rp.CipherMAC)
	}
	if rp.Ciphertext == "" {
		t.Error("Ciphertext is empty")
	}
}

func TestDecodeTrace(t *testing.T) {
	// header 0x25 = Flood | Trace(0x09)<<2; tag 01020304, auth 05060708,
	// flags 0x00 (1-byte hashes), path AABBCC.
	p, err := DecodeHex("2500010203040506070800AABBCC")
	if err != nil {
		t.Fatal(err)
	}
	if p.Trace == nil {
		t.Fatal("Trace is nil")
	}
	tr := p.Trace
	if tr.Tag != "01020304" {
		t.Errorf("Tag = %q, want 01020304", tr.Tag)
	}
	if tr.AuthCode != 0x08070605 {
		t.Errorf("AuthCode = %#x, want 0x08070605", tr.AuthCode)
	}
	if tr.HashSize != 1 {
		t.Errorf("HashSize = %d, want 1", tr.HashSize)
	}
	if len(tr.Path) != 3 || tr.Path[0] != "AA" || tr.Path[2] != "CC" {
		t.Errorf("Path = %v, want [AA BB CC]", tr.Path)
	}
	// Trace message hash uses the trace tag (LE): 0x04030201.
	if p.MessageHash != "04030201" {
		t.Errorf("MessageHash = %q, want 04030201", p.MessageHash)
	}
}

func TestDecodeControlDiscoverResp(t *testing.T) {
	// header 0x2D = Flood | Control(0x0B)<<2; flags 0x92 (DiscoverResp | role
	// Repeater), snr 0x04 (=1.0 dB), tag 01020304, 8-byte key prefix.
	p, err := DecodeHex("2D009204010203041122334455667788")
	if err != nil {
		t.Fatal(err)
	}
	if p.Control == nil {
		t.Fatal("Control is nil")
	}
	c := p.Control
	if c.SubType != ControlNodeDiscoverResp || c.SubName != "DiscoverResp" {
		t.Errorf("SubType = %#x (%s), want DiscoverResp", c.SubType, c.SubName)
	}
	if c.NodeRole != RoleRepeater {
		t.Errorf("NodeRole = %v, want Repeater", c.NodeRole)
	}
	if c.SNR != 1.0 {
		t.Errorf("SNR = %v, want 1.0", c.SNR)
	}
	if c.PublicKey != "1122334455667788" {
		t.Errorf("PublicKey = %q, want 1122334455667788", c.PublicKey)
	}
}

func TestDecodeControlDiscoverReq(t *testing.T) {
	// header 0x2D; flags 0x81 (DiscoverReq | prefix-only), type filter 0x06,
	// tag 01020304, no since field.
	p, err := DecodeHex("2D00810601020304")
	if err != nil {
		t.Fatal(err)
	}
	if p.Control == nil {
		t.Fatal("Control is nil")
	}
	c := p.Control
	if c.SubType != ControlNodeDiscoverReq {
		t.Errorf("SubType = %#x, want DiscoverReq", c.SubType)
	}
	if !c.PrefixOnly {
		t.Error("PrefixOnly = false, want true")
	}
	if c.TypeFilter != 0x06 {
		t.Errorf("TypeFilter = %#x, want 0x06", c.TypeFilter)
	}
	if c.Since != 0 {
		t.Errorf("Since = %d, want 0", c.Since)
	}
}
