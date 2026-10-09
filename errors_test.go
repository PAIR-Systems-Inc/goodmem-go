// White-box unit tests for the Retry-After parser. parseRetryAfter is
// unexported, so these live in package goodmem (the black-box behavioral
// regressions — that a header-rewriting proxy cannot mask the typed 429 — are in
// client_test.go).
package goodmem

import (
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	future := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	past := time.Now().Add(-90 * time.Second).UTC().Format(http.TimeFormat)

	t.Run("absent", func(t *testing.T) {
		if got := parseRetryAfter(""); got != nil {
			t.Errorf("empty header = %v, want nil", *got)
		}
		if got := parseRetryAfter("   "); got != nil {
			t.Errorf("blank header = %v, want nil", *got)
		}
	})

	t.Run("delta-seconds", func(t *testing.T) {
		got := parseRetryAfter("30")
		if got == nil || *got != 30 {
			t.Errorf("delta-seconds = %v, want 30", got)
		}
	})

	t.Run("negative delta clamps to zero", func(t *testing.T) {
		got := parseRetryAfter("-5")
		if got == nil || *got != 0 {
			t.Errorf("negative delta = %v, want 0", got)
		}
	})

	t.Run("http-date in the future", func(t *testing.T) {
		got := parseRetryAfter(future)
		if got == nil {
			t.Fatal("http-date = nil, want a positive delta")
		}
		// Allow a couple of seconds of clock slack around the ~90s target.
		if *got < 80 || *got > 92 {
			t.Errorf("http-date delta = %d, want ~90", *got)
		}
	})

	t.Run("http-date in the past clamps to zero", func(t *testing.T) {
		got := parseRetryAfter(past)
		if got == nil || *got != 0 {
			t.Errorf("past http-date = %v, want 0", got)
		}
	})

	t.Run("unparseable degrades to nil", func(t *testing.T) {
		for _, raw := range []string{"soon", "later!", "12x", "2099-13-99"} {
			if got := parseRetryAfter(raw); got != nil {
				t.Errorf("parseRetryAfter(%q) = %v, want nil", raw, *got)
			}
		}
	})
}
