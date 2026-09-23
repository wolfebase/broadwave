package hdhr

import (
	"encoding/binary"
	"hash/crc32"
	"testing"
)

func TestDiscoverPacketShape(t *testing.T) {
	pkt := DiscoverPacket()
	if binary.BigEndian.Uint16(pkt[0:2]) != typeDiscoverReq {
		t.Fatalf("type %04x", binary.BigEndian.Uint16(pkt[0:2]))
	}
	bodyLen := binary.BigEndian.Uint16(pkt[2:4])
	if int(bodyLen)+8 != len(pkt) {
		t.Fatalf("length header %d packet %d", bodyLen, len(pkt))
	}
	body := pkt[:4+bodyLen]
	got := binary.LittleEndian.Uint32(pkt[4+bodyLen:])
	if crc32.ChecksumIEEE(body) != got {
		t.Fatal("crc does not match IEEE")
	}
}

func TestParseReplyRoundTripFields(t *testing.T) {
	// device id 10611B4C, base URL, tuner count 2. No auth tag.
	payload := []byte{tagDeviceType, 0x04, 0x00, 0x00, 0x00, 0x01}
	payload = append(payload, tagDeviceID, 0x04, 0x10, 0x61, 0x1B, 0x4C)
	base := "http://192.168.1.252"
	payload = append(payload, tagBaseURL, byte(len(base)))
	payload = append(payload, []byte(base)...)
	payload = append(payload, tagTunerCount, 0x01, 0x02)
	body := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint16(body[0:2], typeDiscoverRpy)
	binary.BigEndian.PutUint16(body[2:4], uint16(len(payload)))
	copy(body[4:], payload)
	pkt := make([]byte, len(body)+4)
	copy(pkt, body)
	binary.LittleEndian.PutUint32(pkt[len(body):], crc32.ChecksumIEEE(body))

	reply, err := ParseReply(pkt, "192.168.1.252:65001")
	if err != nil {
		t.Fatal(err)
	}
	if reply.DeviceID != "10611B4C" || reply.BaseURL != base || reply.TunerCount != 2 {
		t.Fatalf("%+v", reply)
	}
}

func TestParseReplyRejectsBadCRC(t *testing.T) {
	pkt := DiscoverPacket()
	pkt[len(pkt)-1] ^= 0xFF
	// A request is also the wrong type; flip type to a reply and keep the bad crc.
	binary.BigEndian.PutUint16(pkt[0:2], typeDiscoverRpy)
	if _, err := ParseReply(pkt, "x"); err == nil {
		t.Fatal("expected crc or type failure")
	}
}
