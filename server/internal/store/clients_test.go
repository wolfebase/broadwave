package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientTokenIsHashedAndRevocable(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	dev, token, err := st.InsertClient(ctx, "Living room", "tv", []string{"admin", "watch"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, "bw_") || dev.ID == "" {
		t.Fatalf("token %q device %+v", token, dev)
	}
	if got := strings.Join(dev.Scopes, ","); got != "watch,admin" {
		t.Fatalf("scopes %s", got)
	}
	found, revoked, err := st.ClientByToken(ctx, token)
	if err != nil || revoked || found.ID != dev.ID {
		t.Fatalf("lookup %+v revoked %v err %v", found, revoked, err)
	}
	if err := st.TouchClient(ctx, dev.ID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	list, err := st.Clients(ctx)
	if err != nil || len(list) != 1 || list[0].LastSeenAt.IsZero() {
		t.Fatalf("list %+v err %v", list, err)
	}
	admin, err := st.HasAdmin(ctx)
	if err != nil || !admin {
		t.Fatalf("admin %v %v", admin, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	secretInDir(t, dir, token)

	st, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.RevokeClient(ctx, dev.ID, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeClient(ctx, dev.ID, now); err == nil {
		t.Fatal("second revoke succeeded")
	}
	found, revoked, err = st.ClientByToken(ctx, token)
	if err != nil || !revoked || found.ID != dev.ID {
		t.Fatalf("after revoke %+v %v %v", found, revoked, err)
	}
	list, err = st.Clients(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("revoked still listed %+v %v", list, err)
	}
	admin, err = st.HasAdmin(ctx)
	if err != nil || admin {
		t.Fatalf("admin after revoke %v %v", admin, err)
	}
	unknown, revoked, err := st.ClientByToken(ctx, "bw_nope")
	if err != nil || revoked || unknown.ID != "" {
		t.Fatalf("unknown %+v %v %v", unknown, revoked, err)
	}
}

func TestPairShowAndClaim(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)

	id, code, secret, expires, err := st.StartPairing(ctx, "show", "Den", "tv", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" || len(code) != 6 || !strings.HasPrefix(secret, "ps_") || !expires.Equal(now.Add(pairTTL)) {
		t.Fatalf("ticket %s %s %s %s", id, code, secret, expires)
	}
	if _, _, _, _, err := st.StartPairing(ctx, "show", "", "tv", nil, now); err == nil {
		t.Fatal("show pairing without a name")
	}
	if _, err := st.PollPairing(ctx, id, "ps_wrong", now); err == nil {
		t.Fatal("wrong poll secret")
	}
	polled, err := st.PollPairing(ctx, id, secret, now)
	if err != nil || polled.State != "pending" || polled.Name != "Den" {
		t.Fatalf("poll %+v %v", polled, err)
	}
	// A claim must not consume a code the TV is showing.
	if _, _, _, err := st.ClaimCode(ctx, code, "Other", "phone", now); err == nil {
		t.Fatal("claim matched a show code")
	}
	pairing, dev, token, err := st.ApproveShow(ctx, " "+code[:3]+" "+code[3:], []string{"record"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if pairing.DeviceID != dev.ID || !ScopeAllows(dev.Scopes, ScopeWatch) || ScopeAllows(dev.Scopes, ScopeAdmin) {
		t.Fatalf("approved %+v %+v", pairing, dev)
	}
	if _, _, _, err := st.ApproveShow(ctx, code, nil, now); err == nil {
		t.Fatal("code approved twice")
	}
	found, revoked, err := st.ClientByToken(ctx, token)
	if err != nil || revoked || found.Name != "Den" || found.Kind != "tv" {
		t.Fatalf("token device %+v %v %v", found, revoked, err)
	}

	_, claimCode, claimSecret, _, err := st.StartPairing(ctx, "claim", "", "phone", []string{"admin"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if claimSecret != "" {
		t.Fatalf("claim returned a poll secret %q", claimSecret)
	}
	if _, _, _, err := st.ApproveShow(ctx, claimCode, nil, now); err == nil {
		t.Fatal("approve matched a claim code")
	}
	_, claimed, claimToken, err := st.ClaimCode(ctx, claimCode, "Kitchen tablet", "phone", now)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Kind != "phone" || !ScopeAllows(claimed.Scopes, ScopeAdmin) {
		t.Fatalf("claimed %+v", claimed)
	}
	if _, _, _, err := st.ClaimCode(ctx, claimCode, "Again", "phone", now); err == nil {
		t.Fatal("claim used twice")
	}

	var stored string
	if err := st.db.QueryRowContext(ctx, `SELECT code_hash FROM pairings WHERE id = ?`, id).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == code || len(stored) != 64 {
		t.Fatalf("code stored as %q", stored)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	secretInDir(t, dir, token)
	secretInDir(t, dir, claimToken)
	secretInDir(t, dir, secret)
}

func TestPairingExpiryAndDenial(t *testing.T) {
	ctx := context.Background()
	st := openClients(t)
	now := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	if _, _, _, _, err := st.StartPairing(ctx, "show", "Hall", "other", []string{"nope"}, now); err == nil {
		t.Fatal("unknown scope accepted")
	}
	id, code, secret, _, err := st.StartPairing(ctx, "show", "Hall", "other", []string{"watch"}, now)
	if err != nil {
		t.Fatal(err)
	}
	later := now.Add(pairTTL + time.Second)
	if _, _, _, err := st.ApproveShow(ctx, code, nil, later); err != ErrPairExpired {
		t.Fatalf("expiry %v", err)
	}
	polled, err := st.PollPairing(ctx, id, secret, later)
	if err != nil || polled.State != "expired" {
		t.Fatalf("poll after expiry %+v %v", polled, err)
	}

	id, code, secret, _, err = st.StartPairing(ctx, "show", "Porch", "tv", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	for i := range pairDenyAfter {
		if _, err := st.PollPairing(ctx, id, "ps_wrong", now); err != ErrPairCode {
			t.Fatalf("try %d %v", i, err)
		}
	}
	if _, err := st.PollPairing(ctx, id, secret, now); err != ErrPairDenied {
		t.Fatalf("denied poll %v", err)
	}
	if _, _, _, err := st.ApproveShow(ctx, code, nil, now); err != ErrPairDenied {
		t.Fatalf("approve denied %v", err)
	}
}

func TestApproveWithoutScopesIsWatch(t *testing.T) {
	ctx := context.Background()
	st := openClients(t)
	now := time.Date(2026, 10, 9, 16, 0, 0, 0, time.UTC)
	_, code, _, _, err := st.StartPairing(ctx, "show", "Den", "tv", []string{"admin"}, now)
	if err != nil {
		t.Fatal(err)
	}
	_, dev, _, err := st.ApproveShow(ctx, code, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if !ScopeAllows(dev.Scopes, ScopeWatch) || ScopeAllows(dev.Scopes, ScopeRecord) || ScopeAllows(dev.Scopes, ScopeAdmin) {
		t.Fatalf("omitted approve scopes %v", dev.Scopes)
	}
	_, code, _, _, err = st.StartPairing(ctx, "show", "Hall", "tv", []string{"watch"}, now)
	if err != nil {
		t.Fatal(err)
	}
	_, dev, _, err = st.ApproveShow(ctx, code, []string{"admin"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !ScopeAllows(dev.Scopes, ScopeAdmin) {
		t.Fatalf("explicit approve scopes %v", dev.Scopes)
	}
}

func TestScopeAllows(t *testing.T) {
	if !ScopeAllows([]string{"admin"}, ScopeWatch) || !ScopeAllows([]string{"record"}, ScopeWatch) {
		t.Fatal("higher scope should cover watch")
	}
	if ScopeAllows([]string{"watch"}, ScopeRecord) || ScopeAllows(nil, ScopeWatch) || ScopeAllows([]string{"watch"}, "owner") {
		t.Fatal("scope check accepted too much")
	}
}

func openClients(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func secretInDir(t *testing.T, dir, secret string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), secret) {
			t.Fatalf("%s contains a pairing secret", entry.Name())
		}
	}
}
