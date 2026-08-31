// 04-automation — creates a row-changed automation that posts a webhook,
// then manually triggers it to confirm the DAG executes.
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
	databaseID := getEnv("WEKNORA_DB_ID", "demo-db")

	auto, err := client.Automation.Create(ctx, kbID, weknora.AutomationInput{
		DatabaseID:  databaseID,
		Name:        "Send Slack on row change",
		TriggerType: weknora.TriggerRowChanged,
		TriggerConfig: map[string]any{
			"watch_column": "status",
		},
		Steps: []weknora.AutomationStep{{
			ID:         "notify",
			ActionType: weknora.ActionNotify,
			Config: map[string]any{
				"channel": "#weknora",
				"message": "Row {{row.id}} changed status to {{row.status}}",
			},
		}},
	})
	if err != nil {
		log.Fatalf("create: %v", err)
	}
	fmt.Printf("created automation id=%s\n", auto.ID)

	run, err := client.Automation.Run(ctx, kbID, auto.ID, map[string]any{
		"row_id": "row-1",
		"values": map[string]any{"status": "approved"},
	})
	if err != nil {
		log.Fatalf("run: %v", err)
	}
	fmt.Printf("run id=%s status=%s\n", run.ID, run.Status)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
