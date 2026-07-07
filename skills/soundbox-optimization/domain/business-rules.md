# Business Rules

## Core Invariants — Never Violate

1. **QR counters must always be updated.** Even for UC-1, `payments_received_count` and `payments_amount_received` on the QR code entity must be incremented on every successful callback. This is QR lifecycle tracking, not payment processing.

2. **Refunds must always be preserved.** `bharatqr_txn` with `txnType=REFUND` must be written regardless of gateway or UC classification. The EZT DB write skip applies to payment records only.

3. **UC-2 gateways are immutable.** Never apply notification-only logic to UC-2 gateways (HDFC, Axis, Kotak, SBI, YBL, FDC). These merchants rely on Razorpay records.

4. **Idempotency is mandatory.** Duplicate `payment_id` callbacks must not trigger a second counter update, notification, or AuditLog write. The gate is a DynamoDB conditional put (`attribute_not_exists(pk)`) on `AuditLog.Create` — returns `ErrDuplicatePayment` on conflict.

5. **HMAC validation is non-negotiable.** Every QRMS→eze-server request must carry a valid HMAC-SHA256 signature. Reject on validation failure.

6. **UPS intent creation is unchanged (Phase 1).** Even for UC-1, UPS must create and persist the intent entity. NPCI compliance requires it. Phase 2 will remove this dependency.

7. **PGR gets 200 regardless.** QRMS must return 200 to PGR even if notification or audit write fails. Notification is best-effort; PGR must not retry on QRMS errors.

8. **UC classification via Splitz feature flag, not hardcoded.** Whether a merchant is UC-1 is determined at runtime by evaluating the Splitz experiment `soundbox_notification_only` against the `org_code`. No org code list in code. Onboarding a new bank = enabling their org code in the experiment, not a code change.

9. **Processor pattern — no if/else in shared code.** UC-1 and UC-2 flows must be implemented as separate processor implementations behind a common interface (`QRCreationProcessor`, `CallbackProcessor`). The correct processor is injected/selected at the entry point. Skip logic (e.g., skipping EZT DB writes, UPS payment processing) is the natural result of the UC-1 processor simply not making those calls — not a conditional in shared code.

---

## Routing Rules

- `intent.category == "soundbox"` (from UPS preprocess response) → UC-1 path (QRMS)
- Any other category or missing category → existing UPS payment flow
- Unknown gateways → default to UC-2 (full flow); never UC-1 by default

---

## Counter Update Rules

Counter update happens **before** notification:

```
1. Idempotency gate: AuditLog.Create (DynamoDB conditional put) → if payment_id exists, return 200, skip all
2. Update QR code: payments_received_count += 1, payments_amount_received += amount_paise
3. Send notification to eze-server (device_id from payload)
4. Write AuditLog status synchronously (status=delivered or status=failed)
5. Return 200 to PGR
```

If the counter update fails (e.g., DynamoDB error) → log error, return 200 to PGR, write AuditLog with `status=failed`.

---

## Notification Delivery Rules

- Notification to eze-server is **best-effort**: log warning on failure, do not fail the handler
- AuditLog write is **synchronous** (not a goroutine): written after eze-server responds, before returning 200 to PGR
  - On eze-server success → `status=delivered`, `latency_ms` set
  - On eze-server failure → `status=failed`, `failure_reason` set
- `device_id` is provided in the PGR callback payload — no device lookup needed

---

## eze-server Role in Phase 1

Eze-server has **no DB write suppression logic** in Phase 1. UC-1 bank callbacks (JnK Bank, Indian Bank) never reach eze-server as payment events — they flow exclusively through PGR → QRMS.

Eze-server's only Phase 1 role is **notification delivery**: it receives HMAC-signed requests from QRMS and forwards them to the Soundbox device via CommX/EPN. No code changes required in eze-server for Phase 1.

---

## Phase Boundary Rules

- Phase 1 scope is **strictly JnK Bank + Indian Bank** — no other gateways
- Kafka/Callback Processing Queue is **Phase 2 only**
- AWS IoT notification channel is **Phase 2 only** — Phase 1 uses CommX/EPN
- UPS intent generation change (removing UPS) is **Phase 2 only**
