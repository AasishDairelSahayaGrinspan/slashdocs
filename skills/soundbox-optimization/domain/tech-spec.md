# Tech Spec — SoundBox SQR Cost Reduction

> Source: Tech Spec v2 (updated Jun 2026) + design review MoM 2026-06-30
> Section 8 of the spec is the authoritative flow description.

---

## Background

SoundBox Static QR transactions generate 4–6 DB writes and full UPS payment processing per transaction. For bank-led merchants (JnK Bank, Indian Bank), the bank owns the transaction record and Razorpay's role is purely notification delivery and QR counter tracking. The full payment processing pipeline is unnecessary.

---

## Domain Entities (from spec Section 6)

| Entity | Description |
|--------|-------------|
| **PaymentCallback** | Raw event from UPI PSP via gateway. Contains transaction ref, amount, device ID. |
| **AuditLog** | Immutable record of notification lifecycle events and delivery status. Replaces EZT transaction records for soundbox merchants. |
| **DeviceMapping** | Lookup entity resolving SoundBox device IDs to their active notification endpoint (CommX vs AWS IoT). |
| **CallbackEvent** | Message schema for the Callback Processing Queue (Phase 2). Facilitates async processing between ingestion and QRMS. |

---

## Current Architecture (as-is)

When a customer pays via SQR:
1. Payment callback → API layer → decrypted → payment entity created via QR Payment Processor
2. Payment auto-captured
3. QR credited event triggers two notifications: Stork webhook + eze-server device notification
4. eze-server resolves device, delivers via CommX/EPN (Kafka) or FCM

All steps happen for every transaction, including those where merchant only needs audio alert.

---

## Phase 1 — Target Architecture

### SQR Creation

1. QRMS determines `category="soundbox"` from merchant **org code** (bank-led merchants)
2. QRMS calls UPS: `CreateIntent(qr_id, category="soundbox")`
3. UPS → Mozart: generates intent string
4. UPS persists intent entity with `category` field set
5. UPS → QRMS: intent_string
6. QRMS stores QR entity in DynamoDB + returns QR string/image URL to merchant

### SQR Payment Callback

**PGR — Step 1: Decrypt**
- PGR calls UPS: `Decrypt(raw_callback)`
- UPS → Mozart gateway adapter → decrypted payload
- UPS → PGR: `decrypted_data`

**PGR — Step 2: Preprocess**
- PGR calls UPS: `Preprocess(decrypted_data)`
- UPS → Mozart: resolves payment_id, retrieves intent entity (including `category`)
- UPS → PGR: `preprocessed_data` + intent details

**PGR — Routing**
- `intent.category == "soundbox"` → forward to QRMS (HTTP, Phase 1)
- all other categories → existing UPS payment flow (no change)

**QRMS — Callback Processing**
1. Idempotency: `AuditLog.Create` with DynamoDB conditional put (`attribute_not_exists(pk)`) — return 200 if `payment_id` already exists
2. Update QR code counters:
   - `payments_received_count += 1`
   - `payments_amount_received += amount_paise`
3. Send HMAC-signed notification to eze-server (device_id is in the PGR payload — no device lookup needed)
4. eze-server → CommX/EPN → SoundBox device (audio + screen update)
5. Write AuditLog status synchronously based on eze-server response (status=delivered or status=failed)
6. Return 200 to PGR

---

## Phase 1 — What Changes

| System | Change |
|--------|--------|
| QRMS | New HTTP endpoint; counter update; AuditLog write; ezetap client |
| PGR | Two-step UPS interaction; category-based routing to QRMS |
| UPS | Accept `category` field on intent creation (additive, no processing change) |
| eze-server | No code changes. Receives HMAC-signed notification from QRMS; delivers via CommX. UC-1 bank callbacks never reach eze-server as payment events. |
| payments-upi | Safety-net early-exit for `soundbox` category |

---

## Phase 1 — What Does NOT Change

- UPS intent creation and persistence
- Mozart gateway adapters
- All non-soundbox payment flows
- Stork webhook for non-soundbox merchants
- eze-server CommX delivery mechanism
- Refund processing (EZT writes preserved for all refunds)

---

## Phase 2 — Placeholder

> To be designed once Phase 1 is stable.

Planned scope from spec:
- Remove UPS from SQR flow entirely (including intent generation)
- Introduce Callback Processing Queue (`integrations-upi` → `CallbackEvent` → QRMS Kafka consumer)
- Replace CommX with AWS IoT for device notification
- Migrate existing gateways (Axis, HDFC, etc.) to new flow

---

## HMAC Authentication (eze-server)

HMAC-SHA256 signing in qr-codes ezetap client (Go stdlib `crypto/hmac`):
- Signed over: `payment_id + amount + device_id + timestamp`
- Secret from `config.EzetapService.Auth.HmacSecret`
- Header: `X-Razorpay-Signature`

Note: no HMAC reference implementation exists in Go repos — implement fresh using `crypto/hmac` + `crypto/sha256` + `encoding/hex`.

---

## Idempotency

- **Key:** `payment_id` (partition key on `soundbox_audit_logs` DynamoDB table)
- **Mechanism:** DynamoDB conditional put — `attribute_not_exists(pk)` on `AuditLog.Create`
- **On duplicate:** `ErrDuplicatePayment` returned → return 200 immediately, skip counter update + notification
- No Redis dependency

---

## Non-Functional Requirements

| NFR | Target |
|-----|--------|
| Notification delivery rate | >99.9% |
| e2e notification latency | p95 <3s, p99 <5s |
| Throughput | 4M+ txns/month, handles 10x burst |
| Idempotency | 100% — no double notification on duplicate callback |
| DB writes eliminated | ~9–12M/month |
| Zero regression | All non-soundbox flows unaffected |
