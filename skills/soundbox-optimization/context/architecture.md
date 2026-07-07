# Architecture — Phase 1

> **STATUS: CONFIRMED** — Category/Source-based Routing aligned by Akash, Rajat (UPS), Piyush on 2026-06-30.
> Source of truth: updated spec (Section 8) + design review MoM.

---

## Phase 1 Flow

### SQR Creation

```
Merchant onboarding
      │
      ▼
qr-codes service
  ├─ Evaluates Splitz experiment "soundbox_notification_only" for org_code
  │     → on  → NewQRCreationProcessor returns SoundboxQRCreationProcessor
  │     → off → NewQRCreationProcessor returns StandardQRCreationProcessor
  ├─ processor.SetIntentCategory(req) sets category="soundbox" (or no-op for standard)
  ├─ Calls UPS: CreateIntent(qr_id, category="soundbox")
  │     UPS → Mozart: generates intent string
  │     UPS: persists intent entity with category field
  │     UPS → qr-codes: intent_string
  └─ Stores QR entity in DynamoDB (with metadata)
  └─ Returns QR string + image URL to merchant
```

**Key:** `category="soundbox"` is derived from a **Splitz feature flag** evaluated per `org_code` at runtime — no hardcoded org code list. UPS persists the category on the intent entity for later routing.

---

### SQR Payment Callback (Phase 1)

```
Customer scans QR → UPI payment completes
      │
      ▼
Bank/Gateway → PGR (encrypted callback)

PGR — Step 1: Decrypt
  PGR → UPS: Decrypt(raw_callback)
  UPS → Mozart gateway adapter: decrypt payload
  UPS → PGR: decrypted_data

PGR — Step 2: Preprocess
  PGR → UPS: Preprocess(decrypted_data)
  UPS → Mozart: resolve payment_id, retrieve intent entity (incl. category)
  UPS → PGR: preprocessed_data + intent { category, device_id, qr_id, ... }

PGR — Routing decision (two-gate check):
  Gate 1: Splitz kill switch "soundbox_callback_optimization"
    ├─ off (or Splitz error) ──► Existing UPS payment flow (no change)
    └─ on ──► Gate 2: check intent.category
                ├─ category == "soundbox" ──► QRMS (direct HTTP, Phase 1)
                └─ other categories       ──► Existing UPS payment flow (no change)

QRMS receives callback (payload contains qr_id, device_id, payment_id, amount_paise):
  Handler (thin):
    1. Parse + validate payload
    2. processor.Process(ctx, payload)   ← always SoundboxCallbackProcessor here
    3. Return 200 to PGR always

  SoundboxCallbackProcessor.Process():
    1. Idempotency gate: AuditLog.Create (DynamoDB conditional put)
       → if payment_id already exists: return immediately
    2. Update QR code counters: payments_received_count += 1, payments_amount_received += amount_paise
    3. Send HMAC-signed notification to eze-server (device_id from payload)
    4. eze-server → CommX/EPN → SoundBox device (audio alert)
    5. Write AuditLog status synchronously (delivered or failed based on eze-server response)
```

---

## What Is SKIPPED for `soundbox` (Phase 1)

| Component | Skipped |
|-----------|---------|
| UPS payment entity creation | ✅ |
| UPS payment processing (captures, CDC, pricing) | ✅ |
| EZT DB writes (bharatqr_txn, upi_txn, txn_request) | ✅ — UC-1 banks never send to eze-server directly; callbacks come exclusively through PGR |
| Stork merchant webhook | ✅ |

## What Is PRESERVED (Phase 1)

| Component | Why |
|-----------|-----|
| UPS intent creation + persistence | Required for NPCI UPI compliance |
| QR code counter updates in QRMS | Merchant QR lifecycle tracking |
| HMAC-authenticated eze-server notification | Existing SoundBox delivery channel |
| AuditLog write | Replaces EZT records; observability |
| Refund writes in EZT | `txnType=REFUND` always preserved |
| All non-soundbox flows | Zero regression |

---

## eze-server Notification Routing

```
eze-server receives HMAC-signed notification request
      │
      ├─ Phase 1: route to CommX/EPN (Kafka → downstream consumers)
      └─ Phase 2: route to AWS IoT (MQTT)
```

Phase 1 uses CommX. AWS IoT is Phase 2 only.

---

## Phase 2 Architecture (PLACEHOLDER)

> To be designed once Phase 1 is stable.

High-level intent:
- Remove UPS from SQR flow entirely (including intent generation)
- Introduce Callback Processing Queue: `integrations-upi` publishes `CallbackEvent` → `qr-codes` Kafka consumer
- Replace CommX with AWS IoT for device notification
- QRMS handles full SQR lifecycle without UPS dependency

---

## Approaches Considered and Rejected

### Approach 1 — Custom Merchant Reference Field
Embed routing hint in UPI merchant reference string.
**Rejected:** Reference field format varies across gateways; fragile and unreliable.

### Approach 2 — Separate Intent Type
Create new UPS intent type (`SOUND_BOX_INTENT`) with lighter processing path.
**Rejected:** Requires UPS internals changes; high blast radius; no clean SDK interface.

### Approach 3 — Category/Source Field ✅ CHOSEN
Pass `category="soundbox"` on intent creation (derived from org code). PGR reads it post-UPS-preprocess and routes accordingly.
**Chosen:** Additive change, deterministic routing, no UPS internals modification.
