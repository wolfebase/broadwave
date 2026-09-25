package fake

// Profile names select Server.Profile. Empty is the original two-tuner fake.
const (
	ProfileHDHR3US       = "HDHR3-US"
	ProfileConnectDuo    = "CONNECT-DUO"
	ProfileConnectQuatro = "CONNECT-QUATRO"
	ProfileFlexDuo       = "FLEX-DUO"
	ProfileFlexQuatro    = "FLEX-QUATRO"
	ProfileFlex4K        = "FLEX-4K"
	ProfilePrime         = "PRIME"
	ProfileExtend        = "EXTEND"
	ProfileScribe        = "SCRIBE"
	ProfileServio        = "SERVIO"
	ProfileOldFirmware   = "OLD-FIRMWARE"

	maxTuners = 8
)

// fakeDeviceAuth is the discover.json credential a tuner would send.
// It is not a real secret. It is never logged or written to disk.
const fakeDeviceAuth = "fake-device-auth"

type profile struct {
	Name      string
	Friendly  string
	Model     string
	Firmware  string
	Version   string
	Upgrade   string
	DeviceID  string
	Tuners    int
	Auth      bool
	legacy    bool
	Lock      string
	Extend    bool
	ATSC3     int
	Storage   bool
	Old       bool
	ownLineup bool
	channels  []Channel
}

func ProfileNames() []string {
	out := make([]string, len(catalog))
	for i, p := range catalog {
		out[i] = p.Name
	}
	return out
}

func defaultProfile() profile {
	return profile{
		Friendly: "Fake HDHomeRun",
		Model:    "HDHR4-2US",
		Firmware: "hdhomerun_fake",
		Version:  "20260101",
		DeviceID: "FAKEHDHR",
		Tuners:   2,
		Auth:     true,
		legacy:   true,
	}
}

func antennaChannels() []Channel {
	return []Channel{
		{Number: "4.1", Name: "WDAF", Freq: 593000000},
		{Number: "4.2", Name: "WDAF2", Freq: 593000000},
		{Number: "5.1", Name: "KCTV", Freq: 533000000},
	}
}

func flex4KChannels() []Channel {
	chs := antennaChannels()
	return append(chs,
		Channel{Number: "104.1", Name: "WDAF", Freq: 599000000, Video: "HEVC", Audio: "AC-4", ATSC3: true},
		Channel{Number: "105.1", Name: "LOCKED", Freq: 605000000, Video: "HEVC", Audio: "AC-4", ATSC3: true, DRM: true},
	)
}

func primeChannels() []Channel {
	return []Channel{
		{Number: "4", Name: "LOCAL", Freq: 507000000},
		{Number: "5", Name: "LOCAL2", Freq: 513000000},
		{Number: "702", Name: "HBO", Freq: 519000000, Copy: "copy-once", DRM: true},
		{Number: "703", Name: "SHOW", Freq: 525000000, Copy: "copy-never", DRM: true},
	}
}

func lookupProfile(name string) (profile, bool) {
	for _, p := range catalog {
		if p.Name == name {
			return p, true
		}
	}
	return profile{}, false
}

// catalog is the fleet. Tuner counts are the model's, not a hardcoded 2.
// Counts 1 and 5–8 are a TunerCount override on a profile, capped at 8.
var catalog = []profile{
	{
		Name: ProfileHDHR3US, Friendly: "HDHomeRun DUAL", Model: "HDHR3-US",
		Firmware: "hdhomerun3_atsc", Version: "20260101", Upgrade: "20260326",
		DeviceID: "A3E00001", Tuners: 2, Auth: true,
	},
	{
		Name: ProfileConnectDuo, Friendly: "HDHomeRun CONNECT DUO", Model: "HDHR5-2US",
		Firmware: "hdhomerun5_atsc", Version: "20260101", Upgrade: "20260326",
		DeviceID: "A3E00002", Tuners: 2, Auth: true,
	},
	{
		Name: ProfileConnectQuatro, Friendly: "HDHomeRun CONNECT QUATRO", Model: "HDHR5-4US",
		Firmware: "hdhomerun5_atsc", Version: "20260101", Upgrade: "20260326",
		DeviceID: "A3E00004", Tuners: 4, Auth: true,
	},
	{
		Name: ProfileFlexDuo, Friendly: "HDHomeRun FLEX DUO", Model: "HDFX-2US",
		Firmware: "hdhomerun_dvr_atsc", Version: "20260101", Upgrade: "20260326",
		DeviceID: "A3E00012", Tuners: 2, Auth: true,
	},
	{
		Name: ProfileFlexQuatro, Friendly: "HDHomeRun FLEX QUATRO", Model: "HDFX-4US",
		Firmware: "hdhomerun_dvr_atsc", Version: "20260101", Upgrade: "20260326",
		DeviceID: "A3E00014", Tuners: 4, Auth: true,
	},
	{
		Name: ProfileFlex4K, Friendly: "HDHomeRun FLEX 4K", Model: "HDFX-4K",
		Firmware: "hdhomerun_dvr_atsc3", Version: "20260101", Upgrade: "20260326",
		DeviceID: "A3E04C01", Tuners: 4, Auth: true, ATSC3: 2,
		ownLineup: true, channels: flex4KChannels(),
	},
	{
		Name: ProfilePrime, Friendly: "HDHomeRun PRIME", Model: "HDHR3-CC",
		Firmware: "hdhomerun3_cablecard", Version: "20260101", Upgrade: "20260326",
		DeviceID: "A3E00003", Tuners: 3, Auth: true, Lock: "qam256",
		ownLineup: true, channels: primeChannels(),
	},
	{
		Name: ProfileExtend, Friendly: "HDHomeRun EXTEND", Model: "HDTC-2US",
		Firmware: "hdhomeruntc_atsc", Version: "20260101", Upgrade: "20260326",
		DeviceID: "A3E00022", Tuners: 2, Auth: true, Extend: true,
	},
	{
		Name: ProfileScribe, Friendly: "HDHomeRun SCRIBE DUO", Model: "HDVR-2US-1TB",
		Firmware: "hdhomerun_dvr_atsc", Version: "20260101", Upgrade: "20260326",
		DeviceID: "A3E00032", Tuners: 2, Auth: true, Storage: true,
	},
	{
		Name: ProfileServio, Friendly: "HDHomeRun SERVIO", Model: "HHDD-2TB",
		Firmware: "hdhomerun_dvr", Version: "20260101",
		DeviceID: "A3E00050", Tuners: 0, Auth: true, Storage: true, ownLineup: true,
	},
	{
		Name: ProfileOldFirmware, Friendly: "HDHomeRun DUAL", Model: "HDHR3-US",
		Firmware: "hdhomerun3_atsc", Version: "20140301",
		DeviceID: "A3E00011", Tuners: 2, Old: true,
	},
}

func transcodeOK(profile string) bool {
	switch profile {
	case "heavy", "mobile", "internet540", "internet480", "internet360", "internet240":
		return true
	default:
		return false
	}
}

func markerPacket(markers ...string) []byte {
	pkt := make([]byte, 188)
	pkt[0] = 0x47
	pkt[3] = 0x10
	off := 4
	for _, marker := range markers {
		if off >= len(pkt) {
			break
		}
		off += copy(pkt[off:], marker)
	}
	return pkt
}
