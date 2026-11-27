package cli

import (
	"testing"
	"time"
)

func TestHooksTimeout(t *testing.T) {
	d := func(v time.Duration) *time.Duration { return &v }

	tests := []struct {
		name        string
		hooks, http *time.Duration
		wantHooks   time.Duration
		wantHttp    time.Duration
	}{
		{"defaults", nil, nil, 0, 15 * time.Second},
		{"hooks-timeout", d(time.Minute), nil, time.Minute, time.Minute},
		{"hooks-timeout zero", d(0), nil, 0, 0},
		{"hooks-http-timeout", nil, d(time.Minute), 0, time.Minute},
		{"both", d(time.Minute), d(time.Second), time.Minute, time.Second},
	}

	original := Flags
	t.Cleanup(func() { Flags = original })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Flags.HooksTimeout = tt.hooks
			Flags.HttpHooksTimeout = tt.http

			if got := hooksTimeout(); got != tt.wantHooks {
				t.Errorf("hooksTimeout() = %s, want %s", got, tt.wantHooks)
			}
			if got := httpHooksTimeout(); got != tt.wantHttp {
				t.Errorf("httpHooksTimeout() = %s, want %s", got, tt.wantHttp)
			}
		})
	}
}
