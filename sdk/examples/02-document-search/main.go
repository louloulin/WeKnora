// 02-document-search — runs hybrid search and a one-shot RAG ask.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	weknora "github.com/tencent/weknora-go"
)

func main() {
	ctx := context.Background()
	client, err := weknora.NewClient(ctx,
		weknora.WithBaseURL(getEnv("WEKNORA_BASE_URL", "http://localhost:8080/api/v1")),
		weknora.WithBearerToken(getEnv("WEKNORA_TOKEN", "dev")),
	)
	if err != nil {
		log.Fatalf("client: %v", err)
	}
	kbID := getEnv("WEKNORA_KB_ID", "demo-kb")

	hits, err := client.Search.Search(ctx, kbID, weknora.SearchRequest{Query: "vector search", TopK: 5, Rerank: true})
	if err != nil {
		log.Fatalf("search: %v", err)
	}
	fmt.Printf("hits=%d\n", len(hits.Hits))
	for _, h := range hits.Hits {
		fmt.Printf("  - %s (%.3f)\n", h.DocumentTitle, h.Score)
	}

	ask, err := client.Chat.Ask(ctx, kbID, weknora.AskRequest{Question: "What is pgvector?"})
	if err != nil {
		log.Fatalf("ask: %v", err)
	}
	fmt.Printf("answer: %s\n", ask.Answer)
	for _, c := range ask.Citations {
		fmt.Printf("  cite: %s — %s\n", c.DocumentTitle, c.Text)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
