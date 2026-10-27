# GoodMem Go SDK

Official Go SDK for [GoodMem](https://goodmem.ai): persistent memory and hybrid
retrieval for AI agents.

- [Go SDK documentation](https://docs.goodmem.ai/docs/reference/sdk/go/)
- [API documentation](https://docs.goodmem.ai/)
- [Examples](examples/basic_rag/main.go)

## Install

Requires Go 1.23 or later:

```sh
go get github.com/PAIR-Systems-Inc/goodmem-go@v0.1.13
```

```go
import (
    goodmem "github.com/PAIR-Systems-Inc/goodmem-go"
    "github.com/PAIR-Systems-Inc/goodmem-go/models"
)
```

The module uses Go's normal module resolution; no Gemfury proxy configuration is
needed for this import path. Existing Gemfury users must update both their module
requirement and imports when moving to this module.

## Quickstart

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
    page, err := client.Spaces().List(context.Background(), nil)
    if err != nil {
        log.Fatal(err)
    }
    for space, err := range page.All(context.Background()) {
        if err != nil {
            log.Fatal(err)
        }
        log.Println(space.SpaceID, space.Name)
    }
}
```

The SDK supports context cancellation, typed models and errors, automatic
pagination, NDJSON streaming, file uploads, and model-registry conveniences.
A running GoodMem server and an API key are required to use it.

## Development and validation

```sh
go build ./...
go vet ./...
go test ./...
```

Unit tests use local HTTP test servers. The documentation integration suite skips
unless `GOODMEM_BASE_URL` and `GOODMEM_API_KEY` are set; some integration examples
also require model-provider credentials. CI runs build, vet, and unit tests without
those credentials.

Source changes and regeneration are maintained in GoodMem's development
repository. Generated files should not be edited here. Please report SDK issues
in this repository.

## Publication status and provenance

Releases are exported and tested by GoodMem's release pipeline, then pushed
here with immutable version tags. No private repository history is imported.
[provenance.json](provenance.json) identifies the source commit and module path.
New releases are published here only. Existing Gemfury users must change both
their module requirement and imports to use this GitHub module.

## License

[Apache License 2.0](LICENSE), consistent with
[GoodMem's published SDK licensing policy](https://goodmem.ai/resources/faq).
The separately deployed GoodMem server has its own license.
