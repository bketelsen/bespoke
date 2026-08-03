# Spec: App search endpoint (`GET /_search`)

The optional `GET /_search?q=` endpoint lets an app contribute results to the
dashboard's global search and the platform-owned `search` MCP/chat tool.
platformd fans out to every registered app's endpoint, forwarding the caller's
identity; the app returns its own user-scoped matching results as JSON.
Consumed by platformd; implemented by apps via `web.Search(mux, provider)`.
Motivated by [ADR-0028](../adr/0028-dashboard-global-search-fan-out.md).

## Interface

`GET /_search?q=<query>` — authenticated (behind `auth.Middleware`; identity
forwarded by platformd exactly as for `/_card`). Registered with:

```go
web.Search(mux, func(ctx context.Context, user auth.User, q string) ([]web.SearchResult, error) { ... })
```

Response body is JSON:

```json
{ "results": [
  { "title": "Buy milk", "snippet": "due tomorrow · high", "url": "/task/42", "timestamp": "2026-08-03T14:00:00Z" }
] }
```

Each result:

| Field | Type | Required | Constraints |
| --- | --- | --- | --- |
| `title` | string | yes | Human-readable label for the hit |
| `snippet` | string | no | Matched text / context; rendered as plain text, keep it short (a line or two) — oversized responses are dropped |
| `url` | string | no | App-relative path; platformd resolves to the app's base URL. Omitted or `/` means "home" |
| `timestamp` | string (RFC 3339) | no | Used only for the app's own ordering |

## Rules

- The endpoint MUST return only rows belonging to the authenticated user
  (`WHERE login=?` or equivalent). It MUST NOT return another user's data.
- The app MAY choose any matching strategy (substring `LIKE` for v1, FTS5
  later). The contract is "user-scoped results matching `q`", agnostic to how.
- An empty or whitespace-only `q` SHOULD return an empty `results` array.
- `url` SHOULD be a specific deep link to the matching item
  (**preferred/best-effort**); a bare home URL (`/`) is an acceptable fallback
  when the app has no per-item route yet.
- The response MUST be valid JSON with a top-level `results` array (possibly
  empty). Malformed, slow (beyond platformd's timeout), oversized (beyond
  platformd's cap), or absent responses cause the app to be dropped from
  results — never an error surfaced to the user.
- Providers MUST be cheap database queries; they MUST NOT run LLM
  completions or call other network services. Sole carve-out
  ([ADR-0029](../adr/0029-embeddings-via-llm-gateway.md)): one `llm.Embed`
  call to embed **the query text only** — corpus embedding happens at write
  time, never during a search request — and the provider MUST still answer
  within platformd's timeout, falling back to lexical results when
  embeddings are unavailable (`llm.ErrEmbedUnavailable`) or slow.
- An app that does not register `/_search` is silently absent from search.

## Derived artifacts

| Artifact | Derivation |
| --- | --- |
| Dashboard search results | platformd groups each app's `results` under the app's manifest name |
| `search` MCP/chat tool output | Same grouped-by-app structure, with resolved deep URLs |

## References

- Rationale: [ADR-0028](../adr/0028-dashboard-global-search-fan-out.md);
  query-embedding carve-out:
  [ADR-0029](../adr/0029-embeddings-via-llm-gateway.md)
- Context: [design/internal-services.md](../design/internal-services.md),
  [specs/app-manifest.md](app-manifest.md)
