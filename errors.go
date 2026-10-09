package goodmem

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Error is the supertype of every error this SDK returns. Catch any
// SDK-originated failure with errors.As(err, &goodmemErr) where goodmemErr is a
// goodmem.Error — it matches HTTP status errors (*APIError and its typed kin),
// transport failures (*NetworkError), and SDK-internal failures (*GoodmemError)
// without enumerating concrete types.
//
// Plain caller-fault argument validation (empty path id, nil request) is
// returned as a standard error (fmt.Errorf), NOT as a goodmem.Error — mirroring
// the cross-language rule (inv-exception-types-mapping) that caller-input
// validation stays a language-standard error rather than collapsing into the
// SDK-error catch bucket.
type Error interface {
	error
	sdkError()
}

// GoodmemError is a generic SDK-internal failure that is neither an HTTP status
// error nor a transport failure — most commonly an unparseable response body.
// Go analogue of Java's GoodmemException base.
type GoodmemError struct {
	Message string
	Cause   error
}

func (e *GoodmemError) Error() string { return e.Message }
func (e *GoodmemError) Unwrap() error { return e.Cause }
func (e *GoodmemError) sdkError()     {}

// NetworkError wraps a transport-level failure (DNS, connect, timeout, mid-read
// drop). The underlying cause is available via errors.Unwrap so users who catch
// goodmem.Error still see transport failures. inv-network-error-wrap.
type NetworkError struct {
	Message string
	Cause   error
}

func (e *NetworkError) Error() string { return e.Message }
func (e *NetworkError) Unwrap() error { return e.Cause }
func (e *NetworkError) sdkError()     {}

// APIError is the base for any HTTP 4xx/5xx returned by the server. Its Message
// embeds the server's response body (truncated to 500 chars) so logging the
// error shows why the call failed instead of a bare status code
// (inv-http-error-body). The full payload is on Body; branch on StatusCode.
//
// Typed subclasses cover the common statuses; a status without a typed subclass
// surfaces as a plain *APIError. Every subclass exposes the base via
// errors.As(err, &apiErr) where apiErr is *APIError.
type APIError struct {
	StatusCode int
	Body       string
	Message    string
}

func (e *APIError) Error() string { return e.Message }
func (e *APIError) sdkError()     {}

// Typed HTTP status errors. Each embeds *APIError (promoting Error/sdkError) and
// unwraps to it so errors.As(err, &apiErr) matches regardless of the subclass.
type (
	// BadRequestError is HTTP 400.
	BadRequestError struct{ *APIError }
	// AuthenticationError is HTTP 401.
	AuthenticationError struct{ *APIError }
	// PermissionDeniedError is HTTP 403.
	PermissionDeniedError struct{ *APIError }
	// NotFoundError is HTTP 404.
	NotFoundError struct{ *APIError }
	// ConflictError is HTTP 409.
	ConflictError struct{ *APIError }
	// UnprocessableEntityError is HTTP 422.
	UnprocessableEntityError struct{ *APIError }
	// InternalServerError is HTTP 5xx.
	InternalServerError struct{ *APIError }
)

func (e *BadRequestError) Unwrap() error          { return e.APIError }
func (e *AuthenticationError) Unwrap() error      { return e.APIError }
func (e *PermissionDeniedError) Unwrap() error    { return e.APIError }
func (e *NotFoundError) Unwrap() error            { return e.APIError }
func (e *ConflictError) Unwrap() error            { return e.APIError }
func (e *UnprocessableEntityError) Unwrap() error { return e.APIError }
func (e *InternalServerError) Unwrap() error      { return e.APIError }

// RateLimitError is HTTP 429. RetryAfterSeconds holds the parsed Retry-After
// header in seconds, or nil when the server did not send it (or sent a value
// the SDK could not parse — see parseRetryAfter). A missing hint never masks
// the error itself: the typed 429 is always returned with the status and body.
type RateLimitError struct {
	*APIError
	RetryAfterSeconds *int64
}

func (e *RateLimitError) Unwrap() error { return e.APIError }

// mapError builds the typed error for a non-2xx response. The message embeds
// the body (truncated) per inv-http-error-body.
func mapError(status int, body string, retryAfterSeconds *int64) error {
	msg := fmt.Sprintf("HTTP %d", status)
	if body != "" {
		msg = fmt.Sprintf("HTTP %d: %s", status, truncate(body, 500))
	}
	base := &APIError{StatusCode: status, Body: body, Message: msg}
	switch status {
	case 400:
		return &BadRequestError{base}
	case 401:
		return &AuthenticationError{base}
	case 403:
		return &PermissionDeniedError{base}
	case 404:
		return &NotFoundError{base}
	case 409:
		return &ConflictError{base}
	case 422:
		return &UnprocessableEntityError{base}
	case 429:
		return &RateLimitError{base, retryAfterSeconds}
	default:
		if status >= 500 {
			return &InternalServerError{base}
		}
		return base
	}
}

// parseRetryAfter parses the Retry-After header per RFC 7231. It accepts BOTH
// forms the spec permits: delta-seconds (what the goodmem server emits) and the
// HTTP-date form that intermediaries — Cloudflare, nginx and other proxies —
// commonly substitute on 429/503. The HTTP-date form is converted to a
// remaining-seconds delta against the current clock and clamped at zero.
//
// It returns nil when the header is absent OR present-but-unparseable. A
// malformed value degrades to "no retry hint" rather than masking the response:
// the caller still receives the typed *RateLimitError/*InternalServerError
// carrying the real status and body (with RetryAfterSeconds == nil), so
// errors.As matching keeps working behind a header-rewriting proxy. This is a
// deliberate divergence from Java's fail-loud HttpExecutor.parseRetryAfter.
func parseRetryAfter(raw string) *int64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if secs, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		if secs < 0 {
			secs = 0
		}
		return &secs
	}
	if t, err := http.ParseTime(trimmed); err == nil {
		secs := int64(time.Until(t) / time.Second)
		if secs < 0 {
			secs = 0
		}
		return &secs
	}
	return nil
}

// truncate returns at most n runes of s, never splitting a multi-byte rune.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// len(s) > n bytes does not imply > n runes for multi-byte text; check the
	// rune count without allocating before falling back to the []rune slice.
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
