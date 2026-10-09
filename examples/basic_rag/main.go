// Building a Basic RAG Agent with GoodMem — Go SDK.
//
// Snippet markers are synced into docs/website/content/docs/how-to/basic-rag.mdx
// by clients/_docs_gen/sync_rag_snippets.py, which rewrites the in-repo import
// path to the published Gemfury path (github.com/PAIR-Systems-Inc/goodmem-go), mirroring
// clients/go/publish.sh.
// [snippet:setup]
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	goodmem "github.com/PAIR-Systems-Inc/goodmem-go"
	"github.com/PAIR-Systems-Inc/goodmem-go/models"
)

func main() {
	// Set up variables
	goodmemBaseURL := os.Getenv("GOODMEM_BASE_URL")
	goodmemAPIKey := os.Getenv("GOODMEM_API_KEY")
	openAIAPIKey := os.Getenv("OPENAI_API_KEY")

	client, err := goodmem.New(goodmemBaseURL, goodmemAPIKey)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	// [/snippet:setup]

	// ----- Step 1.1: Register the embedder -----
	fmt.Println("\n--- Step 1.1: Register the embedder ---")
	// [snippet:register-embedder]
	embedderResp, err := client.Embedders().Create(ctx, &models.EmbedderCreationRequest{
		DisplayName:     "OpenAI test",
		ModelIdentifier: "text-embedding-3-large",
	}, openAIAPIKey)
	if err != nil {
		log.Fatal(err)
	}

	embedderID := embedderResp.EmbedderID
	fmt.Printf("Embedder ID: %s\n", embedderID)
	// [/snippet:register-embedder]

	// ----- Step 1.2: Register the LLM -----
	fmt.Println("\n--- Step 1.2: Register the LLM ---")
	// [snippet:register-llm]
	llmResp, err := client.LLMs().Create(ctx, &models.LLMCreationRequest{
		DisplayName:     "My GPT-5.1",
		ModelIdentifier: "gpt-5.1",
	}, openAIAPIKey)
	if err != nil {
		log.Fatal(err)
	}

	llmID := llmResp.LLM.LLMID
	fmt.Printf("LLM ID: %s\n", llmID)
	// [/snippet:register-llm]

	// ----- Step 2.1: Create a space -----
	fmt.Println("\n--- Step 2.1: Create a space ---")
	// [snippet:create-space]
	space, err := client.Spaces().Create(ctx, &models.SpaceCreationRequest{
		Name: "Goodmem test",
		SpaceEmbedders: []models.SpaceEmbedderConfig{
			{EmbedderID: embedderID},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	spaceID := space.SpaceID
	fmt.Printf("Space ID: %s\n", spaceID)
	// [/snippet:create-space]

	// ----- Step 2.2.1: Ingest plain text memory -----
	fmt.Println("\n--- Step 2.2.1: Ingest plain text memory ---")
	// [snippet:ingest-text]
	memory, err := client.Memories().Create(ctx, &models.JSONMemoryCreationRequest{
		SpaceID: spaceID,
		OriginalContent: goodmem.Ptr(
			"Transformers are a type of neural network architecture that are particularly well-suited for natural language processing tasks. A Transformer model leverages the attention mechanism to capture long-range dependencies in the input sequence.",
		),
	})
	if err != nil {
		log.Fatal(err)
	}

	memoryID := memory.MemoryID
	fmt.Printf("Memory ID: %s\n", memoryID)
	fmt.Printf("Processing Status: %s\n", memory.ProcessingStatus)
	// [/snippet:ingest-text]

	// ----- Check memory status (snippet for tutorial) -----
	// [snippet:check-memory-status]
	memStatus, err := client.Memories().Get(ctx, memoryID, &goodmem.MemoriesGetParams{
		IncludeProcessingHistory: goodmem.Ptr(true),
	})
	if err != nil {
		log.Fatal(err)
	}
	for memStatus.ProcessingStatus == "PENDING" || memStatus.ProcessingStatus == "PROCESSING" {
		time.Sleep(2 * time.Second)
		memStatus, err = client.Memories().Get(ctx, memoryID, nil)
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Printf("Memory ID: %s\n", memStatus.MemoryID)
	fmt.Printf("Processing Status: %s\n", memStatus.ProcessingStatus)
	// [/snippet:check-memory-status]

	// ----- Step 2.2.2: Ingest PDF memory (optional) -----
	fmt.Println("\n--- Step 2.2.2: Ingest PDF memory ---")
	if _, statErr := os.Stat("employee_handbook.pdf"); statErr == nil {
		// [snippet:ingest-pdf]
		pdfMemory, err := client.Memories().CreateFromFile(ctx, "employee_handbook.pdf",
			&models.JSONMemoryCreationRequest{
				SpaceID:           spaceID,
				Metadata:          map[string]any{"source": "FILE"},
				ExtractPageImages: goodmem.Ptr(true),
			})
		if err != nil {
			log.Fatal(err)
		}

		pdfMemoryID := pdfMemory.MemoryID
		fmt.Printf("Memory ID: %s\n", pdfMemoryID)

		pdfStatus, err := client.Memories().Get(ctx, pdfMemoryID, nil)
		if err != nil {
			log.Fatal(err)
		}
		for pdfStatus.ProcessingStatus == "PENDING" || pdfStatus.ProcessingStatus == "PROCESSING" {
			time.Sleep(2 * time.Second)
			pdfStatus, err = client.Memories().Get(ctx, pdfMemoryID, nil)
			if err != nil {
				log.Fatal(err)
			}
		}
		fmt.Printf("Processing Status: %s\n", pdfStatus.ProcessingStatus)
		// [/snippet:ingest-pdf]
	} else {
		fmt.Println("Skipping PDF ingest (employee_handbook.pdf not found)")
	}

	// ----- Step 3: Spin the RAG agent -----
	fmt.Println("\n--- Step 3: Spin the RAG agent ---")
	// [snippet:rag-agent]
	stream, err := client.Memories().Retrieve(ctx, "What do you know about AI?",
		&goodmem.RetrieveOptions{
			SpaceIDs: []string{spaceID},
			LLMID:    &llmID,
		})
	if err != nil {
		log.Fatal(err)
	}

	defer stream.Close()
	for event, err := range stream.Events() {
		if err != nil {
			log.Fatal(err)
		}
		eventJSON, _ := json.MarshalIndent(event, "", "  ")
		fmt.Println(string(eventJSON))
	}
	// [/snippet:rag-agent]

	// ----- Step 3b: RAG agent with token budget -----
	fmt.Println("\n--- Step 3b: RAG agent with increased token budget ---")
	// [snippet:rag-agent-token-budget]
	stream, err = client.Memories().Retrieve(ctx, "What do you know about AI?",
		&goodmem.RetrieveOptions{
			SpaceIDs:       []string{spaceID},
			LLMID:          &llmID,
			GenTokenBudget: goodmem.Ptr(int32(2048)),
		})
	if err != nil {
		log.Fatal(err)
	}

	defer stream.Close()
	for event, err := range stream.Events() {
		if err != nil {
			log.Fatal(err)
		}
		eventJSON, _ := json.MarshalIndent(event, "", "  ")
		fmt.Println(string(eventJSON))
	}
	// [/snippet:rag-agent-token-budget]

	// ----- Step 4: Reranker (optional) -----
	if os.Getenv("VOYAGE_API_KEY") == "" {
		fmt.Println("Skipping reranker steps (VOYAGE_API_KEY not set)")
	} else {
		// ----- Step 4.1: Register the reranker -----
		fmt.Println("\n--- Step 4.1: Register the reranker ---")
		// [snippet:register-reranker]
		voyageAPIKey := os.Getenv("VOYAGE_API_KEY")
		reranker, err := client.Rerankers().Create(ctx, &models.RerankerCreationRequest{
			DisplayName:     "Voyage rerank-2.5",
			ModelIdentifier: "rerank-2.5",
		}, voyageAPIKey)
		if err != nil {
			log.Fatal(err)
		}

		rerankerID := reranker.RerankerID
		fmt.Printf("Reranker ID: %s\n", rerankerID)
		// [/snippet:register-reranker]

		// ----- Step 4.2: RAG agent with reranker -----
		fmt.Println("\n--- Step 4.2: Spin the RAG agent with reranker ---")
		// [snippet:rag-agent-reranker]
		stream, err = client.Memories().Retrieve(ctx, "What do you know about AI?",
			&goodmem.RetrieveOptions{
				SpaceIDs:   []string{spaceID},
				LLMID:      &llmID,
				RerankerID: &rerankerID,
			})
		if err != nil {
			log.Fatal(err)
		}

		defer stream.Close()
		for event, err := range stream.Events() {
			if err != nil {
				log.Fatal(err)
			}
			eventJSON, _ := json.MarshalIndent(event, "", "  ")
			fmt.Println(string(eventJSON))
		}
		// [/snippet:rag-agent-reranker]

		if err := client.Rerankers().Delete(ctx, rerankerID); err != nil {
			log.Fatal(err)
		}
		fmt.Println("  Deleted reranker " + rerankerID)
	}

	// ----- Cleanup -----
	fmt.Println("\n--- Cleanup ---")
	if err := client.Spaces().Delete(ctx, spaceID); err != nil {
		log.Fatal(err)
	}
	fmt.Println("  Deleted space " + spaceID)
	if err := client.LLMs().Delete(ctx, llmID); err != nil {
		log.Fatal(err)
	}
	fmt.Println("  Deleted LLM " + llmID)
	if err := client.Embedders().Delete(ctx, embedderID); err != nil {
		log.Fatal(err)
	}
	fmt.Println("  Deleted embedder " + embedderID)
	fmt.Println("\n--- Done ---")
}

func waitForMemoryReady(ctx context.Context, client *goodmem.Client, memoryID string, label string) {
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		m, err := client.Memories().Get(ctx, memoryID, nil)
		if err != nil {
			log.Fatal(err)
		}
		if m.ProcessingStatus == "COMPLETED" {
			fmt.Printf("  %s %s ready (COMPLETED).\n", label, memoryID)
			return
		}
		if m.ProcessingStatus == "FAILED" {
			log.Fatalf("%s processing FAILED", label)
		}
		fmt.Printf("  %s status: %s, waiting 2s...\n", label, m.ProcessingStatus)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("%s did not reach COMPLETED within 120s", label)
}
