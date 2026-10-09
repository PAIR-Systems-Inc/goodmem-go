// Package goodmem is the Go SDK for the GoodMem memory API.
//
// It is generated from the same universal API IR (api_ir.json) that drives the
// Python, Java and MCP SDKs, so the wire contract is identical across every
// language. Generated wire types live in the models subpackage; the
// user-facing client surface (services, pagination, streaming, convenience
// transforms) is layered on top in later generation phases.
//
// Code in this module is produced by clients/_clients_gen/go — do not edit
// generated files by hand. Regenerate with: ./clients/clients_gen.sh
//
// Deliberate Go-idiomatic divergences from the Python/Java SDKs (see
// clients/notes/new_language.md for the cross-language symmetry checklist):
//
//   - Synchronous only. Every method takes a context.Context; cancellation and
//     deadlines come from the context rather than a parallel async surface
//     (checklist item 15). There is no AsyncClient.
//   - String resource IDs. IDs stay plain strings rather than typed handles
//     (SpaceID, MemoryID, …); Go callers conventionally pass string ids and the
//     compiler-safety win doesn't justify the surface weight (checklist item 5).
//   - Unknown response enums are tolerated for free. Enums are typed string
//     constants, so an unrecognized server value deserializes without error
//     (inv-unknown-server-enum-coerces-not-crash) — no coercion layer needed.
//   - Numeric range guards are request-side only. They run in a model's
//     Validate() (called before a request is sent) but not on response
//     deserialization, so a zero-valued server response is never rejected
//     (inv-numeric-range-from-ir).
package goodmem
