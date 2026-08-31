// Command weknora is a CLI for the WeKnora Enterprise Knowledge Platform.
// It is a thin wrapper around the Go SDK and demonstrates how to embed it
// in a downstream service. Run `weknora --help` for the full subcommand
// list.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	weknora "github.com/tencent/weknora-go"
)

const usage = `weknora — official WeKnora CLI

Usage:
  weknora [global flags] <command> [command flags] [args]

Commands:
  kb list                                List knowledge bases
  kb create <name> [--type=wiki|rag|hybrid]
                                         Create a knowledge base
  kb get <id>                            Get a knowledge base
  search <kb_id> <query> [--top-k=5]     Hybrid search a KB
  ask <kb_id> <question>                 One-shot RAG Q&A
  automation create <kb_id> <file>       Create an automation from JSON file
  automation run <kb_id> <auto_id>       Trigger an automation manually
  formula eval <kb_id> <expr>            Evaluate a formula expression
  agents list                            List Custom Agent Studio agents
  agents run <id> <input-json>           Run an agent

Global flags:
  --base-url URL      API base URL (env WEKNORA_BASE_URL, default https://api.weknora.com/v1)
  --token TOKEN       Bearer token (env WEKNORA_TOKEN)
  --api-key KEY       API key (env WEKNORA_API_KEY)
  --json              Emit JSON instead of human-readable text
  --timeout DURATION  Request timeout (default 30s)
`

type globalFlags struct {
	baseURL string
	token   string
	apiKey  string
	json    bool
	timeout time.Duration
}

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	gf := &globalFlags{}
	flag.StringVar(&gf.baseURL, "base-url", envOr("WEKNORA_BASE_URL", "https://api.weknora.com/v1"), "")
	flag.StringVar(&gf.token, "token", os.Getenv("WEKNORA_TOKEN"), "")
	flag.StringVar(&gf.apiKey, "api-key", os.Getenv("WEKNORA_API_KEY"), "")
	flag.BoolVar(&gf.json, "json", false, "")
	flag.DurationVar(&gf.timeout, "timeout", 30*time.Second, "")
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), gf.timeout)
	defer cancel()

	client, err := newClient(ctx, gf)
	if err != nil {
		fatal(err)
	}

	cmd := flag.Arg(0)
	args := flag.Args()[1:]
	if err := dispatch(ctx, client, gf, cmd, args); err != nil {
		fatal(err)
	}
}

func newClient(ctx context.Context, gf *globalFlags) (*weknora.Client, error) {
	opts := []weknora.Option{
		weknora.WithBaseURL(gf.baseURL),
		weknora.WithRetryPolicy(weknora.RetryPolicy{MaxAttempts: 3, InitialBackoff: 200 * time.Millisecond, MaxBackoff: 2 * time.Second}),
	}
	if gf.token != "" {
		opts = append(opts, weknora.WithBearerToken(gf.token))
	} else if gf.apiKey != "" {
		opts = append(opts, weknora.WithAPIKey(gf.apiKey))
	} else {
		return nil, errors.New("--token or --api-key (or WEKNORA_TOKEN / WEKNORA_API_KEY env) is required")
	}
	return weknora.NewClient(ctx, opts...)
}

func dispatch(ctx context.Context, c *weknora.Client, gf *globalFlags, cmd string, args []string) error {
	switch cmd {
	case "kb":
		return kbCmd(ctx, c, gf, args)
	case "search":
		return searchCmd(ctx, c, gf, args)
	case "ask":
		return askCmd(ctx, c, gf, args)
	case "automation":
		return automationCmd(ctx, c, gf, args)
	case "formula":
		return formulaCmd(ctx, c, gf, args)
	case "agents":
		return agentsCmd(ctx, c, gf, args)
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func kbCmd(ctx context.Context, c *weknora.Client, gf *globalFlags, args []string) error {
	if len(args) < 1 {
		return errors.New("kb subcommand required (list|create|get)")
	}
	switch args[0] {
	case "list":
		page, err := c.KnowledgeBase.List(ctx, 50, "")
		if err != nil {
			return err
		}
		return emit(gf, page.Items)
	case "get":
		if len(args) < 2 {
			return errors.New("kb get requires <id>")
		}
		kb, err := c.KnowledgeBase.Get(ctx, args[1])
		if err != nil {
			return err
		}
		return emit(gf, kb)
	case "create":
		fs := flag.NewFlagSet("kb create", flag.ContinueOnError)
		typ := fs.String("type", "rag", "wiki|rag|hybrid")
		desc := fs.String("description", "", "")
		fs.Parse(args[1:])
		if fs.NArg() < 1 {
			return errors.New("kb create requires <name>")
		}
		kb, err := c.KnowledgeBase.Create(ctx, weknora.KnowledgeBaseInput{Name: fs.Arg(0), Type: *typ, Description: *desc})
		if err != nil {
			return err
		}
		return emit(gf, kb)
	default:
		return fmt.Errorf("unknown kb subcommand: %s", args[0])
	}
}

func searchCmd(ctx context.Context, c *weknora.Client, gf *globalFlags, args []string) error {
	if len(args) < 2 {
		return errors.New("usage: search <kb_id> <query> [--top-k=5]")
	}
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	topK := fs.Int("top-k", 5, "")
	fs.Parse(args[2:])
	resp, err := c.Search.Search(ctx, args[0], weknora.SearchRequest{Query: args[1], TopK: *topK, Rerank: true})
	if err != nil {
		return err
	}
	return emit(gf, resp)
}

func askCmd(ctx context.Context, c *weknora.Client, gf *globalFlags, args []string) error {
	if len(args) < 2 {
		return errors.New("usage: ask <kb_id> <question>")
	}
	resp, err := c.Chat.Ask(ctx, args[0], weknora.AskRequest{Question: strings.Join(args[1:], " ")})
	if err != nil {
		return err
	}
	if gf.json {
		return emit(gf, resp)
	}
	fmt.Println(resp.Answer)
	for _, c := range resp.Citations {
		fmt.Printf("  [%s] %s (%.2f)\n", c.ChunkID, c.DocumentTitle, c.Score)
	}
	return nil
}

func automationCmd(ctx context.Context, c *weknora.Client, gf *globalFlags, args []string) error {
	if len(args) < 1 {
		return errors.New("automation subcommand required (create|run)")
	}
	switch args[0] {
	case "create":
		if len(args) < 3 {
			return errors.New("usage: automation create <kb_id> <file.json>")
		}
		data, err := os.ReadFile(args[2])
		if err != nil {
			return err
		}
		var input weknora.AutomationInput
		if err := json.Unmarshal(data, &input); err != nil {
			return err
		}
		auto, err := c.Automation.Create(ctx, args[1], input)
		if err != nil {
			return err
		}
		return emit(gf, auto)
	case "run":
		if len(args) < 3 {
			return errors.New("usage: automation run <kb_id> <auto_id>")
		}
		run, err := c.Automation.Run(ctx, args[1], args[2], nil)
		if err != nil {
			return err
		}
		return emit(gf, run)
	default:
		return fmt.Errorf("unknown automation subcommand: %s", args[0])
	}
}

func formulaCmd(ctx context.Context, c *weknora.Client, gf *globalFlags, args []string) error {
	if len(args) < 2 {
		return errors.New("usage: formula eval <kb_id> <expression>")
	}
	resp, err := c.Formula.Eval(ctx, args[1], weknora.FormulaEvalRequest{
		Expression: strings.Join(args[2:], " "),
	})
	if err != nil {
		return err
	}
	return emit(gf, resp)
}

func agentsCmd(ctx context.Context, c *weknora.Client, gf *globalFlags, args []string) error {
	if len(args) < 1 {
		return errors.New("agents subcommand required (list|run)")
	}
	switch args[0] {
	case "list":
		agents, err := c.AgentStudio.List(ctx)
		if err != nil {
			return err
		}
		return emit(gf, agents)
	case "run":
		if len(args) < 3 {
			return errors.New("usage: agents run <agent_id> <input-json>")
		}
		var input map[string]any
		if err := json.Unmarshal([]byte(args[2]), &input); err != nil {
			return err
		}
		run, err := c.AgentStudio.Run(ctx, args[1], weknora.AgentRunRequest{Input: input})
		if err != nil {
			return err
		}
		return emit(gf, run)
	default:
		return fmt.Errorf("unknown agents subcommand: %s", args[0])
	}
}

func emit(gf *globalFlags, v any) error {
	if gf.json {
		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		enc.SetIndent("", "  ")
		if err := enc.Encode(v); err != nil {
			return err
		}
		_, err := io.Copy(os.Stdout, buf)
		return err
	}
	// Fallback: pretty-print via JSON encoder for human readability too.
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := io.Copy(os.Stdout, buf)
	return err
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "weknora:", err)
	os.Exit(1)
}

// avoid unused-import error in some build configs.
var _ = url.Values{}
