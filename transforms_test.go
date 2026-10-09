// White-box tests for the convenience transforms + bundled registries.
package goodmem

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/PAIR-Systems-Inc/goodmem-go/models"
)

// decodeStrict fails if a registry entry carries a key the Go entry struct does
// not model — i.e. a registry field the convenience transform would silently
// drop. This is the Go side of the cross-language registry-parity guard
// (inv-convenience-transforms-behavior-parity).
func decodeStrict[T any](t *testing.T, msg json.RawMessage, label string) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(msg))
	dec.DisallowUnknownFields()
	var v T
	if err := dec.Decode(&v); err != nil {
		t.Errorf("%s: registry key not modeled by the Go transform (parity gap): %v", label, err)
	}
}

func TestRegistryParityNoUnknownKeys(t *testing.T) {
	walk := func(raw []byte, fn func(name string, msg json.RawMessage)) {
		var entries map[string]json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			t.Fatalf("registry parse: %v", err)
		}
		if len(entries) == 0 {
			t.Fatal("registry is empty")
		}
		for name, msg := range entries {
			fn(name, msg)
		}
	}
	walk(embeddersRegistryJSON, func(n string, m json.RawMessage) { decodeStrict[embedderRegistryEntry](t, m, "embedders/"+n) })
	walk(llmsRegistryJSON, func(n string, m json.RawMessage) { decodeStrict[llmRegistryEntry](t, m, "llms/"+n) })
	walk(rerankersRegistryJSON, func(n string, m json.RawMessage) { decodeStrict[rerankerRegistryEntry](t, m, "rerankers/"+n) })
	walk(providersRegistryJSON, func(n string, m json.RawMessage) { decodeStrict[providerEntry](t, m, "providers/"+n) })
}

func TestRegistryLookupFillsEmbedderFields(t *testing.T) {
	// text-embedding-3-small: OPENAI, dims default 1536, [TEXT], maxSeqLen 8192.
	req := &models.EmbedderCreationRequest{ModelIdentifier: "text-embedding-3-small"}
	if err := applyConvenienceEmbeddersCreate(req, "sk-test"); err != nil {
		t.Fatalf("transform: %v", err)
	}
	if req.ProviderType != models.ProviderTypeOPENAI {
		t.Errorf("providerType = %q, want OPENAI", req.ProviderType)
	}
	if req.Dimensionality != 1536 {
		t.Errorf("dimensionality = %d, want 1536 (from registry dimensions.default)", req.Dimensionality)
	}
	if len(req.SupportedModalities) == 0 || req.SupportedModalities[0] != models.ModalityTEXT {
		t.Errorf("supportedModalities = %v, want [TEXT]", req.SupportedModalities)
	}
	if req.MaxSequenceLength == nil || *req.MaxSequenceLength != 8192 {
		t.Errorf("maxSequenceLength = %v, want 8192", req.MaxSequenceLength)
	}
	if req.DistributionType != models.DistributionTypeDENSE {
		t.Errorf("distributionType = %q, want DENSE (inject_default)", req.DistributionType)
	}
}

func TestRegistryLookupDoesNotAliasRegistry(t *testing.T) {
	// The registry is a process-global; a value it supplies must be copied, not
	// aliased, so mutating it on one request can't corrupt later lookups.
	req1 := &models.EmbedderCreationRequest{ModelIdentifier: "text-embedding-3-small"}
	if err := applyConvenienceEmbeddersCreate(req1, "sk-test"); err != nil {
		t.Fatalf("transform: %v", err)
	}
	if req1.MaxSequenceLength == nil {
		t.Fatal("expected maxSequenceLength from registry")
	}
	*req1.MaxSequenceLength = 1 // mutate through the returned pointer

	req2 := &models.EmbedderCreationRequest{ModelIdentifier: "text-embedding-3-small"}
	if err := applyConvenienceEmbeddersCreate(req2, "sk-test"); err != nil {
		t.Fatalf("transform: %v", err)
	}
	if req2.MaxSequenceLength == nil || *req2.MaxSequenceLength != 8192 {
		t.Errorf("registry corrupted: maxSequenceLength = %v, want 8192", req2.MaxSequenceLength)
	}
}

func TestApiKeyToCredentials(t *testing.T) {
	req := &models.EmbedderCreationRequest{ModelIdentifier: "text-embedding-3-small"}
	if err := applyConvenienceEmbeddersCreate(req, "sk-secret"); err != nil {
		t.Fatalf("transform: %v", err)
	}
	if req.Credentials == nil || req.Credentials.APIKey == nil || req.Credentials.APIKey.InlineSecret == nil {
		t.Fatalf("apiKey was not wrapped into credentials: %+v", req.Credentials)
	}
	if *req.Credentials.APIKey.InlineSecret != "sk-secret" {
		t.Errorf("inlineSecret = %q", *req.Credentials.APIKey.InlineSecret)
	}
	if req.Credentials.Kind != models.CredentialKindAPIKEY {
		t.Errorf("credential kind = %q", req.Credentials.Kind)
	}
}

func TestApiKeyAndCredentialsConflict(t *testing.T) {
	req := &models.EmbedderCreationRequest{
		ModelIdentifier: "text-embedding-3-small",
		Credentials:     &models.EndpointAuthentication{Kind: models.CredentialKindAPIKEY},
	}
	if err := applyConvenienceEmbeddersCreate(req, "sk-secret"); err == nil {
		t.Error("specifying both apiKey and credentials should error")
	}
}

func TestEndpointInference(t *testing.T) {
	req := &models.EmbedderCreationRequest{ModelIdentifier: "text-embedding-3-small"}
	if err := applyConvenienceEmbeddersCreate(req, "sk-test"); err != nil {
		t.Fatalf("transform: %v", err)
	}
	if req.EndpointURL == "" || !strings.Contains(req.EndpointURL, "openai.com") {
		t.Errorf("endpointURL = %q, want inferred OpenAI URL", req.EndpointURL)
	}
}

func TestCredentialCheckRequiresKeyForSaaS(t *testing.T) {
	// OpenAI endpoint, no api key → should fail credential_check.
	req := &models.EmbedderCreationRequest{
		ProviderType: models.ProviderTypeOPENAI,
		EndpointURL:  "https://api.openai.com/v1",
	}
	if err := applyConvenienceEmbeddersCreate(req, ""); err == nil {
		t.Error("SaaS endpoint with no key should fail credential_check")
	}
	// Hostname match must ignore a trailing slash / path variant.
	req2 := &models.EmbedderCreationRequest{
		ProviderType: models.ProviderTypeOPENAI,
		EndpointURL:  "https://api.openai.com/v1/",
	}
	if err := applyConvenienceEmbeddersCreate(req2, ""); err == nil {
		t.Error("credential_check must match on hostname, not exact URL")
	}
	// With a key → credentials set → check passes.
	req3 := &models.EmbedderCreationRequest{
		ProviderType: models.ProviderTypeOPENAI,
		EndpointURL:  "https://api.openai.com/v1",
	}
	if err := applyConvenienceEmbeddersCreate(req3, "sk-test"); err != nil {
		t.Errorf("with api key the check should pass: %v", err)
	}
}

func TestUserSuppliedValueWins(t *testing.T) {
	// Pre-set provider must not be overwritten by the registry lookup.
	req := &models.EmbedderCreationRequest{
		ModelIdentifier: "text-embedding-3-small", // registry says OPENAI
		ProviderType:    models.ProviderTypeVOYAGE,
		Dimensionality:  256,
	}
	if err := applyConvenienceEmbeddersCreate(req, "sk-test"); err != nil {
		t.Fatalf("transform: %v", err)
	}
	if req.ProviderType != models.ProviderTypeVOYAGE {
		t.Errorf("user providerType was overwritten: %q", req.ProviderType)
	}
	if req.Dimensionality != 256 {
		t.Errorf("user dimensionality was overwritten: %d", req.Dimensionality)
	}
}

func TestLlmAndRerankerRegistryLookup(t *testing.T) {
	llm := &models.LLMCreationRequest{ModelIdentifier: "gpt-5"}
	if err := applyConvenienceLLMsCreate(llm, "sk-test"); err != nil {
		t.Fatalf("llm transform: %v", err)
	}
	if string(llm.ProviderType) != "OPENAI" {
		t.Errorf("llm providerType = %q", llm.ProviderType)
	}
	if llm.MaxContextLength == nil {
		t.Error("llm maxContextLength should be filled from registry")
	} else {
		// The filled *int32 must be a COPY of the registry pointee, never the
		// registry's cached pointer — else mutating it corrupts the process-global
		// registry for every later call. Mutate, re-fill fresh, assert unchanged.
		orig := *llm.MaxContextLength
		*llm.MaxContextLength = orig + 1
		llm2 := &models.LLMCreationRequest{ModelIdentifier: "gpt-5"}
		if err := applyConvenienceLLMsCreate(llm2, "sk-test"); err != nil {
			t.Fatalf("llm transform (2): %v", err)
		}
		if llm2.MaxContextLength == nil || *llm2.MaxContextLength != orig {
			t.Errorf("llm maxContextLength aliased the registry: got %v, want %d", llm2.MaxContextLength, orig)
		}
	}

	rr := &models.RerankerCreationRequest{ModelIdentifier: "rerank-v4.0-pro"}
	if err := applyConvenienceRerankersCreate(rr, "sk-test"); err != nil {
		t.Fatalf("reranker transform: %v", err)
	}
	if rr.ProviderType == "" {
		t.Error("reranker providerType should be filled from registry")
	}
	if len(rr.SupportedModalities) == 0 {
		t.Error("reranker supportedModalities should be filled from registry")
	}
}

func TestSpacesInjectDefaultChunking(t *testing.T) {
	req := &models.SpaceCreationRequest{Name: "s"}
	if err := applyConvenienceSpacesCreate(req); err != nil {
		t.Fatalf("transform: %v", err)
	}
	if req.DefaultChunkingConfig.Recursive == nil {
		t.Fatal("default chunking config should be injected")
	}
	// Assert every default field, not just ChunkSize — a typo in convenience.json's
	// defaults.chunking_config would otherwise slip through.
	rc := req.DefaultChunkingConfig.Recursive
	if rc.ChunkSize != 512 || rc.ChunkOverlap != 64 ||
		rc.KeepStrategy != models.SeparatorKeepStrategyKEEPEND ||
		rc.LengthMeasurement != models.LengthMeasurementCHARACTERCOUNT {
		t.Errorf("default chunking values wrong: size=%d overlap=%d keep=%q len=%q",
			rc.ChunkSize, rc.ChunkOverlap, rc.KeepStrategy, rc.LengthMeasurement)
	}
	// A user-supplied config must be preserved — for every variant slot.
	for _, custom := range []*models.SpaceCreationRequest{
		{Name: "s", DefaultChunkingConfig: models.ChunkingConfiguration{None: &models.NoChunkingConfiguration{}}},
		{Name: "s", DefaultChunkingConfig: models.ChunkingConfiguration{Sentence: &models.SentenceChunkingConfiguration{}}},
	} {
		if err := applyConvenienceSpacesCreate(custom); err != nil {
			t.Fatalf("transform: %v", err)
		}
		if custom.DefaultChunkingConfig.Recursive != nil {
			t.Error("user-supplied chunking config was overwritten with the recursive default")
		}
	}
}

// Content-type inference for memory create (the Go analog of Python's _overrides):
// plain-text originalContent defaults to text/plain; base64 content with no explicit
// contentType is rejected client-side. inv-content-type-inference.
func TestMemoryContentTypeInference(t *testing.T) {
	sp := func(s string) *string { return &s }

	t.Run("plain text content defaults to text/plain", func(t *testing.T) {
		req := &models.JSONMemoryCreationRequest{SpaceID: "s", OriginalContent: sp("hello")}
		if err := applyConvenienceMemoriesCreate(req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.ContentType != "text/plain" {
			t.Errorf("ContentType = %q, want text/plain", req.ContentType)
		}
	})

	t.Run("explicit content type is preserved", func(t *testing.T) {
		req := &models.JSONMemoryCreationRequest{SpaceID: "s", OriginalContent: sp("hi"), ContentType: "text/markdown"}
		if err := applyConvenienceMemoriesCreate(req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.ContentType != "text/markdown" {
			t.Errorf("ContentType = %q, want text/markdown (caller value must win)", req.ContentType)
		}
	})

	t.Run("base64 content without content type is rejected", func(t *testing.T) {
		req := &models.JSONMemoryCreationRequest{SpaceID: "s", OriginalContentB64: sp("ZGF0YQ==")}
		err := applyConvenienceMemoriesCreate(req)
		if err == nil {
			t.Fatal("expected an error for base64 content with no contentType")
		}
		if !strings.Contains(err.Error(), "contentType is required") {
			t.Errorf("error = %q, want it to mention contentType is required", err)
		}
	})

	t.Run("base64 content with content type is accepted", func(t *testing.T) {
		req := &models.JSONMemoryCreationRequest{SpaceID: "s", OriginalContentB64: sp("ZGF0YQ=="), ContentType: "application/pdf"}
		if err := applyConvenienceMemoriesCreate(req); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.ContentType != "application/pdf" {
			t.Errorf("ContentType = %q, want application/pdf", req.ContentType)
		}
	})
}

func TestMemoryBatchContentTypeInference(t *testing.T) {
	sp := func(s string) *string { return &s }

	// Per-item inference: item 0 (text) gets text/plain; item 1 keeps its explicit type.
	orig := []models.JSONMemoryCreationRequest{
		{SpaceID: "s", OriginalContent: sp("a")},
		{SpaceID: "s", OriginalContent: sp("b"), ContentType: "text/markdown"},
	}
	req := &models.JSONBatchMemoryCreationRequest{Requests: orig}
	if err := applyConvenienceMemoriesBatchCreate(req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Requests[0].ContentType != "text/plain" {
		t.Errorf("requests[0].ContentType = %q, want text/plain", req.Requests[0].ContentType)
	}
	if req.Requests[1].ContentType != "text/markdown" {
		t.Errorf("requests[1].ContentType = %q, want text/markdown", req.Requests[1].ContentType)
	}
	// Caller isolation: the batch convenience clones req.Requests before inferring,
	// so the caller's ORIGINAL slice element must be untouched (no text/plain leaked in).
	if orig[0].ContentType != "" {
		t.Errorf("caller's original requests[0] was mutated: ContentType=%q, want empty", orig[0].ContentType)
	}

	// A bad item surfaces its index in the error.
	bad := &models.JSONBatchMemoryCreationRequest{Requests: []models.JSONMemoryCreationRequest{
		{SpaceID: "s", OriginalContent: sp("ok")},
		{SpaceID: "s", OriginalContentB64: sp("ZGF0YQ==")},
	}}
	err := applyConvenienceMemoriesBatchCreate(bad)
	if err == nil || !strings.Contains(err.Error(), "requests[1]") {
		t.Errorf("error = %v, want it to reference requests[1]", err)
	}
}
