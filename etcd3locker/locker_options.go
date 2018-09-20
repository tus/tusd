package etcd3locker

import (
	"strings"
)

var (
	DefaultTtl    = 60
	DefaultPrefix = "/tusd"
)

type LockerOptions struct {
	timeoutSeconds int
	prefix         string
}

func DefaultLockerOptions() LockerOptions {
	return LockerOptions{
		timeoutSeconds: 60,
		prefix:         "/tusd",
	}
}

func NewLockerOptions(timeout int, prefix string) LockerOptions {
	return LockerOptions{
		timeoutSeconds: timeout,
		prefix:         prefix,
	}
}

func (l *LockerOptions) Timeout() int {
	if l.timeoutSeconds == 0 {
		return DefaultTtl
	} else {
		return l.timeoutSeconds
	}
}

func (l *LockerOptions) Prefix() string {
	prefix := l.prefix
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}

	if prefix == "" {
		return DefaultPrefix
	} else {
		return prefix
	}
}

func (l *LockerOptions) SetTimeout(timeout int) {
	l.timeoutSeconds = timeout
}

func (l *LockerOptions) SetPrefix(prefix string) {
	l.prefix = prefix
}
