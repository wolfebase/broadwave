package httpapi

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "image/gif"
)

func (s *Server) art(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	rawURL, label, knownW, knownH, err := s.Store.Artwork(r.Context(), kind, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	width := 320
	if q := r.URL.Query().Get("w"); q != "" {
		if n, err := strconv.Atoi(q); err == nil {
			width = n
		}
	}
	if width < 32 {
		width = 32
	}
	if width > 960 {
		width = 960
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || (!strings.HasPrefix(rawURL, "https://") && !strings.HasPrefix(rawURL, "http://")) {
		writePlaceholder(w, label)
		return
	}
	sum := sha1.Sum([]byte(rawURL))
	name := fmt.Sprintf("%s-%d-w%d-%s", kind, id, width, hex.EncodeToString(sum[:4]))
	dir := ""
	if s.Hub != nil && s.Hub.Dir != "" {
		dir = filepath.Join(s.Hub.Dir, "art")
	}
	if dir != "" {
		if body, ctype, ok := readCached(dir, name); ok {
			if knownW == 0 || knownH == 0 {
				if pw, ph, perr := probeArtSize(r.Context(), rawURL); perr == nil {
					_ = s.Store.SetArtworkSize(r.Context(), kind, id, pw, ph)
				}
			}
			w.Header().Set("Content-Type", ctype)
			w.Header().Set("Cache-Control", "public, max-age=86400")
			_, _ = w.Write(body)
			return
		}
	}
	body, ctype, nativeW, nativeH, err := fetchArt(r.Context(), rawURL, width)
	if err != nil {
		writePlaceholder(w, label)
		return
	}
	if nativeW > 0 && nativeH > 0 {
		_ = s.Store.SetArtworkSize(r.Context(), kind, id, nativeW, nativeH)
	}
	if dir != "" {
		_ = os.MkdirAll(dir, 0o755)
		ext := ".jpg"
		if ctype == "image/png" {
			ext = ".png"
		}
		_ = os.WriteFile(filepath.Join(dir, name+ext), body, 0o644)
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(body)
}

func readCached(dir, name string) ([]byte, string, bool) {
	for _, item := range []struct{ ext, ctype string }{{".jpg", "image/jpeg"}, {".png", "image/png"}} {
		body, err := os.ReadFile(filepath.Join(dir, name+item.ext))
		if err == nil && len(body) > 0 {
			return body, item.ctype, true
		}
	}
	return nil, "", false
}

func getArtBytes(ctx context.Context, rawURL string) ([]byte, error) {
	client := &http.Client{
		Timeout: 12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 3 {
				return fmt.Errorf("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("redirect left http")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Broadwave/0.1")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("art returned %s", res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 4<<20))
}

func probeArtSize(ctx context.Context, rawURL string) (int, int, error) {
	raw, err := getArtBytes(ctx, rawURL)
	if err != nil {
		return 0, 0, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

func fetchArt(ctx context.Context, rawURL string, width int) ([]byte, string, int, int, error) {
	raw, err := getArtBytes(ctx, rawURL)
	if err != nil {
		return nil, "", 0, 0, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, "", 0, 0, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", 0, 0, err
	}
	fitted := fitWidth(img, width)
	var buf bytes.Buffer
	if hasAlpha(fitted) {
		if err := png.Encode(&buf, fitted); err != nil {
			return nil, "", 0, 0, err
		}
		return buf.Bytes(), "image/png", cfg.Width, cfg.Height, nil
	}
	if err := jpeg.Encode(&buf, fitted, &jpeg.Options{Quality: 80}); err != nil {
		return nil, "", 0, 0, err
	}
	return buf.Bytes(), "image/jpeg", cfg.Width, cfg.Height, nil
}

func fitWidth(src image.Image, maxW int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxW || w < 1 || h < 1 {
		return src
	}
	dstH := h * maxW / w
	if dstH < 1 {
		dstH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, maxW, dstH))
	for y := 0; y < dstH; y++ {
		sy0 := y * h / dstH
		sy1 := (y + 1) * h / dstH
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < maxW; x++ {
			sx0 := x * w / maxW
			sx1 := (x + 1) * w / maxW
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, bl, a, n uint32
			for yy := sy0; yy < sy1; yy++ {
				for xx := sx0; xx < sx1; xx++ {
					pr, pg, pb, pa := src.At(b.Min.X+xx, b.Min.Y+yy).RGBA()
					r += pr
					g += pg
					bl += pb
					a += pa
					n++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8((r / n) >> 8), uint8((g / n) >> 8), uint8((bl / n) >> 8), uint8((a / n) >> 8)})
		}
	}
	return dst
}

func hasAlpha(img image.Image) bool {
	switch img.(type) {
	case *image.YCbCr, *image.Gray, *image.CMYK:
		return false
	}
	b := img.Bounds()
	points := []image.Point{
		b.Min,
		{X: b.Min.X + b.Dx()/2, Y: b.Min.Y + b.Dy()/2},
		{X: b.Max.X - 1, Y: b.Max.Y - 1},
	}
	for _, p := range points {
		_, _, _, a := img.At(p.X, p.Y).RGBA()
		if a < 0xff00 {
			return true
		}
	}
	return false
}

func writePlaceholder(w http.ResponseWriter, label string) {
	label = strings.TrimSpace(label)
	if label == "" {
		label = "Guide"
	}
	if len(label) > 28 {
		label = label[:28]
	}
	body := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180" viewBox="0 0 320 180"><rect width="320" height="180" fill="#1c2430"/><text x="160" y="96" text-anchor="middle" fill="#d7dee8" font-family="sans-serif" font-size="22">%s</text></svg>`, html.EscapeString(label))
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(body))
}
