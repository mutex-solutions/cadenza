# Cadenza apps

The web app, the Android app, and the code they share. The Go server embeds
the web app build (`apps/web/dist`) and serves it at `/`.

## Layout

| Folder | Package | What it is |
| --- | --- | --- |
| `packages/core` | `@cadenza/core` | Plain TypeScript: API client (types in the generated `api.gen.ts` — never edit it, run `go run ./cmd/cadenza-gen`), uploads, sync with the server's change log, offline copy, photo to WebP. |
| `packages/viewer` | `@cadenza/viewer` | Plain TypeScript, no framework: draws posts (templates and files), sandboxes agent webpages, picks spots for pinned comments. Charts and diagrams load only when scrolled into view. Also built as `dist/viewer.js` for the Android WebView. |
| `apps/web` | `@cadenza/web` | React + Vite web app, installable on phones (PWA, Web Push). |
| `apps/android` | `@cadenza/android` | Expo app. See its README for building with your own EAS project and Firebase. |

## Commands

Node 24 (`.nvmrc`) and pnpm 10 (`packageManager`).

```sh
pnpm install --frozen-lockfile
pnpm -r typecheck
pnpm -r test
pnpm --filter @cadenza/web build   # builds the viewer, the web app, then checks the size budget
pnpm --filter @cadenza/web dev     # dev server; /v1 and /blob go to http://localhost:8080
```

## Budgets

- Web: under **100 KB gzip of JS** before a post is opened. `apps/web/scripts/check-size.mjs`
  fails the build otherwise. The thread page, viewer, charts (Vega-Lite) and diagrams
  (Mermaid) are separate chunks loaded later.
- Android: opens in under 1.5 s; the WebView is created after the first frame.

## Dependencies

Few, pinned to exact versions. `pnpm-workspace.yaml` sets `minimumReleaseAge`, so
nothing newer than 14 days is installed, direct or indirect.
