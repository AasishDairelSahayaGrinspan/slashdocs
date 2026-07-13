package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"syscall"
	"testing"

	"github.com/razorpay/aidocs/api/internal/kvevents"
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
			if got := factory != nil; got != tc.wantFactory {
				t.Fatalf("factory present = %v, want %v", got, tc.wantFactory)
			}
		})
	}
}
