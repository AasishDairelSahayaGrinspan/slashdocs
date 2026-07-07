# SoundBox SQR Cost Reduction — Agent Entry Point

> **Objective:** Implement the SoundBox & SQR Sticker Cost Reduction initiative end-to-end.
> For each task: plan → implement → test → raise PR, repo by repo.

---

## What This Agent Does

This agent's primary job is to **implement the SoundBox notification optimization** — reducing per-transaction cost from 14.21p to ~>3p by routing notification-only callbacks directly to QRMS and skipping full UPS payment processing.

Given a task (e.g. "implement P1-7" or "implement Phase 1"), the agent will:

1. **Plan** — load the relevant context, confirm scope, list files to create/modify, call out risks
2. **Implement** — write production-ready Go (or Java) code following existing codebase patterns
3. **Test** — write unit tests (90% coverage), verify logic with table-driven cases
4. **PR** — produce one PR per repo with a structured description; never mix repo changes in one PR

---

## Agent Workflow

### Step 1 — Plan

Before writing any code, the agent MUST:

```
1. Load SKILL.md → identify which files to load for this task
2. Load context/architecture.md → confirm the approach is still valid
3. Load implementation/phase1-tasks.md (or phase2-tasks.md) → find the task spec
4. Load domain/business-rules.md → note invariants that must not be broken
5. Load context/repos-and-files.md → identify which repos and files are touched
6. Output a written plan:
   - Which repos are changed
   - Which files are created vs modified
   - Any risks or blockers
   - Confirm with user before writing code if scope is ambiguous
```

### Step 2 — Implement (repo by repo)

Work through one repo at a time in dependency order (see phase1-tasks.md dependency graph):

```
For each repo:
  1. Read existing files before editing (never write blind)
  2. Follow patterns from code-patterns/ for the file type being written
  3. Add config to default.toml if adding a new service client
  4. Register new handlers/consumers in boot.go
  5. Run: go build ./... to confirm no compile errors
```

### Step 3 — Test

```
For each new file:
  1. Create <filename>_test.go alongside it
  2. Use ServiceTestSuite + gomock pattern (see testing/patterns.md)
  3. Cover: happy path, all error/edge cases, idempotency, best-effort failures
  4. Run: go test ./... -cover
  5. Confirm coverage ≥ 90% for new files
```

### Step 4 — PR (one per repo)

Raise a separate PR for each repo touched. PR description must include:

```markdown
## What
One-line summary of what changed.

## Why
Link to initiative: SoundBox SQR Cost Reduction (14.21p → ~1p/txn)
Task reference: P1-X

## Changes
- `path/to/file.go` — what it does
- `path/to/other.go` — what changed

## Testing
- Unit tests: TestXxx covers happy path, duplicate request, ezetap failure
- How to test manually (if applicable)

## Deploy note
Must be deployed before / after: <other repo> (see operations/backward-compat.md)
```

---

## Repo-by-Repo PR Order (Phase 1)

Deploy in this order to avoid downtime:

| Order | Repo | What changes | PR depends on |
|-------|------|-------------|---------------|
| 1 | `qr-codes` | Callback handler, ezetap client, audit table | — |
| 2 | `eze-server` | DB write suppression for UC-1 gateways | — |
| 3 | `payments-upi` | Early-exit safety net for `sound_box` category | — |
| 4 | `pg-router` | Route `sound_box` callbacks to QRMS | `qr-codes` must be live first |

---

## How to Navigate This Agent

| Directory | What's inside |
|-----------|---------------|
| `context/` | Project overview, confirmed architecture, repo scope, key decisions |
| `domain/` | Tech spec, UC classification, business rules, DB schema |
| `implementation/` | Phase 1 and Phase 2 task breakdowns with code-level instructions |
| `code-patterns/` | Go patterns from the qr-codes codebase (errors, logging, metrics, config, HTTP handlers) |
| `testing/` | Testing strategy, unit test patterns (gomock/testify), UAT checklist |
| `operations/` | NFRs, backward compatibility, glossary |

See `SKILL.md` for the skill graph — which files to load for each task type.

---

## Quick Facts

- **Module:** `github.com/razorpay/qr-codes`
- **Primary repo:** `/Users/akash.mukherjee/go/src/github.com/razorpay/qr-codes`
- **Phase 1 target gateways:** JnK Bank, Indian Bank
- **Cost target:** 14.21p → ~1p per txn
- **Architecture decided:** Category/Source-based Routing (see `context/architecture.md`)
