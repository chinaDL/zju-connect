package atrust

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mythologyli/zju-connect/client/atrust/auth"
)

// setupRefreshDialer keeps both node probes and the VIP exchange local.
type setupRefreshDialer struct {
	authAddress string
	tlsConfig   *tls.Config
	mu          sync.Mutex
	connections []net.Conn
	wg          sync.WaitGroup
}

func (d *setupRefreshDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if address == d.authAddress {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	if address != "node.example:443" {
		return nil, fmt.Errorf("unexpected setup test destination %q", address)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	local, remote := net.Pipe()
	_ = remote.SetDeadline(time.Now().Add(5 * time.Second))
	d.mu.Lock()
	d.connections = append(d.connections, local, remote)
	d.mu.Unlock()
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer remote.Close()
		conn := tls.Server(remote, d.tlsConfig)
		if err := conn.HandshakeContext(ctx); err != nil {
			return
		}
		var header [7]byte
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			// Node probes close immediately after the TLS handshake.
			return
		}
		length := int64(binary.BigEndian.Uint16(header[5:]))
		if _, err := io.CopyN(io.Discard, conn, length+10); err != nil {
			return
		}
		payload := []byte("{\"code\":0}")
		response := []byte{0x05, 0xD0, 0x53, 0x00, 0x00, byte(len(payload))}
		response = append(response, payload...)
		response = append(response, 0x05, 0x00, 0x00, 0x01, 10, 249, 8, 102, 0, 0)
		_, _ = conn.Write(response)
	}()
	return local, nil
}

func (*setupRefreshDialer) ExcludeIP(net.IP) {}

func (d *setupRefreshDialer) close() {
	d.mu.Lock()
	for _, conn := range d.connections {
		_ = conn.Close()
	}
	d.mu.Unlock()
	d.wg.Wait()
}

func TestSetupContextRefreshSurvivesReturnAndCallerCancellation(t *testing.T) {
	setupReturned := make(chan struct{})
	refreshBlocked := make(chan struct{})
	refreshCanceled := make(chan struct{})
	var refreshes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/public/manifest":
			_, _ = io.WriteString(w, "{\"code\":0,\"data\":{}}")
		case "/passport/v1/public/authConfig":
			select {
			case <-setupReturned:
			case <-r.Context().Done():
				return
			}
			attempt := refreshes.Add(1)
			if attempt == 4 {
				close(refreshBlocked)
				<-r.Context().Done()
				close(refreshCanceled)
				return
			}
			if r.Method != http.MethodGet || r.URL.Query().Get("needTicket") != "0" {
				t.Error("unexpected session refresh request")
			}
			cookie, err := r.Cookie("sid")
			if err != nil || cookie.Value != fmt.Sprintf("sid-%d", attempt-1) {
				t.Errorf("refresh %d did not use the latest session cookie", attempt)
			}
			wantCSRF := ""
			if attempt > 1 {
				wantCSRF = fmt.Sprintf("csrf-%d", attempt-1)
			}
			if r.Header.Get("x-csrf-token") != wantCSRF {
				t.Errorf("refresh %d did not use the latest CSRF token", attempt)
			}
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: fmt.Sprintf("sid-%d", attempt), Path: "/"})
			_, _ = fmt.Fprintf(w, "{\"code\":0,\"data\":{\"isLogin\":1,\"security\":{\"csrfToken\":\"csrf-%d\"}}}", attempt)
		default:
			t.Errorf("unexpected setup request path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	dialer := &setupRefreshDialer{authAddress: server.Listener.Addr().String(), tlsConfig: server.TLS.Clone()}
	t.Cleanup(dialer.close)
	c := NewClient(ClientOptions{
		Session:        SessionOptions{SID: "sid-0", DeviceID: "test-device"},
		UnderlayDialer: dialer,
	})
	t.Cleanup(c.Close)
	host, portText, err := net.SplitHostPort(dialer.authAddress)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	resource := []byte("{\"data\":{\"appList\":{\"data\":{\"config\":{\"nodeGroupConf\":{\"majorNodeGroup\":{\"ID\":\"group\"},\"nodeGroupList\":[{\"ID\":\"group\",\"addressInfo\":[{\"address\":\"node.example:443\",\"type\":\"WAN\"}]}]}}}}}}")
	persisted := make(chan auth.ClientAuthData, 8)
	setupCtx, cancelSetup := context.WithCancel(t.Context())
	defer cancelSetup()
	_, err = c.SetupContext(setupCtx, SetupOptions{
		ServerAddress:          host,
		ServerPort:             port,
		ResourceData:           resource,
		SessionRefreshInterval: 20 * time.Millisecond,
		SaveClientData: func(data []byte) error {
			var snapshot auth.ClientAuthData
			if err := json.Unmarshal(data, &snapshot); err != nil {
				return err
			}
			select {
			case persisted <- snapshot:
			default:
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("SetupContext failed: %v", err)
	}
	cancelSetup()
	close(setupReturned)
	// Consume the initial saved snapshot, then require three real HTTP refreshes.
	<-persisted
	for attempt := 1; attempt <= 3; attempt++ {
		select {
		case saved := <-persisted:
			if len(saved.Cookies) != 1 || saved.Cookies[0].Value != fmt.Sprintf("sid-%d", attempt) {
				t.Fatalf("refresh %d did not persist the updated cookie", attempt)
			}
		case <-time.After(time.Second):
			t.Fatalf("refresh %d did not succeed after SetupContext returned and its caller canceled", attempt)
		}
	}
	select {
	case <-refreshBlocked:
	case <-time.After(time.Second):
		t.Fatal("next refresh did not reach the local server")
	}
	if sid, err := c.sessionSID(); err != nil || sid != "sid-3" {
		t.Fatal("client did not publish the refreshed session")
	}
	closed := make(chan struct{})
	go func() { c.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Client.Close did not stop the blocked refresh")
	}
	select {
	case <-refreshCanceled:
	case <-time.After(time.Second):
		t.Fatal("Client.Close did not cancel the in-flight HTTP request")
	}
}
