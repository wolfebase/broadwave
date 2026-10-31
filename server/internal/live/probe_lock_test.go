package live

import (
	"testing"
	"time"

	"broadwave/internal/hdhr/fake"
)

// A cable or DVB tuner locks as qam256 or t8qam64, never 8vsb. The probe
// waited out its 4 s ceiling on every first tune there, holding every watch.
func TestProbeTakesAnyLock(t *testing.T) {
	for _, profile := range []string{fake.ProfilePrime, fake.ProfileDVBT} {
		t.Run(profile, func(t *testing.T) {
			srv := &fake.Server{TS: t.TempDir() + "/none.ts", Profile: profile}
			_, port, err := srv.Start()
			if err != nil {
				t.Fatal(err)
			}
			defer srv.Close()
			t.Setenv("HDHR_CONTROL_PORT", port)
			guide := srv.Channels[0].Number
			began := time.Now()
			freq, programs, err := probe("127.0.0.1", 0, guide, darkProbe)
			if err != nil {
				t.Fatal(err)
			}
			if took := time.Since(began); took > 2*time.Second {
				t.Fatalf("probe took %v on a %s lock", took, profile)
			}
			if freq != srv.Channels[0].Freq || programFor(programs, guide) == 0 {
				t.Fatalf("freq %d programs %+v", freq, programs)
			}
		})
	}
}
