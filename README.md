# GoodMem Go SDK

Official Go SDK for [GoodMem](https://goodmem.ai): persistent memory and hybrid
retrieval for AI agents.

The SDK provides a synchronous, `context.Context`-driven client, typed models and
errors, automatic pagination, NDJSON streaming, file uploads, and model-registry
conveniences.

- [Go SDK documentation](https://docs.goodmem.ai/docs/reference/sdk/go/)
- [API documentation](https://docs.goodmem.ai/)
- [Basic RAG example](examples/basic_rag/main.go)

## Installation

The SDK is published from the public [goodmem-go repository](https://github.com/PAIR-Systems-Inc/goodmem-go). Install with Go's standard module resolution:

```bash
go get github.com/PAIR-Systems-Inc/goodmem-go@latest
```

```go
import goodmem "github.com/PAIR-Systems-Inc/goodmem-go"
```

New releases are published to GitHub only. Existing Gemfury users must change their module requirement and all imports (including `/models`) from `fury.io/pairsys/goodmem` to `github.com/PAIR-Systems-Inc/goodmem-go`. No custom proxy or checksum-database exemption is needed for the GitHub module. Previously published Gemfury versions are not deleted by this migration.

Requires Go 1.23+ (for range-over-func iterators / `iter.Seq2`, the newest language feature the SDK uses).

## Quickstart

You need a running GoodMem server and an API key. Set `GOODMEM_BASE_URL` to your
server URL and `GOODMEM_API_KEY` to your key, then run:

```go
package main

import (
	"context"
	"log"
	"os"

	goodmem "github.com/PAIR-Systems-Inc/goodmem-go"
)

func main() {
	client, err := goodmem.New(os.Getenv("GOODMEM_BASE_URL"), os.Getenv("GOODMEM_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	page, err := client.Spaces().List(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	for space, err := range page.All(ctx) {
		if err != nil {
			log.Fatal(err)
		}
		log.Println(space.SpaceID, space.Name)
	}
}
```

Every method takes a `context.Context`; cancellation and deadlines come from the context.
The following examples use the same `client` and `ctx`.

## Features

| Area | Support |
|---|---|
| Synchronous client (`context.Context`-driven) | ✓ |
| NDJSON streaming (`memories.Retrieve`, `ping.Stream`) via `Stream[T].Events()` | ✓ |
| Auto-pagination (`Page[T]` with `.All(ctx)` range-over-func iterator) | ✓ |
| Typed-string enums (e.g. `ProviderType`, `MemoryProcessingStatus`) | ✓ |
| Model-registry auto-inference on `Embedders/LLMs/Rerankers.Create` | ✓ |
| Flat retrieve convenience (`Memories.Retrieve(ctx, message, opts)`) | ✓ |
| Content-type inference on `Memories.Create` / `BatchCreate` | ✓ |
| File upload (`Memories.CreateFromFile` multipart, `OCR.DocumentFromFile`) | ✓ |
| Timeout-budget pre-check on `Admin.Drain` / `Ping.Once` / `Ping.Stream` | ✓ (no-op without a fixed client timeout) |
| Bytes responses (`Memories.Content`, `Memories.PagesImage`) | ✓ |
| Typed errors via `errors.As` (`*APIError` + status subtypes, `*NetworkError`) | ✓ |
| Secret redaction in `String()` for credential-bearing models | ✓ |
| BYO `*http.Client` (`WithHTTPClient`) for advanced transport config | ✓ |
| Async client | ✗ — synchronous only by design (use a `context.Context`) |
| Typed id handles (`SpaceID`, `MemoryID`, …) | ✗ — IDs are plain `string`s |

Services are accessed through methods such as `client.Spaces()`, `client.Memories()`,
`client.Embedders()`, and `client.AccessPolicy()`. See the
[API reference](https://docs.goodmem.ai/docs/reference/sdk/go/) for all services.

## Streaming example

```go
stream, err := client.Memories().Retrieve(ctx, "What do you know about distributed systems?",
	&goodmem.RetrieveOptions{SpaceIDs: []string{spaceID}})
if err != nil {
	log.Fatal(err)
}
defer stream.Close()

for event, err := range stream.Events() { // range-over-func iterator
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", event)
}
```

## Pagination example

```go
page, err := client.Spaces().List(ctx, nil)
if err != nil {
	log.Fatal(err)
}
for space, err := range page.All(ctx) { // iterates across pages automatically
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(space.SpaceID, space.Name)
}
```

## Error handling

```go
import "errors"

_, err := client.Spaces().Get(ctx, "does-not-exist")
var notFound *goodmem.NotFoundError
if errors.As(err, &notFound) {
	fmt.Println("not found:", notFound.StatusCode)
}
```

Caller-side validation failures (empty id, missing required convenience input) return a plain `error`; server and transport failures return SDK error types that satisfy the `goodmem.Error` interface (`*APIError` and its per-status subtypes, `*RateLimitError`, `*NetworkError`, `*GoodmemError`).

## Download request headers

Use `ContentWithHeaders` or `PagesImageWithHeaders` to send download controls
without changing the client's other requests. Existing `Content` and `PagesImage`
calls remain available.

```go
part, err := client.Memories().ContentWithHeaders(ctx, memoryID, map[string]string{
    "Range": "bytes=1024-",
    "Accept-Encoding": "identity",
    "If-Match": savedETag, // Preserve the quotes around the strong ETag.
})
```

These methods return buffered bytes, including successful HTTP 206 responses.
HTTP 412 (source changed) and 416 (invalid range) return SDK errors.

## Development and validation

From the root of a `goodmem-go` checkout:

```bash
go build ./...
go vet ./...
go test ./...
```

Unit tests use local HTTP test servers. The documentation integration suite skips
unless `GOODMEM_BASE_URL` and `GOODMEM_API_KEY` are set; some integration examples
also require model-provider credentials. Release verification runs build, vet, and
unit tests without those credentials. Use a disposable server for integration tests,
which create and delete resources.

Source changes, documentation, and regeneration are maintained in GoodMem's development
repository and exported here for releases. Generated files should not be edited by
hand. Please report SDK issues in [this repository](https://github.com/PAIR-Systems-Inc/goodmem-go/issues).

## Layout

The public repository contains the SDK source and tests:

```
goodmem-go/
├── go.mod                       # module github.com/PAIR-Systems-Inc/goodmem-go (Go 1.23)
├── doc.go                       # package doc + the documented Go divergences
├── client.go                    # New(baseURL, apiKey, opts...) + Option (WithHTTPClient)
├── transport.go                 # doJSON / doBytes / openStream (HTTP + JSON plumbing)
├── errors.go                    # Error interface + typed status errors
├── pagination.go                # Page[T] (auto-pagination)
├── streaming.go                 # Stream[T] (NDJSON)
├── transforms.go                # hand-written convenience (registry inference, content-type)
├── retrieve.go                  # Memories.Retrieve flat convenience
├── registries.go                # embedded model registries (go:embed)
├── gen_api_<ns>.go              # GENERATED per-namespace services
├── models/                      # GENERATED wire types (package models)
└── *_test.go                    # httptest-backed unit tests + doc-example integration test
```

## Design decisions

| Area | Choice | Why |
|------|--------|-----|
| Concurrency | Synchronous, `context.Context` only | Idiomatic Go; cancellation/deadlines via context, no parallel async surface |
| Resource IDs | Plain `string` | Go callers conventionally pass string ids; IDs work directly with standard Go string APIs |
| Optional fields | `*T` pointers + `,omitempty` | Distinguishes "unset" from zero value; drops null fields on the wire (the analog of Java's boxed + `@JsonInclude(NON_NULL)`) |
| oneOf | Struct with pointer variant slots + strict `Validate()` | Exactly-one enforcement on send |
| Pagination / streaming | Generics + `iter.Seq2` range-over-func | `for x, err := range page.All(ctx)` / `stream.Events()` reads naturally |
| Unknown response enums | Accepted | Typed-string enums deserialize unfamiliar server values without error |
| Numeric range guards | Request-side only | Run in `Validate()` before send, not on response deserialization, so a zero-valued response is never rejected |
| Merged paths | Single options-bag method (`Users.Get(ctx, *UsersGetOptions)`) | Avoids per-path overload bloat; the id/email mutex is enforced in the method |

## Publication and provenance

Releases export committed SDK files from the GoodMem development repository and publish them to `goodmem-go` with immutable version tags. No private Git history is published. In a public release, [provenance.json](https://github.com/PAIR-Systems-Inc/goodmem-go/blob/main/provenance.json) records the source commit and module path; each release tag contains its own copy.

## License

[Apache License 2.0](https://github.com/PAIR-Systems-Inc/goodmem-go/blob/main/LICENSE), consistent with [GoodMem's published SDK licensing policy](https://goodmem.ai/resources/faq). The separately deployed GoodMem server has its own license.
