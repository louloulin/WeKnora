// 01-kb-lifecycle — exercises the CRUD surface on /knowledge-bases.
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

	kb, err := client.KnowledgeBase.Create(ctx, weknora.KnowledgeBaseInput{
		Name:        "Engineering KB",
		Description: "Sample KB created by the SDK example.",
		Type:        "rag",
	})
	if err != nil {
		log.Fatalf("create: %v", err)
	}
	fmt.Printf("created kb id=%s name=%s\n", kb.ID, kb.Name)

	if err := client.KnowledgeBase.Delete(ctx, kb.ID); err != nil {
		log.Fatalf("delete: %v", err)
	}
	fmt.Println("deleted kb")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
