# WeKnora SDK Examples

Five end-to-end examples that exercise the Go SDK against a local WeKnora
 server. Run with `go run sdk/examples/<name>/main.go` and supply the
 `WEKNORA_BASE_URL` + `WEKNORA_TOKEN` (or `--base-url` / `--token`) env vars.

| Example | What it demonstrates |
|---|---|
| `01-kb-lifecycle` | KB create → get → list → patch → delete |
| `02-document-search` | Hybrid search + ask RAG question with citations |
| `03-collab-doc` | Create a collab doc + upload/download a .docx |
| `04-automation` | Create a row-changed automation + manually trigger it |
| `05-formula` | Evaluate a Build #32 formula expression |
