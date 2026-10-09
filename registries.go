package goodmem

import (
	_ "embed"
	"encoding/json"
)

// Bundled model registries, shared across every SDK (the same JSON files drive
// the Python/Java/MCP convenience layers). Copied into the module by
// _clients_gen/go.emit and embedded so the convenience transforms work with no
// network or filesystem dependency. DO NOT edit these copies — edit
// clients/registries/*.json and regenerate.

//go:embed registries/embedders.json
var embeddersRegistryJSON []byte

//go:embed registries/llms.json
var llmsRegistryJSON []byte

//go:embed registries/rerankers.json
var rerankersRegistryJSON []byte

//go:embed registries/providers.json
var providersRegistryJSON []byte

// embedderRegistryEntry models every key an embedders.json entry can carry.
// Decoding is strict (DisallowUnknownFields in the parity test), so a new
// registry key that the convenience transform doesn't consume fails the build's
// tests rather than silently dropping on the wire (inv-registry... parity).
type embedderRegistryEntry struct {
	ProviderType        string   `json:"provider_type"`
	SupportedModalities []string `json:"supported_modalities"`
	MaxSequenceLength   *int32   `json:"max_sequence_length"`
	Dimensions          *struct {
		Default int32   `json:"default"`
		Range   []int32 `json:"range"`
		Allowed []int32 `json:"allowed"`
	} `json:"dimensions"`
}

type llmRegistryEntry struct {
	ProviderType        string   `json:"provider_type"`
	SupportedModalities []string `json:"supported_modalities"`
	MaxContextLength    *int32   `json:"max_context_length"`
}

type rerankerRegistryEntry struct {
	ProviderType        string   `json:"provider_type"`
	SupportedModalities []string `json:"supported_modalities"`
	// MaxContextLength is modeled so the strict-decode parity test accepts
	// the entry, but it is deliberately NOT auto-filled: it is a console
	// catalog field (RerankerModelInfo.contextWindow, added in #1811) and
	// models.RerankerCreationRequest has no counterpart, unlike LLMs.
	MaxContextLength *int32 `json:"max_context_length"`
	// Description is an optional per-model UI blurb the registry may carry.
	// Unlike MaxContextLength it IS a field on models.RerankerCreationRequest,
	// so it is auto-filled like any other create field.
	Description *string `json:"description"`
}

type providerEntry struct {
	DefaultBaseURL          string   `json:"default_base_url"`
	BaseURLsRequiringAPIKey []string `json:"base_urls_requiring_api_key"`
}

var (
	embedderRegistry = mustParseRegistry[embedderRegistryEntry](embeddersRegistryJSON)
	llmRegistry      = mustParseRegistry[llmRegistryEntry](llmsRegistryJSON)
	rerankerRegistry = mustParseRegistry[rerankerRegistryEntry](rerankersRegistryJSON)
	providerRegistry = mustParseRegistry[providerEntry](providersRegistryJSON)
)

func mustParseRegistry[T any](data []byte) map[string]T {
	var m map[string]T
	if err := json.Unmarshal(data, &m); err != nil {
		panic("goodmem: corrupt embedded registry: " + err.Error())
	}
	return m
}
