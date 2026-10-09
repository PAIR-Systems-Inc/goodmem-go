package goodmem

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"sync"
)

// streamAccept is the wire content type the SDK negotiates for streaming
// endpoints (memories.retrieve, ping.stream). NDJSON: one JSON event per line.
const streamAccept = "application/x-ndjson"

// defaultStreamMaxLineBytes caps a single NDJSON event (bufio's own default is
// 64 KiB, far too small — one retrievedItem carries a whole memory's content
// when FetchMemoryContent is set). Override per-client with
// WithStreamMaxLineBytes.
const defaultStreamMaxLineBytes = 8 * 1024 * 1024 // 8 MiB

// Stream is a lazily-consumed NDJSON event stream. Iterate it with Events and
// always Close it (the underlying HTTP response holds a connection):
//
//	stream, err := client.Ping().Stream(ctx, req)
//	if err != nil { ... }
//	defer stream.Close()
//	for ev, err := range stream.Events() {
//	    if err != nil { ... }
//	    // handle ev
//	}
type Stream[T any] struct {
	resp         *http.Response
	scanner      *bufio.Scanner
	maxLineBytes int
	closeOnce    sync.Once
	closeErr     error
}

func newStream[T any](c *Client, resp *http.Response) *Stream[T] {
	maxLine := c.streamMaxLineBytes
	if maxLine <= 0 {
		maxLine = defaultStreamMaxLineBytes
	}
	// bufio.Scanner's effective max token size is max(maxLine, cap(initialBuf)),
	// so the initial buffer must not exceed maxLine — otherwise a caller-lowered
	// cap would be silently overridden. Start at 64 KiB (or maxLine if smaller)
	// and let it grow up to maxLine.
	initial := 64 * 1024
	if maxLine < initial {
		initial = maxLine
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, initial), maxLine)
	return &Stream[T]{resp: resp, scanner: sc, maxLineBytes: maxLine}
}

// Close releases the underlying HTTP response. Safe to call multiple times: the
// body is closed exactly once (Events() also closes on iteration end, and the
// documented usage adds a defer Close()), so concurrent/duplicate calls are a
// no-op after the first.
func (s *Stream[T]) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.resp.Body.Close() })
	return s.closeErr
}

// Events returns a range-over-func iterator over each NDJSON event.
//
// All-null oneOf frames — the server's *_NOT_SET sentinel, serialized as `{}`
// or `{"retrievedItem":{}}` — are skipped silently so a single empty frame
// doesn't tear down the stream (inv-stream-tolerates-null-oneof-frames). A
// non-empty line that fails to parse surfaces as a *GoodmemError; a transport
// read failure surfaces as a *NetworkError. The response is closed when
// iteration ends.
func (s *Stream[T]) Events() iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		defer s.Close()
		for s.scanner.Scan() {
			line := bytes.TrimSpace(s.scanner.Bytes())
			if len(line) == 0 || isAllNullFrame(line) {
				continue
			}
			var ev T
			if err := json.Unmarshal(line, &ev); err != nil {
				var zero T
				yield(zero, &GoodmemError{
					Message: "malformed stream event: " + truncate(string(line), 200),
					Cause:   err,
				})
				return
			}
			if !yield(ev, nil) {
				return
			}
		}
		if err := s.scanner.Err(); err != nil {
			var zero T
			if errors.Is(err, bufio.ErrTooLong) {
				// A single event exceeded the line cap — an SDK-side limit, not a
				// transport failure — so name the cap and the two ways out
				// instead of surfacing an opaque "token too long".
				yield(zero, &GoodmemError{
					Message: fmt.Sprintf(
						"stream event exceeded the %d-byte per-event line cap; raise it with "+
							"goodmem.WithStreamMaxLineBytes, or avoid inlining large bodies by leaving "+
							"Retrieve's FetchMemoryContent unset and fetching content via Memories().Content",
						s.maxLineBytes),
					Cause: err,
				})
				return
			}
			if !yield(zero, &NetworkError{Message: "stream read failed: " + err.Error(), Cause: err}) {
				return
			}
		}
	}
}

// isAllNullFrame reports whether a top-level JSON object has only null or
// empty-object values — the server's representation of a oneOf *_NOT_SET case.
func isAllNullFrame(line []byte) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(line, &m); err != nil {
		return false // not a JSON object — let the real Unmarshal report it
	}
	if len(m) == 0 {
		return true
	}
	for _, v := range m {
		switch string(bytes.TrimSpace(v)) {
		case "null", "{}":
			continue
		default:
			return false
		}
	}
	return true
}
