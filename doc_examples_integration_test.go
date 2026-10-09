// SDK doc examples — minimal snippets that compile against the live SDK and run
// against a real server. Each snippet is wrapped in
// `// docs:begin sdk.<ns>.<method>` / `// docs:end ...` markers so _docs_gen can
// splice them into the generated reference docs. The cross-language parity
// meta-test asserts this file carries the same snippet IDs as the Python and
// Java doc-example suites (inv-snippet-parity).
//
// The whole suite self-skips unless GOODMEM_BASE_URL and GOODMEM_API_KEY are
// set; embedder/LLM steps additionally need OPENAI_API_KEY, reranker steps need
// VOYAGE_API_KEY, and admin.drain only runs when GOODMEM_RUN_DRAIN=1 (drain
// quiesces the server — never run by default, inv-integration-tests-never-drain-by-default).
package goodmem_test

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"os"
	"testing"

	goodmem "github.com/PAIR-Systems-Inc/goodmem-go"
	"github.com/PAIR-Systems-Inc/goodmem-go/models"
)

func TestDocSdkExamples(t *testing.T) {
	baseURL := os.Getenv("GOODMEM_BASE_URL")
	apiKey := os.Getenv("GOODMEM_API_KEY")
	if baseURL == "" || apiKey == "" {
		t.Skip("set GOODMEM_BASE_URL and GOODMEM_API_KEY to run the doc-example integration suite")
	}
	openaiKey := os.Getenv("OPENAI_API_KEY")
	voyageKey := os.Getenv("VOYAGE_API_KEY")

	client, err := goodmem.New(baseURL, apiKey)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	ctx := context.Background()
	ids := map[string]string{}

	// Abort-safe teardown. The ordered delete subtests below are the real assertions
	// that the delete APIs work and they stay; this is the safety net for the case
	// they cannot cover — a subtest calling t.Fatal, or the run being interrupted,
	// never reaches them and would otherwise leak resources onto the server.
	// Best-effort: ignores already-deleted ids, so it is a no-op after a clean run.
	t.Cleanup(func() {
		for _, k := range []string{"memory", "memory_pages"} {
			if id := ids[k]; id != "" {
				_ = client.Memories().Delete(ctx, id)
			}
		}
		if id := ids["space"]; id != "" {
			_ = client.Spaces().Delete(ctx, id)
		}
		if id := ids["reranker"]; id != "" {
			_ = client.Rerankers().Delete(ctx, id)
		}
		if id := ids["llm"]; id != "" {
			_ = client.LLMs().Delete(ctx, id)
		}
		if id := ids["embedder"]; id != "" {
			_ = client.Embedders().Delete(ctx, id)
		}
	})

	must := func(t *testing.T, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	t.Run("system.info", func(t *testing.T) {
		// docs:begin sdk.system.info
		info, err := client.System().Info(ctx)
		must(t, err)
		_ = info.Version
		// docs:end sdk.system.info
	})

	t.Run("system.init", func(t *testing.T) {
		// docs:begin sdk.system.init
		result, err := client.System().Init(ctx)
		must(t, err)
		_ = result.AlreadyInitialized
		// docs:end sdk.system.init
	})

	t.Run("instance.get", func(t *testing.T) {
		// docs:begin sdk.instance.get
		instance, err := client.Instance().Get(ctx)
		must(t, err)
		_ = instance.InstanceID
		_ = instance.OwnerID
		// docs:end sdk.instance.get
	})

	t.Run("users.me", func(t *testing.T) {
		// docs:begin sdk.users.me
		user, err := client.Users().Me(ctx)
		must(t, err)
		_ = user.Email
		// docs:end sdk.users.me
		ids["user_id"] = user.UserID
		ids["user_email"] = user.Email
	})

	t.Run("users.get", func(t *testing.T) {
		// docs:begin sdk.users.get
		user, err := client.Users().Get(ctx, &goodmem.UsersGetOptions{ID: goodmem.Ptr(ids["user_id"])})
		must(t, err)
		_ = user.Email
		// docs:end sdk.users.get
	})

	t.Run("users.get.2", func(t *testing.T) {
		// docs:begin sdk.users.get.2
		user, err := client.Users().Get(ctx, &goodmem.UsersGetOptions{Email: goodmem.Ptr(ids["user_email"])})
		must(t, err)
		_ = user.UserID
		// docs:end sdk.users.get.2
	})

	// Principal lifecycle examples intentionally use placeholders and do not
	// mutate a shared live-server fixture. They are still compiled and harvested
	// into the generated reference documentation.
	t.Run("principal lifecycle documentation", func(t *testing.T) {
		t.Skip("requires disposable principals and enrollment credentials")

		// docs:begin sdk.users.create
		createdUser, err := client.Users().Create(ctx, &models.CreateUserRequest{
			Email:       "alex@example.com",
			Username:    goodmem.Ptr("alex"),
			DisplayName: goodmem.Ptr("Alex Example"),
			Labels:      map[string]string{"team": "search"},
		})
		must(t, err)
		_ = createdUser.UserID
		// docs:end sdk.users.create

		// docs:begin sdk.users.list
		users, err := client.Users().List(ctx, &goodmem.UsersListParams{
			MaxResults: goodmem.Ptr(int32(25)),
		})
		must(t, err)
		for _, user := range users.Items() {
			_ = user.Email
		}
		// docs:end sdk.users.list

		// docs:begin sdk.users.get_by_username
		userByUsername, err := client.Users().GetByUsername(ctx, "alex", nil)
		must(t, err)
		_ = userByUsername.Email
		// docs:end sdk.users.get_by_username

		// docs:begin sdk.users.update
		updatedUser, err := client.Users().Update(ctx, "your-user-id", &models.UpdateUserRequest{
			DisplayName: goodmem.Ptr("Alexandra Example"),
		})
		must(t, err)
		_ = updatedUser.DisplayName
		// docs:end sdk.users.update

		// docs:begin sdk.users.create_enrollment
		createdEnrollment, err := client.Users().CreateEnrollment(
			ctx,
			"your-user-id",
			&models.CreateUserEnrollmentRequest{RotateExisting: goodmem.Ptr(true)},
		)
		must(t, err)
		// Save createdEnrollment.EnrollmentToken now; it is returned only once.
		_ = createdEnrollment.EnrollmentToken
		// docs:end sdk.users.create_enrollment

		// docs:begin sdk.users.list_enrollments
		enrollments, err := client.Users().ListEnrollments(ctx, "your-user-id", nil)
		must(t, err)
		for _, enrollment := range enrollments.Items() {
			_ = enrollment.Status
		}
		// docs:end sdk.users.list_enrollments

		// docs:begin sdk.users.get_enrollment
		enrollment, err := client.Users().GetEnrollment(
			ctx,
			"your-user-id",
			"your-enrollment-id",
		)
		must(t, err)
		_ = enrollment.Status
		// docs:end sdk.users.get_enrollment

		// docs:begin sdk.users.revoke_enrollment
		revokeErr := client.Users().RevokeEnrollment(
			ctx,
			"your-user-id",
			"your-enrollment-id",
		)
		if revokeErr != nil {
			log.Fatal(revokeErr)
		}
		// docs:end sdk.users.revoke_enrollment

		// docs:begin sdk.user_enrollments.complete
		completed, err := client.UserEnrollments().Complete(
			ctx,
			&models.CompleteUserEnrollmentRequest{EnrollmentToken: "gme_..."},
		)
		must(t, err)
		// Server-generated raw API keys are disclosed only on fresh completion.
		_ = completed.RawAPIKey
		// docs:end sdk.user_enrollments.complete

		// docs:begin sdk.service_identities.create
		identity, err := client.ServiceIdentities().Create(ctx, &models.CreateServiceIdentityRequest{
			DisplayName: "production-indexer",
			Description: goodmem.Ptr("Indexes newly uploaded memories"),
			Labels:      map[string]string{"environment": "production"},
		})
		must(t, err)
		_ = identity.ServiceIdentityID
		// docs:end sdk.service_identities.create

		// docs:begin sdk.service_identities.list
		identities, err := client.ServiceIdentities().List(
			ctx,
			&goodmem.ServiceIdentitiesListParams{MaxResults: goodmem.Ptr(int32(25))},
		)
		must(t, err)
		for _, item := range identities.Items() {
			_ = item.DisplayName
		}
		// docs:end sdk.service_identities.list

		// docs:begin sdk.service_identities.get
		fetchedIdentity, err := client.ServiceIdentities().Get(
			ctx,
			"your-service-identity-id",
			nil,
		)
		must(t, err)
		_ = fetchedIdentity.DisplayName
		// docs:end sdk.service_identities.get

		// docs:begin sdk.service_identities.update
		updatedIdentity, err := client.ServiceIdentities().Update(
			ctx,
			"your-service-identity-id",
			&models.UpdateServiceIdentityRequest{
				Description: goodmem.Ptr("Indexes production knowledge sources"),
			},
		)
		must(t, err)
		_ = updatedIdentity.Description
		// docs:end sdk.service_identities.update

		// docs:begin sdk.service_identities.transfer_ownership
		transferred, err := client.ServiceIdentities().TransferOwnership(
			ctx,
			"your-service-identity-id",
			&models.TransferOwnershipRequest{
				NewOwnerID: "70e025f6-76ca-4cbe-b8fc-7dab8e84590a",
			},
		)
		must(t, err)
		_ = transferred.ServiceIdentity.OwnerPrincipalID
		// docs:end sdk.service_identities.transfer_ownership

		// docs:begin sdk.service_identities.delete
		deleteIdentityErr := client.ServiceIdentities().Delete(
			ctx,
			"your-service-identity-id",
		)
		if deleteIdentityErr != nil {
			log.Fatal(deleteIdentityErr)
		}
		// docs:end sdk.service_identities.delete

		// docs:begin sdk.users.delete
		deleteUserErr := client.Users().Delete(ctx, "your-user-id")
		if deleteUserErr != nil {
			log.Fatal(deleteUserErr)
		}
		// docs:end sdk.users.delete
	})

	// Non-snippet coverage (not harvested for docs): a real 404 from the
	// server MUST map to *NotFoundError, exercising the typed-error status
	// mapping in errors.go end-to-end (otherwise only httptest-tested).
	t.Run("errors.not_found", func(t *testing.T) {
		_, err := client.Spaces().Get(ctx, "f47ac10b-58cc-4372-a567-0e02b2c3d479")
		if err == nil {
			t.Fatal("expected an error for a non-existent space id")
		}
		var nf *goodmem.NotFoundError
		if !errors.As(err, &nf) {
			t.Fatalf("expected *NotFoundError, got %T: %v", err, err)
		}
		_ = nf.StatusCode
	})

	// ---- Embedders -------------------------------------------------------
	t.Run("embedders.create", func(t *testing.T) {
		if openaiKey == "" {
			t.Skip("OPENAI_API_KEY not set")
		}
		// docs:begin sdk.embedders.create
		embedder, err := client.Embedders().Create(ctx, &models.EmbedderCreationRequest{
			DisplayName:     "Doc Embedder",
			ModelIdentifier: "text-embedding-3-small",
			Labels:          map[string]string{"env": "docs"},
		}, openaiKey)
		must(t, err)
		// docs:end sdk.embedders.create
		ids["embedder"] = embedder.EmbedderID
	})

	requireEmbedder := func(t *testing.T) {
		if ids["embedder"] == "" {
			t.Skip("embedder not created (OPENAI_API_KEY missing)")
		}
	}

	t.Run("embedders.get", func(t *testing.T) {
		requireEmbedder(t)
		// docs:begin sdk.embedders.get
		embedder, err := client.Embedders().Get(ctx, ids["embedder"], nil)
		must(t, err)
		_ = embedder.DisplayName
		// docs:end sdk.embedders.get
	})

	t.Run("embedders.list", func(t *testing.T) {
		// docs:begin sdk.embedders.list
		resp, err := client.Embedders().List(ctx, nil)
		must(t, err)
		for _, emb := range resp.Embedders {
			_ = emb.EmbedderID
			_ = emb.DisplayName
		}
		// docs:end sdk.embedders.list
	})

	t.Run("embedders.update", func(t *testing.T) {
		requireEmbedder(t)
		// docs:begin sdk.embedders.update
		updated, err := client.Embedders().Update(ctx, ids["embedder"], &models.UpdateEmbedderRequest{
			DisplayName: goodmem.Ptr("Doc Embedder (updated)"),
			MergeLabels: map[string]string{"version": "2"},
		})
		must(t, err)
		_ = updated.EmbedderID
		// docs:end sdk.embedders.update
	})

	// ---- LLMs ------------------------------------------------------------
	t.Run("llms.create", func(t *testing.T) {
		if openaiKey == "" {
			t.Skip("OPENAI_API_KEY not set")
		}
		// docs:begin sdk.llms.create
		resp, err := client.LLMs().Create(ctx, &models.LLMCreationRequest{
			DisplayName:     "Doc LLM",
			ModelIdentifier: "gpt-4o-mini",
			Labels:          map[string]string{"env": "docs"},
		}, openaiKey)
		must(t, err)
		llm := resp.LLM
		// docs:end sdk.llms.create
		ids["llm"] = llm.LLMID
	})

	requireLLM := func(t *testing.T) {
		if ids["llm"] == "" {
			t.Skip("llm not created (OPENAI_API_KEY missing)")
		}
	}

	t.Run("llms.get", func(t *testing.T) {
		requireLLM(t)
		// docs:begin sdk.llms.get
		llm, err := client.LLMs().Get(ctx, ids["llm"], nil)
		must(t, err)
		_ = llm.DisplayName
		// docs:end sdk.llms.get
	})

	t.Run("llms.list", func(t *testing.T) {
		// docs:begin sdk.llms.list
		resp, err := client.LLMs().List(ctx, nil)
		must(t, err)
		for _, llm := range resp.Llms {
			_ = llm.LLMID
			_ = llm.DisplayName
		}
		// docs:end sdk.llms.list
	})

	t.Run("llms.update", func(t *testing.T) {
		requireLLM(t)
		// docs:begin sdk.llms.update
		updated, err := client.LLMs().Update(ctx, ids["llm"], &models.LLMUpdateRequest{
			DisplayName: goodmem.Ptr("Doc LLM (updated)"),
			MergeLabels: map[string]string{"version": "2"},
		})
		must(t, err)
		_ = updated.LLMID
		// docs:end sdk.llms.update
	})

	// ---- Rerankers -------------------------------------------------------
	t.Run("rerankers.create", func(t *testing.T) {
		if voyageKey == "" {
			t.Skip("VOYAGE_API_KEY not set")
		}
		// docs:begin sdk.rerankers.create
		reranker, err := client.Rerankers().Create(ctx, &models.RerankerCreationRequest{
			DisplayName:     "Doc Reranker",
			ModelIdentifier: "rerank-2",
			Labels:          map[string]string{"env": "docs"},
		}, voyageKey)
		must(t, err)
		// docs:end sdk.rerankers.create
		ids["reranker"] = reranker.RerankerID
	})

	requireReranker := func(t *testing.T) {
		if ids["reranker"] == "" {
			t.Skip("reranker not created (VOYAGE_API_KEY missing)")
		}
	}

	t.Run("rerankers.get", func(t *testing.T) {
		requireReranker(t)
		// docs:begin sdk.rerankers.get
		reranker, err := client.Rerankers().Get(ctx, ids["reranker"], nil)
		must(t, err)
		_ = reranker.DisplayName
		// docs:end sdk.rerankers.get
	})

	t.Run("rerankers.list", func(t *testing.T) {
		// docs:begin sdk.rerankers.list
		resp, err := client.Rerankers().List(ctx, nil)
		must(t, err)
		for _, rr := range resp.Rerankers {
			_ = rr.RerankerID
			_ = rr.DisplayName
		}
		// docs:end sdk.rerankers.list
	})

	t.Run("rerankers.update", func(t *testing.T) {
		requireReranker(t)
		// docs:begin sdk.rerankers.update
		updated, err := client.Rerankers().Update(ctx, ids["reranker"], &models.UpdateRerankerRequest{
			DisplayName: goodmem.Ptr("Doc Reranker (updated)"),
			MergeLabels: map[string]string{"version": "2"},
		})
		must(t, err)
		_ = updated.RerankerID
		// docs:end sdk.rerankers.update
	})

	// ---- Spaces ----------------------------------------------------------
	t.Run("spaces.create", func(t *testing.T) {
		requireEmbedder(t)
		// docs:begin sdk.spaces.create
		space, err := client.Spaces().Create(ctx, &models.SpaceCreationRequest{
			Name: "Doc Space",
			SpaceEmbedders: []models.SpaceEmbedderConfig{
				{EmbedderID: ids["embedder"], DefaultRetrievalWeight: goodmem.Ptr(1.0)},
			},
			Labels: map[string]string{"env": "docs"},
		})
		must(t, err)
		// docs:end sdk.spaces.create
		ids["space"] = space.SpaceID
	})

	requireSpace := func(t *testing.T) {
		if ids["space"] == "" {
			t.Skip("space not created")
		}
	}

	t.Run("spaces.get", func(t *testing.T) {
		requireSpace(t)
		// docs:begin sdk.spaces.get
		space, err := client.Spaces().Get(ctx, ids["space"])
		must(t, err)
		_ = space.Name
		// docs:end sdk.spaces.get
	})

	t.Run("spaces.list", func(t *testing.T) {
		// docs:begin sdk.spaces.list
		// Auto-paginate ALL spaces, fetching pages on demand.
		page, err := client.Spaces().List(ctx, nil)
		must(t, err)
		for space, err := range page.All(ctx) {
			must(t, err)
			_ = space.SpaceID
			_ = space.Name
		}
		// docs:end sdk.spaces.list
	})

	t.Run("spaces.list.page_size", func(t *testing.T) {
		// docs:begin sdk.spaces.list.page_size
		// One page. MaxResults controls how many items the server returns per
		// API call.
		page, err := client.Spaces().List(ctx, &goodmem.SpacesListParams{MaxResults: goodmem.Ptr(int32(5))})
		must(t, err)
		_ = page.Items()     // up to 5 spaces
		_ = page.NextToken() // "" when there are no more
		// docs:end sdk.spaces.list.page_size
	})

	t.Run("spaces.list.max_items", func(t *testing.T) {
		// docs:begin sdk.spaces.list.max_items
		// Cap the number of items you consume, regardless of page size.
		page, err := client.Spaces().List(ctx, nil)
		must(t, err)
		var spaces []models.Space
		for space, err := range page.All(ctx) {
			must(t, err)
			if len(spaces) >= 3 {
				break
			}
			spaces = append(spaces, space)
		}
		// docs:end sdk.spaces.list.max_items
	})

	t.Run("spaces.list.for_loop_max", func(t *testing.T) {
		// docs:begin sdk.spaces.list.for_loop_max
		// Auto-paginate but stop after 2 items total.
		page, err := client.Spaces().List(ctx, nil)
		must(t, err)
		count := 0
		for space, err := range page.All(ctx) {
			must(t, err)
			if count >= 2 {
				break
			}
			_ = space.SpaceID
			count++
		}
		// docs:end sdk.spaces.list.for_loop_max
	})

	t.Run("spaces.list.page_size_max_items", func(t *testing.T) {
		// docs:begin sdk.spaces.list.page_size_max_items
		// MaxResults (server batch size) and a client-side cap do different
		// things: fetch 2 per call, stop after 5 total.
		page, err := client.Spaces().List(ctx, &goodmem.SpacesListParams{MaxResults: goodmem.Ptr(int32(2))})
		must(t, err)
		count := 0
		for space, err := range page.All(ctx) {
			must(t, err)
			if count >= 5 {
				break
			}
			_ = space.SpaceID
			count++
		}
		// docs:end sdk.spaces.list.page_size_max_items
	})

	t.Run("spaces.list.resume", func(t *testing.T) {
		// docs:begin sdk.spaces.list.resume
		// Save the token from the first page, then resume later.
		page, err := client.Spaces().List(ctx, &goodmem.SpacesListParams{MaxResults: goodmem.Ptr(int32(1))})
		must(t, err)
		savedToken := page.NextToken()
		if savedToken != "" {
			resumed, err := client.Spaces().List(ctx, &goodmem.SpacesListParams{NextToken: goodmem.Ptr(savedToken)})
			must(t, err)
			for space, err := range resumed.All(ctx) {
				must(t, err)
				_ = space.Name
			}
		}
		// docs:end sdk.spaces.list.resume
	})

	t.Run("spaces.list.resume_max", func(t *testing.T) {
		// docs:begin sdk.spaces.list.resume_max
		// Resume from a saved token but only consume up to 10 more items.
		page, err := client.Spaces().List(ctx, &goodmem.SpacesListParams{MaxResults: goodmem.Ptr(int32(1))})
		must(t, err)
		savedToken := page.NextToken()
		if savedToken != "" {
			resumed, err := client.Spaces().List(ctx, &goodmem.SpacesListParams{NextToken: goodmem.Ptr(savedToken)})
			must(t, err)
			n := 0
			for _, err := range resumed.All(ctx) {
				must(t, err)
				if n >= 10 {
					break
				}
				n++
			}
		}
		// docs:end sdk.spaces.list.resume_max
	})

	t.Run("spaces.list.iter_pages", func(t *testing.T) {
		// docs:begin sdk.spaces.list.iter_pages
		// Iterate page-by-page instead of item-by-item.
		page, err := client.Spaces().List(ctx, &goodmem.SpacesListParams{MaxResults: goodmem.Ptr(int32(5))})
		must(t, err)
		for page != nil {
			_ = len(page.Items())
			_ = page.NextToken()
			page, err = page.Next(ctx)
			must(t, err)
		}
		// docs:end sdk.spaces.list.iter_pages
	})

	t.Run("spaces.list.iter_pages_max", func(t *testing.T) {
		// docs:begin sdk.spaces.list.iter_pages_max
		// Page-by-page with a total cap across all pages.
		page, err := client.Spaces().List(ctx, &goodmem.SpacesListParams{MaxResults: goodmem.Ptr(int32(2))})
		must(t, err)
		total := 0
		for page != nil && total < 2 {
			total += len(page.Items())
			page, err = page.Next(ctx)
			must(t, err)
		}
		// docs:end sdk.spaces.list.iter_pages_max
	})

	t.Run("spaces.update", func(t *testing.T) {
		requireSpace(t)
		// docs:begin sdk.spaces.update
		updated, err := client.Spaces().Update(ctx, ids["space"], &models.UpdateSpaceRequest{
			Name:        goodmem.Ptr("Doc Space (updated)"),
			MergeLabels: map[string]string{"version": "2"},
		})
		must(t, err)
		_ = updated.SpaceID
		// docs:end sdk.spaces.update
	})

	// ---- Memories --------------------------------------------------------
	t.Run("memories.create", func(t *testing.T) {
		requireSpace(t)
		// docs:begin sdk.memories.create
		memory, err := client.Memories().Create(ctx, &models.JSONMemoryCreationRequest{
			SpaceID:         ids["space"],
			OriginalContent: goodmem.Ptr("GoodMem stores and retrieves vectorized memories for RAG applications."),
			ContentType:     "text/plain",
			Metadata:        map[string]any{"source": "sdk-doc-test"},
		})
		must(t, err)
		// docs:end sdk.memories.create
		ids["memory"] = memory.MemoryID
	})

	t.Run("memories.create.2", func(t *testing.T) {
		requireSpace(t)
		// docs:begin sdk.memories.create.2
		// Read a file and send it as base64-encoded content.
		data, err := os.ReadFile("testdata/example.txt")
		if err != nil {
			t.Skip("no test file")
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		memory, err := client.Memories().Create(ctx, &models.JSONMemoryCreationRequest{
			SpaceID:            ids["space"],
			OriginalContentB64: goodmem.Ptr(b64),
			ContentType:        "text/plain",
		})
		must(t, err)
		_ = memory.MemoryID
		// docs:end sdk.memories.create.2
	})

	t.Run("memories.create.3", func(t *testing.T) {
		requireSpace(t)
		// docs:begin sdk.memories.create.3
		b64 := base64.StdEncoding.EncodeToString([]byte("Base64-encoded content."))
		memory, err := client.Memories().Create(ctx, &models.JSONMemoryCreationRequest{
			SpaceID:            ids["space"],
			OriginalContentB64: goodmem.Ptr(b64),
			ContentType:        "text/plain",
		})
		must(t, err)
		_ = memory.MemoryID
		// docs:end sdk.memories.create.3
	})

	// Non-snippet coverage (not harvested for docs): the multipart
	// file-upload path (uploads.go CreateFromFile / doMultipart) — content
	// type is inferred from the .txt extension; the created memory is
	// cascade-deleted when the space is removed in cleanup.
	t.Run("memories.create_from_file", func(t *testing.T) {
		requireSpace(t)
		memory, err := client.Memories().CreateFromFile(ctx, "testdata/example.txt", &models.JSONMemoryCreationRequest{
			SpaceID: ids["space"],
		})
		must(t, err)
		_ = memory.MemoryID
	})

	requireMemory := func(t *testing.T) {
		if ids["memory"] == "" {
			t.Skip("memory not created")
		}
	}

	t.Run("memories.get", func(t *testing.T) {
		requireMemory(t)
		// docs:begin sdk.memories.get
		memory, err := client.Memories().Get(ctx, ids["memory"], nil)
		must(t, err)
		_ = memory.ProcessingStatus
		// docs:end sdk.memories.get
	})

	t.Run("memories.list", func(t *testing.T) {
		requireSpace(t)
		// docs:begin sdk.memories.list
		page, err := client.Memories().List(ctx, ids["space"], nil)
		must(t, err)
		for mem, err := range page.All(ctx) {
			must(t, err)
			_ = mem.MemoryID
			_ = mem.ProcessingStatus
		}
		// docs:end sdk.memories.list
	})

	t.Run("memories.list.2", func(t *testing.T) {
		requireSpace(t)
		// docs:begin sdk.memories.list.2
		// First page, then resume from the saved token.
		page, err := client.Memories().List(ctx, ids["space"], &goodmem.MemoriesListParams{MaxResults: goodmem.Ptr(int32(1))})
		must(t, err)
		savedToken := page.NextToken()
		if savedToken != "" {
			resumed, err := client.Memories().List(ctx, ids["space"], &goodmem.MemoriesListParams{NextToken: goodmem.Ptr(savedToken)})
			must(t, err)
			for mem, err := range resumed.All(ctx) {
				must(t, err)
				_ = mem.MemoryID
			}
		}
		// docs:end sdk.memories.list.2
	})

	t.Run("memories.content", func(t *testing.T) {
		requireMemory(t)
		// docs:begin sdk.memories.content
		content, err := client.Memories().Content(ctx, ids["memory"])
		must(t, err)
		_ = content // raw bytes
		// docs:end sdk.memories.content
	})

	t.Run("memories.batch_create", func(t *testing.T) {
		requireSpace(t)
		// docs:begin sdk.memories.batch_create
		result, err := client.Memories().BatchCreate(ctx, &models.JSONBatchMemoryCreationRequest{
			Requests: []models.JSONMemoryCreationRequest{
				{SpaceID: ids["space"], OriginalContent: goodmem.Ptr("Batch memory one."), ContentType: "text/plain"},
				{SpaceID: ids["space"], OriginalContent: goodmem.Ptr("Batch memory two."), ContentType: "text/plain"},
			},
		})
		must(t, err)
		// docs:end sdk.memories.batch_create
		var batch []string
		for _, r := range result.Results {
			if r.Memory != nil {
				batch = append(batch, r.Memory.MemoryID)
			}
		}
		ids["batch_0"] = ""
		if len(batch) > 0 {
			ids["batch_0"] = batch[0]
		}
	})

	t.Run("memories.batch_get", func(t *testing.T) {
		if ids["batch_0"] == "" {
			t.Skip("no batch memories")
		}
		// docs:begin sdk.memories.batch_get
		result, err := client.Memories().BatchGet(ctx, &models.BatchMemoryRetrievalRequest{
			MemoryIds: []string{ids["batch_0"]},
		})
		must(t, err)
		for _, r := range result.Results {
			if r.Memory != nil {
				_ = r.Memory.ProcessingStatus
			}
		}
		// docs:end sdk.memories.batch_get
	})

	t.Run("memories.batch_delete", func(t *testing.T) {
		if ids["batch_0"] == "" {
			t.Skip("no batch memories")
		}
		// docs:begin sdk.memories.batch_delete
		result, err := client.Memories().BatchDelete(ctx, &models.BatchMemoryDeletionRequest{
			Requests: []models.BatchDeleteMemorySelectorRequest{
				{MemoryID: goodmem.Ptr(ids["batch_0"])},
			},
		})
		must(t, err)
		_ = result.Results
		// docs:end sdk.memories.batch_delete
	})

	t.Run("memories.pages", func(t *testing.T) {
		requireMemory(t)
		// docs:begin sdk.memories.pages
		// Auto-paginate page-image metadata, fetching pages on demand.
		pages, err := client.Memories().Pages(ctx, ids["memory"], nil)
		must(t, err)
		for pi, err := range pages.All(ctx) {
			must(t, err)
			_ = pi.PageIndex
		}
		// docs:end sdk.memories.pages
	})

	t.Run("memories.pages_image", func(t *testing.T) {
		requireMemory(t)
		// docs:begin sdk.memories.pages_image
		imageBytes, err := client.Memories().PagesImage(ctx, ids["memory"], "0", nil)
		if err == nil {
			_ = len(imageBytes)
		}
		// docs:end sdk.memories.pages_image
	})

	t.Run("memories.retrieve", func(t *testing.T) {
		requireSpace(t)
		// docs:begin sdk.memories.retrieve
		stream, err := client.Memories().Retrieve(ctx, "What is GoodMem?", &goodmem.RetrieveOptions{
			SpaceIDs:      []string{ids["space"]},
			RequestedSize: goodmem.Ptr(int32(5)),
		})
		must(t, err)
		defer stream.Close()
		for event, err := range stream.Events() {
			must(t, err)
			if event.RetrievedItem != nil {
				_ = event.RetrievedItem
			}
		}
		// docs:end sdk.memories.retrieve
	})

	t.Run("memories.retrieve.2", func(t *testing.T) {
		requireSpace(t)
		// docs:begin sdk.memories.retrieve.2
		// RetrieveRaw takes a fully-formed request for advanced control.
		stream, err := client.Memories().RetrieveRaw(ctx, &models.RetrieveMemoryRequest{
			Message:       "What is GoodMem?",
			SpaceKeys:     []models.SpaceKey{{SpaceID: ids["space"]}},
			RequestedSize: goodmem.Ptr(int32(5)),
		})
		must(t, err)
		defer stream.Close()
		for event, err := range stream.Events() {
			must(t, err)
			_ = event
		}
		// docs:end sdk.memories.retrieve.2
	})

	// Per-space metadata filters + conversation context on the flat convenience
	// layer (#1834). Asserts the SERVER honors SpaceKey.Filter, not merely that the
	// SDK serialized it: the same query is run with a filter that matches this
	// suite's own memory metadata and with one that cannot match.
	t.Run("memories.retrieve.filter_and_context", func(t *testing.T) {
		requireSpace(t)
		collect := func(filter *string, items []models.ContextItem) []string {
			t.Helper()
			stream, err := client.Memories().Retrieve(ctx, "What is GoodMem?", &goodmem.RetrieveOptions{
				SpaceKeys:     []models.SpaceKey{{SpaceID: ids["space"], Filter: filter}},
				Context:       items,
				RequestedSize: goodmem.Ptr(int32(5)),
			})
			must(t, err)
			defer stream.Close()
			var got []string
			for event, err := range stream.Events() {
				must(t, err)
				if event.MemoryDefinition != nil {
					got = append(got, event.MemoryDefinition.MemoryID)
				}
			}
			return got
		}

		// memories.create tagged its memory with {"source": "sdk-doc-test"}.
		matching := goodmem.Ptr(`CAST(val('$.source') AS TEXT) = 'sdk-doc-test'`)
		impossible := goodmem.Ptr(`CAST(val('$.source') AS TEXT) = 'no-such-source-value'`)

		// Context must be accepted alongside the filter — the combination #1834 says
		// previously forced callers onto RetrieveRaw.
		turns := []models.ContextItem{
			{Text: goodmem.Ptr("User asked: remind me what this is about?")},
			{Text: goodmem.Ptr("Assistant: it is a RAG memory store.")},
		}

		kept := collect(matching, turns)
		if len(kept) == 0 {
			t.Skip("no embedded memories matched yet; nothing to compare the filter against")
		}
		if excluded := collect(impossible, nil); len(excluded) != 0 {
			t.Errorf("filter that cannot match should exclude every memory, got %d", len(excluded))
		}
	})

	// ---- OCR -------------------------------------------------------------
	t.Run("ocr.document", func(t *testing.T) {
		// docs:begin sdk.ocr.document
		imageB64 := base64.StdEncoding.EncodeToString([]byte("not-a-real-image"))
		result, err := client.OCR().Document(ctx, &models.OCRDocumentRequest{
			Content:         goodmem.Ptr(imageB64),
			Format:          goodmem.Ptr(models.OCRInputFormatPNG),
			IncludeMarkdown: goodmem.Ptr(true),
		})
		if err == nil {
			_ = result.PageCount
		}
		// docs:end sdk.ocr.document
	})

	// Non-snippet coverage (not harvested for docs): the file→base64 OCR
	// path (uploads.go DocumentFromFile). A .txt file isn't a real
	// document, so the server may reject it — we only assert the SDK's
	// read/encode/request-build path runs without panicking.
	t.Run("ocr.document_from_file", func(t *testing.T) {
		result, err := client.OCR().DocumentFromFile(ctx, "testdata/example.txt", &models.OCRDocumentRequest{
			IncludeMarkdown: goodmem.Ptr(true),
		})
		if err == nil {
			_ = result.PageCount
		}
	})

	// ---- API keys --------------------------------------------------------
	t.Run("apikeys.create", func(t *testing.T) {
		// docs:begin sdk.apikeys.create
		key, err := client.APIKeys().Create(ctx, &models.CreateAPIKeyRequest{
			Labels: map[string]string{"env": "docs", "purpose": "sdk-doc-test"},
		})
		must(t, err)
		// docs:end sdk.apikeys.create
		if key.APIKeyMetadata != nil {
			ids["api_key"] = key.APIKeyMetadata.APIKeyID
		}
	})

	t.Run("apikeys.list", func(t *testing.T) {
		// docs:begin sdk.apikeys.list
		page, err := client.APIKeys().List(ctx, nil)
		must(t, err)
		for key, err := range page.All(ctx) {
			must(t, err)
			_ = key.APIKeyID
			_ = key.Status
		}
		// docs:end sdk.apikeys.list
	})

	t.Run("apikeys.get", func(t *testing.T) {
		if ids["api_key"] == "" {
			t.Skip("no api key")
		}
		// docs:begin sdk.apikeys.get
		key, err := client.APIKeys().Get(ctx, ids["api_key"])
		must(t, err)
		_ = key.APIKeyID
		_ = key.LifecycleState
		// docs:end sdk.apikeys.get
	})

	t.Run("apikeys.update", func(t *testing.T) {
		if ids["api_key"] == "" {
			t.Skip("no api key")
		}
		// docs:begin sdk.apikeys.update
		updated, err := client.APIKeys().Update(ctx, ids["api_key"], &models.UpdateAPIKeyRequest{
			MergeLabels: map[string]string{"updated": "true"},
		})
		must(t, err)
		_ = updated.APIKeyID
		// docs:end sdk.apikeys.update
	})

	t.Run("apikeys.delete", func(t *testing.T) {
		if ids["api_key"] == "" {
			t.Skip("no api key")
		}
		// docs:begin sdk.apikeys.delete
		err := client.APIKeys().Delete(ctx, ids["api_key"])
		must(t, err)
		// docs:end sdk.apikeys.delete
	})

	// ---- Ping ------------------------------------------------------------
	t.Run("ping.once", func(t *testing.T) {
		requireEmbedder(t)
		// docs:begin sdk.ping.once
		result, err := client.Ping().Once(ctx, &models.PingOnceRequest{TargetID: ids["embedder"]})
		must(t, err)
		_ = result.RttMs
		_ = result.Ok
		// docs:end sdk.ping.once
	})

	t.Run("ping.stream", func(t *testing.T) {
		requireEmbedder(t)
		// docs:begin sdk.ping.stream
		stream, err := client.Ping().Stream(ctx, &models.PingStreamRequest{TargetID: ids["embedder"], Count: goodmem.Ptr(int32(2))})
		must(t, err)
		defer stream.Close()
		for event, err := range stream.Events() {
			must(t, err)
			_ = event
		}
		// docs:end sdk.ping.stream
	})

	// ---- Admin -----------------------------------------------------------
	t.Run("admin.background_jobs.purge", func(t *testing.T) {
		// docs:begin sdk.admin.background_jobs.purge
		result, err := client.Admin().BackgroundJobsPurge(ctx, &models.AdminPurgeJobsRequest{
			OlderThan: goodmem.Ptr("2000-01-01T00:00:00Z"),
			DryRun:    goodmem.Ptr(true),
		})
		must(t, err)
		_ = result.JobsPurged
		// docs:end sdk.admin.background_jobs.purge
	})

	t.Run("admin.license.reload", func(t *testing.T) {
		// docs:begin sdk.admin.license.reload
		result, err := client.Admin().LicenseReload(ctx)
		must(t, err)
		_ = result.Status
		// docs:end sdk.admin.license.reload
	})

	t.Run("admin.retrieve_memory_log_policies.create", func(t *testing.T) {
		// docs:begin sdk.admin.retrieve_memory_log_policies.create
		created, err := client.Admin().RetrieveMemoryLogPoliciesCreate(ctx, &models.CreateRetrieveMemoryLogPolicyRequest{
			DisplayName: "audit-doc-example",
			Condition:   models.RetrieveMemoryLogPolicyCondition{MatchAll: goodmem.Ptr(true)},
		})
		must(t, err)
		_ = created.PolicyID
		// docs:end sdk.admin.retrieve_memory_log_policies.create
		ids["policy"] = created.PolicyID
	})

	t.Run("admin.retrieve_memory_log_policies.list", func(t *testing.T) {
		// docs:begin sdk.admin.retrieve_memory_log_policies.list
		// Auto-paginate every retrieve-memory log policy.
		page, err := client.Admin().RetrieveMemoryLogPoliciesList(ctx, nil)
		must(t, err)
		for policy, err := range page.All(ctx) {
			must(t, err)
			_ = policy.DisplayName
		}
		// docs:end sdk.admin.retrieve_memory_log_policies.list
	})

	t.Run("admin.retrieve_memory_log_policies.get", func(t *testing.T) {
		if ids["policy"] == "" {
			t.Skip("no policy")
		}
		// docs:begin sdk.admin.retrieve_memory_log_policies.get
		fetched, err := client.Admin().RetrieveMemoryLogPoliciesGet(ctx, ids["policy"], nil)
		must(t, err)
		_ = fetched.DisplayName
		// docs:end sdk.admin.retrieve_memory_log_policies.get
	})

	t.Run("admin.retrieve_memory_log_policies.delete", func(t *testing.T) {
		if ids["policy"] == "" {
			t.Skip("no policy")
		}
		// docs:begin sdk.admin.retrieve_memory_log_policies.delete
		tombstoned, err := client.Admin().RetrieveMemoryLogPoliciesDelete(ctx, ids["policy"],
			&models.DeleteRetrieveMemoryLogPolicyRequest{Reason: goodmem.Ptr("doc example cleanup")})
		must(t, err)
		_ = tombstoned.PolicyID
		// docs:end sdk.admin.retrieve_memory_log_policies.delete
	})

	t.Run("admin.drain", func(t *testing.T) {
		if os.Getenv("GOODMEM_RUN_DRAIN") != "1" {
			t.Skip("admin.drain quiesces the server; set GOODMEM_RUN_DRAIN=1 to run it")
		}
		// docs:begin sdk.admin.drain
		result, err := client.Admin().Drain(ctx, &models.AdminDrainRequest{TimeoutSec: goodmem.Ptr(int32(0))})
		must(t, err)
		_ = result.Quiesced
		// docs:end sdk.admin.drain
	})

	// Policy and ownership examples compile with the SDK, but are not
	// executed because they require purpose-built disposable principals.
	t.Run("access_policy", func(t *testing.T) {
		t.Skip("documentation-only policy examples require disposable principals")
		// docs:begin sdk.access_policy.check
		decisions, err := client.AccessPolicy().Check(ctx, &models.CheckAuthorizationsRequest{
			Checks: []models.AuthorizationCheck{
				{
					Operation: models.OperationREADINSTANCE,
					Target:    models.AccessPolicyTarget{Kind: models.ResourceKindINSTANCE},
				},
			},
		})
		must(t, err)
		_ = decisions.Results[0].Allowed
		// docs:end sdk.access_policy.check

		// docs:begin sdk.access_policy.grants.create
		grant, err := client.AccessPolicy().GrantsCreate(ctx, &models.CreateAuthorizationGrantRequest{
			Audience: models.GrantAudience{
				PrincipalID: goodmem.Ptr("b3303d0a-1a4a-493f-b9bf-38e37153b5a2"),
			},
			Rule: models.AccessPolicyRule{
				Operation: models.OperationREADSPACE,
				Selector:  models.SelectorEXACT,
				AssignedResource: &models.AccessPolicyTarget{
					Kind:       models.ResourceKindSPACE,
					ResourceID: goodmem.Ptr("70e025f6-76ca-4cbe-b8fc-7dab8e84590a"),
				},
			},
		})
		must(t, err)
		// docs:end sdk.access_policy.grants.create

		// docs:begin sdk.access_policy.grants.list
		grants, err := client.AccessPolicy().GrantsList(ctx, &goodmem.AccessPolicyGrantsListParams{
			ResourceKind: goodmem.Ptr("SPACE"),
			ResourceID:   goodmem.Ptr("70e025f6-76ca-4cbe-b8fc-7dab8e84590a"),
		})
		must(t, err)
		_ = grants.Items()
		// docs:end sdk.access_policy.grants.list

		// docs:begin sdk.access_policy.grants.get
		fetchedGrant, err := client.AccessPolicy().GrantsGet(
			ctx,
			"499f5f02-22ac-479f-a913-0e7ee4bb0fb4",
			nil,
		)
		must(t, err)
		// docs:end sdk.access_policy.grants.get

		// docs:begin sdk.access_policy.grants.delete
		revokedGrant, err := client.AccessPolicy().GrantsDelete(
			ctx,
			"499f5f02-22ac-479f-a913-0e7ee4bb0fb4",
		)
		must(t, err)
		// docs:end sdk.access_policy.grants.delete

		// docs:begin sdk.access_policy.role_assignments.create
		assignment, err := client.AccessPolicy().RoleAssignmentsCreate(ctx, &models.AssignRoleRequest{
			PrincipalID: "b3303d0a-1a4a-493f-b9bf-38e37153b5a2",
			Role:        "SPACE_VIEWER",
			AssignedResource: models.RoleAssignmentTarget{
				Kind:       "SPACE",
				ResourceID: goodmem.Ptr("70e025f6-76ca-4cbe-b8fc-7dab8e84590a"),
			},
		})
		must(t, err)
		// docs:end sdk.access_policy.role_assignments.create

		// docs:begin sdk.access_policy.role_assignments.list
		assignments, err := client.AccessPolicy().RoleAssignmentsList(
			ctx,
			&goodmem.AccessPolicyRoleAssignmentsListParams{
				ResourceKind: goodmem.Ptr("SPACE"),
				ResourceID:   goodmem.Ptr("70e025f6-76ca-4cbe-b8fc-7dab8e84590a"),
			},
		)
		must(t, err)
		_ = assignments.Items()
		// docs:end sdk.access_policy.role_assignments.list

		// docs:begin sdk.access_policy.role_assignments.get
		fetchedAssignment, err := client.AccessPolicy().RoleAssignmentsGet(
			ctx,
			"3d74bbb6-bc0b-420d-baa8-b433e9476399",
			nil,
		)
		must(t, err)
		// docs:end sdk.access_policy.role_assignments.get

		// docs:begin sdk.access_policy.role_assignments.delete
		revokedAssignment, err := client.AccessPolicy().RoleAssignmentsDelete(
			ctx,
			"3d74bbb6-bc0b-420d-baa8-b433e9476399",
		)
		must(t, err)
		// docs:end sdk.access_policy.role_assignments.delete

		_ = grant
		_ = assignment
		_ = fetchedGrant
		_ = revokedGrant
		_ = fetchedAssignment
		_ = revokedAssignment
	})

	t.Run("ownership_transfer", func(t *testing.T) {
		t.Skip("ownership-transfer examples are intentionally non-executing")
		// docs:begin sdk.spaces.transfer_ownership
		transferredSpace, err := client.Spaces().TransferOwnership(
			ctx,
			"70e025f6-76ca-4cbe-b8fc-7dab8e84590a",
			&models.TransferOwnershipRequest{
				NewOwnerID: "b3303d0a-1a4a-493f-b9bf-38e37153b5a2",
			},
		)
		must(t, err)
		// docs:end sdk.spaces.transfer_ownership

		// docs:begin sdk.admin.transfer_instance_ownership
		transferredInstance, err := client.Admin().TransferInstanceOwnership(
			ctx,
			&models.TransferOwnershipRequest{
				NewOwnerID: "b3303d0a-1a4a-493f-b9bf-38e37153b5a2",
			},
		)
		must(t, err)
		// docs:end sdk.admin.transfer_instance_ownership

		_ = transferredSpace
		_ = transferredInstance
	})

	// ---- Cleanup (delete snippets) --------------------------------------
	t.Run("memories.delete", func(t *testing.T) {
		requireMemory(t)
		// docs:begin sdk.memories.delete
		err := client.Memories().Delete(ctx, ids["memory"])
		must(t, err)
		// docs:end sdk.memories.delete
	})

	t.Run("spaces.delete", func(t *testing.T) {
		requireSpace(t)
		// docs:begin sdk.spaces.delete
		err := client.Spaces().Delete(ctx, ids["space"])
		must(t, err)
		// docs:end sdk.spaces.delete
	})

	t.Run("rerankers.delete", func(t *testing.T) {
		requireReranker(t)
		// docs:begin sdk.rerankers.delete
		err := client.Rerankers().Delete(ctx, ids["reranker"])
		must(t, err)
		// docs:end sdk.rerankers.delete
	})

	t.Run("llms.delete", func(t *testing.T) {
		requireLLM(t)
		// docs:begin sdk.llms.delete
		err := client.LLMs().Delete(ctx, ids["llm"])
		must(t, err)
		// docs:end sdk.llms.delete
	})

	t.Run("embedders.delete", func(t *testing.T) {
		requireEmbedder(t)
		// docs:begin sdk.embedders.delete
		err := client.Embedders().Delete(ctx, ids["embedder"])
		must(t, err)
		// docs:end sdk.embedders.delete
	})
}
