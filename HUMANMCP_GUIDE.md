# humanMCP — Project Guide
## Version 0.3 — April 2026

**Live:** https://kapoost-humanmcp.fly.dev
**Repo:** https://github.com/kapoost/humanmcp-go
**Pages:** https://kapoost.github.io/humanmcp-go
**MCP:** `io.github.kapoost/humanmcp`
**myśloodsiewnia:** https://github.com/kapoost/mysloodsiewnia

---

## What it is

A personal Go server that publishes poems, images, and data with:
- Ed25519 cryptographic signatures (proof of authorship)
- MCP tools so AI agents can read, verify, and license your work
- Local knowledge vault (myśloodsiewnia) via query_vault tool
- Web UI for humans at `kapoost-humanmcp.fly.dev`

---

## Project structure

```
humanmcp-go/
├── cmd/server/main.go          — entry point, shared stores
├── internal/
│   ├── auth/auth.go
│   ├── config/config.go        — VaultURL added in v0.3
│   ├── content/
│   │   ├── content.go, blob.go, messages.go, stats.go
│   │   ├── signing.go, copyright.go, frontmatter.go, cache.go
│   │   ├── skill.go            — SkillStore + PersonaStore
│   │   ├── session.go          — SessionCode z polskiej poezji
│   │   └── memory.go           — MemoryStore z GC (limit 500)
│   ├── mcp/handler.go          — MCP tools, vault tools
│   └── web/
│       ├── handler_web.go      ← THE REAL FILE (not handler.go)
│       └── templates.go
├── deploy.zsh                  — safe deploy (always use this)
├── .github/workflows/
│   ├── deploy.yml              — Go tests + Fly.io deploy
│   └── test_vault.yml          — Python tests (myśloodsiewnia)
└── go.mod
```

---

## Deploy

```zsh
# Pliki z czatu zawsze jako files.zip
cd ~/Downloads && unzip -o files.zip
cd ~/humanmcp/humanmcp
zsh ~/Downloads/<skrypt>.zsh
```

GitHub Actions: push na `main` → testy → deploy (jeśli zielone).

```zsh
fly logs --app kapoost-humanmcp
```

---

## Fly.io

**App:** `kapoost-humanmcp` | **Region:** `ams` | **Volume:** `humanmcp_data` at `/data`

| Secret | Purpose |
|---|---|
| EDIT_TOKEN | Owner login |
| AGENT_TOKEN | Trusted agents write access |
| SIGNING_PRIVATE_KEY | Ed25519 private key (base64) |
| SIGNING_PUBLIC_KEY | Ed25519 public key (hex) |
| VAULT_URL | URL myśloodsiewni lokalnej |
| AUTHOR_BIO, AUTHOR_NAME, DOMAIN | Author data |

---

## MCP Tools

Publiczne: `get_author_profile`, `list_content`, `read_content`, `list_skills`,
`list_personas`, `verify_content`, `get_certificate`, `request_license`,
`leave_comment`, `leave_message`, `about_humanmcp`, `list_blobs`, `read_blob`,
`request_access`, `submit_answer`, **`query_vault`**, **`list_vault`**

Po haśle sesji: `bootstrap_session`, `recall`, `remember`,
`get_skill`, `get_persona`, `upsert_skill`, `upsert_persona`

---

## myśloodsiewnia

Lokalny Python RAG server — osobne repo:
https://github.com/kapoost/mysloodsiewnia

```zsh
vault &                          # start
vs search "VTEC solenoid valve"  # szukaj
open http://localhost:7331/search # UI
```

Vault API contract (3 endpointy = kompatybilność z humanMCP):
```
GET  /health
GET  /documents
POST /query  {"query": "...", "limit": 5}
```

---

## Testy

```zsh
# Go
go test ./... -race

# Python (myśloodsiewnia)
cd ~/mysloodsiewnia && python -m pytest test_mysloodsiewnia.py -v

# E2E (wymaga vault &)
vault & && python -m pytest test_e2e.py -v --headed
```

---

## Znane pułapki

| Problem | Rozwiązanie |
|---|---|
| `handler.go` w `internal/web/` | deploy.zsh go usuwa automatycznie |
| `go build ./... # komentarz` | Go traktuje jako package args |
| Wiele plików z czatu | Zawsze `unzip -o files.zip` przed deploy |
| HTML w Python string | Osobny plik `search_ui.html` |
| `SyntaxError: unterminated string` | HTML poza Python stringiem |
| SessionCode rozjazd | Shared stores w main.go — nie twórz osobnych |

---

## Skille na serwerze

17 skillli w 8 kategoriach — aktualizuj przez `seed_all_skills.zsh` lub `upsert_skill`.

Kategorie: `tech`, `workflow`, `security`, `cars`, `business`, `writing`, `roadmap`

Skille są jedynym źródłem prawdy o aktualnym stanie projektu.
