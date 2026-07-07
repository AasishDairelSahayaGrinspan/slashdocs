# Testing Strategy

---

## Coverage Requirement

90% unit test coverage on all new Go files. No exceptions for handler and service layers.

---

## Unit Tests

Each Phase 1 task has unit test requirements inline in `implementation/phase1-tasks.md`. General rules:

- Mock all external dependencies: DB, Kafka, HTTP clients, eze-server
- Table-driven tests for routing/whitelist logic
- Test happy path AND all failure/edge cases:
  - Duplicate `payment_id` (DynamoDB conditional put returns `ErrDuplicatePayment`)
  - eze-server timeout / non-2xx
  - QR code is closed (`AtomicIncrementPaymentCounters` condition fails)
  - Invalid/missing required fields

---

## Integration Tests

- `qr-codes` callback handler E2E: POST payload → assert `soundbox_audit_logs` row written + ezetap mock called
- `payments-upi` early-exit: assert `soundbox` category callback returns success without touching any downstream service
- PGR routing: `soundbox` category → QRMS forwarded; other categories → existing UPS flow untouched

---

## Load / Performance Tests

- Sustained: 3–4M txns/month ≈ ~1.5 txns/second average; design for 10x burst
- Target: notification latency p95 < 3s, p99 < 5s
- Validate no DynamoDB connection pool exhaustion under burst

---

## UAT Checklist

- [ ] JnK Bank test payment → sound plays on device, `soundbox_audit_logs` row created with `status=delivered`, NO payment entity in DB
- [ ] Indian Bank test payment → same as above
- [ ] Existing gateway (e.g., Axis BharatQR) → payment entity created normally, no regression
- [ ] Duplicate `payment_id` replay → idempotent, only one `soundbox_audit_logs` row, no double notification
- [ ] PGR kill switch OFF → payment goes through existing UPS flow, no `soundbox_audit_logs` entry
- [ ] PGR kill switch ON → `soundbox` category routed to QRMS, `soundbox_audit_logs` entry created
- [ ] PGR correctly routes all other categories to existing UPS flow (zero regression)

---

## Rollback Plan

- New flows are gated by `NotificationOnlyOrgCodes` constant
- Remove an org code from the constant → all its callbacks fall through to existing flow
- No DB schema changes are destructive; `soundbox_audit_logs` DynamoDB table is additive
- eze-server change is a conditional skip — reverting removes the skip
