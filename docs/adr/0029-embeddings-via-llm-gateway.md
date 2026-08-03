# 0029 — Embeddings via the LLM gateway

- **Status:** Accepted
- **Date:** 2026-08-03

## Context

Global search (ADR-0028) is live as lexical per-app search behind `GET
/_search` — substring or FTS5, the app's choice. Semantic matching ("coffee
maker" finding the espresso page) needs text embeddings, and the pieces
already exist: a Lemonade server runs on the app host serving the audio
gateway (ADR-0014), with `nomic-embed-text-v2-moe` downloaded — the
internal-services catalog lists Embeddings as a Tier-2 candidate via a
future `llm.Embed`. Two invariants constrain the design: apps never call a
model provider directly (ADR-0009 — all inference rides platformd's
gateway), and the app-search spec forbids search providers from calling the
LLM gateway so a fan-out with a 900ms timeout is never blocked behind
completions. Embedding a short query is a different cost class from a
completion (~tens of milliseconds on-host, no model reasoning), so the
blanket prohibition overshoots for this one case. Vector *storage* has its
own constraint: deploys cross-compile with `CGO_ENABLED=0`, so C extensions
like sqlite-vec are unavailable.

## Decision

The LLM gateway grows an embeddings endpoint, and apps consume it only
through `pkg/llm`:

- platformd serves **`POST /llm/embed`** on the internal 4001 plane
  (ADR-0012): request `{app, kind?, texts[]}`, response `{model,
  embeddings[][]}`, one vector per input text, order preserved. Bounded: at
  most 64 texts per call, 8KB per text. `kind` is `"document"` (default) or
  `"query"` — retrieval models treat the two asymmetrically, and the
  gateway applies model-specific task prefixes (nomic's `search_document: `
  / `search_query: `) so apps stay model-blind.
- The backend is **Lemonade's OpenAI-compatible `/embeddings`** endpoint
  (`BESPOKE_LEMONADE_URL`, the same env the audio gateway uses), model from
  `BESPOKE_EMBED_MODEL` (default `nomic-embed-text-v2-moe`). Apps never see
  the backend.
- **No stub mode.** Without a backend the endpoint returns 503 and
  `pkg/llm` surfaces `llm.ErrEmbedUnavailable`; fake vectors would silently
  poison stored indexes. Callers MUST degrade — semantic features switch
  off, lexical paths keep working.
- **`pkg/llm` interface:** `Embed(ctx, texts)` for documents and
  `EmbedQuery(ctx, q)` for queries, both returning `*llm.Embedded{Model,
  Vectors}` — apps store `Model` beside vectors and re-embed on mismatch —
  plus the pure helper `llm.Cosine(a, b)` so every app ranks the same way.
  No options: embeddings are mechanical, never user-brief-tagged.
- **Vectors live in each app's own SQLite** as BLOB columns, embedded at
  write time (and backfilled opportunistically), compared brute-force with
  cosine in Go. No central vector store, no shared index — the same
  sovereignty rule as ADR-0028 — and no sqlite-vec (C extension, breaks the
  CGO-free build). At personal scale (thousands of rows) brute force is
  sub-millisecond.
- **App-search carve-out:** the app-search spec's "no LLM gateway calls"
  rule gains one exception — a search provider MAY make a single
  `llm.Embed` call to embed **the query text only** (the corpus is embedded
  at write time, never during a search request). The provider must still
  answer within platformd's fan-out timeout and fall back to lexical
  results when embeddings are unavailable or slow.

## Consequences

- Semantic and hybrid search become per-app upgrades behind the unchanged
  `/_search` contract, exactly as FTS5 was — no platform change per app.
- One more soft dependency on Lemonade: instances without it lose semantic
  features but nothing else, matching the audio gateway's degradation
  story.
- Write paths that embed acquire a network dependency; apps must treat
  embedding as best-effort (a failed embed never fails the write) and
  tolerate rows with missing vectors.
- Embedding calls are not counted in `/llm/activity` quiesce (ADR-0023):
  they are sub-second and interruption-safe, so deploys need not wait on
  them.
- A future model change (`BESPOKE_EMBED_MODEL`) silently invalidates stored
  vectors — apps MUST store `Embedded.Model` alongside vectors, filter
  ranking to matching-model rows, and let their re-embed sweep converge.

## Alternatives considered

- **Apps call Lemonade directly:** breaks the ADR-0009 invariant that all
  inference flows through the gateway; loses the single point of
  observability, bounding, and config. Rejected.
- **Central platform vector index:** same indexing/invalidation/delete
  protocol problem that killed the central search index in ADR-0028, plus a
  shared store eroding per-app isolation (ADR-0007). Rejected.
- **sqlite-vec (or another vector extension):** C extension; the platform
  cross-compiles with `CGO_ENABLED=0` and the driver stays modernc.
  Rejected on build constraints alone; brute-force cosine suffices at this
  scale.
- **Stub embeddings in dev (like transcription's stub):** a stub transcript
  is visibly fake to a human; a stub vector is invisibly wrong and would be
  persisted. Rejected — absence must be explicit (`ErrEmbedUnavailable`).

## References

- Shapes: [design/llm-gateway.md](../design/llm-gateway.md),
  [design/internal-services.md](../design/internal-services.md),
  [specs/app-search.md](../specs/app-search.md)
- Builds on: [ADR-0007](0007-sqlite-per-app-litestream.md),
  [ADR-0009](0009-copilot-sdk-llm-gateway.md),
  [ADR-0012](0012-internal-services-two-tier.md),
  [ADR-0014](0014-audio-service-transcription.md),
  [ADR-0028](0028-dashboard-global-search-fan-out.md)
