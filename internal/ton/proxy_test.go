package ton

import (
	"context"
	"net"
	"testing"
	"time"
)

// freePort finds an available TCP port and closes the listener,
// leaving the port available (with a tiny race window acceptable for tests).
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: could not bind: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func TestProxyStatusOnPort_NoListener(t *testing.T) {
	port := freePort(t)

	running, gotPort, err := ProxyStatusOnPort(port)

	if running {
		t.Errorf("ProxyStatusOnPort(%d): expected running=false, got true", port)
	}
	if gotPort != port {
		t.Errorf("ProxyStatusOnPort(%d): expected returned port=%d, got %d", port, port, gotPort)
	}
	if err != nil {
		t.Errorf("ProxyStatusOnPort(%d): expected err=nil, got %v", port, err)
	}
}

func TestWaitForProxyOnPort_CanceledContext(t *testing.T) {
	port := freePort(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel before calling

	// Short timeout so the test ends quickly even if ctx.Done() isn't picked first.
	err := WaitForProxyOnPort(ctx, port, 200*time.Millisecond)

	if err == nil {
		t.Fatal("WaitForProxyOnPort with canceled context: expected error, got nil")
	}
}
