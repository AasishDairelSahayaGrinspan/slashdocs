# Project Overview

## Problem

SoundBox Static QR transactions cost **14.21 paise each** because every payment triggers 4–6 DB writes and full UPS payment processing — even when the merchant only needs a real-time audio notification.

At **3–4M txns/month**, that's **9–12M unnecessary DB writes per month**.

### Current Cost Breakdown (14.21p/txn)

| Component | Cost |
|-----------|------|
| bharatqr_txn DB write | ~3p |
| upi_txn DB write | ~3p |
| txn_request DB write | ~2p |
| pos_notification write (Ezetap) | ~2p |
| UPS payment processing | ~4p |

### Phase 1 Target: eliminate EZT DB writes + UPS payment processing

---

## Solution

Route `soundbox` callbacks directly from PGR to QRMS. QRMS:
1. Updates the QR code's payment counters (`payments_received_count`, `payments_amount_received`)
2. Resolves the device
3. Delivers notification to eze-server
4. Records an AuditLog entry

No UPS payment entity created. No EZT DB writes.

---

## Four Strategic Axes (from spec)

Scenario 1 is the Phase 1 target. Others are not in scope.

| Scenario | Description | Status |
|----------|-------------|--------|
| 1 | SoundBox — real-time notification only | **Phase 1** |
| 2 | SoundBox — notification + transaction history | No change |
| 3 | SQR Sticker — no physical device | Future |
| 4 | SQR Sticker — with transaction history | Future |

---

## Business Context

- **Bank-led merchants** (JnK Bank, Indian Bank): Bank owns transaction record. Razorpay's role is QR counter update + notification only.
- **No Metabase/Trino queries** rely on full Razorpay transaction records for these new gateways.
- **SoundBox will NOT migrate to Razorpay stack** within 12 months — optimization remains valuable long-term.
- **Existing gateways** (Axis, HDFC, etc.) continue full payment processing unchanged.

---

## Implementation Phases

### Phase 1 — New Gateway Onboarding (ACTIVE)
- Move SQR payment processing out of UPS for JnK Bank + Indian Bank
- QRMS handles pre-processing, counter updates, and notification
- UPS retained only for intent string generation
- Replace EZT transaction logging with AuditLog entity
- Eliminate EZT DB writes (bharatqr_txn, upi_txn, txn_request)

### Phase 2 — Full Optimization (FUTURE — placeholder)
> To be detailed once Phase 1 is stable in production.
- Remove UPS completely from SQR flow (including intent generation)
- Deprecate COMX/EPN notification channel
- Introduce AWS IoT as primary device notification channel
- Migrate existing gateways to new flow

---

## Phase 1 Repositories in Scope

| Repo | Role | Language |
|------|------|----------|
| `qr-codes` | Callback handler, counter updates, device resolution, notification, audit | Go |
| `pg-router` | Two-step UPS interaction + route `soundbox` callbacks to QRMS | Go |
| `eze-server` | Suppress EZT DB writes for soundbox; deliver via CommX | Java |
| `payments-upi` | Safety-net early-exit for `soundbox` category | Go |

## Phase 2 Repositories (FUTURE)
> To be filled when Phase 2 is scoped.
- `integrations-upi`, `qr-codes` (Kafka consumer), `eze-server` (AWS IoT)
