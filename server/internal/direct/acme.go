package direct

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
)

// Let's Encrypt directories. BROADWAVE_DIRECT_ACME=staging picks the test one.
const (
	LetsEncrypt        = "https://acme-v02.api.letsencrypt.org/directory"
	LetsEncryptStaging = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// Challenge publishes and removes the DNS-01 record for the name.
type Challenge interface {
	Set(ctx context.Context, value string) error
	Clear(ctx context.Context) error
}

// Issuer gets a certificate for one name with a DNS-01 challenge. It returns
// the chain and the private key, both PEM.
type Issuer interface {
	Issue(ctx context.Context, name string, ch Challenge) (chain, key []byte, err error)
}

// ACME is an Issuer for an ACME certificate authority such as Let's Encrypt.
type ACME struct {
	Directory string
	// AccountKey is the PEM file for the ACME account key; made the first time.
	AccountKey string
	HTTP       *http.Client
	// Propagated waits until the TXT record is visible. Nil asks Cloudflare's resolver.
	Propagated func(ctx context.Context, fqdn, value string) error
}

func (a *ACME) client(ctx context.Context) (*acme.Client, error) {
	key, err := loadECKey(a.AccountKey)
	if err != nil {
		return nil, err
	}
	c := &acme.Client{Key: key, DirectoryURL: a.Directory, HTTPClient: a.HTTP, UserAgent: "broadwave-direct"}
	if _, err := c.Register(ctx, &acme.Account{}, acme.AcceptTOS); err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return nil, fmt.Errorf("acme account: %w", err)
	}
	return c, nil
}

func (a *ACME) Issue(ctx context.Context, name string, ch Challenge) ([]byte, []byte, error) {
	c, err := a.client(ctx)
	if err != nil {
		return nil, nil, err
	}
	order, err := c.AuthorizeOrder(ctx, acme.DomainIDs(name))
	if err != nil {
		return nil, nil, fmt.Errorf("acme order: %w", err)
	}
	for _, u := range order.AuthzURLs {
		if err := a.authorize(ctx, c, u, ch); err != nil {
			return nil, nil, err
		}
	}
	order, err = c.WaitOrder(ctx, order.URI)
	if err != nil {
		return nil, nil, fmt.Errorf("acme order: %w", err)
	}
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:  pkix.Name{CommonName: name},
		DNSNames: []string{name},
	}, certKey)
	if err != nil {
		return nil, nil, err
	}
	ders, _, err := c.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return nil, nil, fmt.Errorf("acme certificate: %w", err)
	}
	var chain []byte
	for _, der := range ders {
		chain = append(chain, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	keyPEM, err := ecPEM(certKey)
	if err != nil {
		return nil, nil, err
	}
	return chain, keyPEM, nil
}

func (a *ACME) authorize(ctx context.Context, c *acme.Client, authzURL string, ch Challenge) error {
	z, err := c.GetAuthorization(ctx, authzURL)
	if err != nil {
		return fmt.Errorf("acme authorization: %w", err)
	}
	if z.Status == acme.StatusValid {
		return nil
	}
	var chal *acme.Challenge
	for _, x := range z.Challenges {
		if x.Type == "dns-01" {
			chal = x
		}
	}
	if chal == nil {
		return errors.New("acme: no dns-01 challenge offered")
	}
	value, err := c.DNS01ChallengeRecord(chal.Token)
	if err != nil {
		return err
	}
	if err := ch.Set(ctx, value); err != nil {
		return fmt.Errorf("challenge record: %w", err)
	}
	defer func() {
		clear, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		_ = ch.Clear(clear)
	}()
	wait := a.Propagated
	if wait == nil {
		wait = dohPropagated(a.HTTP)
	}
	if err := wait(ctx, "_acme-challenge."+z.Identifier.Value, value); err != nil {
		return err
	}
	if _, err := c.Accept(ctx, chal); err != nil {
		return fmt.Errorf("acme accept: %w", err)
	}
	if _, err := c.WaitAuthorization(ctx, authzURL); err != nil {
		return fmt.Errorf("acme validation: %w", err)
	}
	return nil
}

// dohPropagated asks Cloudflare's public resolver, which answers fresh for a
// Cloudflare zone, until the TXT value shows, for up to two minutes.
func dohPropagated(hc *http.Client) func(ctx context.Context, fqdn, value string) error {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return func(ctx context.Context, fqdn, value string) error {
		deadline := time.Now().Add(2 * time.Minute)
		for {
			if txtHas(ctx, hc, fqdn, value) {
				// Other resolvers the CA asks may lag the first one a little.
				return sleep(ctx, 5*time.Second)
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("the challenge record for %s did not appear", fqdn)
			}
			if err := sleep(ctx, 4*time.Second); err != nil {
				return err
			}
		}
	}
}

func txtHas(ctx context.Context, hc *http.Client, fqdn, value string) bool {
	u := "https://cloudflare-dns.com/dns-query?type=TXT&name=" + url.QueryEscape(fqdn)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/dns-json")
	res, err := hc.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var out struct {
		Answer []struct {
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if json.NewDecoder(res.Body).Decode(&out) != nil {
		return false
	}
	for _, a := range out.Answer {
		if strings.Trim(a.Data, `"`) == value {
			return true
		}
	}
	return false
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func loadECKey(path string) (crypto.Signer, error) {
	if raw, err := os.ReadFile(path); err == nil {
		if block, _ := pem.Decode(raw); block != nil {
			if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
				return k, nil
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	out, err := ecPEM(k)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return nil, err
	}
	return k, nil
}

func ecPEM(k *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalECPrivateKey(k)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), nil
}
