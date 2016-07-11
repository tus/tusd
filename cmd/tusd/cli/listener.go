package cli

import (
	"net"
	"time"
)

// Listener wraps a net.Listener, and gives a place to store the timeout
// parameters. On Accept, it will wrap the net.Conn with our own Conn for us.
// Original implementation taken from https://gist.github.com/jbardin/9663312
// Thanks! <3
type Listener struct {
	net.Listener
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func (l *Listener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	go MetricsOpenConnections.Inc()

	tc := &Conn{
		Conn:         c,
		ReadTimeout:  l.ReadTimeout,
		WriteTimeout: l.WriteTimeout,
	}
	return tc, nil
}

// Conn wraps a net.Conn, and sets a deadline for every read
// and write operation.
type Conn struct {
	net.Conn
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func (c *Conn) Read(b []byte) (int, error) {
	var err error
	if c.ReadTimeout > 0 {
		err = c.Conn.SetReadDeadline(time.Now().Add(c.ReadTimeout))
	} else {
		err = c.Conn.SetReadDeadline(time.Time{})
	}

	if err != nil {
		return 0, err
	}

	return c.Conn.Read(b)
}

func (c *Conn) Write(b []byte) (int, error) {
	var err error
	if c.WriteTimeout > 0 {
		err = c.Conn.SetWriteDeadline(time.Now().Add(c.WriteTimeout))
	} else {
		err = c.Conn.SetWriteDeadline(time.Time{})
	}

	if err != nil {
		return 0, err
	}

	return c.Conn.Write(b)
}

func (c *Conn) Close() error {
	go MetricsOpenConnections.Dec()
	return c.Conn.Close()
}

func NewListener(addr string, readTimeout, writeTimeout time.Duration) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	tl := &Listener{
		Listener:     l,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}
	return tl, nil
}
