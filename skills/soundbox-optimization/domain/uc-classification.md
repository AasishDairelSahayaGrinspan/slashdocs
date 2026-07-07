# Use Case Classification

## UC-1: Notification Only (Phase 1 target)

**Definition:** Bank owns transaction record. Razorpay delivers QR counter update + audio alert only.

**Identification:** Splitz experiment `soundbox_notification_only` returns `on` for the merchant. `category="soundbox"` is set on the UPS intent at QR creation time by the `SoundboxQRCreationProcessor`.

**What is skipped:**
- EZT DB writes: `bharatqr_txn`, `upi_txn`, `txn_request`
- UPS payment entity creation
- UPS payment processing (captures, CDC events, pricing)
- Stork webhook dispatch

**What is preserved:**
- QR code counter update in QRMS (`payments_received_count`, `payments_amount_received`)
- HMAC-authenticated notification to eze-server
- AuditLog write in QRMS
- EZT refund writes (`txnType=REFUND` always preserved)

---

## UC-2: Full History (no change)

**Definition:** Razorpay owns/mirrors transaction record. Refunds, portal, reconciliation required.

**No changes to this flow.** All existing DB writes and payment processing continue unchanged.

---

## Phase 1 Gateway Matrix

| Gateway | Use Case | Phase | Status |
|---------|----------|-------|--------|
| JnK Bank | UC-1 | Phase 1 | 🟢 New onboarding |
| Indian Bank | UC-1 | Phase 1 | 🟢 New onboarding |
| HDFC Mintoak | UC-2 | — | No change |
| Axis BharatQR | UC-2 | — | No change |
| HDFC BharatQR | UC-2 | — | No change |
| Kotak BharatQR | UC-2 | — | No change |
| SBI BharatQR | UC-2 | — | No change |
| YBL BharatQR | UC-2 | — | No change |
| FDC BharatQR | UC-2 | — | No change |

## Phase 2 Gateway Migration (PLACEHOLDER)

> To be filled when Phase 2 is scoped. Existing UC-2 gateways may be reclassified to UC-1 after migration.

---

## How Category Is Determined at Runtime

1. **At QR creation:** `SoundboxQRCreationProcessor` evaluates Splitz experiment `soundbox_notification_only` for the `org_code` → if `on`, passes `category="soundbox"` to UPS intent creation; UPS stores it on the intent entity
2. **At callback:** PGR reads `intent.category` from CPS/UPS preprocess response → routes to QRMS if `"soundbox"`; all other categories continue through existing UPS payment flow unchanged
3. **At eze-server:** No changes needed in Phase 1. UC-1 bank callbacks never reach eze-server as payment events — they flow exclusively through PGR → QRMS. Eze-server only receives HMAC-signed notification requests from QRMS and delivers them via CommX.

---

## Feature Flag (qr-codes)

UC-1 classification uses the existing **Splitz** client (`pkg/splitzservice/`):

```go
// internal/qrcodes/service/qr_creation_processor.go

const ExperimentSoundboxNotificationOnly = "soundbox_notification_only"

func isSoundboxMerchant(ctx context.Context, splitzClient splitzservice.ClientImplementor, orgCode string) bool {
    resp, err := splitzClient.EvaluateExperiment(ctx, &splitzservice.EvaluateRequest{
        ID:           orgCode,
        ExperimentID: splitzClient.GetExperimentID(ExperimentSoundboxNotificationOnly),
    })
    if err != nil {
        // fail closed — unknown merchants follow standard flow
        return false
    }
    return resp.IsExperimentEnabled()
}
```

**No org code list in code.** Onboarding JnK Bank or Indian Bank = enabling their merchants in the Splitz experiment dashboard. Adding a new bank = no code deploy.

---

## Processor Interfaces (qr-codes)

```go
// internal/qrcodes/service/interfaces.go

// QRCreationProcessor handles the UC-specific steps of QR code creation.
type QRCreationProcessor interface {
    // SetIntentCategory sets (or skips) the category field on the UPS CreateIntentRequest.
    // org_code-based selection already happened in NewQRCreationProcessor — no params needed here.
    SetIntentCategory(ctx context.Context, req *ups.CreateIntentRequest) error
}

// CallbackProcessor handles the UC-specific steps when a payment callback arrives.
type CallbackProcessor interface {
    Process(ctx context.Context, payload *CallbackPayload) error
}
```

**Implementations:**

| Interface | UC-1 impl | UC-2 impl |
|-----------|-----------|-----------|
| `QRCreationProcessor` | `SoundboxQRCreationProcessor` — sets `category="soundbox"` | `StandardQRCreationProcessor` — no-op |
| `CallbackProcessor` | `SoundboxCallbackProcessor` — counter + notify + audit | Not needed (PGR never routes UC-2 here) |

**Processor is selected at the entry point, not inside business logic:**
- QR creation: evaluate Splitz → inject correct `QRCreationProcessor`
- Callback handler: `intent.category == "soundbox"` is guaranteed by PGR routing → always inject `SoundboxCallbackProcessor`
