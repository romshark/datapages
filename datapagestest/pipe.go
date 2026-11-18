package datapagestest

import (
	"context"
	"net"
	"sync"
)

// pipeListener connects clients and the server with [net.Pipe]. net/http still handles
// body disposal and cancellation, while blocked pipes let testing/synctest detect idle.
type pipeListener struct {
	conns     chan net.Conn
	closed    chan struct{}
	closeOnce sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{conns: make(chan net.Conn), closed: make(chan struct{})}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr }

// dial ignores the requested address and connects to the listener.
func (l *pipeListener) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	server, client := net.Pipe()
	select {
	case l.conns <- loopbackConn{server}:
		return client, nil
	case <-l.closed:
		return nil, net.ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Loopback lets the cookie jar send Secure session cookies over plain HTTP.
var pipeAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80}

// loopbackConn gives requests a socket-like RemoteAddr.
type loopbackConn struct{ net.Conn }

func (loopbackConn) LocalAddr() net.Addr  { return pipeAddr }
func (loopbackConn) RemoteAddr() net.Addr { return pipeAddr }
