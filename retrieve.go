package goodmem

import (
	"context"

	"github.com/PAIR-Systems-Inc/goodmem-go/models"
)

// Retrieve runs a memory retrieval using the flat convenience parameters,
// expanding SpaceIDs into SpaceKeys and the flat post-processor keys into a
// ChatPostProcessor (space_ids_to_space_keys + flat_to_nested transforms). The
// returned Stream must be closed. Use RetrieveRaw to pass a fully-formed
// RetrieveMemoryRequest.
//
// RetrieveOptions and the request assembly (buildRetrieveRequest /
// buildChatPostProcessor) are generated into gen_convenience.go from
// convenience.json; this thin wrapper is the only hand-written piece because it
// depends on the RetrieveRaw service method signature.
func (s *MemoriesService) Retrieve(ctx context.Context, message string, opts *RetrieveOptions) (*Stream[models.RetrieveMemoryEvent], error) {
	req, err := buildRetrieveRequest(message, opts)
	if err != nil {
		return nil, err
	}
	return s.RetrieveRaw(ctx, req)
}
