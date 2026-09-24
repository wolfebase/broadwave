package live

import "testing"

func tuner(index int, busy bool) Tuner {
	t := Tuner{Index: index}
	if busy {
		t.Guide = "4.1"
	}
	return t
}

func TestPickTunerFailsOverToTheNextDevice(t *testing.T) {
	devices := []DeviceTuners{
		{Host: "192.168.1.20", Tuners: []Tuner{tuner(0, true), tuner(1, true)}},
		{Host: "192.168.1.30", Tuners: []Tuner{tuner(0, true), tuner(1, false)}},
	}
	host, n, ok := PickTuner(devices, nil, nil)
	if !ok || host != "192.168.1.30" || n != 1 {
		t.Fatalf("host %s tuner %d ok %v", host, n, ok)
	}
}

func TestHoldBackLeavesATunerForARecording(t *testing.T) {
	tuners := []Tuner{tuner(0, false), tuner(1, false)}
	held := HoldBack(tuners, 1)
	host, n, ok := PickTuner([]DeviceTuners{{Host: "192.168.1.20", Tuners: tuners}}, nil, held)
	if !ok || host != "192.168.1.20" || n != 0 {
		t.Fatalf("watching should take tuner 0, got %s %d %v", host, n, ok)
	}
	if _, _, ok := PickTuner([]DeviceTuners{{Host: "192.168.1.20", Tuners: []Tuner{tuner(0, true), tuner(1, false)}}}, nil, held); ok {
		t.Fatal("the last tuner is held for the recording")
	}
}

func TestPickTunerStaysOnTheFirstDevice(t *testing.T) {
	devices := []DeviceTuners{
		{Host: "192.168.1.20", Tuners: []Tuner{tuner(0, false), tuner(1, true)}},
		{Host: "192.168.1.30", Tuners: []Tuner{tuner(0, false)}},
	}
	host, n, ok := PickTuner(devices, nil, nil)
	if !ok || host != "192.168.1.20" || n != 0 {
		t.Fatalf("host %s tuner %d ok %v", host, n, ok)
	}
}
