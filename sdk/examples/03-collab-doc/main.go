// 03-collab-doc — creates a collab doc, uploads a .docx, then downloads
// it and verifies the round-trip SHA256.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	doc, err := client.CollabDoc.Create(ctx, weknora.CollabDocInput{
		KBID:  kbID,
		Title: "SDK Demo Doc",
		Kind:  "doc",
	})
	if err != nil {
		log.Fatalf("create doc: %v", err)
	}
	fmt.Printf("created doc id=%s\n", doc.ID)

	payload := []byte("fake .docx bytes for the SDK example — replace with real file")
	up, err := client.CollabDoc.UploadBytes(ctx, doc.ID, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", payload)
	if err != nil {
		log.Fatalf("upload: %v", err)
	}
	fmt.Printf("uploaded version=%d sha256=%s\n", up.Version, up.SHA256)

	down, err := client.CollabDoc.DownloadBytes(ctx, doc.ID)
	if err != nil {
		log.Fatalf("download: %v", err)
	}
	sum := sha256.Sum256(down)
	got := hex.EncodeToString(sum[:])
	if got != up.SHA256 {
		log.Fatalf("sha256 mismatch: got %s want %s", got, up.SHA256)
	}
	fmt.Println("round-trip verified ✓")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
