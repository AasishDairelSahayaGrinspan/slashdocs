package main

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/razorpay/aidocs/api/internal/kvevents"
	"github.com/razorpay/aidocs/api/internal/repo"
	"github.com/razorpay/aidocs/api/internal/server"
)

func TestServeShutsDownSiblingWhenListenerFails(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve listener address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release listener address: %v", err)
	}

	// Both servers race to bind the same address. One succeeds and the other
	// reports EADDRINUSE; serve must close the successful sibling before it
	// returns the original listener error.
	err = serve(context.Background(), http.NewServeMux(), addr, addr)
	if !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("serve error = %v, want EADDRINUSE", err)
	}

	rebound, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("sibling listener still owns %s after serve returned: %v", addr, err)
	}
	if err := rebound.Close(); err != nil {
		t.Fatalf("close rebound listener: %v", err)
	}
}

func TestValidateOrigin(t *testing.T) {
	t.Parallel()

	for _, origin := range []string{
		"https://example.com",
		"https://EXAMPLE.com:443",
		"http://localhost:8080",
		"http://[::1]:8080",
	} {
		t.Run("accepts "+origin, func(t *testing.T) {
			t.Parallel()
			if err := validateOrigin("TEST_ORIGIN", origin); err != nil {
				t.Fatalf("validateOrigin(%q) returned %v", origin, err)
			}
		})
	}

	for _, origin := range []string{
		"example.com",
		"ftp://example.com",
		"https://",
		"https://:443",
		"https://user@example.com",
		"https://example.com/path",
		"https://example.com/",
		"https://example.com?mode=login",
		"https://example.com?",
		"https://example.com#fragment",
		"https://example.com#",
		"https://example.com:65536",
	} {
		t.Run("rejects "+origin, func(t *testing.T) {
			t.Parallel()
			if err := validateOrigin("TEST_ORIGIN", origin); err == nil {
				t.Fatalf("validateOrigin(%q) unexpectedly succeeded", origin)
			}
		})
	}
}

func TestUnsafeRenderOriginOverrideDoesNotAllowInvalidOrigins(t *testing.T) {
	t.Setenv("AIDOCS_APP_ORIGIN", "http://127.1:8080")
	t.Setenv("AIDOCS_RENDER_ORIGIN", "http://127.0.0.1:8080")
	t.Setenv("AIDOCS_ALLOW_UNSAFE_RENDER_ORIGIN", "true")
	t.Setenv("SESSION_SECRET", "test-session-secret-at-least-32-bytes")

	if _, err := loadOriginSettings(); err == nil {
		t.Fatal("loadOriginSettings accepted a browser-ambiguous invalid origin")
	}
}

func TestUnsafeRenderOriginOverrideAllowsValidSameHostname(t *testing.T) {
	t.Setenv("AIDOCS_APP_ORIGIN", "http://localhost:8080")
	t.Setenv("AIDOCS_RENDER_ORIGIN", "https://LOCALHOST:9090")
	t.Setenv("AIDOCS_ALLOW_UNSAFE_RENDER_ORIGIN", "true")
	t.Setenv("SESSION_SECRET", "test-session-secret-at-least-32-bytes")

	if _, err := loadOriginSettings(); err != nil {
		t.Fatalf("loadOriginSettings rejected the explicit valid same-origin override: %v", err)
	}
}

func TestOriginSettingsRejectSameHostnameAcrossSchemeAndPort(t *testing.T) {
	t.Setenv("AIDOCS_APP_ORIGIN", "https://app.example")
	t.Setenv("AIDOCS_RENDER_ORIGIN", "http://APP.EXAMPLE:8080")
	t.Setenv("AIDOCS_ALLOW_UNSAFE_RENDER_ORIGIN", "")
	t.Setenv("SESSION_SECRET", "test-session-secret-at-least-32-bytes")

	if _, err := loadOriginSettings(); !errors.Is(err, server.ErrUnsafeRenderOrigin) {
		t.Fatalf("loadOriginSettings error = %v, want ErrUnsafeRenderOrigin", err)
	}
}

func TestWebhookSettingsValidateDevRevSecretLength(t *testing.T) {
	t.Setenv("AIDOCS_GITHUB_WEBHOOK_SECRET", "github-secret")
	t.Setenv("AIDOCS_SLACK_SIGNING_SECRET", "slack-secret")

	for _, tc := range []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{name: "unset"},
		{name: "minimum", secret: "12345678"},
		{name: "maximum", secret: "12345678901234567890123456789012"},
		{name: "too short", secret: "1234567", wantErr: true},
		{name: "too long", secret: "123456789012345678901234567890123", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AIDOCS_DEVREV_WEBHOOK_SECRET", tc.secret)
			settings, err := loadWebhookSettings()
			if (err != nil) != tc.wantErr {
				t.Fatalf("loadWebhookSettings() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && (settings.githubSecret != "github-secret" || settings.slackSecret != "slack-secret" || settings.devRevSecret != tc.secret) {
				t.Fatalf("settings = %#v", settings)
			}
		})
	}
}

func TestPersonalConnectionSettingsRequireCompleteOAuthAndStableKey(t *testing.T) {
	clear := func(t *testing.T) {
		t.Helper()
		for _, name := range []string{
			"AIDOCS_CONNECTION_ENCRYPTION_KEY",
			"AIDOCS_GITHUB_OAUTH_CLIENT_ID", "AIDOCS_GITHUB_OAUTH_CLIENT_SECRET",
			"AIDOCS_SLACK_OAUTH_CLIENT_ID", "AIDOCS_SLACK_OAUTH_CLIENT_SECRET",
			"AIDOCS_GOOGLE_DRIVE_OAUTH_CLIENT_ID", "AIDOCS_GOOGLE_DRIVE_OAUTH_CLIENT_SECRET",
			"AIDOCS_GOOGLE_PICKER_API_KEY", "AIDOCS_GOOGLE_PICKER_APP_ID",
			"AIDOCS_DEVREV_API_BASE_URL",
		} {
			t.Setenv(name, "")
		}
	}

	t.Run("disabled stays backwards compatible", func(t *testing.T) {
		clear(t)
		settings, err := loadPersonalConnectionSettings()
		if err != nil || settings.cipher != nil {
			t.Fatalf("settings=%#v err=%v", settings, err)
		}
	})

	t.Run("client pair is atomic", func(t *testing.T) {
		clear(t)
		t.Setenv("AIDOCS_SLACK_OAUTH_CLIENT_ID", "client")
		if _, err := loadPersonalConnectionSettings(); err == nil {
			t.Fatal("accepted OAuth client ID without secret")
		}
	})

	t.Run("configured provider requires encryption key", func(t *testing.T) {
		clear(t)
		t.Setenv("AIDOCS_GITHUB_OAUTH_CLIENT_ID", "client")
		t.Setenv("AIDOCS_GITHUB_OAUTH_CLIENT_SECRET", "secret")
		if _, err := loadPersonalConnectionSettings(); err == nil {
			t.Fatal("accepted provider credentials without encryption key")
		}
	})

	t.Run("valid base64 key enables connections", func(t *testing.T) {
		clear(t)
		t.Setenv("AIDOCS_GITHUB_OAUTH_CLIENT_ID", "client")
		t.Setenv("AIDOCS_GITHUB_OAUTH_CLIENT_SECRET", "secret")
		t.Setenv("AIDOCS_CONNECTION_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
		settings, err := loadPersonalConnectionSettings()
		if err != nil || settings.cipher == nil || settings.providers.GitHub.ClientID != "client" {
			t.Fatalf("settings=%#v err=%v", settings, err)
		}
	})

	t.Run("wrong key size is rejected", func(t *testing.T) {
		clear(t)
		t.Setenv("AIDOCS_CONNECTION_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("too-short")))
		if _, err := loadPersonalConnectionSettings(); err == nil {
			t.Fatal("accepted invalid connection encryption key")
		}
	})
}

func TestSourcePollIntervalConfiguration(t *testing.T) {
	for _, test := range []struct {
		value   string
		want    time.Duration
		wantErr bool
	}{
		{want: 15 * time.Minute},
		{value: "0", want: 0},
		{value: "1m", want: time.Minute},
		{value: "30m", want: 30 * time.Minute},
		{value: "10s", wantErr: true},
		{value: "not-a-duration", wantErr: true},
		{value: "-1m", wantErr: true},
	} {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv("AIDOCS_SOURCE_POLL_INTERVAL", test.value)
			got, err := loadSourcePollInterval()
			if (err != nil) != test.wantErr || (!test.wantErr && got != test.want) {
				t.Fatalf("loadSourcePollInterval()=%s err=%v, want=%s wantErr=%v", got, err, test.want, test.wantErr)
			}
		})
	}
}

func TestKVEventListenerFactorySelection(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		mode        kvevents.Mode
		listenerURL string
		wantFactory bool
	}{
		{name: "auto without explicit listener URL polls", mode: kvevents.ModeAuto},
		{name: "auto with explicit listener URL probes notify", mode: kvevents.ModeAuto, listenerURL: "postgres://listener", wantFactory: true},
		{name: "notify falls back to database URL", mode: kvevents.ModeNotify, wantFactory: true},
		{name: "notify accepts dedicated listener URL", mode: kvevents.ModeNotify, listenerURL: "postgres://listener", wantFactory: true},
		{name: "poll never creates listener", mode: kvevents.ModePoll, listenerURL: "postgres://listener"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			factory := newKVEventListenerFactory(tc.mode, "postgres://application", tc.listenerURL, nil)
			// factory is the kvevents.ListenerFactory interface, so this nil
			// comparison asserts interface nilness: a nil concrete pointer boxed
			// into the interface would compare non-nil, defeat the broker's
			// nil-factory guard, and panic on Open at boot.
			if got := factory != nil; got != tc.wantFactory {
				t.Fatalf("factory present = %v (%T), want %v", got, factory, tc.wantFactory)
			}
		})
	}
}

type stubKVEventSource struct{}

func (stubKVEventSource) CurrentDocumentKVRevision(context.Context, string) (int64, error) {
	return 0, nil
}

func (stubKVEventSource) ListDocumentKVEvents(context.Context, string, int64, int32) ([]repo.DocumentKVEvent, error) {
	return nil, nil
}

// Regression test for the default configuration (auto mode, no dedicated
// listener database URL): the constructed factory must wire through
// kvevents.New as a true nil interface so the broker falls back to polling
// instead of panicking on a typed-nil listener factory.
func TestAutoModeWithoutListenerURLFallsBackToPolling(t *testing.T) {
	t.Parallel()

	factory := newKVEventListenerFactory(kvevents.ModeAuto, "postgres://application", "", nil)
	if factory != nil {
		t.Fatalf("factory = %T, want nil interface in auto mode without a listener URL", factory)
	}

	broker, err := kvevents.New(context.Background(), kvevents.Config{Mode: kvevents.ModeAuto}, stubKVEventSource{}, factory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(broker.Close)
	if broker.ActiveMode() != kvevents.ModePoll {
		t.Fatalf("active mode = %q, want %q", broker.ActiveMode(), kvevents.ModePoll)
	}
	_, unsubscribe, err := broker.Subscribe(context.Background(), "doc_boot")
	if err != nil {
		t.Fatal(err)
	}
	unsubscribe()
}
