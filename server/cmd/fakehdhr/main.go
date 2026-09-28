package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"

	"broadwave/internal/hdhr/fake"
)

func main() {
	ts := flag.String("ts", "", "MPEG-TS file to loop")
	realtime := flag.Bool("realtime", false, "play the file over four seconds, then loop")
	source := flag.String("source", "", "TS file to stream at its own pace on a loop instead of the test pattern")
	source5 := flag.String("source5", "", "TS file for 5.x channels; -source plays the others")
	raw := flag.Bool("raw", false, "play -ts byte for byte at its own pace, so each loop is a real timestamp break")
	quad := flag.Bool("quad", false, "add 5.2 beside 5.1 so four channels share two tuners")
	flag.Parse()
	if *ts == "" {
		fmt.Fprintln(os.Stderr, "need -ts")
		os.Exit(2)
	}
	if *raw && (*realtime || *source != "" || *source5 != "") {
		fmt.Fprintln(os.Stderr, "-raw plays -ts on its own; drop -realtime and -source")
		os.Exit(2)
	}
	srv := &fake.Server{TS: *ts, Realtime: *realtime, Raw: *raw, Source: *source, Source5: *source5}
	if *quad {
		srv.Channels = fake.QuadLineup()
	}
	base, port, err := srv.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := serveAdmin(srv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		srv.Close()
		os.Exit(1)
	}
	fmt.Printf("BASE=%s\nCONTROL_PORT=%s\n", base, port)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	srv.Close()
}

// serveAdmin listens on FAKEHDHR_ADMIN for the local harness. Unset, it does nothing.
// The address has to be localhost. It is how a test holds the tuners or stops them answering.
func serveAdmin(srv *fake.Server) error {
	addr := os.Getenv("FAKEHDHR_ADMIN")
	if addr == "" {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host != "127.0.0.1" && host != "localhost" {
		return fmt.Errorf("FAKEHDHR_ADMIN must stay on localhost")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	post := func(path string, fn func()) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "post", http.StatusMethodNotAllowed)
				return
			}
			fn()
			w.WriteHeader(http.StatusNoContent)
		})
	}
	post("/hold", srv.HoldAll)
	post("/free", srv.FreeAll)
	post("/silence", srv.Silence)
	post("/answer", srv.Answer)
	channel := func(path string, fn func(string)) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "post", http.StatusMethodNotAllowed)
				return
			}
			number := r.URL.Query().Get("channel")
			if number == "" {
				http.Error(w, "channel", http.StatusBadRequest)
				return
			}
			fn(number)
			w.WriteHeader(http.StatusNoContent)
		})
	}
	channel("/dark", srv.Dark)
	channel("/light", srv.Light)
	go func() { _ = http.Serve(ln, mux) }()
	return nil
}
