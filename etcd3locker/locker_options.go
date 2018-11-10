package etcd3locker

import (
	"strings"
)

var (
	DefaultTtl    = 60
	DefaultPrefix = "/tusd"
)

type LockerOptions struct {
	ttl    int
	prefix string
}

// DefaultLockerOptions() instantiates an instance of LockerOptions
// with default 60 second time to live and an etcd3 prefix of "/tusd"
func DefaultLockerOptions() LockerOptions {
	return LockerOptions{
		ttl:    60,
		prefix: "/tusd",
	}
}

// NewLockerOptions instantiates an instance of LockerOptions with a
// provided TTL (time to live) and string prefix for keys to be stored in etcd3
func NewLockerOptions(ttl int, prefix string) LockerOptions {
	return LockerOptions{
		ttl:    ttl,
		prefix: prefix,
	}
}

// Returns the TTL (time to live) of sessions in etcd3
func (l *LockerOptions) Ttl() int {
	if l.ttl == 0 {
		return DefaultTtl
	} else {
		return l.ttl
	}
}

// Returns the string prefix used to store keys in etcd3
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

// Set etcd3 session TTL (time to live)
func (l *LockerOptions) SetTtl(ttl int) {
	l.ttl = ttl
}

// Set string prefix to be used in keys stored into etcd3 by the locker
func (l *LockerOptions) SetPrefix(prefix string) {
	l.prefix = prefix
}
