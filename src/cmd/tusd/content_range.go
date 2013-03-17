package main

import (
	"errors"
	"strconv"
	"strings"
)

var errInvalidRange = errors.New("invalid Content-Range")

type contentRange struct {
	Start int64
	End   int64
	Size  int64
}

// parseContentRange parse a Content-Range string like "5-10/100".
// see http://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html#sec14.16 .
// Asterisks "*" will result in End/Size being set to -1.
func parseContentRange(s string) (*contentRange, error) {
	const prefix = "bytes "
	offset := strings.Index(s, prefix)
	if offset != 0 {
		return nil, errInvalidRange
	}
	s = s[len(prefix):]

	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return nil, errInvalidRange
	}

	r := new(contentRange)

	if parts[0] == "*" {
		r.Start = 0
		r.End = -1
	} else {
		offsets := strings.Split(parts[0], "-")
		if len(offsets) != 2 {
			return nil, errInvalidRange
		}

		if offset, err := strconv.ParseInt(offsets[0], 10, 64); err == nil {
			r.Start = offset
		} else {
			return nil, errInvalidRange
		}

		if offset, err := strconv.ParseInt(offsets[1], 10, 64); err == nil {
			r.End = offset
		} else {
			return nil, errInvalidRange
		}

		// A byte-content-range-spec with a byte-range-resp-spec whose last-
		// byte-pos value is less than its first-byte-pos value, or whose
		// instance-length value is less than or equal to its last-byte-pos value,
		// is invalid. The recipient of an invalid byte-content-range- spec MUST
		// ignore it and any content transferred along with it.
		if r.End <= r.Start {
			return nil, errInvalidRange
		}
	}

	if parts[1] == "*" {
		r.Size = -1
		return r, nil
	} else if size, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
		r.Size = size
		return r, nil
	}
	return nil, errInvalidRange
}
