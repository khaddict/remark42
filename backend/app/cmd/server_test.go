package cmd

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-pkgz/auth/v2/token"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jessevdk/go-flags"
	"go.uber.org/goleak"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerApp(t *testing.T) {
	port := chooseRandomUnusedPort()
	app, ctx, cancel := prepServerApp(t, func(o ServerCommand) ServerCommand {
		o.Port = port
		return o
	})

	go func() { _ = app.run(ctx) }()
	waitForHTTPServerStart(port)

	// send ping
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/v1/ping", port))
	defer http.DefaultClient.CloseIdleConnections()
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "pong", string(body))

	// add comment
	client := http.Client{Timeout: 10 * time.Second}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/api/v1/comment?site=remark", port),
		strings.NewReader(`{"text": "test 123", "locator":{"url": "https://radio-t.com/blah1", "site": "remark"}}`))
	require.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	body, _ = io.ReadAll(resp.Body)
	t.Log(string(body))

	email, err := app.dataService.AdminStore.Email("")
	assert.NoError(t, err)
	assert.Equal(t, "admin@demo.remark42.com", email, "default admin email")

	cancel()
	app.Wait()
}

func TestServerApp_Failed(t *testing.T) {
	opts := ServerCommand{}
	opts.SetCommon(CommonOpts{RemarkURL: "https://demo.remark42.com", SharedSecret: "123456"})

	p := flags.NewParser(&opts, flags.Default)

	// RO bolt location
	_, err := p.ParseArgs([]string{"--backup=/tmp", "--store.bolt.path=/dev/null", "--image.fs.path=/tmp"})
	assert.NoError(t, err)
	_, err = opts.newServerApp(context.Background())
	assert.EqualError(t, err, "failed to make data store engine: failed to create bolt store: can't make directory /dev/null: mkdir /dev/null: not a directory")
	t.Log(err)

	// RO backup location
	opts = ServerCommand{}
	opts.SetCommon(CommonOpts{RemarkURL: "https://demo.remark42.com", SharedSecret: "123456"})

	_, err = p.ParseArgs([]string{"--store.bolt.path=/tmp", "--backup=/dev/null/not-writable"})
	assert.NoError(t, err)
	defer os.Remove("/tmp/remark.db")
	_, err = opts.newServerApp(context.Background())
	assert.EqualError(t, err, "failed to create backup store: can't make directory /dev/null/not-writable: mkdir /dev/null: not a directory")
	t.Log(err)

	// invalid url
	opts = ServerCommand{}
	opts.SetCommon(CommonOpts{RemarkURL: "demo.remark42.com", SharedSecret: "123456"})

	_, err = p.ParseArgs([]string{"--backup=/tmp", "----store.bolt.path=/tmp"})
	assert.NoError(t, err)
	_, err = opts.newServerApp(context.Background())
	assert.EqualError(t, err, "invalid remark42 url demo.remark42.com")
	t.Log(err)

	// invalid trusted proxy CIDR fails fast, before any resource is created
	opts = ServerCommand{}
	opts.SetCommon(CommonOpts{RemarkURL: "https://demo.remark42.com", SharedSecret: "123456"})
	p = flags.NewParser(&opts, flags.Default)
	_, err = p.ParseArgs([]string{"--backup=/tmp", "--trusted-proxy=nonsense"})
	assert.NoError(t, err)
	_, err = opts.newServerApp(context.Background())
	assert.EqualError(t, err, `invalid --trusted-proxy: invalid trusted proxy "nonsense"`)
	t.Log(err)

	// wrong store type
	opts = ServerCommand{}
	opts.SetCommon(CommonOpts{RemarkURL: "https://demo.remark42.com", SharedSecret: "123456"})

	_, err = p.ParseArgs([]string{"--backup=/tmp", "--store.type=blah"})
	assert.Error(t, err, "blah is invalid type")

	opts.Store.Type = "blah"
	_, err = opts.newServerApp(context.Background())
	assert.EqualError(t, err, "failed to make data store engine: unsupported store type blah")
	t.Log(err)
}

func TestServerApp_Shutdown(t *testing.T) {
	app, ctx, cancel := prepServerApp(t, func(o ServerCommand) ServerCommand {
		o.Port = chooseRandomUnusedPort()
		return o
	})
	time.AfterFunc(100*time.Millisecond, func() {
		cancel()
	})
	st := time.Now()
	err := app.run(ctx)
	assert.NoError(t, err)
	assert.True(t, time.Since(st).Seconds() < 1, "should take about 100msec")
	app.Wait()
}

func TestServerApp_MainSignal(t *testing.T) {
	done := make(chan struct{})
	go func() {
		<-done
		time.Sleep(250 * time.Millisecond)
		err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
		require.NoError(t, err)
	}()

	s := ServerCommand{}
	s.SetCommon(CommonOpts{RemarkURL: "https://demo.remark42.com", SharedSecret: "123456"})

	p := flags.NewParser(&s, flags.Default)
	port := chooseRandomUnusedPort()
	args := []string{"test", "--store.bolt.path=/tmp/xyz", "--backup=/tmp",
		"--port=" + strconv.Itoa(port), "--image.fs.path=/tmp"}
	defer os.Remove("/tmp/xyz")
	defer os.Remove("/tmp/xyz/remark.db")
	_, err := p.ParseArgs(args)
	require.NoError(t, err)
	st := time.Now()
	close(done)
	err = s.Execute(args)
	assert.NoError(t, err, "execute should be without errors")
	assert.True(t, time.Since(st).Seconds() < 5, "should take under five sec", time.Since(st).Seconds())
}

func TestServerApp_RunCanceledBeforeRESTStart(t *testing.T) {
	port := chooseRandomUnusedPort()
	app, ctx, cancel := prepServerApp(t, func(o ServerCommand) ServerCommand {
		o.Port = port
		return o
	})
	cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- app.run(ctx) }()

	select {
	case err := <-errCh:
		require.NoError(t, err)
		app.Wait()
	case <-time.After(time.Second):
		waitForHTTPServerStart(port)
		app.restSrv.Shutdown()
		select {
		case <-errCh:
			app.Wait()
		case <-time.After(time.Second):
			t.Fatal("server app did not stop after forced REST shutdown")
		}
		t.Fatal("server app should exit when context is canceled before REST server starts")
	}
}

func TestServerApp_DeprecatedArgs(t *testing.T) {
	s := ServerCommand{}
	s.SetCommon(CommonOpts{RemarkURL: "https://demo.remark42.com", SharedSecret: "123456"})

	p := flags.NewParser(&s, flags.Default)
	args := []string{
		"test",
		"--notify.type=email",
		"--notify.users=none",
		"--notify.admins=none",
		"--img-proxy",
		"--notify.email.notify_admin",
	}
	_, err := p.ParseArgs(args)
	require.NoError(t, err)
	deprecatedFlags := s.HandleDeprecatedFlags()
	assert.ElementsMatch(t,
		[]DeprecatedFlag{
			{Old: "img-proxy", New: "image-proxy.http2https", Version: "1.5"},
			{Old: "notify.email.notify_admin", New: "notify.admins=email", Version: "1.9"},
			{Old: "notify.type", New: "notify.(users|admins)", Version: "1.9"},
		},
		deprecatedFlags)
}

func TestServerApp_DeprecatedArgsCollisions(t *testing.T) {
	s := ServerCommand{}
	s.SetCommon(CommonOpts{RemarkURL: "https://demo.remark42.com", SharedSecret: "123456"})

	p := flags.NewParser(&s, flags.Default)
	args := []string{
		"test",
		"--notify.type=email",
		"--notify.users=email",
		"--notify.admins=none",
	}
	_, err := p.ParseArgs(args)
	require.NoError(t, err)
	deprecatedFlagsCollisions := s.findDeprecatedFlagsCollisions()
	assert.ElementsMatch(t,
		[]DeprecatedFlag{
			{Old: "notify.type", New: "notify.(users|admins)", Collision: true},
		},
		deprecatedFlagsCollisions)

	// case which should return nothing
	s = ServerCommand{}
	s.SetCommon(CommonOpts{RemarkURL: "https://demo.remark42.com", SharedSecret: "123456"})
	p = flags.NewParser(&s, flags.Default)
	args = []string{"test"}
	_, err = p.ParseArgs(args)
	require.NoError(t, err)
	deprecatedFlagsCollisions = s.findDeprecatedFlagsCollisions()
	assert.Empty(t, []DeprecatedFlag{}, deprecatedFlagsCollisions)
}

func TestServerAuthHooks(t *testing.T) {
	port := chooseRandomUnusedPort()
	app, ctx, cancel := prepServerApp(t, func(o ServerCommand) ServerCommand {
		o.Port = port
		return o
	})

	go func() { _ = app.run(ctx) }()
	waitForHTTPServerStart(port)

	// make a token for user dev
	tkService := app.restSrv.Authenticator.TokenService()
	tkService.TokenDuration = time.Second

	claims := token.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Audience:  jwt.ClaimStrings{"remark"},
			Issuer:    "remark",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Second)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
		},
		User: &token.User{
			ID:   "github_dev",
			Name: "developer one",
		},
	}
	tk, err := tkService.Token(claims)
	require.NoError(t, err)
	t.Log(tk)

	client := http.Client{Timeout: 10 * time.Second}
	defer client.CloseIdleConnections()

	// add comment
	req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/api/v1/comment?site=remark", port),
		strings.NewReader(`{"text": "test 123", "locator":{"url": "https://radio-t.com/p/2018/12/29/podcast-630/", "site": "remark"}}`))
	require.NoError(t, err)
	req.Header.Set("X-JWT", tk)
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusCreated, resp.StatusCode, "non-blocked user able to post")

	// try to add comment with no-aud claim
	badClaimsNoAud := claims
	badClaimsNoAud.Audience = jwt.ClaimStrings{""}
	tkNoAud, err := tkService.Token(badClaimsNoAud)
	require.NoError(t, err)
	t.Logf("no-aud claims: %s", tkNoAud)
	req, err = http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/api/v1/comment?site=remark", port),
		strings.NewReader(`{"text": "test 123", "locator":{"url": "https://radio-t.com/p/2018/12/29/podcast-631/",
	"site": "remark"}}`))
	require.NoError(t, err)
	req.Header.Set("X-JWT", tkNoAud)
	resp, err = client.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "user without aud claim rejected, \n"+tkNoAud+"\n"+string(body))

	// try to add comment with multiple auds
	badClaimsMultipleAud := claims
	badClaimsMultipleAud.Audience = jwt.ClaimStrings{"remark", "second_aud"}
	tkMultipleAuds, err := tkService.Token(badClaimsMultipleAud)
	require.NoError(t, err)
	t.Logf("multiple aud claims: %s", tkMultipleAuds)
	req, err = http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/api/v1/comment?site=remark", port),
		strings.NewReader(`{"text": "test 123", "locator":{"url": "https://radio-t.com/p/2018/12/29/podcast-631/",
	"site": "remark"}}`))
	require.NoError(t, err)
	req.Header.Set("X-JWT", tkMultipleAuds)
	resp, err = client.Do(req)
	require.NoError(t, err)
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "user with multiple auds claim rejected, \n"+tkMultipleAuds+"\n"+string(body))

	// try to add comment without user set
	badClaimsNoUser := claims
	badClaimsNoUser.Audience = jwt.ClaimStrings{"remark"}
	badClaimsNoUser.User = nil
	tkNoUser, err := tkService.Token(badClaimsNoUser)
	require.NoError(t, err)
	t.Logf("no user claims: %s", tkNoUser)
	req, err = http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/api/v1/comment?site=remark", port),
		strings.NewReader(`{"text": "test 123", "locator":{"url": "https://radio-t.com/p/2018/12/29/podcast-631/",
	"site": "remark"}}`))
	require.NoError(t, err)
	req.Header.Set("X-JWT", tkNoUser)
	resp, err = client.Do(req)
	require.NoError(t, err)
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "user without user information rejected, \n"+tkNoUser+"\n"+string(body))

	// block user github_dev as admin
	req, err = http.NewRequest(http.MethodPut,
		fmt.Sprintf("http://localhost:%d/api/v1/admin/user/github_dev?site=remark&block=1&ttl=10d", port), http.NoBody)
	assert.NoError(t, err)
	req.SetBasicAuth("admin", "password")
	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "user github_dev blocked")
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	t.Log(string(b))

	// try add a comment with blocked user
	req, err = http.NewRequest("POST", fmt.Sprintf("http://localhost:%d/api/v1/comment?site=remark", port),
		strings.NewReader(`{"text": "test 123 blah", "locator":{"url": "https://radio-t.com/blah1", "site": "remark"}}`))
	require.NoError(t, err)
	req.Header.Set("X-JWT", tk)
	resp, err = client.Do(req)
	require.NoError(t, err)
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.True(t, resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized,
		"blocked user can't post, \n"+tk+"\n"+string(body))

	cancel()
	app.Wait()
	client.CloseIdleConnections()
}

func TestServerCommand_parseSameSite(t *testing.T) {
	tbl := []struct {
		inp string
		res http.SameSite
	}{
		{"", http.SameSiteDefaultMode},
		{"default", http.SameSiteDefaultMode},
		{"blah", http.SameSiteDefaultMode},
		{"none", http.SameSiteNoneMode},
		{"lax", http.SameSiteLaxMode},
		{"strict", http.SameSiteStrictMode},
	}

	cmd := ServerCommand{}
	for i, tt := range tbl {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			assert.Equal(t, tt.res, cmd.parseSameSite(tt.inp))
		})
	}
}

func Test_getAllowedDomains(t *testing.T) {
	tbl := []struct {
		s              ServerCommand
		allowedDomains []string
	}{
		// correct example, parsed and returned as allowed domain
		{ServerCommand{AllowedHosts: []string{}, CommonOpts: CommonOpts{RemarkURL: "https://remark42.example.org"}}, []string{"example.org"}},
		{ServerCommand{AllowedHosts: []string{}, CommonOpts: CommonOpts{RemarkURL: "http://remark42.example.org"}}, []string{"example.org"}},
		{ServerCommand{AllowedHosts: []string{}, CommonOpts: CommonOpts{RemarkURL: "http://localhost"}}, []string{"localhost"}},
		// incorrect URLs, so Hostname is empty but returned list doesn't include empty string as it would allow any domain
		{ServerCommand{AllowedHosts: []string{}, CommonOpts: CommonOpts{RemarkURL: "bad hostname"}}, []string{}},
		{ServerCommand{AllowedHosts: []string{}, CommonOpts: CommonOpts{RemarkURL: "not_a_hostname"}}, []string{}},
		// test removal of 'self', multiple AllowedHosts. No deduplication is expected
		{ServerCommand{AllowedHosts: []string{"'self'", "example.org", "test.example.org", "remark42.com"}, CommonOpts: CommonOpts{RemarkURL: "https://example.org"}}, []string{"example.org", "test.example.org", "remark42.com", "example.org"}},
	}
	for i, tt := range tbl {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			assert.Equal(t, tt.allowedDomains, tt.s.getAllowedDomains())
		})
	}
}

func Test_getAllowedRedirectHosts(t *testing.T) {
	tbl := []struct {
		name  string
		hosts []string
		want  []string
	}{
		{name: "empty", hosts: nil, want: []string{}},
		{name: "bare hostnames pass through", hosts: []string{"example.com", "admin.example.com"}, want: []string{"example.com", "admin.example.com"}},
		{name: "https scheme stripped", hosts: []string{"https://example.com"}, want: []string{"example.com"}},
		{name: "http scheme stripped", hosts: []string{"http://example.com"}, want: []string{"example.com"}},
		{name: "scheme with path strips path", hosts: []string{"https://example.com/embed"}, want: []string{"example.com"}},
		{name: "explicit port preserved as host:port", hosts: []string{"example.com:8080"}, want: []string{"example.com:8080"}},
		{name: "scheme with explicit port preserved", hosts: []string{"https://example.com:8443"}, want: []string{"example.com:8443"}},
		{name: "scheme without port stays bare host", hosts: []string{"https://example.com"}, want: []string{"example.com"}},
		{name: "self sentinel filtered", hosts: []string{"'self'", "self", `"self"`, "example.com"}, want: []string{"example.com"}},
		{name: "wildcards filtered", hosts: []string{"*", "*.example.com", "https://*.example.com", "example.com"}, want: []string{"example.com"}},
		{name: "empty entries filtered", hosts: []string{"", "  ", "example.com"}, want: []string{"example.com"}},
		{name: "mixed real-world", hosts: []string{"'self'", "https://blog.example.com", "admin.example.com:8443", "*.cdn.example.com"},
			want: []string{"blog.example.com", "admin.example.com:8443"}},
	}
	for _, tt := range tbl {
		t.Run(tt.name, func(t *testing.T) {
			s := ServerCommand{AllowedHosts: tt.hosts}
			assert.Equal(t, tt.want, s.getAllowedRedirectHosts())
		})
	}
}

func chooseRandomUnusedPort() (port int) {
	for range 10 {
		port = 40000 + int(rand.Int31n(10000))
		if ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port)); err == nil {
			_ = ln.Close()
			break
		}
	}
	return port
}

func waitForHTTPServerStart(port int) {
	// wait for up to 3 seconds for server to start before returning it
	client := http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	for range 300 {
		time.Sleep(time.Millisecond * 10)
		if resp, err := client.Get(fmt.Sprintf("http://localhost:%d", port)); err == nil {
			_ = resp.Body.Close()
			return
		}
	}
}

func prepServerApp(t *testing.T, fn func(o ServerCommand) ServerCommand) (*serverApp, context.Context, context.CancelFunc) {
	cmd := ServerCommand{}
	cmd.SetCommon(CommonOpts{RemarkURL: "https://demo.remark42.com", SharedSecret: "secret"})

	// prepare options
	p := flags.NewParser(&cmd, flags.Default)
	_, err := p.ParseArgs([]string{"--admin-passwd=password", "--site=remark"})
	require.NoError(t, err)
	cmd.Avatar.FS.Path, cmd.Avatar.Type, cmd.BackupLocation, cmd.Image.FS.Path = "/tmp/remark42_test", "fs", "/tmp/remark42_test", "/tmp/remark42_test"
	cmd.Store.Bolt.Timeout = 10 * time.Second
	cmd.Auth.Github.CSEC, cmd.Auth.Github.CID = "csec", "cid"
	cmd.BackupLocation = "/tmp"
	cmd.Notify.Users = []string{"email"}
	cmd.Notify.Admins = []string{"email"}
	cmd.Notify.Email.From = "from@example.org"
	cmd.Notify.Email.VerificationSubject = "test verification email subject"
	cmd.SMTP.Host = "127.0.0.1"
	cmd.SMTP.Port = 25
	cmd.SMTP.Username = "test_user"
	cmd.SMTP.Password = "test_password"
	cmd.SMTP.TimeOut = time.Second
	cmd.UpdateLimit = 10
	cmd.Admin.Type = "shared"
	cmd.Admin.Shared.Admins = []string{"id1", "id2"}
	cmd.RestrictedNames = []string{"umputun", "bobuk"}
	cmd.emailMsgTemplatePath = "../../templates/email_reply.html.tmpl"
	cmd.emailVerificationTemplatePath = "../../templates/email_confirmation_subscription.html.tmpl"

	cmd = fn(cmd)
	// as is uses port, call it after fn which could set it
	cmd.Store.Bolt.Path = fmt.Sprintf("/tmp/%d", cmd.Port)

	app, ctx, cancel := createAppFromCmd(t, cmd)

	// cleanup the remark.db file after context is canceled
	go func() {
		<-ctx.Done()
		os.RemoveAll(cmd.Store.Bolt.Path)
		os.RemoveAll(cmd.Avatar.FS.Path)

	}()

	return app, ctx, cancel
}

func createAppFromCmd(t *testing.T, cmd ServerCommand) (*serverApp, context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	app, err := cmd.newServerApp(ctx)
	require.NoError(t, err)
	return app, ctx, cancel
}

func TestMain(m *testing.M) {
	// ignore is added only for GitHub Actions, can't reproduce locally
	goleak.VerifyTestMain(
		m,
		goleak.IgnoreTopFunction("net/http.(*Server).Shutdown"),
		// this will be fixed in https://github.com/hashicorp/golang-lru/issues/159
		goleak.IgnoreTopFunction("github.com/hashicorp/golang-lru/v2/expirable.NewLRU[...].func1"),
	)
}
