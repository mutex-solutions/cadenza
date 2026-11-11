# Contributing

## Development

Requires Go 1.26 or later. The web and Android apps need Node 24 and pnpm 10.

```sh
git clone https://github.com/mutex-solutions/cadenza.git
cd cadenza
make check                  # gofmt, go vet, all Go tests with -race
cd web && pnpm install && pnpm -r test
```

Tests need no network and no services: storage tests run on in-memory SQLite, object storage and push providers are
faked with `httptest`. To also run the storage tests on Postgres:

```sh
docker run -d --rm -p 5432:5432 -e POSTGRES_PASSWORD=cadenza -e POSTGRES_DB=cadenza postgres:17
make test-postgres CADENZA_TEST_POSTGRES='postgres://postgres:cadenza@localhost:5432/cadenza?sslmode=disable'
```

## How the code is organised

One Go module, two programs: `cmd/cadenza` (runs on agent machines) and `cmd/cadenza-server`.

| Path | What it is |
|---|---|
| `internal/spec` | The contract: every API call and agent command, defined once as Go types. Routes, CLI commands, MCP tools, TypeScript types and the agent guide are all built from it; tests fail if any of them drift. |
| `internal/kernel` | Ids, error kinds (with exact "fix" text), secrets, clock. |
| `internal/access` | Owner login, machines, pairing codes, session keys. |
| `internal/review` | The core: threads, posts, questions, replies, delivery to agents. |
| `internal/template`, `internal/media` | Pure rules shared by CLI and server: template blocks and validation, file formats and limits. |
| `internal/catalog`, `internal/files`, `internal/notify`, `internal/eventlog` | Template catalog, content-addressed files, notifications, the numbered change log. |
| `internal/adapter/*` | Implementations of the ports above: SQL storage (SQLite and Postgres), S3, local disk, Web Push, Expo, FCM, markdown. |
| `internal/server` | The composition root and HTTP side: API, server-side MCP, embedded web app. |
| `internal/cli` | The agent CLI, including file conversion (`cli/prepare`). |
| `web/` | pnpm workspace: `core` (API client, sync), `viewer` (framework-free renderer), `apps/web`, `apps/android`. |

Rules:

- Domain packages (`access`, `review`, `catalog`, `files`, `notify`, `template`, `media`) import only the standard
  library, `kernel` and each other's types. No SQL, HTTP or SDKs.
- Interfaces live in the package that uses them; adapters implement them.
- Every change to a thread, post, question or reply appends an event in the same transaction.
- Errors that reach agents are `kernel` errors with a plain message and, where possible, the exact fix.

## Generated files

`make gen` regenerates sqlc code (needs [sqlc](https://sqlc.dev)), `postgres/repo.go` (copied from
`sqlite/repo.go`), `web/packages/core/src/api.gen.ts` and the agent guide. Edit the sources, never the output. CI
fails if generated files are stale.

## Pull requests

- Every new or changed piece of code has tests.
- `make check` passes, and the web checks if you touched `web/`.
- Put no secrets in tests. Build synthetic keys by string concatenation so the secret scanner does not flag them.
- Keep dependencies few and pinned to exact versions.

## License

Contributions are licensed under the [Apache License 2.0](LICENSE).
