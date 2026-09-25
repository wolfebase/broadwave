package live

import "testing"

func TestAudioTracksPMT(t *testing.T) {
	// 4.1 shape: English complete main, Spanish SAP on 0x102, described English.
	raw := audioTS(1, []esAudio{
		{pid: 0x101, lang: "eng", audioType: 0, bsmod: 0},
		{pid: 0x102, lang: "spa", audioType: 0, bsmod: 0},
		{pid: 0x103, lang: "eng", audioType: 0, bsmod: 2},
	})
	tracks := AudioTracks(raw, 1)
	if len(tracks) != 3 {
		t.Fatalf("tracks: %+v", tracks)
	}
	if tracks[0].Role != "main" || tracks[0].PID != 0x101 || tracks[0].Label != "English" {
		t.Fatalf("main: %+v", tracks[0])
	}
	if tracks[1].Role != "language" || tracks[1].PID != 0x102 || tracks[1].Label != "Spanish" {
		t.Fatalf("sap: %+v", tracks[1])
	}
	if tracks[2].Role != "described" || tracks[2].PID != 0x103 || tracks[2].Label != "Described video" {
		t.Fatalf("vi: %+v", tracks[2])
	}
	sap, ok := PickTrack(tracks, "language")
	if !ok || sap.PID != 0x102 {
		t.Fatalf("pick sap: %+v %v", sap, ok)
	}
	if got := AudioTracks(raw, 99); len(got) != 0 {
		t.Fatalf("wrong program: %+v", got)
	}
}

func TestAudioTracksSameLanguageAlternate(t *testing.T) {
	// 41.1 shape: a second English track with no bsmod is described video.
	raw := audioTS(1, []esAudio{
		{pid: 0x101, lang: "eng", audioType: 0, bsmod: 0},
		{pid: 0x104, lang: "eng", audioType: -1, bsmod: -1},
	})
	tracks := AudioTracks(raw, 1)
	if len(tracks) != 2 || tracks[1].Role != "described" || tracks[1].PID != 0x104 {
		t.Fatalf("alternate: %+v", tracks)
	}
}

func TestAudioTracksISOAudioType(t *testing.T) {
	raw := audioTS(1, []esAudio{
		{pid: 0x101, lang: "eng", audioType: 0, bsmod: 0},
		{pid: 0x105, lang: "eng", audioType: 3, bsmod: 0},
	})
	tracks := AudioTracks(raw, 1)
	if tracks[1].Role != "described" {
		t.Fatalf("audio_type 3: %+v", tracks)
	}
}

func TestSplitPMTKeepsTheTailAudio(t *testing.T) {
	// The last elementary stream rides only in the pointer field of the
	// packet that starts the next section. A continuation payload never
	// carries it, which is how a real mux packs a PMT that spans packets.
	audios := []esAudio{
		{pid: 0x101, lang: "eng", audioType: 0, bsmod: 0},
		{pid: 0x102, lang: "spa", audioType: 0, bsmod: 0},
		{pid: 0x103, lang: "eng", audioType: 0, bsmod: 2},
	}
	// 318 bytes of program info: 183 bytes in the first packet, one full
	// continuation, and the last audio stream only in the pointer field.
	section := paddedAudioSection(1, audios, 318)
	const tail = 16 + 4 // last ES plus CRC
	if len(section) != 183+184+tail {
		t.Fatalf("section is %d bytes, wanted %d", len(section), 183+184+tail)
	}
	if section[len(section)-tail] != streamAC3 {
		t.Fatalf("tail does not start at the last audio stream: %x", section[len(section)-tail])
	}
	raw := splitAudioTS(1, section, tail)
	tracks := AudioTracks(raw, 1)
	if len(tracks) != 3 {
		t.Fatalf("tracks: %+v", tracks)
	}
	if tracks[1].Role != "language" || tracks[1].PID != 0x102 || tracks[1].Label != "Spanish" {
		t.Fatalf("sap: %+v", tracks[1])
	}
	if tracks[2].Role != "described" || tracks[2].PID != 0x103 || tracks[2].Label != "Described video" {
		t.Fatalf("vi: %+v", tracks[2])
	}
}

func TestSourceForPicksSAP(t *testing.T) {
	f := &feed{tracks: AudioTracks(audioTS(1, []esAudio{
		{pid: 0x101, lang: "eng", audioType: 0, bsmod: 0},
		{pid: 0x102, lang: "spa", audioType: 0, bsmod: 0},
	}), 1)}
	src := f.sourceFor(Rendition{Video: "1080", Audio: "copy", Track: "lang"})
	if src.AudioPID != 0x102 || src.AudioCodec != "AC3" {
		t.Fatalf("source: %+v", src)
	}
}

type esAudio struct {
	pid       int
	lang      string
	audioType int
	bsmod     int
}

func audioTS(program int, audios []esAudio) []byte {
	const pmtPID = 0x1000
	pat := psiSection(0x00, append([]byte{0x00, 0x01, 0xc1, 0x00, 0x00}, progPID(program, pmtPID)...))
	pmt := psiSection(0x02, audioPMT(program, audios))
	var out []byte
	out = append(out, tsPacket(0, true, pat)...)
	out = append(out, tsPacket(pmtPID, true, pmt)...)
	return out
}

// paddedAudioSection inserts pad bytes of program info so the section
// crosses a packet boundary. The returned bytes are the section only.
func paddedAudioSection(program int, audios []esAudio, pad int) []byte {
	body := audioPMT(program, audios)
	stuffed := make([]byte, 0, len(body)+pad)
	stuffed = append(stuffed, body[:9]...)
	stuffed[7] = 0xf0 | byte(pad>>8)
	stuffed[8] = byte(pad)
	stuffed = append(stuffed, make([]byte, pad)...)
	stuffed = append(stuffed, body[9:]...)
	wrapped := psiSection(0x02, stuffed)
	return wrapped[1:]
}

// splitAudioTS packetizes section so the last tail bytes are only the
// pointer field of the packet that starts the next section.
func splitAudioTS(program int, section []byte, tail int) []byte {
	const pmtPID = 0x1000
	pat := psiSection(0x00, append([]byte{0x00, 0x01, 0xc1, 0x00, 0x00}, progPID(program, pmtPID)...))
	var out []byte
	out = append(out, tsPacket(0, true, pat)...)
	head := section[:len(section)-tail]
	first := make([]byte, 184)
	copy(first[1:], head[:183])
	out = append(out, tsPacket(pmtPID, true, first)...)
	for rest := head[183:]; len(rest) > 0; {
		n := 184
		if n > len(rest) {
			n = len(rest)
		}
		out = append(out, tsPacketExact(pmtPID, false, rest[:n])...)
		rest = rest[n:]
	}
	tailPkt := make([]byte, 1+tail+1)
	tailPkt[0] = byte(tail)
	copy(tailPkt[1:], section[len(section)-tail:])
	tailPkt[len(tailPkt)-1] = 0xFF
	out = append(out, tsPacketExact(pmtPID, true, tailPkt)...)
	return out
}

// tsPacketExact keeps a short payload from being zero-padded into the
// section. eachPacket reads through the end of the packet.
func tsPacketExact(pid int, start bool, payload []byte) []byte {
	if len(payload) == 184 {
		return tsPacket(pid, start, payload)
	}
	if len(payload) > 184 {
		panic("payload exceeds a TS packet")
	}
	pkt := make([]byte, 188)
	pkt[0] = 0x47
	pkt[1] = byte(pid >> 8)
	if start {
		pkt[1] |= 0x40
	}
	pkt[2] = byte(pid)
	pkt[3] = 0x30
	afl := 183 - len(payload)
	pkt[4] = byte(afl)
	copy(pkt[5+afl:], payload)
	return pkt
}

func audioPMT(program int, audios []esAudio) []byte {
	body := []byte{
		byte(program >> 8), byte(program),
		0xc1, 0x00, 0x00,
		0xe0, 0x00,
		0xf0, 0x00,
		streamMPEG2,
		0xe1, 0x00,
		0xf0, 0x00,
	}
	for _, a := range audios {
		desc := audioDesc(a)
		body = append(body, streamAC3)
		body = append(body, 0xe0|byte(a.pid>>8), byte(a.pid))
		body = append(body, 0xf0|byte(len(desc)>>8), byte(len(desc)))
		body = append(body, desc...)
	}
	return body
}

func audioDesc(a esAudio) []byte {
	var d []byte
	if a.lang != "" && a.audioType >= 0 {
		d = append(d, descISO639, 4, a.lang[0], a.lang[1], a.lang[2], byte(a.audioType))
	}
	if a.bsmod >= 0 {
		// sample_rate_code 0, bsid 8, bit_rate 0, surround 0, bsmod in the top 3 bits.
		d = append(d, descAC3, 3, 0x08, 0x00, byte(a.bsmod<<5))
	}
	return d
}
