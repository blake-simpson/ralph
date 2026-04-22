# Belmont Learnings — Superseded

This file has been replaced by the **`knowledge/`** tree. Start at [`knowledge/KNOWLEDGE.md`](knowledge/KNOWLEDGE.md) — it's an index that routes you to self-contained topic entries grouped by domain.

## Why the move

The append-only log-of-sessions shape was write-optimised and read-expensive: every session added more prose regardless of domain, so every future agent had to read the whole file to find the parts that matched their task. The `knowledge/` tree is retrieval-optimised — domain-separated, one topic per file, amended in place, ~200 lines per entry. Cross-topic chronology comes from `git log -- knowledge/` rather than from a global log file.

Content that used to live here has been distilled into these entries:

- `knowledge/auto-mode/scope-guard-runtime.md`
- `knowledge/auto-mode/parallel-wave-orchestration.md`
- `knowledge/auto-mode/verify-evidence.md`
- `knowledge/cross-cutting/milestone-immutability.md`
- `knowledge/cross-cutting/port-isolation.md`
- `knowledge/cross-cutting/steering.md`
- `knowledge/meta/validated-runs.md`

No information was lost in the migration; rewrites amended rather than replaced.

## Do not reopen this file

If you're tempted to add an entry here to "match the old format," stop — add to the appropriate `knowledge/<domain>/<topic>.md` entry's body + `Revisions` footer instead. If the concept doesn't fit any existing entry, create a new one and add a row to `knowledge/KNOWLEDGE.md`. The old per-session append pattern is the thing we're actively moving away from.

File kept as a pointer so existing links and historical references don't 404.
