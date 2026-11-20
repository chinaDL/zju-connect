package atrust

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type observedHeartbeatTransport struct {
	net.Conn
	observe atomic.Bool
	started chan struct{}
	once    sync.Once
}

func (c *observedHeartbeatTransport) Write(p []byte) (int, error) {
	if c.observe.Load() {
		c.once.Do(func() { close(c.started) })
	}
	return c.Conn.Write(p)
}

func TestHeartbeatWatchdogUnblocksTLSDataWrite(t *testing.T) {
	// Use the standard local test certificate; no HTTP listener is needed
	// after obtaining its TLS configuration.
	certificateServer := httptest.NewTLSServer(http.NotFoundHandler())
	serverConfig := certificateServer.TLS.Clone()
	certificateServer.Close()
	serverConfig.MaxVersion = tls.VersionTLS12
	local, remote := net.Pipe()
	transport := &observedHeartbeatTransport{Conn: local, started: make(chan struct{})}
	secure := tls.Client(transport, &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12})
	server := tls.Server(remote, serverConfig)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	handshakeDone := make(chan error, 1)
	go func() { handshakeDone <- server.HandshakeContext(ctx) }()
	t.Cleanup(func() { _ = local.Close(); _ = remote.Close() })
	if err := secure.HandshakeContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-handshakeDone; err != nil {
		t.Fatal(err)
	}
	conn := &l3TunnelConn{tlsConn: secure, closeCh: make(chan struct{}), conntrackMgr: newConntrackMgr(), heartbeatInterval: 3 * time.Millisecond, heartbeatMissLimit: 3}
	transport.observe.Store(true)
	dataDone, loopDone := make(chan struct{}), make(chan struct{})
	var dataErr error
	go func() { dataErr = conn.writeFrame([]byte{l3Version, cmdDataReq, 0, 0}); close(dataDone) }()
	t.Cleanup(func() {
		_ = conn.Close()
		for _, done := range []chan struct{}{dataDone, loopDone} {
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("TLS writer or heartbeat watchdog did not stop")
			}
		}
		conn.conntrackMgr.close()
	})
	select {
	case <-transport.started:
	case <-time.After(time.Second):
		t.Fatal("TLS data write did not start")
	}
	go func() { conn.heartbeatLoop(); close(loopDone) }()
	select {
	case <-dataDone:
		if dataErr == nil {
			t.Fatal("data write succeeded while the peer stopped reading")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("heartbeat watchdog did not unblock the stalled TLS data write")
	}
}

func TestHeartbeatWatchdogClosesWhenProbeWriteBlocks(t *testing.T) {
	writeStarted, writeDone := make(chan struct{}), make(chan struct{})
	conn := &l3TunnelConn{
		tlsConn: tls.Client(&trackingNetConn{closed: make(chan struct{})}, &tls.Config{InsecureSkipVerify: true}),
		closeCh: make(chan struct{}), conntrackMgr: newConntrackMgr(),
		heartbeatInterval: 3 * time.Millisecond, heartbeatMissLimit: 3,
	}
	conn.writeFrameHook = func([]byte) error {
		close(writeStarted)
		<-conn.closeCh
		close(writeDone)
		return net.ErrClosed
	}
	loopDone := make(chan struct{})
	go func() { conn.heartbeatLoop(); close(loopDone) }()
	t.Cleanup(func() {
		_ = conn.Close()
		for _, done := range []chan struct{}{loopDone, writeDone} {
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("blocked heartbeat writer did not stop")
			}
		}
		conn.conntrackMgr.close()
	})
	select {
	case <-writeStarted:
	case <-time.After(time.Second):
		t.Fatal("heartbeat write did not start")
	}
	select {
	case <-conn.closeCh:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("blocked heartbeat write prevented the watchdog from closing the tunnel")
	}
}

func TestServerFramesClearUnansweredHeartbeatCount(t *testing.T) {
	for _, command := range []byte{cmdHeartbeatResp, cmdDataResp, cmdAuthResp, cmdSecondVipResp} {
		t.Run(stringKey(int(command)), func(t *testing.T) {
			payload := []byte(nil)
			frame := []byte{l3Version, command}
			if command == cmdDataResp {
				payload = makeUDPPacket(12345, 53)
			}
			if command == cmdAuthResp || command == cmdSecondVipResp {
				frame = append(frame, 0)
			}
			frame = binary.BigEndian.AppendUint16(frame, uint16(len(payload)))
			frame = append(frame, payload...)
			conn := &l3TunnelConn{
				tlsConn: tls.Client(&trackingNetConn{closed: make(chan struct{})}, &tls.Config{InsecureSkipVerify: true}),
				reader:  bufio.NewReader(bytes.NewReader(frame)), incoming: make(chan []byte, 1),
				closeCh: make(chan struct{}), conntrackMgr: newConntrackMgr(),
			}
			atomic.StoreInt32(&conn.heartbeatMisses, 2)
			conn.readLoop()
			if misses := atomic.LoadInt32(&conn.heartbeatMisses); misses != 0 {
				t.Fatalf("missed responses after server command 0x%02x = %d, want 0", command, misses)
			}
			conn.conntrackMgr.close()
		})
	}
}

func TestResponsiveTunnelSurvivesMultipleHeartbeatWindows(t *testing.T) {
	for _, command := range []byte{cmdHeartbeatResp, cmdDataResp} {
		t.Run(stringKey(int(command)), func(t *testing.T) {
			reader, writer := io.Pipe()
			payload := []byte(nil)
			if command == cmdDataResp {
				payload = makeUDPPacket(12345, 53)
			}
			response := binary.BigEndian.AppendUint16([]byte{l3Version, command}, uint16(len(payload)))
			response = append(response, payload...)
			written := make(chan struct{}, 16)
			conn := &l3TunnelConn{
				tlsConn: tls.Client(&trackingNetConn{closed: make(chan struct{})}, &tls.Config{InsecureSkipVerify: true}),
				reader:  bufio.NewReader(reader), incoming: make(chan []byte, 1), closeCh: make(chan struct{}),
				conntrackMgr: newConntrackMgr(), heartbeatInterval: 5 * time.Millisecond, heartbeatMissLimit: 3,
				writeFrameHook: func([]byte) error {
					_, err := writer.Write(response)
					if err == nil {
						written <- struct{}{}
					}
					return err
				},
			}
			heartbeatDone, readDone := make(chan struct{}), make(chan struct{})
			go func() { conn.readLoop(); close(readDone) }()
			go func() { conn.heartbeatLoop(); close(heartbeatDone) }()
			t.Cleanup(func() {
				_ = conn.Close()
				_ = reader.Close()
				_ = writer.Close()
				for _, done := range []chan struct{}{heartbeatDone, readDone} {
					select {
					case <-done:
					case <-time.After(time.Second):
						t.Error("tunnel loop did not stop")
					}
				}
				conn.conntrackMgr.close()
			})
			for replies := 0; replies < 6; replies++ {
				select {
				case <-written:
				case <-conn.closeCh:
					t.Fatal("responsive tunnel was incorrectly closed")
				case <-time.After(time.Second):
					t.Fatal("heartbeat did not continue while server was responsive")
				}
			}
		})
	}
}

func TestUnansweredHeartbeatsReconnectAndResumePackets(t *testing.T) {
	for _, outboundTraffic := range []bool{false, true} {
		name := "idle"
		if outboundTraffic {
			name = "outbound traffic still succeeds"
		}
		t.Run(name, func(t *testing.T) {
			client := newSessionTestClient()
			t.Cleanup(client.Close)
			client.BestNodes = map[string]string{"group": "node:443"}
			manager := newConntrackMgr()
			var probes atomic.Int32
			old := &l3TunnelConn{
				tlsConn:  tls.Client(&trackingNetConn{closed: make(chan struct{})}, &tls.Config{InsecureSkipVerify: true}),
				incoming: make(chan []byte), closeCh: make(chan struct{}),
				conntrackMgr: manager, heartbeatInterval: 3 * time.Millisecond, heartbeatMissLimit: 3,
				writeFrameHook: func(frame []byte) error {
					if len(frame) >= 2 && frame[1] == cmdHeartbeatReq {
						probes.Add(1)
					}
					return nil
				},
			}
			replacement := make(chan *l3TunnelConn, 1)
			tunnel := &L3Tunnel{
				client: client, conns: map[string]*l3TunnelConn{"group": old},
				connecting: make(map[string]*l3TunnelConnectCall), conntrackMgrs: map[string]*conntrackMgr{"group": manager},
				dataChan: make(chan []byte, 1), closeCh: make(chan struct{}), reconnectInterval: time.Millisecond,
			}
			tunnel.connect = func(_ context.Context, _ string, gotManager *conntrackMgr) (*l3TunnelConn, error) {
				conn := &l3TunnelConn{
					tlsConn:  tls.Client(&trackingNetConn{closed: make(chan struct{})}, &tls.Config{InsecureSkipVerify: true}),
					incoming: make(chan []byte, 1), closeCh: make(chan struct{}), conntrackMgr: gotManager,
				}
				replacement <- conn
				return conn, nil
			}
			done := make(chan struct{})
			go func() { old.heartbeatLoop(); close(done) }()
			go tunnel.forwardFromConn("group", old)
			t.Cleanup(func() {
				tunnel.Close()
				_ = old.Close()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Error("heartbeat loop did not stop")
				}
			})
			if outboundTraffic {
				trafficDone := make(chan struct{})
				go func() {
					defer close(trafficDone)
					ticker := time.NewTicker(time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-old.closeCh:
							return
						case <-ticker.C:
							_ = old.writeFrame([]byte{l3Version, cmdDataReq})
						}
					}
				}()
				t.Cleanup(func() {
					_ = old.Close()
					select {
					case <-trafficDone:
					case <-time.After(time.Second):
						t.Error("outbound traffic producer did not stop")
					}
				})
			}
			var conn *l3TunnelConn
			select {
			case conn = <-replacement:
			case <-time.After(300 * time.Millisecond):
				t.Fatalf("no replacement after %d unanswered probes; misses=%d", probes.Load(), atomic.LoadInt32(&old.heartbeatMisses))
			}
			if conn.conntrackMgr != manager {
				t.Fatal("reconnection discarded the shared conntrack manager")
			}
			want := makeUDPPacket(12345, 53)
			conn.incoming <- want
			select {
			case got := <-tunnel.dataChan:
				if !bytes.Equal(got, want) {
					t.Fatal("replacement did not preserve the packet")
				}
			case <-time.After(time.Second):
				t.Fatal("replacement did not resume packet forwarding")
			}
		})
	}
}
