package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"

	"waveguide/internal/hdhr/fake"
)

func main() {
	ts := flag.String("ts", "", "MPEG-TS file to loop")
	realtime := flag.Bool("realtime", false, "play the file over four seconds, then loop")
	flag.Parse()
	if *ts == "" {
		fmt.Fprintln(os.Stderr, "need -ts")
		os.Exit(2)
	}
	srv := &fake.Server{TS: *ts, Realtime: *realtime}
	base, port, err := srv.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("BASE=%s\nCONTROL_PORT=%s\n", base, port)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	srv.Close()
}
