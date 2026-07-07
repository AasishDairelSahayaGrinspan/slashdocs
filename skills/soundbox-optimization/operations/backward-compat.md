# Backward Compatibility & Deploy Order

## Safety Gates

### Layer 1 — PGR kill switch (instant, no deploy)
Splitz experiment `soundbox_callback_optimization` in pg-router must be `on` for any soundbox routing to happen. Flip it `off` → 100% of traffic falls through to the existing UPS flow immediately. No code deploy. No other service affected.

### Layer 2 — Category routing
PGR only routes to QRMS when `intent.category == "soundbox"`. Any merchant whose UPS intent was created without `category="soundbox"` (Layer 3 gate) is automatically safe.

### Layer 3 — qr-codes Splitz per org_code
`category="soundbox"` is only set at QR creation time when the merchant's org_code has the `soundbox_notification_only` Splitz experiment enabled. Disabling an org in this experiment prevents any new QR codes from being created as soundbox. Existing QR codes retain their category.

### Other gates
- **`soundbox_audit_logs` DynamoDB table is additive.** No existing table is modified.
- **eze-server has no Phase 1 changes.** UC-1 bank callbacks never reach eze-server as payment events — only HMAC-signed notification requests from QRMS arrive there. Nothing to revert.
- **Counter update reuses existing QR repo.** The `payments_received_count` / `payments_amount_received` update uses the existing DynamoDB update path — no schema change.
- **`payments-upi` early-exit is a safety net.** If PGR accidentally routes a `soundbox` callback to UPS, the early-exit prevents payment entity creation without breaking the flow.

---

## Phase 1 Deploy Order

Deploy in this exact sequence. PGR is the activation step — nothing is live until PGR is deployed.

| Step | Repo | What changes | Gate before next step |
|------|------|-------------|----------------------|
| 1 | `qr-codes` | Callback handler, ezetap client, audit_log, counter update, metrics | Smoke test: POST to `/v1/internal/soundbox/callback` returns 200 |
| 2 | `payments-upi` | Early-exit safety net | Unit tests pass |
| 3 | `pg-router` | Splitz kill switch + soundbox routing (kill switch deployed **off**) | Enable Splitz `soundbox_callback_optimization` → `on` after deploy; **monitor dashboards for 30 min** |

---

## Rollback Plan

| Scenario | Action | Impact |
|----------|--------|--------|
| **Something is wrong — stop immediately** | Flip Splitz `soundbox_callback_optimization` → `off` in pg-router | Instant. 100% traffic back to existing UPS flow. No deploy needed. |
| **Offboard a specific bank** | Disable their org_code in Splitz `soundbox_notification_only` (qr-codes) | New QR codes for that org no longer get `category="soundbox"`. Existing QR codes already in flight still route through soundbox until TTL expires. |
| **Full code revert needed** | Revert pg-router PR | Clean; qr-codes handler never called without PGR routing. payments-upi revert is optional (safety net only). |

---

## Phase 2 Compatibility Notes (PLACEHOLDER)

> To be filled when Phase 2 is scoped.
> Key concern: removing UPS from intent generation must be coordinated with NPCI compliance review.
