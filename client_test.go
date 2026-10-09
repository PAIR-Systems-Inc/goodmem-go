// Hand-written runtime tests for the Go SDK transport + generated services.
// Drives the emitted service methods against an httptest server to verify path
// building, auth header, JSON (de)serialization, query/deepObject encoding,
// the typed error hierarchy, transport-error wrapping, and base-URL mount
// preservation.
package goodmem_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	goodmem "github.com/PAIR-Systems-Inc/goodmem-go"
	"github.com/PAIR-Systems-Inc/goodmem-go/models"
)

func strptr(s string) *string { return &s }

// newServerClient spins an httptest server with the given handler and a client
// pointed at it.
func newServerClient(t *testing.T, handler http.HandlerFunc) (*goodmem.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := goodmem.New(srv.URL, "gm_test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, srv
}

func TestGetSendsAuthAndDecodes(t *testing.T) {
	var gotPath, gotKey, gotAccept string
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey, gotAccept = r.URL.Path, r.Header.Get("x-api-key"), r.Header.Get("Accept")
		_ = json.NewEncoder(w).Encode(models.Space{SpaceID: "s1", Name: "Doc"})
	})
	sp, err := c.Spaces().Get(context.Background(), "s1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sp.SpaceID != "s1" || sp.Name != "Doc" {
		t.Errorf("decoded = %+v", sp)
	}
	if gotPath != "/v1/spaces/s1" {
		t.Errorf("path = %q, want /v1/spaces/s1", gotPath)
	}
	if gotKey != "gm_test" {
		t.Errorf("x-api-key = %q", gotKey)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q", gotAccept)
	}
}

func TestCreateMarshalsBodyAndSetsContentType(t *testing.T) {
	var method, contentType string
	var body map[string]any
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		method, contentType = r.Method, r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(models.Space{SpaceID: "new"})
	})
	sp, err := c.Spaces().Create(context.Background(), &models.SpaceCreationRequest{Name: "Doc"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sp.SpaceID != "new" {
		t.Errorf("decoded SpaceID = %q", sp.SpaceID)
	}
	if method != "POST" || contentType != "application/json" {
		t.Errorf("method=%q contentType=%q", method, contentType)
	}
	if body["name"] != "Doc" {
		t.Errorf("request body name = %v", body["name"])
	}
}

func TestDeleteUsesVerbAndPathAndReturnsNoError(t *testing.T) {
	var method, path string
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.Spaces().Delete(context.Background(), "s1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if method != "DELETE" || path != "/v1/spaces/s1" {
		t.Errorf("method=%q path=%q", method, path)
	}
}

func TestListEncodesQueryWithEnumAndDeepObjectLabel(t *testing.T) {
	var query url.Values
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		_ = json.NewEncoder(w).Encode(models.ListSpacesResponse{})
	})
	order := models.SortOrderASCENDING
	_, err := c.Spaces().List(context.Background(), &goodmem.SpacesListParams{
		NameFilter: strptr("doc*"),
		SortOrder:  &order,
		Label:      map[string]string{"env": "prod"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := query.Get("nameFilter"); got != "doc*" {
		t.Errorf("nameFilter = %q", got)
	}
	if got := query.Get("sortOrder"); got != "ASCENDING" {
		t.Errorf("sortOrder = %q (enum should serialize to wire token)", got)
	}
	// deepObject: dict label expands to label[key]=value.
	if got := query.Get("label[env]"); got != "prod" {
		t.Errorf("label[env] = %q, want prod (deepObject expansion)", got)
	}
}

func TestNilListParamsSendsNoQuery(t *testing.T) {
	var rawQuery string
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(models.ListSpacesResponse{})
	})
	if _, err := c.Spaces().List(context.Background(), nil); err != nil {
		t.Fatalf("List(nil): %v", err)
	}
	if rawQuery != "" {
		t.Errorf("nil params should send no query, got %q", rawQuery)
	}
}

func TestErrorMappingTypedAndBodyEmbedded(t *testing.T) {
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"space not found"}`))
	})
	_, err := c.Spaces().Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	var nf *goodmem.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("expected *NotFoundError, got %T", err)
	}
	var api *goodmem.APIError
	if !errors.As(err, &api) {
		t.Fatalf("404 should also unwrap to *APIError, got %T", err)
	}
	if api.StatusCode != 404 {
		t.Errorf("StatusCode = %d", api.StatusCode)
	}
	if !strings.Contains(api.Error(), "space not found") {
		t.Errorf("error message must embed server body (inv-http-error-body): %q", api.Error())
	}
	var sdkErr goodmem.Error
	if !errors.As(err, &sdkErr) {
		t.Error("server error should satisfy the goodmem.Error supertype")
	}
}

func TestUnauthorizedMapsToAuthenticationError(t *testing.T) {
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("nope"))
	})
	_, err := c.Spaces().Get(context.Background(), "s1")
	var ae *goodmem.AuthenticationError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AuthenticationError, got %T", err)
	}
}

func TestRateLimitParsesDeltaSecondsRetryAfter(t *testing.T) {
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "42")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"slow down"}`))
	})
	_, err := c.Spaces().Get(context.Background(), "s1")
	var rl *goodmem.RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("expected *RateLimitError, got %T", err)
	}
	if rl.RetryAfterSeconds == nil || *rl.RetryAfterSeconds != 42 {
		t.Errorf("RetryAfterSeconds = %v, want 42", rl.RetryAfterSeconds)
	}
}

// A proxy that rewrites Retry-After into the RFC 7231 HTTP-date form must NOT
// mask the typed 429: the caller still receives *RateLimitError carrying the
// status and body, and the date is converted to a remaining-seconds hint.
// (inv-http-error-body must survive a header-rewriting CDN.)
func TestRateLimitHTTPDateRetryAfterDoesNotMaskTypedError(t *testing.T) {
	future := time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", future)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"slow down"}`))
	})
	_, err := c.Spaces().Get(context.Background(), "s1")
	var rl *goodmem.RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("HTTP-date Retry-After must still map to *RateLimitError, got %T (%v)", err, err)
	}
	var api *goodmem.APIError
	if !errors.As(err, &api) || api.StatusCode != 429 {
		t.Fatalf("must carry status+body as *APIError, got %T", err)
	}
	if !strings.Contains(api.Error(), "slow down") {
		t.Errorf("must embed server body, got %q", api.Error())
	}
	if rl.RetryAfterSeconds == nil || *rl.RetryAfterSeconds <= 0 || *rl.RetryAfterSeconds > 121 {
		t.Errorf("RetryAfterSeconds from HTTP-date = %v, want ~120", rl.RetryAfterSeconds)
	}
}

// A Retry-After a proxy garbles into an unparseable value degrades to "no hint"
// — it must never replace the underlying typed 5xx error.
func TestUnparseableRetryAfterDegradesToTypedError(t *testing.T) {
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "soon-ish")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	})
	_, err := c.Spaces().Get(context.Background(), "s1")
	var ise *goodmem.InternalServerError
	if !errors.As(err, &ise) {
		t.Fatalf("expected *InternalServerError, got %T (%v)", err, err)
	}
}

func TestTransportFailureWrapsAsNetworkError(t *testing.T) {
	// Point at a server we immediately close → connection refused.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	c, err := goodmem.New(url, "k")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = c.Spaces().Get(context.Background(), "s1")
	var ne *goodmem.NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("expected *NetworkError, got %T (%v)", err, err)
	}
	if ne.Unwrap() == nil {
		t.Error("NetworkError should attach the underlying cause")
	}
}

func TestEmptyPathArgIsPlainErrorNotSDKError(t *testing.T) {
	c, _ := newServerClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("server must not be hit when the path arg is empty")
	})
	_, err := c.Spaces().Get(context.Background(), "")
	if err == nil {
		t.Fatal("expected validation error for empty id")
	}
	var sdkErr goodmem.Error
	if errors.As(err, &sdkErr) {
		t.Errorf("caller-fault validation should be a plain error, not goodmem.Error: %T", err)
	}
}

func TestBytesEndpointReturnsRawBody(t *testing.T) {
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/memories/m1/content" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte("PDFBYTES"))
	})
	data, err := c.Memories().Content(context.Background(), "m1")
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if string(data) != "PDFBYTES" {
		t.Errorf("bytes body = %q", data)
	}
}

func TestBaseURLMountPrefixPreserved(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(models.Space{SpaceID: "s1"})
	}))
	t.Cleanup(srv.Close)
	c, err := goodmem.New(srv.URL+"/api", "k")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Spaces().Get(context.Background(), "s1"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotPath != "/api/v1/spaces/s1" {
		t.Errorf("mount prefix lost: path = %q, want /api/v1/spaces/s1", gotPath)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	leaked := false
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/leak" {
			leaked = true
			return
		}
		http.Redirect(w, r, "/leak", http.StatusFound)
	})
	// The SDK must not follow the redirect (would leak x-api-key cross-host) and
	// surfaces it as a clear *GoodmemError rather than a confusing parse failure.
	_, err := c.Spaces().Get(context.Background(), "s1")
	if leaked {
		t.Error("client followed a redirect — x-api-key could leak across hosts (inv-no-redirect-with-custom-auth-header)")
	}
	var ge *goodmem.GoodmemError
	if !errors.As(err, &ge) {
		t.Errorf("redirect should surface as *GoodmemError, got %T: %v", err, err)
	} else if !strings.Contains(ge.Error(), "redirect") {
		t.Errorf("redirect error message = %q, want it to mention the redirect", ge.Error())
	}
}

func TestCreateDoesNotMutateCallerRequest(t *testing.T) {
	// The convenience layer fills defaults/inference on a shallow copy, so the
	// caller's request struct is never written back.
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{}"))
	})
	req := &models.EmbedderCreationRequest{ModelIdentifier: "text-embedding-3-small"}
	if _, err := c.Embedders().Create(context.Background(), req, "sk-test"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if req.ProviderType != "" {
		t.Errorf("caller request was mutated: ProviderType = %q, want empty", req.ProviderType)
	}
	if req.Dimensionality != 0 {
		t.Errorf("caller request was mutated: Dimensionality = %d, want 0", req.Dimensionality)
	}
	if req.Credentials != nil {
		t.Error("caller request was mutated: Credentials was set from apiKey")
	}
}

func TestStreamCloseIsIdempotent(t *testing.T) {
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{}\n"))
	})
	stream, err := c.Ping().Stream(context.Background(), &models.PingStreamRequest{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for range stream.Events() { // drain (Events also closes on completion)
	}
	if err := stream.Close(); err != nil {
		t.Errorf("explicit Close after Events drained: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Errorf("third Close should be a no-op, got: %v", err)
	}
}

func TestPaginationThreadsTokenAcrossPages(t *testing.T) {
	page := 0
	var gotTokens []string
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotTokens = append(gotTokens, r.URL.Query().Get("nextToken"))
		page++
		if page == 1 {
			next := "tok2"
			_ = json.NewEncoder(w).Encode(models.ListSpacesResponse{
				Spaces:    []models.Space{{SpaceID: "s1"}, {SpaceID: "s2"}},
				NextToken: &next,
			})
		} else {
			_ = json.NewEncoder(w).Encode(models.ListSpacesResponse{
				Spaces: []models.Space{{SpaceID: "s3"}},
			})
		}
	})
	p, err := c.Spaces().List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var ids []string
	for sp, err := range p.All(context.Background()) {
		if err != nil {
			t.Fatalf("pagination: %v", err)
		}
		ids = append(ids, sp.SpaceID)
	}
	if strings.Join(ids, ",") != "s1,s2,s3" {
		t.Errorf("collected ids = %v, want [s1 s2 s3]", ids)
	}
	// First request carries no token; the second carries the page-1 nextToken.
	if len(gotTokens) != 2 || gotTokens[0] != "" || gotTokens[1] != "tok2" {
		t.Errorf("token threading = %v, want ['' 'tok2']", gotTokens)
	}
}

func TestPaginationSinglePageNoSecondFetch(t *testing.T) {
	calls := 0
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = json.NewEncoder(w).Encode(models.ListSpacesResponse{Spaces: []models.Space{{SpaceID: "only"}}})
	})
	p, err := c.Spaces().List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var n int
	for _, err := range p.All(context.Background()) {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 1 {
		t.Errorf("items = %d, want 1", n)
	}
	if calls != 1 {
		t.Errorf("server hit %d times, want 1 (no extra fetch without a token)", calls)
	}
}

func TestStreamYieldsEventsAndSkipsNullFrames(t *testing.T) {
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		enc := json.NewEncoder(w)
		_ = enc.Encode(models.PingEvent{Result: &models.PingResult{Seq: 1}})
		_, _ = w.Write([]byte("{}\n")) // all-null oneOf frame → must be skipped
		_ = enc.Encode(models.PingEvent{Notice: &models.PingNotice{Code: "bye"}})
	})
	stream, err := c.Ping().Stream(context.Background(), &models.PingStreamRequest{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()

	var events []models.PingEvent
	for ev, err := range stream.Events() {
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		events = append(events, ev)
	}
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2 (the {} frame must be skipped)", len(events))
	}
	if events[0].Result == nil || events[0].Result.Seq != 1 {
		t.Errorf("event[0] = %+v, want Result.Seq=1", events[0])
	}
	if events[1].Notice == nil || events[1].Notice.Code != "bye" {
		t.Errorf("event[1] = %+v, want Notice.Code=bye", events[1])
	}
}

func TestStreamMalformedLineSurfacesError(t *testing.T) {
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{\"result\":{\"seq\":1}}\nnot json\n"))
	})
	stream, err := c.Ping().Stream(context.Background(), &models.PingStreamRequest{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()

	var gotErr error
	valid := 0
	for ev, err := range stream.Events() {
		if err != nil {
			gotErr = err
			break
		}
		_ = ev
		valid++
	}
	if valid != 1 {
		t.Errorf("expected 1 valid event before the malformed line, got %d", valid)
	}
	var ge *goodmem.GoodmemError
	if !errors.As(gotErr, &ge) {
		t.Fatalf("malformed line should surface as *GoodmemError, got %T (%v)", gotErr, gotErr)
	}
}

// A single NDJSON event larger than the per-event line cap ends iteration with a
// *GoodmemError whose message names the cap and the two ways out — not an opaque
// bufio "token too long".
func TestStreamLineCapExceededGivesActionableError(t *testing.T) {
	big := strings.Repeat("x", 4096)
	handler := func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(models.PingEvent{Notice: &models.PingNotice{Code: big}})
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(srv.Close)
	c, err := goodmem.New(srv.URL, "k", goodmem.WithStreamMaxLineBytes(512))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	stream, err := c.Ping().Stream(context.Background(), &models.PingStreamRequest{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()

	var gotErr error
	for _, err := range stream.Events() {
		if err != nil {
			gotErr = err
			break
		}
	}
	var ge *goodmem.GoodmemError
	if !errors.As(gotErr, &ge) {
		t.Fatalf("oversized event should surface as *GoodmemError, got %T (%v)", gotErr, gotErr)
	}
	for _, want := range []string{"line cap", "WithStreamMaxLineBytes", "FetchMemoryContent"} {
		if !strings.Contains(ge.Error(), want) {
			t.Errorf("error message must mention %q, got %q", want, ge.Error())
		}
	}
}

// Raising the cap with WithStreamMaxLineBytes lets the same event through.
func TestStreamMaxLineBytesRaisesCap(t *testing.T) {
	big := strings.Repeat("x", 4096)
	handler := func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(models.PingEvent{Notice: &models.PingNotice{Code: big}})
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(srv.Close)
	c, err := goodmem.New(srv.URL, "k", goodmem.WithStreamMaxLineBytes(1<<20))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	stream, err := c.Ping().Stream(context.Background(), &models.PingStreamRequest{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer stream.Close()

	got := 0
	for ev, err := range stream.Events() {
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		if ev.Notice == nil || len(ev.Notice.Code) != len(big) {
			t.Errorf("event content truncated: %+v", ev)
		}
		got++
	}
	if got != 1 {
		t.Errorf("got %d events, want 1", got)
	}
}

func TestUsersGetMergedRouting(t *testing.T) {
	var gotPath string
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_ = json.NewEncoder(w).Encode(models.UserResponse{})
	})
	ctx := context.Background()

	// id route
	if _, err := c.Users().Get(ctx, &goodmem.UsersGetOptions{ID: strptr("u1")}); err != nil {
		t.Fatalf("Get by id: %v", err)
	}
	if gotPath != "/v1/users/u1" {
		t.Errorf("id route path = %q", gotPath)
	}

	// email route — the value lands in the email path segment. ('@' is legal in
	// a path per RFC 3986, so url.PathEscape leaves it as-is; routing-breaking
	// characters like spaces/slashes are what get percent-encoded.)
	if _, err := c.Users().Get(ctx, &goodmem.UsersGetOptions{Email: strptr("a@b.com")}); err != nil {
		t.Fatalf("Get by email: %v", err)
	}
	if gotPath != "/v1/users/email/a@b.com" {
		t.Errorf("email route path = %q, want /v1/users/email/a@b.com", gotPath)
	}

	// A routing-breaking character (space) must be percent-encoded.
	if _, err := c.Users().Get(ctx, &goodmem.UsersGetOptions{Email: strptr("a b@c")}); err != nil {
		t.Fatalf("Get by email with space: %v", err)
	}
	if gotPath != "/v1/users/email/a%20b@c" {
		t.Errorf("space in email should be encoded: %q", gotPath)
	}
}

func TestUsersGetMergedMutex(t *testing.T) {
	c, _ := newServerClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("server must not be hit when the options bag is invalid")
	})
	ctx := context.Background()
	// neither set
	if _, err := c.Users().Get(ctx, &goodmem.UsersGetOptions{}); err == nil {
		t.Error("no key set should error")
	}
	// both set
	if _, err := c.Users().Get(ctx, &goodmem.UsersGetOptions{ID: strptr("u1"), Email: strptr("a@b.com")}); err == nil {
		t.Error("both keys set should error")
	}
	// nil opts
	if _, err := c.Users().Get(ctx, nil); err == nil {
		t.Error("nil opts should error")
	}
}

func TestRetrieveConvenienceSendsAssembledRequest(t *testing.T) {
	var body map[string]any
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/x-ndjson")
		// no events — this test only asserts the outbound request shape
	})
	llmID, prompt := "llm-1", "hi"
	temp := 0.5
	stream, err := c.Memories().Retrieve(context.Background(), "find docs", &goodmem.RetrieveOptions{
		SpaceIDs: []string{"sp1", "sp2"},
		LLMID:    &llmID,
		LLMTemp:  &temp,
		Prompt:   &prompt,
	})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	defer stream.Close()
	for _, err := range stream.Events() {
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
	}

	if body["message"] != "find docs" {
		t.Errorf("message = %v", body["message"])
	}
	keys, _ := body["spaceKeys"].([]any)
	if len(keys) != 2 {
		t.Fatalf("spaceKeys = %v", body["spaceKeys"])
	}
	if k0, _ := keys[0].(map[string]any); k0["spaceId"] != "sp1" {
		t.Errorf("spaceKeys[0] = %v", keys[0])
	}
	pp, _ := body["postProcessor"].(map[string]any)
	if pp == nil {
		t.Fatal("postProcessor missing from wire request")
	}
	if pp["name"] != chatPostProcessorFactoryWire {
		t.Errorf("postProcessor.name = %v", pp["name"])
	}
	cfg, _ := pp["config"].(map[string]any)
	if cfg["llm_id"] != "llm-1" || cfg["llm_temp"] != 0.5 || cfg["prompt"] != "hi" {
		t.Errorf("postProcessor.config = %v", cfg)
	}
}

// mirror of the unexported chatPostProcessorFactory for the black-box test.
const chatPostProcessorFactoryWire = "com.goodmem.retrieval.postprocess.ChatPostProcessorFactory"

func TestNewRejectsEmptyBaseURL(t *testing.T) {
	if _, err := goodmem.New("", "k"); err == nil {
		t.Error("empty baseURL must be rejected")
	}
}

// The generated embedders.Create applies the registry/api-key convenience
// transforms to the request body before it is sent over the wire.
func TestEmbeddersCreateAppliesConvenienceOnWire(t *testing.T) {
	var body map[string]any
	c, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(models.EmbedderResponse{})
	})
	_, err := c.Embedders().Create(context.Background(),
		&models.EmbedderCreationRequest{DisplayName: "e", ModelIdentifier: "text-embedding-3-small"},
		"sk-secret")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if body["providerType"] != "OPENAI" {
		t.Errorf("providerType not filled from registry on the wire: %v", body["providerType"])
	}
	if body["dimensionality"] != float64(1536) {
		t.Errorf("dimensionality not filled on the wire: %v", body["dimensionality"])
	}
	if _, ok := body["credentials"]; !ok {
		t.Error("apiKey was not wrapped into credentials on the wire")
	}
}

// A body that fails model-layer enforcement must be rejected at the call site,
// before any HTTP request is sent.
func TestServiceValidatesBodyBeforeSending(t *testing.T) {
	c, _ := newServerClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("server must not be hit when the request body fails Validate()")
	})
	// UpdateSpaceRequest with both replaceLabels and mergeLabels → at-most-one mutex.
	_, err := c.Spaces().Update(context.Background(), "s1", &models.UpdateSpaceRequest{
		ReplaceLabels: map[string]string{"a": "b"},
		MergeLabels:   map[string]string{"c": "d"},
	})
	if err == nil {
		t.Fatal("expected a validation error before the request is sent")
	}
	if !strings.Contains(err.Error(), "replaceLabels") {
		t.Errorf("expected mutex validation error, got: %v", err)
	}
}
