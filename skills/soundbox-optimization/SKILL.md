# SoundBox Agent — Skill Graph

Load only what the current task requires. Never load everything at once.

---

## Tier 0 — Always Load First (every task)

```
context/project-overview.md      ← problem, solution, phases, repos
context/architecture.md          ← confirmed flow (two-step UPS, counter update, routing)
context/key-decisions.md         ← log of confirmed decisions
```

~3 pages. Sets the frame. Load before anything else.

---

## Tier 1 — Load by Task Type

### Starting an implementation task

```
implementation/phase1-tasks.md   ← task spec with code, test requirements, dependency order
domain/business-rules.md         ← invariants (counter update order, idempotency, refund rule)
```

### Writing Go code

```
code-patterns/error-handling.md  ← error class + wrap + best-effort patterns
code-patterns/logging.md         ← log levels, structured fields, casing
```

### Adding metrics

```
code-patterns/metrics.md         ← OTel counter/histogram, Init() pattern, RecordXxx helpers
```

### Adding a new service client / config

```
code-patterns/config.md          ← three-struct TOML pattern (Config/Auth/ConnPool)
```

### Writing the HTTP callback handler

```
code-patterns/http-handler.md    ← handler struct, registration, always-200 rule
code-patterns/error-handling.md
domain/business-rules.md         ← counter-update-before-notification order
```

### Writing tests

```
testing/patterns.md              ← ServiceTestSuite, gomock, httptest, table-driven
testing/strategy.md              ← coverage rules, UAT checklist
```

### Checking which gateways / UC classification

```
domain/uc-classification.md      ← UC-1 vs UC-2, gateway matrix, org code note
domain/business-rules.md
```

### Working on DB schema / model / repo

```
domain/db-schema.md              ← audit_log schema + Go model; QR counter fields note
```

### Finding which files to create/modify

```
context/repos-and-files.md       ← per-repo file map, Phase 1 scope
```

### New plan arrived / architecture question

```
context/key-decisions.md
context/architecture.md
domain/tech-spec.md              ← full spec with all approaches and updated flow
```

---

## Tier 2 — Load Only If Directly Needed

| File | Load when |
|------|-----------|
| `domain/tech-spec.md` | Resolving architecture disputes; full spec reference |
| `operations/nfrs.md` | Performance targets, load test design |
| `operations/backward-compat.md` | Planning deploy order or rollback |
| `operations/glossary.md` | Unfamiliar term (PaymentCallback, AuditLog, DeviceMapping, etc.) |
| `implementation/phase2-tasks.md` | Scoping Phase 2 work (placeholder only) |

---

## Loading Examples

### "Implement P1-7 (soundbox callback handler)"

```
Tier 0
implementation/phase1-tasks.md   (P1-7 spec: proto → server.go → SoundboxCallbackProcessor)
domain/business-rules.md         (always-200, idempotency, counter-before-notification)
code-patterns/http-handler.md    (gRPC-Gateway route registration, server.go pattern)
code-patterns/error-handling.md
code-patterns/logging.md
context/repos-and-files.md       (proto files to create/modify, buf generate, proto repo sync)
```

### "Write unit tests for P1-7"

```
Tier 0
testing/patterns.md              (httptest pattern, mock setup)
implementation/phase1-tasks.md   (test cases listed under P1-7)
```

### "Add OTel metrics for counter update and notification"

```
Tier 0
code-patterns/metrics.md
implementation/phase1-tasks.md   (P1-8 spec)
```

### "New spec version arrived"

```
Tier 0 (already loaded)
domain/tech-spec.md
implementation/phase1-tasks.md
context/key-decisions.md         (compare; add new row if decision changed)
```

---

## What NOT to Load Unless Asked

- `domain/tech-spec.md` — heavy; only for architecture debates
- `implementation/phase2-tasks.md` — placeholder; skip unless explicitly scoping Phase 2
- All of `code-patterns/` at once — load only the file relevant to what you're currently writing
- `operations/*` — only for deploy/ops concerns
