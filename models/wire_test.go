// Hand-written tests for the generated wire models. Not generated — exercises
// the JSON contract the emitter produces (camelCase aliases, omitempty on
// optional fields, typed-string enum wire values, oneOf variant slots, base64
// bytes, format-driven int widths).
package models_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/PAIR-Systems-Inc/goodmem-go/models"
)

// Required, non-nullable fields are always present on the wire under their
// camelCase alias; they carry no omitempty.
func TestRequiredFieldsUseCamelCaseAliasAndAlwaysEmit(t *testing.T) {
	s := models.Space{} // all zero values
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	for _, key := range []string{"spaceId", "name", "labels", "spaceEmbedders", "createdAt", "ownerId"} {
		if !strings.Contains(got, `"`+key+`"`) {
			t.Errorf("required field %q missing from wire output: %s", key, got)
		}
	}
	// snake_case names must never leak onto the wire.
	for _, bad := range []string{"space_id", "owner_id"} {
		if strings.Contains(got, bad) {
			t.Errorf("snake_case key %q leaked onto the wire: %s", bad, got)
		}
	}
}

// Optional (pointer) fields are omitted when nil and present when set.
func TestOptionalPointerFieldsOmitWhenNil(t *testing.T) {
	req := models.EmbedderCreationRequest{
		DisplayName:     "e",
		ProviderType:    models.ProviderTypeOPENAI,
		EndpointURL:     "https://api.openai.com/v1",
		ModelIdentifier: "text-embedding-3-small",
		Dimensionality:  1536,
	}
	b, _ := json.Marshal(req)
	if strings.Contains(string(b), "description") {
		t.Errorf("nil optional Description should be omitted: %s", b)
	}

	desc := "a description"
	req.Description = &desc
	b, _ = json.Marshal(req)
	if !strings.Contains(string(b), `"description":"a description"`) {
		t.Errorf("set optional Description should be present: %s", b)
	}
}

// Enum consts carry the exact wire token, and marshal as that string.
func TestEnumWireValues(t *testing.T) {
	cases := map[string]string{
		string(models.ProviderTypeOPENAI):   "OPENAI",
		string(models.ProviderTypeLLAMACPP): "LLAMA_CPP",
		string(models.CredentialKindAPIKEY): "CREDENTIAL_KIND_API_KEY",
		string(models.CredentialKindGCPADC): "CREDENTIAL_KIND_GCP_ADC",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("enum const wire value = %q, want %q", got, want)
		}
	}

	req := models.EmbedderCreationRequest{ProviderType: models.ProviderTypeVOYAGE}
	b, _ := json.Marshal(req)
	if !strings.Contains(string(b), `"providerType":"VOYAGE"`) {
		t.Errorf("enum field should marshal to its wire token: %s", b)
	}
}

// A oneOf marshals only the populated variant slot and round-trips cleanly.
func TestOneOfOnlySetVariantSerialized(t *testing.T) {
	cfg := models.ChunkingConfiguration{
		Recursive: &models.RecursiveChunkingConfiguration{ChunkSize: 512},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, "recursive") {
		t.Errorf("set variant 'recursive' missing: %s", got)
	}
	if strings.Contains(got, "sentence") || strings.Contains(got, `"none"`) {
		t.Errorf("unset oneOf variants should be omitted: %s", got)
	}

	var back models.ChunkingConfiguration
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Recursive == nil || back.Sentence != nil || back.None != nil {
		t.Errorf("oneOf round-trip lost variant exclusivity: %+v", back)
	}
}

// bytes fields marshal as base64 (the format=binary wire contract) and
// round-trip back to the original bytes.
func TestBytesFieldBase64RoundTrip(t *testing.T) {
	m := models.Memory{OriginalContent: []byte("hello")}
	b, _ := json.Marshal(m)
	if !strings.Contains(string(b), `"originalContent":"aGVsbG8="`) {
		t.Errorf("bytes should marshal as base64: %s", b)
	}
	var back models.Memory
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(back.OriginalContent) != "hello" {
		t.Errorf("bytes round-trip = %q, want %q", back.OriginalContent, "hello")
	}
}

// Full struct round-trip preserves both set and unset optional fields.
func TestStructRoundTripPreservesOptionalState(t *testing.T) {
	exp := int64(1234567890)
	orig := models.APIKeyResponse{
		APIKeyID:           "k1",
		SubjectPrincipalID: "u1",
		OwnerPrincipalID:   "u1",
		AuthorityMode:      models.APIKeyAuthorityModeINHERITSUBJECT,
		KeyPrefix:          "gm_",
		Status:             "ACTIVE",
		Labels:             map[string]string{"env": "prod"},
		ExpiresAt:          &exp,
		CreatedAt:          1,
		UpdatedAt:          2,
	}
	b, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back models.APIKeyResponse
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(orig, back) {
		t.Errorf("round-trip mismatch:\n orig=%+v\n back=%+v", orig, back)
	}
	if back.LastUsedAt != nil {
		t.Errorf("unset optional LastUsedAt should stay nil, got %v", *back.LastUsedAt)
	}
}

// format=int32 fields map to Go int32; format=int64/unspecified to int64.
// Compile-time assertions: these would fail to build if the widths drifted.
func TestIntFormatWidths(t *testing.T) {
	var _ int32 = models.EmbedderCreationRequest{}.Dimensionality
	var _ int64 = models.Space{}.CreatedAt
}

// extra_fields from convenience.json (server-emitted, not in the OpenAPI spec)
// round-trip into the augmented models, so a caller iterating a batch result sees
// the real wire shape (requestIndex/deletedCount/totalDeleted). inv-extra-fields.
func TestExtraFieldsRoundTrip(t *testing.T) {
	wire := `{"results":[{"success":true,"requestIndex":2,"deletedCount":5}],"totalDeleted":5}`
	var resp models.BatchMemoryResponse
	if err := json.Unmarshal([]byte(wire), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.TotalDeleted == nil || *resp.TotalDeleted != 5 {
		t.Errorf("TotalDeleted = %v, want 5", resp.TotalDeleted)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("results len = %d, want 1", len(resp.Results))
	}
	r := resp.Results[0]
	if r.RequestIndex == nil || *r.RequestIndex != 2 {
		t.Errorf("RequestIndex = %v, want 2", r.RequestIndex)
	}
	if r.DeletedCount == nil || *r.DeletedCount != 5 {
		t.Errorf("DeletedCount = %v, want 5", r.DeletedCount)
	}
	// Absent on a non-delete batch response: pointers stay nil and omitempty drops them.
	out, err := json.Marshal(models.BatchMemoryResponse{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(out), "totalDeleted") {
		t.Errorf("empty response should omit totalDeleted, got %s", out)
	}
}
