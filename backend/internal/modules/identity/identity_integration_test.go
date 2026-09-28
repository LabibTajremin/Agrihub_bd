//go:build integration

package identity_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"testing"
	"testing/iotest"
	"time"

	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/domain"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/repository"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/transport"
	"github.com/labibtajremin/agrihub_bd/backend/internal/modules/identity/usecase"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authn"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/authz"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/database"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/eventbus"
	"github.com/labibtajremin/agrihub_bd/backend/internal/platform/ratelimit"
	"github.com/labibtajremin/agrihub_bd/backend/test/apitest"
	"github.com/labibtajremin/agrihub_bd/backend/test/harness"
)

var policy = usecase.Policy{OTPTTL: 5 * time.Minute, RefreshTTL: 720 * time.Hour, OTPLength: 6, OTPMaxAttempts: 3,
	OTPPerNumber: 3, OTPPerIP: 10, GuestMode: true, ExposeOTP: true}

func hasher(r io.Reader) *authn.Argon2 {
	return authn.NewArgon2(authn.Argon2Params{Time: 1, Memory: 64, Threads: 1, KeyLen: 16}, r)
}

func setup(t *testing.T, p usecase.Policy) *apitest.API {
	t.Helper()
	a := apitest.New(t, harness.DB(t))
	m, err := identity.New(identity.Deps{Kernel: a.Kernel, Signer: a.Signer, Hasher: hasher(rand.Reader),
		Limiter: ratelimit.NewMemory(a.Clock), Random: rand.Reader, Policy: p})
	if err != nil {
		t.Fatal(err)
	}
	a.Mount(m.Routes())
	return a
}

func requestCode(t *testing.T, a *apitest.API, phone, token string) string {
	t.Helper()
	var out transport.OTPRequested
	a.Do("POST", "/v1/auth/otp/request", transport.PhoneRequest{Phone: phone}, token).Expect(t, 202).Decode(t, &out)
	if len(out.DevCode) != 6 || !out.ExpiresAt.Equal(apitest.Epoch.Add(5*time.Minute)) {
		t.Fatalf("%+v", out)
	}
	return out.DevCode
}

func signIn(t *testing.T, a *apitest.API, phone, token string) transport.Session {
	t.Helper()
	code := requestCode(t, a, phone, token)
	var s transport.Session
	a.Do("POST", "/v1/auth/otp/verify", transport.VerifyRequest{Phone: phone, Code: code}, token).Expect(t, 200).Decode(t, &s)
	return s
}

func wrong(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}

func TestOTP_SignInCreatesFarmerOnceAndPublishes(t *testing.T) {
	a := setup(t, policy)
	s := signIn(t, a, "01711-000000", "")
	if s.User.Role != "farmer" || s.User.Phone != "+8801711000000" || s.TokenType != "Bearer" || s.AccessToken == "" || s.RefreshToken == "" {
		t.Fatalf("%+v", s)
	}
	again := signIn(t, a, "+8801711000000", "")
	if again.User.ID != s.User.ID {
		t.Fatal("same phone must sign into the same user")
	}
	var me transport.User
	a.Do("GET", "/v1/me", nil, s.AccessToken).Expect(t, 200).Decode(t, &me)
	if me.ID != s.User.ID {
		t.Fatal(me)
	}
	var got []eventbus.Event
	a.Kernel.Bus.Subscribe(usecase.TopicUserRegistered, func(_ context.Context, e eventbus.Event) error { got = append(got, e); return nil })
	if _, err := a.Relay.Flush(context.Background()); err != nil || len(got) != 1 || got[0].ActorID != s.User.ID {
		t.Fatal("exactly one registration event", got, err)
	}
}

func TestOTP_WrongCodeCountsAndLocks(t *testing.T) {
	a := setup(t, policy)
	phone := "01811000000"
	code := requestCode(t, a, phone, "")
	for range policy.OTPMaxAttempts {
		if c := a.Do("POST", "/v1/auth/otp/verify", transport.VerifyRequest{Phone: phone, Code: wrong(code)}, "").Expect(t, 401).ErrorCode(); c != "auth.otp_invalid" {
			t.Fatal(c)
		}
	}
	if c := a.Do("POST", "/v1/auth/otp/verify", transport.VerifyRequest{Phone: phone, Code: code}, "").Expect(t, 429).ErrorCode(); c != "auth.otp_locked" {
		t.Fatal("failed attempts must persist and lock the challenge", c)
	}
}

func TestOTP_ExpiryMissingChallengeAndValidation(t *testing.T) {
	a := setup(t, policy)
	code := requestCode(t, a, "01911000000", "")
	a.Clock.Advance(5 * time.Minute)
	if c := a.Do("POST", "/v1/auth/otp/verify", transport.VerifyRequest{Phone: "01911000000", Code: code}, "").Expect(t, 401).ErrorCode(); c != "auth.otp_expired" {
		t.Fatal(c)
	}
	if c := a.Do("POST", "/v1/auth/otp/verify", transport.VerifyRequest{Phone: "01511000000", Code: "123456"}, "").Expect(t, 401).ErrorCode(); c != "auth.otp_invalid" {
		t.Fatal(c)
	}
	a.Do("POST", "/v1/auth/otp/request", transport.PhoneRequest{Phone: "12"}, "").Expect(t, 400)
	a.Do("POST", "/v1/auth/otp/verify", transport.VerifyRequest{Phone: "12", Code: "123456"}, "").Expect(t, 400)
	a.Do("POST", "/v1/auth/otp/request", "{", "").Expect(t, 400)
	a.Do("POST", "/v1/auth/otp/verify", "{", "").Expect(t, 400)
	a.Do("POST", "/v1/auth/refresh", "{", "").Expect(t, 400)
	a.Do("PATCH", "/v1/me", "{", a.Token("x", authz.Farmer)).Expect(t, 400)
}

func TestOTP_RateLimitsPerNumberAndPerIP(t *testing.T) {
	p := policy
	p.OTPPerNumber, p.OTPPerIP = 2, 3
	a := setup(t, p)
	requestCode(t, a, "01611000000", "")
	requestCode(t, a, "01611000000", "")
	if c := a.Do("POST", "/v1/auth/otp/request", transport.PhoneRequest{Phone: "01611000000"}, "").Expect(t, 429).ErrorCode(); c != "auth.otp_rate_limited" {
		t.Fatal(c)
	}
	// the 4th request from this IP is limited regardless of number
	if c := a.Do("POST", "/v1/auth/otp/request", transport.PhoneRequest{Phone: "01611000001"}, "").Expect(t, 429).ErrorCode(); c != "auth.otp_rate_limited" {
		t.Fatal(c)
	}
}

func TestGuest_FlowAndUpgradeKeepsIdentity(t *testing.T) {
	a := setup(t, policy)
	var g transport.Session
	a.Do("POST", "/v1/auth/guest", nil, "").Expect(t, 201).Decode(t, &g)
	if g.User.Role != "guest" || g.User.Phone != "" {
		t.Fatalf("%+v", g)
	}
	a.Do("GET", "/v1/me", nil, g.AccessToken).Expect(t, 200)
	a.Do("PATCH", "/v1/me", transport.UpdateMeRequest{}, g.AccessToken).Expect(t, 403)
	s := signIn(t, a, "01311000000", g.AccessToken)
	if s.User.ID != g.User.ID || s.User.Role != "farmer" {
		t.Fatal("a guest signing in is upgraded in place", s.User, g.User)
	}
}

func TestGuest_Disabled(t *testing.T) {
	p := policy
	p.GuestMode = false
	a := setup(t, p)
	if c := a.Do("POST", "/v1/auth/guest", nil, "").Expect(t, 403).ErrorCode(); c != "auth.guest_disabled" {
		t.Fatal(c)
	}
}

// TestRefreshToken_ReusedToken_RevokesFamily proves replay detection.
func TestRefreshToken_ReusedToken_RevokesFamily(t *testing.T) {
	a := setup(t, policy)
	s := signIn(t, a, "01411000000", "")
	var next transport.Tokens
	a.Do("POST", "/v1/auth/refresh", transport.RefreshRequest{RefreshToken: s.RefreshToken}, "").Expect(t, 200).Decode(t, &next)
	if next.RefreshToken == s.RefreshToken || next.AccessToken == "" {
		t.Fatal("rotation must mint a new token")
	}
	if c := a.Do("POST", "/v1/auth/refresh", transport.RefreshRequest{RefreshToken: s.RefreshToken}, "").Expect(t, 401).ErrorCode(); c != "auth.refresh_reused" {
		t.Fatal(c)
	}
	if c := a.Do("POST", "/v1/auth/refresh", transport.RefreshRequest{RefreshToken: next.RefreshToken}, "").Expect(t, 401).ErrorCode(); c != "auth.session_revoked" {
		t.Fatal("the whole family is revoked after a replay", c)
	}
}

func TestRefresh_InvalidExpiredAndLogout(t *testing.T) {
	a := setup(t, policy)
	if c := a.Do("POST", "/v1/auth/refresh", transport.RefreshRequest{RefreshToken: "nope"}, "").Expect(t, 401).ErrorCode(); c != "auth.refresh_invalid" {
		t.Fatal(c)
	}
	s := signIn(t, a, "01411000001", "")
	a.Do("POST", "/v1/auth/logout", nil, s.AccessToken).Expect(t, 204)
	if c := a.Do("POST", "/v1/auth/refresh", transport.RefreshRequest{RefreshToken: s.RefreshToken}, "").Expect(t, 401).ErrorCode(); c != "auth.session_revoked" {
		t.Fatal(c)
	}
	s2 := signIn(t, a, "01411000001", "")
	a.Clock.Advance(policy.RefreshTTL)
	if c := a.Do("POST", "/v1/auth/refresh", transport.RefreshRequest{RefreshToken: s2.RefreshToken}, "").Expect(t, 401).ErrorCode(); c != "auth.refresh_invalid" {
		t.Fatal(c)
	}
	a.Do("POST", "/v1/auth/logout", nil, "").Expect(t, 401)
}

func TestMe_UpdateAndMissingUser(t *testing.T) {
	a := setup(t, policy)
	s := signIn(t, a, "01711000009", "")
	name, lang, district := "Rahim", "en", "Bogura"
	var u transport.User
	a.Do("PATCH", "/v1/me", transport.UpdateMeRequest{Name: &name, Language: &lang, District: &district}, s.AccessToken).Expect(t, 200).Decode(t, &u)
	if u.Name != name || u.Language != lang || u.District != district {
		t.Fatal(u)
	}
	bad := "xx"
	a.Do("PATCH", "/v1/me", transport.UpdateMeRequest{Language: &bad}, s.AccessToken).Expect(t, 400)
	ghost := a.Token("00000000-0000-7000-8000-00000000abcd", authz.Farmer)
	if c := a.Do("GET", "/v1/me", nil, ghost).Expect(t, 404).ErrorCode(); c != "user.not_found" {
		t.Fatal(c)
	}
	a.Do("PATCH", "/v1/me", transport.UpdateMeRequest{Name: &name}, ghost).Expect(t, 404)
	a.Do("GET", "/v1/me", nil, "").Expect(t, 401)
}

// ---- use-case level tests: validation and storage/adapter failures ----

type fakeSMS struct{ err error }

func (f fakeSMS) SendOTP(context.Context, string, string) error { return f.err }

type fakeLimiter struct {
	err error
}

func (f fakeLimiter) AllowPerHour(context.Context, string, int) (bool, error) { return true, f.err }

type fakeIssuer struct{ err error }

func (f fakeIssuer) IssueAccess(string, string, string) (string, time.Time, error) {
	return "tok", time.Time{}, f.err
}

var boom = errors.New("boom")

func deps(t *testing.T, db *database.DB) usecase.Deps {
	a := apitest.New(t, db)
	return usecase.Deps{Users: repository.Users{DB: db}, Challenges: repository.Challenges{DB: db}, Sessions: repository.Sessions{DB: db},
		Hasher: hasher(rand.Reader), SMS: fakeSMS{}, Limiter: fakeLimiter{}, Access: fakeIssuer{}, Tx: db, Clock: a.Clock,
		IDs: a.Kernel.IDs, Random: rand.Reader, Events: a.Kernel.Events, Factory: a.Kernel.Factory(), Policy: policy}
}

func guestCtx(t *testing.T, d usecase.Deps) (context.Context, usecase.AuthResult) {
	res, err := usecase.StartGuest{Deps: d}.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return authn.WithPrincipal(context.Background(), authn.Principal{Subject: res.User.ID, SessionID: "s", Role: authz.Guest}), res
}

func TestRequestOTP_AdapterFailures(t *testing.T) {
	ctx := context.Background()
	in := usecase.RequestOTPInput{Phone: "01711000000", ClientIP: "1.1.1.1"}
	cases := map[string]func(d *usecase.Deps){
		"limiter": func(d *usecase.Deps) { d.Limiter = fakeLimiter{err: boom} },
		"random":  func(d *usecase.Deps) { d.Random = iotest.ErrReader(boom) },
		"hasher":  func(d *usecase.Deps) { d.Hasher = hasher(iotest.ErrReader(boom)) },
		"sms":     func(d *usecase.Deps) { d.SMS = fakeSMS{err: boom} },
		"insert":  func(d *usecase.Deps) {},
	}
	for name, mutate := range cases {
		db := harness.DB(t)
		if name == "insert" {
			db = harness.FaultyDB(t, harness.Fault{Op: "exec", Match: "INSERT INTO identity_otp_challenges", Err: boom})
		}
		d := deps(t, db)
		mutate(&d)
		if _, err := (usecase.RequestOTP{Deps: d}).Execute(ctx, in); !errors.Is(err, boom) {
			t.Errorf("%s: want boom, got %v", name, err)
		}
	}
	d := deps(t, harness.DB(t))
	d.Policy.ExposeOTP = false
	res, err := usecase.RequestOTP{Deps: d}.Execute(ctx, in)
	if err != nil || res.DevCode != "" {
		t.Fatal("code must not be exposed unless enabled", res, err)
	}
}

func TestVerifyOTP_StorageFailures(t *testing.T) {
	type tc struct {
		fault  harness.Fault
		guest  bool
		mutate func(*usecase.Deps)
	}
	cases := map[string]tc{
		"latest":         {fault: harness.Fault{Op: "queryrow", Match: "FROM identity_otp_challenges", Err: boom}},
		"consume":        {fault: harness.Fault{Op: "exec", Match: "SET consumed_at", Err: boom}},
		"increment":      {fault: harness.Fault{Op: "exec", Match: "attempts + 1", Err: boom}, mutate: func(d *usecase.Deps) {}},
		"lookup phone":   {fault: harness.Fault{Op: "queryrow", Match: "FROM identity_users WHERE phone", Err: boom}},
		"create user":    {fault: harness.Fault{Op: "exec", Match: "INSERT INTO identity_users", Err: boom}},
		"load guest":     {fault: harness.Fault{Op: "queryrow", Match: "WHERE id = $1", Err: boom}, guest: true},
		"upgrade guest":  {fault: harness.Fault{Op: "exec", Match: "UPDATE identity_users", Err: boom}, guest: true},
		"publish":        {fault: harness.Fault{Op: "exec", Match: "INSERT INTO outbox", Err: boom}},
		"create session": {fault: harness.Fault{Op: "exec", Match: "INSERT INTO identity_sessions", Err: boom}},
		"add token":      {fault: harness.Fault{Op: "exec", Match: "INSERT INTO identity_refresh_tokens", Err: boom}},
		"issue access":   {mutate: func(d *usecase.Deps) { d.Access = fakeIssuer{err: boom} }},
		"token entropy":  {mutate: func(d *usecase.Deps) { d.Random = iotest.ErrReader(boom) }},
	}
	n := 0
	for name, c := range cases {
		n++
		phone := fmt.Sprintf("0171120%04d", n)
		db := harness.FaultyDB(t, c.fault)
		d := deps(t, db)
		ctx := context.Background()
		if c.guest {
			ctx, _ = guestCtx(t, d)
		}
		d.Policy.ExposeOTP = true
		res, err := usecase.RequestOTP{Deps: d}.Execute(ctx, usecase.RequestOTPInput{Phone: phone, ClientIP: "ip"})
		if err != nil {
			t.Fatalf("%s: setup: %v", name, err)
		}
		if c.mutate != nil {
			c.mutate(&d)
		}
		code := res.DevCode
		if name == "increment" {
			code = wrong(code)
		}
		if _, err := (usecase.VerifyOTP{Deps: d}).Execute(ctx, usecase.VerifyOTPInput{Phone: phone, Code: code}); !errors.Is(err, boom) {
			t.Errorf("%s: want boom, got %v", name, err)
		}
	}
}

func TestStartGuestAndRefresh_StorageFailures(t *testing.T) {
	ctx := context.Background()
	d := deps(t, harness.FaultyDB(t, harness.Fault{Op: "exec", Match: "INSERT INTO identity_users", Err: boom}))
	if _, err := (usecase.StartGuest{Deps: d}).Execute(ctx); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	cases := map[string]harness.Fault{
		"session":   {Op: "queryrow", Match: "FROM identity_sessions", Err: boom},
		"mark used": {Op: "exec", Match: "SET used_at", Err: boom},
		"user":      {Op: "queryrow", Match: "FROM identity_users", Err: boom},
		"mint":      {Op: "exec", Match: "INSERT INTO identity_refresh_tokens", Skip: 1, Err: boom},
	}
	for name, f := range cases {
		d := deps(t, harness.FaultyDB(t, f))
		_, g := guestCtx(t, d)
		if _, err := (usecase.Refresh{Deps: d}).Execute(ctx, g.Tokens.RefreshToken); !errors.Is(err, boom) {
			t.Errorf("%s: want boom, got %v", name, err)
		}
	}
	d = deps(t, harness.FaultyDB(t, harness.Fault{Op: "exec", Match: "SET revoked_at", Err: boom}))
	_, g := guestCtx(t, d)
	_, _ = usecase.Refresh{Deps: d}.Execute(ctx, g.Tokens.RefreshToken)
	if _, err := (usecase.Refresh{Deps: d}).Execute(ctx, g.Tokens.RefreshToken); !errors.Is(err, boom) {
		t.Fatal("revocation failure on replay must surface", err)
	}
}

func TestUseCases_RequireAuthentication(t *testing.T) {
	d := deps(t, harness.DB(t))
	anon := context.Background()
	if err := (usecase.Logout{Deps: d}).Execute(anon); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
	if _, err := (usecase.GetMe{Deps: d}).Execute(anon); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
	if _, err := (usecase.UpdateMe{Deps: d}).Execute(anon, usecase.UpdateMeInput{}); !errors.Is(err, authn.ErrRequired) {
		t.Fatal(err)
	}
}

func TestLogout_StorageFailureOverHTTP(t *testing.T) {
	a := apitest.New(t, harness.FaultyDB(t, harness.Fault{Op: "exec", Match: "SET revoked_at", Err: boom}))
	m, _ := identity.New(identity.Deps{Kernel: a.Kernel, Signer: a.Signer, Hasher: hasher(rand.Reader),
		Limiter: ratelimit.NewMemory(a.Clock), Random: rand.Reader, Policy: policy})
	a.Mount(m.Routes())
	a.Do("POST", "/v1/auth/logout", nil, a.Token("u", authz.Farmer)).Expect(t, 500)
}

func TestUpdateMe_UseCaseValidatesLanguage(t *testing.T) {
	d := deps(t, harness.DB(t))
	ctx := authn.WithPrincipal(context.Background(), authn.Principal{Subject: "u", Role: authz.Farmer})
	bad := "xx"
	if _, err := (usecase.UpdateMe{Deps: d}).Execute(ctx, usecase.UpdateMeInput{Language: &bad}); !errors.Is(err, domain.ErrInvalidProfile) {
		t.Fatal(err)
	}
}

func TestRepository_PhoneConflictAndLogSMS(t *testing.T) {
	ctx := context.Background()
	users := repository.Users{DB: harness.DB(t)}
	now := apitest.Epoch
	u := domain.User{ID: "00000000-0000-7000-8000-000000000001", Phone: "+8801711000000", Role: "farmer", Language: "bn", CreatedAt: now, UpdatedAt: now}
	if err := users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	u.ID = "00000000-0000-7000-8000-000000000002"
	if err := users.Create(ctx, u); !errors.Is(err, domain.ErrPhoneTaken) {
		t.Fatal(err)
	}
	if err := (identity.LogSMS{Logger: apitest.New(t, harness.DB(t)).Kernel.Logger}).SendOTP(ctx, "+880", "123456"); err != nil {
		t.Fatal(err)
	}
	if len(identity.Errors()) != len(domain.Errors()) {
		t.Fatal()
	}
}
