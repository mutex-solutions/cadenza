# Vizo

**Show your agents' work, review it, and unblock them — from any device.**

Vizo is a fast web surface where AI agents publish what they've made — code, diffs,
diagrams, pages, video — and where you review it and answer their questions, on your
phone or laptop. It is built for an agent-first workflow: the agents do the work
locally, and Vizo is where *you* look, judge, and steer.

> You are the lock. Agents run free, but at the sections that matter they must acquire
> your review before proceeding — and they block (or keep working async) until you
> release them with feedback.

## What it is

- A **review surface**, not an editor. You don't write code here; you look at what the
  agent made and respond.
- A **human-in-the-loop protocol**. Agents call `show(...)` to publish an artifact and
  `ask(...)` to request your judgment, anchored to a specific region. You reply; they
  `poll(...)` and unblock.
- **Device-agnostic and performance-first.** Highly optimized for both desktop and
  mobile, so review from your pocket is a first-class experience.

## Viewers (what an agent can "show")

- **Diff / code review** — line-anchored, grouped by intent.
- **Single file with annotations** — the agent pins its own uncertainty to line ranges.
- **Directory / repo** — read-only tree + syntax highlight, scoped to an allowlisted root.
- **Diagram** — Mermaid (native), Excalidraw / tldraw for freeform.
- **Webpage** — sandboxed live page, comments anchored to DOM elements.
- **Video** — screen recordings, comments anchored to timestamps.
- **Markdown / plan** — the agent's writeup, with approve / annotate.

All viewers share one primitive: **region-anchored comment + approve**.

## The feedback loop

```
agent (local) ──show(artifact)──►  Vizo surface (edge)  ◄──review──  you (phone/laptop)
      ▲                                                                    │
      └──────────────────────────  poll(answer) / unblock  ◄──────────────┘
```

Agent-side API (delivered as an MCP server):

```
show(kind, payload, {root})            -> handle      # file | diff | dir | diagram | page | video | md
annotate(handle, region, note)                        # the agent's own margin notes
ask(question, {handle?, region?, mode, choices?}) -> requestId   # mode: "block" | "async"
poll(requestId)   -> "pending" | { decision, text, annotations[] }
```

## Guardrails & security

- Only the **local bridge** touches the filesystem; the edge and web client never do.
- The bridge is configured with allowed `roots`, `maxFileBytes`, and redaction rules;
  anything outside the roots is refused before it leaves the machine.
- Owner auth via **WebAuthn passkey**; the bridge authenticates with a one-time
  pre-shared key, then short-lived signed tokens.
- Artifacts are **ephemeral by default** (TTL).

## Architecture (proposed)

- **Agent bridge** — local daemon + MCP server; has FS access, enforces the root
  allowlist, does syntax highlighting / diffing before pushing.
- **Edge surface** — Cloudflare Workers + a Durable Object per review space, holding live
  request state and fanning out updates over WebSocket/SSE.
- **Web client** — lightweight framework (Svelte / Solid), virtualized rendering, lazy
  directory loading.

## Status

Early — name and concept locked, scoping the first build. See the thin-MVP plan (coming).

---

Part of [mutex-solutions](https://github.com/mutex-solutions).
