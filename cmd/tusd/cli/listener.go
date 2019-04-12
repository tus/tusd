package cli

import (
	"errors"
	"net"
	"os"
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

	// closeRecorded will be true if the connection has been closed and the
	// corresponding prometheus counter has been decremented. It will be used to
	// avoid duplicated modifications to this metric.
	closeRecorded bool
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
	// Only decremented the prometheus counter if the Close function has not been
	// invoked before to avoid duplicated modifications.
	if !c.closeRecorded {
		c.closeRecorded = true
		MetricsOpenConnections.Dec()
	}

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

// Binds to a UNIX socket. If the file already exists, try to remove it before
// binding again. This logic is borrowed from Gunicorn
// (see https://github.com/benoitc/gunicorn/blob/a8963ef1a5a76f3df75ce477b55fe0297e3b617d/gunicorn/sock.py#L106)
func NewUnixListener(path string, readTimeout, writeTimeout time.Duration) (net.Listener, error) {
	stat, err := os.Stat(path)

	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		if stat.Mode()&os.ModeSocket != 0 {
			err = os.Remove(path)

			if err != nil {
				return nil, err
			}
		} else {
			return nil, errors.New("specified path is not a socket")
		}
	}

	l, err := net.Listen("unix", path)

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
