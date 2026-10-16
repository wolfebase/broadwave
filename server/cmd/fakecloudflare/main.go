// Command fakecloudflare serves a fake Cloudflare sign-in and API for the
// local harness, so Settings, Away from home can be walked end to end without
// an account. Point the server at it with BROADWAVE_CF_DASH_BASE=<base>,
// BROADWAVE_CF_LOGIN_BASE=<base>/login, and BROADWAVE_CF_API_BASE=<base>/client/v4.
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"broadwave/internal/cloudflare/cftest"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18990", "listen address")
	zone := flag.String("zone", "example.com", "the zone's name")
	hidden := flag.Bool("zone-hidden", false, "refuse the zone lookup")
	noZone := flag.Bool("no-zone", false, "hand out a certificate without a zone")
	taken := flag.String("taken", "", "a name already in DNS, such as tv.example.com")
	flag.Parse()
	f := cftest.New()
	f.ZoneName = *zone
	f.ZoneHidden = *hidden
	f.NoZone = *noZone
	f.Hold = 20 * time.Second
	if *taken != "" {
		f.AddDNS(*taken, "203.0.113.10")
	}
	log.Printf("fake cloudflare on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, f))
}
