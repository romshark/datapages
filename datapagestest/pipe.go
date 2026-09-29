package datapagestest

import (
	"context"
	"net"
	"sync"
)

// pipeListener hands the server one end of a [net.Pipe] for each connection a client
// dials. The net/http server serves a pipe the way it serves a socket: it discards an
// unread request body once the response headers are flushed and cancels the request
// context when the client closes the connection. Unlike a goroutine blocked on a
// socket, one blocked on a pipe lets a testing/synctest bubble go idle.
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

// dial connects a client to the listener. It is the DialContext of the visitors'
// transport, which ignores the address it is asked for.
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

// pipeAddr is the address the server reports and its URL names. The cookie jar sends
// a Secure cookie, which the session cookie is, to a loopback host over plain HTTP.
var pipeAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 80}

// loopbackConn reports loopback addresses instead of the "pipe" of a net.Pipe, which
// makes the request's RemoteAddr the host and port a socket gives it.
type loopbackConn struct{ net.Conn }

func (loopbackConn) LocalAddr() net.Addr  { return pipeAddr }
func (loopbackConn) RemoteAddr() net.Addr { return pipeAddr }
