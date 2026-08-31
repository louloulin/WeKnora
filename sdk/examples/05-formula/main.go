// 05-formula — exercises the Build #32 formula engine via /formula/eval.
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

	resp, err := client.Formula.Eval(ctx, kbID, weknora.FormulaEvalRequest{
		Expression: "price * (1 + tax_rate)",
		Context: map[string]any{
			"price":    100.0,
			"tax_rate": 0.1,
		},
	})
	if err != nil {
		log.Fatalf("eval: %v", err)
	}
	fmt.Printf("formula=%v -> %v (%s)\n", "price * (1 + tax_rate)", resp.Value, resp.Type)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
