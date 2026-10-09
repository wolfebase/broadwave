package guide

import "testing"

func FuzzParse(f *testing.F) {
	f.Add([]byte(`<tv><channel id="1"><display-name>News</display-name></channel><programme start="20260101120000 +0000" stop="20260101130000 +0000" channel="1"><title>News</title></programme></tv>`))
	f.Add([]byte(`<tv>`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			data = data[:64<<10]
		}
		_, _, _ = Parse(data, nil)
	})
}
