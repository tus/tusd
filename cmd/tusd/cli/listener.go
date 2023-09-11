package cli

import (
	"errors"
	"net"
	"os"
)

func NewListener(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

// Binds to a UNIX socket. If the file already exists, try to remove it before
// binding again. This logic is borrowed from Gunicorn
// (see https://github.com/benoitc/gunicorn/blob/a8963ef1a5a76f3df75ce477b55fe0297e3b617d/gunicorn/sock.py#L106)
func NewUnixListener(path string) (net.Listener, error) {
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

	return net.Listen("unix", path)
}
