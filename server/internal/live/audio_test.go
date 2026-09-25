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
