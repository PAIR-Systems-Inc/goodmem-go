package goodmem

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// redirectError reports a non-followed 3xx response as a clear SDK error. The
// SDK refuses to follow redirects so a stored x-api-key can never leak to
// another origin (inv-no-redirect-with-custom-auth-header); when the server
// answers with a 3xx anyway we surface it explicitly instead of letting an
// empty/HTML redirect body fall through to a confusing JSON parse failure.
func redirectError(resp *http.Response) error {
	msg := fmt.Sprintf("server returned HTTP %d redirect", resp.StatusCode)
	if loc := resp.Header.Get("Location"); loc != "" {
		msg += " to " + loc
	}
	msg += "; the SDK does not follow redirects (a stored API key must not leak across hosts)"
	return &GoodmemError{Message: msg}
}

// pathEscape escapes a value for safe inclusion in a URL path segment. UUIDs
// and numeric ids pass through unchanged; values that can carry reserved
// characters (e.g. an email path segment) are percent-encoded so the request
// reaches the intended endpoint (inv-url-encode-path-segments).
func pathEscape(s string) string { return url.PathEscape(s) }

func (c *Client) buildURL(path string, query url.Values) string {
	full := c.baseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	return full
}

func (c *Client) newRequest(ctx context.Context, method, path string, query url.Values, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.buildURL(path, query), body)
	if err != nil {
		return nil, &GoodmemError{Message: "failed to build request: " + err.Error(), Cause: err}
	}
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// doJSON executes a request and, when out is non-nil, decodes the JSON response
// body into it. body is marshaled to JSON when non-nil (pass an untyped nil to
// send no body — DELETE/POST endpoints whose requestBody is optional). out may
// be nil for endpoints with no response payload.
func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var reader io.Reader
	hasBody := body != nil
	if hasBody {
		buf, err := json.Marshal(body)
		if err != nil {
			return &GoodmemError{Message: "failed to serialize request body: " + err.Error(), Cause: err}
		}
		reader = bytes.NewReader(buf)
	}
	req, err := c.newRequest(ctx, method, path, query, reader)
	if err != nil {
		return err
	}
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &NetworkError{Message: "HTTP request failed: " + err.Error(), Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return redirectError(resp)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return &NetworkError{Message: "failed to read response body: " + err.Error(), Cause: err}
	}

	if resp.StatusCode >= 400 {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return mapError(resp.StatusCode, string(data), retryAfter)
	}

	if out == nil || resp.StatusCode == http.StatusNoContent || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		// A parse failure is NOT a transport error — the round-trip
		// succeeded, the body just didn't match the schema. Surface it as the
		// generic SDK error so a `catch NetworkError` doesn't absorb a parse
		// bug (inv-exception-types-mapping).
		return &GoodmemError{Message: "malformed response body: " + truncate(string(data), 200), Cause: err}
	}
	return nil
}

// openStream POSTs a body and returns the live response for NDJSON streaming.
// The caller owns the response and must close it (via the Stream wrapper).
// A 4xx/5xx is read, closed, and mapped to a typed error before returning.
func (c *Client) openStream(ctx context.Context, method, path string, body any, accept string) (*http.Response, error) {
	var reader io.Reader
	hasBody := body != nil
	if hasBody {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, &GoodmemError{Message: "failed to serialize request body: " + err.Error(), Cause: err}
		}
		reader = bytes.NewReader(buf)
	}
	req, err := c.newRequest(ctx, method, path, nil, reader)
	if err != nil {
		return nil, err
	}
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &NetworkError{Message: "HTTP request failed: " + err.Error(), Cause: err}
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		resp.Body.Close()
		return nil, redirectError(resp)
	}
	if resp.StatusCode >= 400 {
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, &NetworkError{Message: "failed to read error response body: " + readErr.Error(), Cause: readErr}
		}
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, mapError(resp.StatusCode, string(data), retryAfter)
	}
	return resp, nil
}

// doBytes executes a request and returns the raw response body — used by
// binary-returning endpoints (memories.content, memories.pages_image).
func (c *Client) doBytes(ctx context.Context, method, path string, query url.Values) ([]byte, error) {
	req, err := c.newRequest(ctx, method, path, query, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &NetworkError{Message: "HTTP request failed: " + err.Error(), Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, redirectError(resp)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &NetworkError{Message: "failed to read response body: " + err.Error(), Cause: err}
	}
	if resp.StatusCode >= 400 {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, mapError(resp.StatusCode, string(data), retryAfter)
	}
	return data, nil
}
