# Cadenza

**Your coding agents show you their work and ask you questions. You review and answer from your phone or laptop.**

Agents do real work on your machines, but the only way to see it is a long transcript. A screenshot, a short recording,
a page or a chart tells you in seconds. And when an agent needs a decision, it waits idle or guesses.

Cadenza is a tool any coding agent calls:

```sh
cadenza show --title "Checkout redesign" --template ui-change \
  --data '{"summary": "Promo field below the total; one step instead of two.",
           "before": {"file": "./shots/before.png"}, "after": {"file": "./shots/after.png"}}'

cadenza ask "Keep the promo field visible?" --choice yes --choice no --waiting
cadenza wait q_7k2… --timeout 30m      # run in the background; exits with the answer
```

Your phone buzzes with *"Claude Code · vm-03 — Checkout redesign: keep promo field visible?"*. You look at the
before/after, tap **yes**, add a photo, and the agent carries on with both.

- **Any agent.** Claude Code, Codex, Gemini, Hermes, Cursor: anything with a shell runs `cadenza`; MCP clients use
  `cadenza mcp`; cloud agents use MCP on the server.
- **Show, don't tell.** 15 built-in templates (ui-change, bug-fix, test-report, perf-compare, plan, options…),
  custom templates agents make themselves, or plain files: images, video, PDFs, single-file web pages.
- **Answer easily.** Pick a choice, write a note, attach a photo, approve or request changes, pin a comment to a
  line or a spot in a screenshot, sketch.
- **Never blocks an agent.** Asking returns at once; waiting runs in the background with a timeout the agent picks.
- **One inbox** for every machine and agent. Web app and Android app, with notifications only when you're needed.
- **Yours.** Self-hosted, open source (Apache-2.0). Files go straight between your machines, your phone and your
  own S3-compatible storage; the server never touches file bytes.

> **Status:** early release (v0.1). Expect rough edges; issues and pull requests are welcome.

## Try it in a minute

```sh
# 1. Run a server (trial mode: SQLite and files on local disk).
docker run -p 8080:8080 -v cadenza:/data ghcr.io/mutex-solutions/cadenza
#    Open http://localhost:8080 and set your password.

# 2. On each machine where agents run:
curl -fsSL https://raw.githubusercontent.com/mutex-solutions/cadenza/main/install.sh | sh
cadenza login http://localhost:8080      # type the code it shows into the app
cadenza setup                            # teaches Claude Code, Codex and Gemini to use Cadenza
```

For real use (Postgres, S3 storage, HTTPS, Android notifications) see [docs/self-hosting.md](docs/self-hosting.md).
The guide agents read is [docs/cadenza.md](docs/cadenza.md).

## How it works

```
 each machine                         server (Go)                    you
 ─────────────                        ───────────                    ───
 agent ──► cadenza show / ask ──────► API · inbox · notifications ──► web app, Android app
             │  converts files                │ Postgres or SQLite        ▲
             └── uploads by signed link ──► S3-compatible storage ───────┘ (signed links)
```

Each machine is paired once with a short code; each agent session gets its own key, tied to the agent's session
id so resuming keeps its threads. Nothing runs in the background on your machines.

## Testing

- Every pull request runs Go tests (with `-race`, on SQLite and Postgres), the S3 adapter against a real
  SeaweedFS, an end-to-end run of the server and CLI with performance budgets, the web and Android unit tests,
  the Android release build, the server image and a secret scan.
- Real agents were run against a local server: Claude Code completed show → ask → wait → apply → show → done
  through both the `cadenza` command and `cadenza mcp`.
- **The Android app is accepted on the Android emulator only** (Pixel 6a and 7.6" foldable profiles, including
  fold/unfold and half-opened postures). Checks on real phones (performance on a Pixel 6a or Galaxy Z Fold,
  a foldable's two physical displays) are waived: no test device is available. Emulator timings are not
  representative of real hardware.

## Building from source

Go 1.26+, and Node 24 with pnpm 10 for the web app.

```sh
make build        # bin/cadenza and bin/cadenza-server (without the web app)
make web build    # with the web app embedded
make test         # Go tests; web tests: cd web && pnpm -r test
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for how the code is organised.

## License

[Apache License 2.0](LICENSE).
