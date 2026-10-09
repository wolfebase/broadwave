package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestEnableDeviceAuthRollsBackWhenTheTokenCannotBeSaved(t *testing.T) {
	ctx := context.Background()
	st := openClients(t)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	if _, err := st.db.Exec(`CREATE TRIGGER client_devices_block BEFORE INSERT ON client_devices
BEGIN
	SELECT RAISE(ABORT, 'mint failed');
END`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnableDeviceAuth(ctx, map[string]string{"deviceAuth": "1", "pictureMode": "film"}, now); err == nil {
		t.Fatal("mint succeeded")
	}
	values, err := st.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if values["deviceAuth"] == "1" || values["pictureMode"] == "film" {
		t.Fatalf("settings saved with the failed mint: %v", values)
	}
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM client_devices`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("devices %d", n)
	}
	if _, err := st.db.Exec(`DROP TRIGGER client_devices_block`); err != nil {
		t.Fatal(err)
	}
	token, err := st.EnableDeviceAuth(ctx, map[string]string{"deviceAuth": "1", "pictureMode": "film"}, now)
	if err != nil || !strings.HasPrefix(token, "bw_") {
		t.Fatalf("enable %q %v", token, err)
	}
	values, err = st.Settings(ctx)
	if err != nil || values["deviceAuth"] != "1" || values["pictureMode"] != "film" {
		t.Fatalf("settings %v err %v", values, err)
	}
}

func TestPendingPairingCodesCannotCollide(t *testing.T) {
	ctx := context.Background()
	st := openClients(t)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	_, code, _, _, err := st.StartPairing(ctx, "claim", "", "other", []string{"watch"}, now)
	if err != nil {
		t.Fatal(err)
	}
	stamp := now.UTC().Format(time.RFC3339)
	_, err = st.db.Exec(`INSERT INTO pairings (id, mode, code_hash, scopes, created_at, expires_at, state)
VALUES ('dup', 'claim', ?, 'watch', ?, ?, 'pending')`, hashSecret(code), stamp, now.Add(time.Minute).UTC().Format(time.RFC3339))
	if err == nil {
		t.Fatal("two pending rows stored one code")
	}
}

func TestStartPairingStopsAtTwentyWaitingCodes(t *testing.T) {
	ctx := context.Background()
	st := openClients(t)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	var mu sync.Mutex
	okN, fullN := 0, 0
	var other []string
	codes := map[string]int{}
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, code, _, _, err := st.StartPairing(ctx, "claim", "", "other", nil, now)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				okN++
				codes[code]++
				return
			}
			if errors.Is(err, ErrPairFull) {
				fullN++
				return
			}
			other = append(other, err.Error())
		}()
	}
	wg.Wait()
	if len(other) > 0 {
		t.Fatal(other)
	}
	if okN != pairPendingMax || fullN != 20 {
		t.Fatalf("started %d full %d", okN, fullN)
	}
	if len(codes) != okN {
		t.Fatalf("duplicate codes %+v", codes)
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
