// Package cloudflare signs in to a Cloudflare account the way cloudflared's
// login does, then makes the one tunnel and DNS name Broadwave needs. The
// sign-in certificate controls every tunnel in the account, so it lives only
// in memory and is dropped once setup is done. The tunnel's own run token is
// kept in a file beside the catalog, never in it.
package cloudflare

import (
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"strings"
)

// Cert is what the sign-in hands back: one zone, its account, and an API
// token for tunnels.
type Cert struct {
	ZoneID    string `json:"zoneID"`
	AccountID string `json:"accountID"`
	APIToken  string `json:"apiToken"`
}

// DecodeCert reads the PEM block cloudflared calls ARGO TUNNEL TOKEN.
func DecodeCert(raw []byte) (Cert, error) {
	for blk, rest := pem.Decode(raw); blk != nil; blk, rest = pem.Decode(rest) {
		if blk.Type != "ARGO TUNNEL TOKEN" {
			continue
		}
		var c Cert
		if err := json.Unmarshal(blk.Bytes, &c); err != nil {
			return Cert{}, errors.New("cloudflare sent a certificate Broadwave can't read")
		}
		if c.AccountID == "" || c.APIToken == "" {
			return Cert{}, errors.New("cloudflare sent a certificate without a token")
		}
		return c, nil
	}
	return Cert{}, errors.New("cloudflare sent something that is not a tunnel certificate")
}

// Endpoints are Cloudflare's addresses. Tests and the local harness point
// them at a fake.
type Endpoints struct {
	// Dash serves the page where the owner picks a domain and authorizes.
	Dash string
	// Login holds the certificate until the server picks it up.
	Login string
	// API is the v4 REST API.
	API string
}

// DefaultEndpoints are Cloudflare's. BROADWAVE_CF_DASH_BASE,
// BROADWAVE_CF_LOGIN_BASE, and BROADWAVE_CF_API_BASE replace them, for tests
// only.
func DefaultEndpoints() Endpoints {
	e := Endpoints{
		Dash:  "https://dash.cloudflare.com",
		Login: "https://login.cloudflareaccess.org",
		API:   "https://api.cloudflare.com/client/v4",
	}
	if v := os.Getenv("BROADWAVE_CF_DASH_BASE"); v != "" {
		e.Dash = v
	}
	if v := os.Getenv("BROADWAVE_CF_LOGIN_BASE"); v != "" {
		e.Login = v
	}
	if v := os.Getenv("BROADWAVE_CF_API_BASE"); v != "" {
		e.API = v
	}
	e.Dash = strings.TrimRight(e.Dash, "/")
	e.Login = strings.TrimRight(e.Login, "/")
	e.API = strings.TrimRight(e.API, "/")
	return e
}
