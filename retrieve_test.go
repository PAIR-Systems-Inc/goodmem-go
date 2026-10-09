package goodmem

import (
	"strings"
	"testing"

	"github.com/PAIR-Systems-Inc/goodmem-go/models"
)

func f64(v float64) *float64 { return &v }
func i32p(v int32) *int32    { return &v }
func strp(v string) *string  { return &v }

func TestBuildRetrieveExpandsSpaceIDs(t *testing.T) {
	req, err := buildRetrieveRequest("q", &RetrieveOptions{SpaceIDs: []string{"a", "b"}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if req.Message != "q" {
		t.Errorf("message = %q", req.Message)
	}
	if len(req.SpaceKeys) != 2 || req.SpaceKeys[0].SpaceID != "a" || req.SpaceKeys[1].SpaceID != "b" {
		t.Errorf("space keys = %+v", req.SpaceKeys)
	}
}

func TestBuildRetrieveNoFlatKeysMeansNoPostProcessor(t *testing.T) {
	req, err := buildRetrieveRequest("q", &RetrieveOptions{SpaceIDs: []string{"a"}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if req.PostProcessor != nil {
		t.Errorf("no flat keys should produce no post-processor, got %+v", req.PostProcessor)
	}
}

func TestBuildRetrieveAssemblesChatPostProcessor(t *testing.T) {
	req, err := buildRetrieveRequest("q", &RetrieveOptions{
		SpaceIDs: []string{"a"},
		LLMID:    strp("llm-1"),
		LLMTemp:  f64(0.5),
		Prompt:   strp("hello"),
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	pp := req.PostProcessor
	if pp == nil {
		t.Fatal("expected a post-processor")
	}
	if pp.Name != chatPostProcessorFactory {
		t.Errorf("factory = %q", pp.Name)
	}
	// Config uses the server's canonical snake_case keys.
	if pp.Config["llm_id"] != "llm-1" || pp.Config["llm_temp"] != 0.5 || pp.Config["prompt"] != "hello" {
		t.Errorf("config = %+v", pp.Config)
	}
}

func TestBuildRetrieveOrphanLLMKeyRejected(t *testing.T) {
	// llm_temp without llm_id → orphan error.
	_, err := buildRetrieveRequest("q", &RetrieveOptions{LLMTemp: f64(0.5)})
	if err == nil {
		t.Fatal("llm_temp without llm_id should fail")
	}
	if !strings.Contains(err.Error(), "llm_id") {
		t.Errorf("error should name the activating key: %v", err)
	}
}

func TestBuildRetrieveOrphanRerankerKeyRejected(t *testing.T) {
	_, err := buildRetrieveRequest("q", &RetrieveOptions{RelevanceThreshold: f64(0.2)})
	if err == nil || !strings.Contains(err.Error(), "reranker_id") {
		t.Errorf("relevance_threshold without reranker_id should fail naming reranker_id: %v", err)
	}
}

func TestBuildRetrievePipelineKeyNeedsEitherActivator(t *testing.T) {
	// max_results with neither llm_id nor reranker_id → error.
	if _, err := buildRetrieveRequest("q", &RetrieveOptions{MaxResults: i32p(5)}); err == nil {
		t.Error("max_results with no activator should fail")
	}
	// max_results with reranker_id → ok.
	if _, err := buildRetrieveRequest("q", &RetrieveOptions{SpaceIDs: []string{"a"}, MaxResults: i32p(5), RerankerID: strp("r")}); err != nil {
		t.Errorf("max_results with reranker_id should pass: %v", err)
	}
}

func TestBuildRetrieveActivatingKeyAloneIsValid(t *testing.T) {
	req, err := buildRetrieveRequest("q", &RetrieveOptions{SpaceIDs: []string{"a"}, LLMID: strp("llm-1")})
	if err != nil {
		t.Fatalf("llm_id alone should be valid: %v", err)
	}
	if req.PostProcessor == nil || req.PostProcessor.Config["llm_id"] != "llm-1" {
		t.Errorf("post-processor = %+v", req.PostProcessor)
	}
}

// --- #1834: conversation context + per-space filters on the convenience layer ---

func TestBuildRetrieveForwardsSpaceKeysVerbatim(t *testing.T) {
	// Different filters for different spaces, including one deliberately unfiltered.
	req, err := buildRetrieveRequest("q", &RetrieveOptions{
		SpaceKeys: []models.SpaceKey{
			{SpaceID: "articles", Filter: strp("label.category=dotnet")},
			{SpaceID: "alice-history"},
		},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(req.SpaceKeys) != 2 {
		t.Fatalf("space keys = %+v", req.SpaceKeys)
	}
	if req.SpaceKeys[0].SpaceID != "articles" || req.SpaceKeys[0].Filter == nil ||
		*req.SpaceKeys[0].Filter != "label.category=dotnet" {
		t.Errorf("filtered space key reached the wire changed: %+v", req.SpaceKeys[0])
	}
	if req.SpaceKeys[1].SpaceID != "alice-history" || req.SpaceKeys[1].Filter != nil {
		t.Errorf("unfiltered space key should stay unfiltered: %+v", req.SpaceKeys[1])
	}
}

func TestBuildRetrieveForwardsContext(t *testing.T) {
	req, err := buildRetrieveRequest("q", &RetrieveOptions{
		SpaceIDs: []string{"a"},
		Context: []models.ContextItem{
			{Text: strp("User asked: what is my project name?")},
			{Text: strp("Assistant: Cedar-Quartz-731.")},
		},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(req.Context) != 2 {
		t.Fatalf("context = %+v", req.Context)
	}
	if req.Context[0].Text == nil || *req.Context[0].Text != "User asked: what is my project name?" {
		t.Errorf("context[0] reached the wire changed: %+v", req.Context[0])
	}
	if req.Context[1].Text == nil || *req.Context[1].Text != "Assistant: Cedar-Quartz-731." {
		t.Errorf("context[1] reached the wire changed: %+v", req.Context[1])
	}
}

func TestBuildRetrieveSpaceIDsAndSpaceKeysAreMutuallyExclusive(t *testing.T) {
	_, err := buildRetrieveRequest("q", &RetrieveOptions{
		SpaceIDs:  []string{"a"},
		SpaceKeys: []models.SpaceKey{{SpaceID: "b"}},
	})
	if err == nil {
		t.Fatal("setting both SpaceIDs and SpaceKeys should fail")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should explain the mutex: %v", err)
	}
}

func TestBuildRetrieveRequiresAtLeastOneSpace(t *testing.T) {
	// space_keys is required:true in the IR; failing locally beats a server 4xx.
	for name, opts := range map[string]*RetrieveOptions{
		"nil options":   nil,
		"empty options": {},
		"empty slices":  {SpaceIDs: []string{}, SpaceKeys: []models.SpaceKey{}},
	} {
		if _, err := buildRetrieveRequest("q", opts); err == nil {
			t.Errorf("%s: should require a space", name)
		}
	}
}

func TestBuildRetrieveKeepsTypedPostProcessorAlongsideContextAndFilters(t *testing.T) {
	// The #1834 regression: supplying context/filters must NOT cost the caller the
	// typed post-processor assembly.
	req, err := buildRetrieveRequest("q", &RetrieveOptions{
		SpaceKeys: []models.SpaceKey{
			{SpaceID: "articles", Filter: strp("label.category=dotnet")},
		},
		Context:        []models.ContextItem{{Text: strp("recent turn")}},
		LLMID:          strp("llm-1"),
		GenTokenBudget: i32p(512),
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if req.PostProcessor == nil {
		t.Fatal("post-processor should still be assembled")
	}
	if req.PostProcessor.Config["llm_id"] != "llm-1" {
		t.Errorf("llm_id = %v", req.PostProcessor.Config["llm_id"])
	}
	if req.PostProcessor.Config["gen_token_budget"] != int32(512) {
		t.Errorf("gen_token_budget = %#v", req.PostProcessor.Config["gen_token_budget"])
	}
	if len(req.Context) != 1 || req.SpaceKeys[0].Filter == nil {
		t.Errorf("context/filter lost while assembling the post-processor: %+v", req)
	}
}

func TestBuildRetrieveOrphanDiagnosticWinsOverSpaceError(t *testing.T) {
	// Wrong in both ways: the more specific orphan message is the useful one.
	_, err := buildRetrieveRequest("q", &RetrieveOptions{LLMTemp: f64(0.5)})
	if err == nil || !strings.Contains(err.Error(), "llm_id") {
		t.Errorf("orphan diagnostic should win: %v", err)
	}
}

func TestBuildRetrieveSharedArticleSpacePlusUserHistory(t *testing.T) {
	// Canonical composition from #1838: a shared article space plus one conversation
	// space per user, with recent turns supplied separately as context.
	req, err := buildRetrieveRequest("what did I name my project?", &RetrieveOptions{
		SpaceKeys: []models.SpaceKey{
			{SpaceID: "shared-articles"},
			{SpaceID: "conv-alice"},
		},
		Context: []models.ContextItem{{Text: strp("User asked: remind me?")}},
		LLMID:   strp("llm-1"),
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(req.SpaceKeys) != 2 || req.SpaceKeys[0].SpaceID != "shared-articles" ||
		req.SpaceKeys[1].SpaceID != "conv-alice" {
		t.Errorf("both spaces should reach the wire in order: %+v", req.SpaceKeys)
	}
	if len(req.Context) != 1 {
		t.Errorf("recent turns should ride along as context: %+v", req.Context)
	}
}
