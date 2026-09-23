package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"ota-viewer/internal/dvr"
	"ota-viewer/internal/httpapi"
	"ota-viewer/internal/live"
	"ota-viewer/internal/source"
	"ota-viewer/internal/store"
)

//go:embed all:assets
var embedded embed.FS

func main() {
	addr := flag.String("addr", ":8477", "listen address")
	configDir := flag.String("config", "data", "directory for the catalog database")
	dev := flag.Bool("dev", false, "allow a local Vite dev server to call the API")
	hdhrHost := flag.String("hdhr", os.Getenv("HDHR_HOST"), "tuner address when the container cannot hear broadcast discovery")
	flag.Parse()

	st, err := store.Open(*configDir)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	assets, err := fs.Sub(embedded, "assets")
	if err != nil {
		log.Fatal(err)
	}
	work := filepath.Join(*configDir, "work")
	ffmpegPath, _ := execLook("ffmpeg")
	encoder := live.DetectEncoder(ffmpegPath)
	hub := live.New(st, work, ffmpegPath, encoder)
	hub.OnSaved = func(rec store.Recording) {
		dvr.OnSaved(context.Background(), st, hub, rec)
	}
	log.Printf("encoder: %s deint: %s smooth: %s blend: %v", encoder, hub.DeintBroadcast, hub.DeintSmooth, hub.Blend)
	api := &httpapi.Server{Store: st, Assets: assets, Dev: *dev, Hub: hub}
	handler := api.Handler()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		n, err := source.Sync(ctx, st, nil, *hdhrHost)
		if err != nil {
			log.Printf("discovery: %v", err)
			return
		}
		log.Printf("discovery: %d device(s)", n)
		gctx, gcancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer gcancel()
		count, err := api.RefreshGuide(gctx)
		if err != nil {
			log.Printf("guide: %v", err)
			return
		}
		log.Printf("guide: %d airings", count)
	}()
	go func() {
		tick := time.NewTicker(20 * time.Second)
		defer tick.Stop()
		for range tick.C {
			dvr.Tick(context.Background(), st, hub)
			if hub != nil {
				hub.ReleaseAbandoned(45 * time.Second)
				httpapi.SyncEmulator(st, hub.FFmpeg)
			}
		}
	}()

	log.Printf("OTA Viewer listening on %s", *addr)
	server := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func execLook(name string) (string, error) {
	return exec.LookPath(name)
}
