package goodmem

import (
	"fmt"
	"net/http"
	"strings"
)

// Client is a synchronous client for the GoodMem REST API. Construct it with
// New, then reach each namespace through its accessor — c.Spaces().Create(...),
// c.Memories().Get(...), and so on. Methods take a context.Context (cancel or
// set a deadline on it to bound a call) and return a typed response plus an
// error; non-2xx responses come back as *APIError subclasses and transport
// failures as *NetworkError.
//
// A Client is safe for concurrent use by multiple goroutines.
type Client struct {
	baseURL            string // trailing slash trimmed
	apiKey             string
	httpClient         *http.Client
	streamMaxLineBytes int // per-event NDJSON line cap; <=0 means the default
}

// Option customizes a Client at construction time.
type Option func(*Client)

// WithHTTPClient supplies a pre-configured *http.Client (BYO mode) instead of
// the SDK-built default. In BYO mode the caller owns the redirect policy; the
// SDK-built default refuses to follow redirects so the x-api-key custom header
// cannot leak across a host boundary (inv-no-redirect-with-custom-auth-header).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithStreamMaxLineBytes overrides the maximum size of a single NDJSON stream
// event (memories.retrieve, ping.stream). The default is 8 MiB; a single
// retrieved item can approach that when Retrieve runs with FetchMemoryContent
// set, so raise this if a large document ends iteration with a "line cap"
// error. A value <= 0 keeps the default.
func WithStreamMaxLineBytes(n int) Option {
	return func(c *Client) { c.streamMaxLineBytes = n }
}

// New returns a Client for the given base URL and API key. The base URL may
// carry a mount path prefix (e.g. https://host/api) — it is preserved on every
// request. An empty base URL is rejected here because there is no host to send
// to; an empty apiKey is allowed (the unauthenticated bootstrap endpoints
// /v1/system/info and /v1/system/init are callable without a key, and every
// other endpoint returns 401 from the server) — so the server, not the SDK,
// owns the "missing key" error (inv-empty-string-required-args).
func New(baseURL, apiKey string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("goodmem.New: baseURL must not be empty")
	}
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{
			// Refuse to follow redirects on the SDK-built client. Go's stdlib
			// (like most HTTP libraries) only strips sensitive headers across
			// hosts for the Authorization header; x-api-key would otherwise
			// ride a 3xx to a typoed or hostile host. Returning
			// ErrUseLastResponse stops following without erroring.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return c, nil
}

// Ptr returns a pointer to v. Optional request fields are modeled as *T so an
// unset field (nil) is distinguishable from a zero value, which otherwise forces
// callers to introduce a temporary just to take its address. Ptr removes that
// boilerplate at the call site:
//
//	req := &models.EmbedderCreationRequest{
//	    DisplayName:     "OpenAI Small",
//	    ModelIdentifier: "text-embedding-3-small",
//	    MaxSequenceLen:  goodmem.Ptr(int32(512)),
//	}
//
// The type parameter is inferred from the argument, so a numeric literal keeps
// its default type — write goodmem.Ptr(int32(5)) when the field is *int32.
func Ptr[T any](v T) *T { return &v }
