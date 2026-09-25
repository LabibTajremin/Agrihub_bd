# Localization (text and voice)

## Model
- **Languages (7):** `bn` (Bangla, default), `en`, `hi`, `es`, `fr`, `ar` (**RTL**), `pt`.
  Each language record carries `is_rtl`; clients derive layout direction from that flag only.
- **Dictionary:** one key→value map per language. Keys follow `screen.section.element`
  (`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`), e.g. `home.quickscan.title`.
  Table `dictionary_entries(lang_code, namespace, key, value, version, updated_at)`,
  PK `(lang_code, namespace, key)`; `namespace` is the first key segment.
- **Voice:** same keys, pre-recorded clips instead of strings:
  `voice_assets(lang_code, key, media_id, url, duration_ms, checksum, version)`.
  No TTS, no STT, no inference — clips are recorded by people and registered by an admin.
- **Versions:** every write takes `nextval('localization_version_seq')`, so versions are monotonic
  per language; unchanged values keep their version (seeding is idempotent).

## Serving
| Endpoint | Behaviour |
|---|---|
| `GET /v1/i18n/languages` | `code, name, native_name, is_rtl, is_default, has_voice, version` |
| `GET /v1/i18n/{lang}` | full snapshot, strong `ETag`, `Cache-Control: public, max-age=300`, `304` on `If-None-Match` |
| `GET /v1/i18n/{lang}?since=V` | delta: only entries with `version > V` (O(changed)) |
| `GET /v1/voice/{lang}[?since=V]` | manifest `key → {url, duration_ms, checksum}` with the same caching rules |
| `PUT /v1/i18n/{lang}/entries` | admin (`dictionary:write`) upsert |
| `PUT /v1/voice/{lang}/assets` | admin (`voice:write`) clip registration |

### In-memory catalog (§6.1)
`domain.Catalog` holds an immutable `map[lang]*Snapshot` behind `atomic.Pointer`. Lookups are two map
reads — O(1), lock-free, zero allocations (`BenchmarkCatalogLookup`: ~6 ns/op, 0 allocs/op;
`TestCatalog_LookupIsAllocationFree` asserts it). An update copies the outer map and swaps the
pointer with compare-and-swap; readers never block. The full-snapshot endpoint compares the cached
version with the stored one (one indexed `max(version)` query) and rebuilds on change, so several
instances converge without coordination.

## Seed dictionaries
- Canonical files: `backend/internal/modules/localization/seed/{lang}.json` (embedded; loaded by
  `make seed`). `mobile/assets/i18n/` holds byte-identical copies — run `make i18n-sync` after
  editing; `TestSeed_MobileCopiesAreIdentical` fails otherwise.
- `TestSeed_SevenLanguagesSameKeysValidFormat` asserts: 7 languages, identical key sets, valid key
  format, non-empty values, matching `{placeholder}` counts.
- Error messages are keys too: every API error `code` has `errors.<code>` in every dictionary.

## Adding a string
1. Add the key to all seven JSON files (same key, translated value).
2. `make i18n-sync`, then `make verify`.
3. Deploy + `make seed` (or `PUT /v1/i18n/{lang}/entries`): clients receive it as a delta.
