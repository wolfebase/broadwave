// Package direct gives a home server a publicly trusted HTTPS name on its own
// network, with nothing for the owner to set up. The Wolfe Up direct service
// hands each server a random label, points lan.<label>.<base> at the server's
// home network address (private ranges only), and sets the ACME DNS-01 record,
// so the server can get a Let's Encrypt certificate for that name. Apps that
// reach the name get HTTP/2, which low-latency HLS on Apple devices requires.
package direct

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultAPI is the Wolfe Up direct service.
const DefaultAPI = "https://api.direct.wolfeup.com"

// message is what a call's signature covers. The Worker builds the same bytes
// (infra/direct/src/sign.ts).
func message(method, path, unix string, body []byte) []byte {
	var b bytes.Buffer
	b.WriteString("broadwave-direct-v1\n")
	b.WriteString(strings.ToUpper(method))
	b.WriteByte('\n')
	b.WriteString(path)
	b.WriteByte('\n')
	b.WriteString(unix)
	b.WriteByte('\n')
	b.Write(body)
	return b.Bytes()
}

// Client calls the direct service with a server's key.
type Client struct {
	API  string
	Key  ed25519.PrivateKey
	HTTP *http.Client
	Now  func() time.Time
}

// Registration is the label the service gave this server and the name it serves.
type Registration struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// apiError is a refusal from the service, with its words.
type apiError struct {
	Status int
	Text   string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("direct service: %d %s", e.Status, e.Text)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			return err
		}
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	unix := strconv.FormatInt(now().Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.API, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Broadwave-Time", unix)
	req.Header.Set("X-Broadwave-Signature", base64.StdEncoding.EncodeToString(ed25519.Sign(c.Key, message(method, path, unix, raw))))
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if res.StatusCode/100 != 2 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = http.StatusText(res.StatusCode)
		}
		return &apiError{Status: res.StatusCode, Text: e.Error}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Register asks for a label. The same key always gets the same label back.
func (c *Client) Register(ctx context.Context, serverID string) (Registration, error) {
	var reg Registration
	body := map[string]string{
		"publicKey": base64.StdEncoding.EncodeToString(c.Key.Public().(ed25519.PublicKey)),
		"serverId":  serverID,
	}
	if err := c.do(ctx, http.MethodPost, "/v1/register", body, &reg); err != nil {
		return Registration{}, err
	}
	if !validLabel(reg.ID) || reg.Name == "" {
		return Registration{}, errors.New("direct service: bad registration reply")
	}
	return reg, nil
}

// Addresses points the name at these addresses, replacing the ones before.
func (c *Client) Addresses(ctx context.Context, id string, addrs []netip.Addr) error {
	body := struct {
		IPv4 []string `json:"ipv4"`
		IPv6 []string `json:"ipv6"`
	}{IPv4: []string{}, IPv6: []string{}}
	for _, a := range addrs {
		if a.Is4() {
			body.IPv4 = append(body.IPv4, a.String())
		} else {
			body.IPv6 = append(body.IPv6, a.String())
		}
	}
	return c.do(ctx, http.MethodPut, "/v1/"+id+"/addresses", body, nil)
}

// SetChallenge publishes the DNS-01 value for the name.
func (c *Client) SetChallenge(ctx context.Context, id, value string) error {
	return c.do(ctx, http.MethodPut, "/v1/"+id+"/acme", map[string]string{"value": value}, nil)
}

// ClearChallenge removes the DNS-01 record.
func (c *Client) ClearChallenge(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/"+id+"/acme", nil, nil)
}

func validLabel(id string) bool {
	if len(id) != 10 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= '2' && c <= '7') {
			return false
		}
	}
	return true
}

// LoadKey reads the server's direct key, or makes one the first time. It
// lives in the config folder, never in the catalog, so a backup restored on
// another computer does not take this server's name.
func LoadKey(path string) (ed25519.PrivateKey, error) {
	if b, err := os.ReadFile(path); err == nil && len(b) == ed25519.PrivateKeySize {
		return ed25519.PrivateKey(b), nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, priv, 0o600); err != nil {
		return nil, err
	}
	return priv, nil
}
