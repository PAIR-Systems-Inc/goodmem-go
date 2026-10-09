package goodmem_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	goodmem "github.com/PAIR-Systems-Inc/goodmem-go"
	"github.com/PAIR-Systems-Inc/goodmem-go/models"
)

func TestRetrievalUnknownStatusPreservesStream(t *testing.T) {
	fixture, err := os.ReadFile("testdata/unknown-retrieval-status.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	for _, malformed := range []bool{false, true} {
		name := "unknown status"
		if malformed {
			name = "corrupted JSON"
		}
		t.Run(name, func(t *testing.T) {
			lines := strings.Split(strings.TrimSpace(string(fixture)), "\n")
			if malformed {
				lines[1] = "{malformed-json"
			}
			client, _ := newServerClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/v1/memories:retrieve" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/x-ndjson")
				_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
			})
			stream, err := client.Memories().Retrieve(context.Background(), "query", &goodmem.RetrieveOptions{SpaceIDs: []string{"space-1"}})
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			var events []models.RetrieveMemoryEvent
			var streamErr error
			for event, err := range stream.Events() {
				if err != nil {
					streamErr = err
					break
				}
				events = append(events, event)
			}
			if malformed {
				var parseErr *goodmem.GoodmemError
				if !errors.As(streamErr, &parseErr) || len(events) != 1 {
					t.Fatalf("corruption must fail after the first event: events=%d, err=%v", len(events), streamErr)
				}
				return
			}
			if streamErr != nil || len(events) != 3 {
				t.Fatalf("unfamiliar status truncated retrieval: events=%d, err=%v", len(events), streamErr)
			}
			before, after := events[0].RetrievedItem.Chunk, events[2].RetrievedItem.Chunk
			if before.Chunk.ChunkText != "Before the unfamiliar status" || before.RelevanceScore != 0.75 ||
				after.Chunk.ChunkText != "After the unfamiliar status" || after.RelevanceScore != 0.5 {
				t.Fatalf("retrieved content changed: before=%+v, after=%+v", before, after)
			}
			status := events[1].Status
			if status.Code != "TEST_ONLY_FUTURE_RETRIEVAL_STATUS_9E4AD2" ||
				status.Message != "Future server notice" || status.Details["feature"] != "future-feature" {
				t.Fatalf("unfamiliar status was not preserved: %+v", status)
			}
		})
	}
}
