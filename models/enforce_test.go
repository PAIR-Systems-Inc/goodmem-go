// Hand-written tests for the generated model-layer enforcement: oneOf strict,
// mutex groups, required-companion fields, nested recursion, secret redaction,
// and the UnmarshalJSON validation path.
package models_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PAIR-Systems-Inc/goodmem-go/models"
)

func sp(s string) *string { return &s }

func TestOneOfStrictExactlyOne(t *testing.T) {
	// zero set → error
	if err := (&models.ChunkingConfiguration{}).Validate(); err == nil {
		t.Error("zero variants set should fail oneOf strict")
	}
	// two set → error
	two := &models.ChunkingConfiguration{
		None:      &models.NoChunkingConfiguration{},
		Recursive: &models.RecursiveChunkingConfiguration{ChunkSize: 512},
	}
	if err := two.Validate(); err == nil {
		t.Error("two variants set should fail oneOf strict")
	}
	// exactly one → ok
	one := &models.ChunkingConfiguration{Recursive: &models.RecursiveChunkingConfiguration{ChunkSize: 512}}
	if err := one.Validate(); err != nil {
		t.Errorf("one variant should pass: %v", err)
	}
}

func TestOneOfStrictOnUnmarshal(t *testing.T) {
	// All-null oneOf frame must be rejected on the deserialization path.
	var cfg models.ChunkingConfiguration
	if err := json.Unmarshal([]byte(`{}`), &cfg); err == nil {
		t.Error("UnmarshalJSON of an all-null oneOf should fail Validate")
	}
	if err := json.Unmarshal([]byte(`{"recursive":{"chunkSize":512}}`), &cfg); err != nil {
		t.Errorf("UnmarshalJSON of a valid oneOf should pass: %v", err)
	}
}

func TestMutexRequiredExactlyOne(t *testing.T) {
	// APIKeyAuth: required=true → exactly one of inlineSecret/secretRef.
	if err := (&models.APIKeyAuth{}).Validate(); err == nil {
		t.Error("required mutex with zero set should fail")
	}
	both := &models.APIKeyAuth{InlineSecret: sp("x"), SecretRef: &models.SecretReference{}}
	if err := both.Validate(); err == nil {
		t.Error("required mutex with both set should fail")
	}
	if err := (&models.APIKeyAuth{InlineSecret: sp("x")}).Validate(); err != nil {
		t.Errorf("required mutex with one set should pass: %v", err)
	}
}

func TestMutexAtMostOne(t *testing.T) {
	// UpdateSpaceRequest: required=false → at most one of replaceLabels/mergeLabels.
	if err := (&models.UpdateSpaceRequest{}).Validate(); err != nil {
		t.Errorf("at-most-one mutex with zero set should pass: %v", err)
	}
	both := &models.UpdateSpaceRequest{
		ReplaceLabels: map[string]string{"a": "b"},
		MergeLabels:   map[string]string{"c": "d"},
	}
	if err := both.Validate(); err == nil {
		t.Error("at-most-one mutex with both set should fail")
	}
}

func TestRequiredCompanionField(t *testing.T) {
	// originalContentB64 set without contentType → error.
	bad := &models.JSONMemoryCreationRequest{
		SpaceID:            "s1",
		OriginalContentB64: sp("aGVsbG8="),
	}
	err := bad.Validate()
	if err == nil {
		t.Fatal("originalContentB64 without contentType should fail companion check")
	}
	if !strings.Contains(err.Error(), "MIME") {
		t.Errorf("companion error should explain the reason: %v", err)
	}
	// With contentType → passes.
	good := &models.JSONMemoryCreationRequest{
		SpaceID:            "s1",
		OriginalContentB64: sp("aGVsbG8="),
		ContentType:        "application/pdf",
	}
	if err := good.Validate(); err != nil {
		t.Errorf("companion satisfied should pass: %v", err)
	}
}

func TestNestedValidationRecurses(t *testing.T) {
	// Valid top-level fields but an invalid nested oneOf → parent.Validate fails.
	req := &models.JSONMemoryCreationRequest{
		SpaceID:         "s1",
		OriginalContent: sp("hello"),
		ContentType:     "text/plain",
		ChunkingConfig:  &models.ChunkingConfiguration{}, // zero variants → invalid
	}
	err := req.Validate()
	if err == nil {
		t.Fatal("invalid nested ChunkingConfiguration should fail parent Validate")
	}
	if !strings.Contains(err.Error(), "ChunkingConfiguration") {
		t.Errorf("error should surface the nested failure: %v", err)
	}
}

func TestRangeGuardOnRequestConstruction(t *testing.T) {
	// dimensionality has minimum=1; 0 must fail Validate.
	bad := &models.EmbedderCreationRequest{ModelIdentifier: "m", Dimensionality: 0}
	if err := bad.Validate(); err == nil {
		t.Error("dimensionality 0 should fail the >= 1 range guard")
	}
	good := &models.EmbedderCreationRequest{ModelIdentifier: "m", Dimensionality: 1536}
	if err := good.Validate(); err != nil {
		t.Errorf("valid dimensionality should pass: %v", err)
	}
}

func TestRequestFieldLocalEnumSubset(t *testing.T) {
	bad := &models.RerankerCreationRequest{ProviderType: models.ProviderTypeGEMINI}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "providerType") {
		t.Fatalf("GEMINI must be rejected for a reranker request, got: %v", err)
	}
	good := &models.RerankerCreationRequest{ProviderType: models.ProviderTypeVLLM}
	if err := good.Validate(); err != nil {
		t.Fatalf("VLLM must remain valid for a reranker request: %v", err)
	}
}

func TestNestedRequestFieldLocalEnumSubset(t *testing.T) {
	badStatus := "FUTURE_STATUS"
	bad := &models.BatchMemoryDeletionRequest{Requests: []models.BatchDeleteMemorySelectorRequest{{
		FilterSelector: &models.FilteredDeleteMemorySelectorRequest{
			SpaceID:      "00000000-0000-0000-0000-000000000001",
			StatusFilter: &badStatus,
		},
	}}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "statusFilter") {
		t.Fatalf("nested request enum violation must reach the outer validator, got: %v", err)
	}

	completed := "COMPLETED"
	good := &models.BatchMemoryDeletionRequest{Requests: []models.BatchDeleteMemorySelectorRequest{{
		FilterSelector: &models.FilteredDeleteMemorySelectorRequest{
			SpaceID:      "00000000-0000-0000-0000-000000000001",
			StatusFilter: &completed,
		},
	}}}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid nested request enum must remain accepted: %v", err)
	}
}

// Range guards are request-side only: a response model with a zero in a
// minimum>=1 field must still deserialize (no UnmarshalJSON range check).
func TestResponseModelNotRangeValidatedOnUnmarshal(t *testing.T) {
	var resp models.EmbedderResponse
	if err := json.Unmarshal([]byte(`{"dimensionality":0}`), &resp); err != nil {
		t.Errorf("response deserialization must not enforce request-side ranges: %v", err)
	}
}

func TestSecretRedactionInString(t *testing.T) {
	resp := models.CreateAPIKeyResponse{RawAPIKey: sp("gm_supersecret")}
	s := resp.String()
	if strings.Contains(s, "gm_supersecret") {
		t.Errorf("String() leaked the secret: %s", s)
	}
	if !strings.Contains(s, "[REDACTED]") {
		t.Errorf("String() should mark the redaction: %s", s)
	}

	auth := models.APIKeyAuth{InlineSecret: sp("topsecret")}
	if strings.Contains(auth.String(), "topsecret") {
		t.Errorf("APIKeyAuth.String() leaked inlineSecret: %s", auth.String())
	}
}
