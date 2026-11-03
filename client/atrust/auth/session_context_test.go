package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type blockingRoundTripper struct{}

func (blockingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestSessionRequestKeepsBodyReadable(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-release
		_, _ = io.WriteString(w, "streamed body")
	}))
	defer server.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()

	session := NewSession(strings.TrimPrefix(server.URL, "https://"), nil)
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := session.request(req)
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || string(body) != "streamed body" {
		t.Fatalf("response body = %q, err=%v", body, err)
	}
}

func TestNewSessionContextCancelsRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := NewSessionContext(ctx, "vpn.example", nil)
	session.client.Transport = blockingRoundTripper{}
	req, err := http.NewRequest(http.MethodGet, "https://example.invalid", nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := session.request(req)
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not observe session cancellation")
	}
}

func TestSessionWithContextPreservesRefreshState(t *testing.T) {
	var calls atomic.Int32
	blocked := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if call == 3 {
			close(blocked)
			<-r.Context().Done()
			return
		}
		cookie, err := r.Cookie("sid")
		if err != nil || cookie.Value != fmt.Sprintf("sid-%d", call-1) {
			t.Error("request lost the session cookie during context handoff")
		}
		if r.Header.Get("x-csrf-token") != fmt.Sprintf("csrf-%d", call-1) {
			t.Error("request lost the CSRF token during context handoff")
		}
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: fmt.Sprintf("sid-%d", call), Path: "/"})
		_, _ = fmt.Fprintf(w, "{\"code\":0,\"data\":{\"isLogin\":1,\"security\":{\"csrfToken\":\"csrf-%d\"}}}", call)
	}))
	t.Cleanup(server.Close)
	setupCtx, cancelSetup := context.WithCancel(t.Context())
	t.Cleanup(cancelSetup)
	session := NewSessionContext(setupCtx, strings.TrimPrefix(server.URL, "https://"), nil)
	session.Restore("device", "sid-0", nil)
	session.csrfToken = "csrf-0"
	if _, err := session.Refresh(t.Context()); err != nil {
		t.Fatalf("initial refresh failed: %v", err)
	}
	lifetime, cancelLifetime := context.WithCancel(t.Context())
	t.Cleanup(cancelLifetime)
	refreshSession := session.WithContext(lifetime)
	cancelSetup()
	if session.ctx != setupCtx || refreshSession.ctx != lifetime {
		t.Fatal("WithContext changed the original session lifetime")
	}
	if _, err := refreshSession.Refresh(t.Context()); err != nil {
		t.Fatalf("refresh failed after setup context ended: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := refreshSession.Refresh(t.Context())
		done <- err
	}()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("refresh did not reach the test server")
	}
	cancelLifetime()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("refresh ignored its new lifetime: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh did not stop after its new lifetime ended")
	}
}
