# Phase 1 Implementation Tasks

**Goal:** Onboard JnK Bank + Indian Bank as notification-only gateways.

Execute in the dependency order shown at the bottom. Each task includes test requirements inline.

---

## P1-1: Splitz experiment for UC-1 classification

**Repo:** `qr-codes` + Splitz dashboard

No org code list in code. UC-1 classification is runtime-evaluated via the existing **Splitz** client at `pkg/splitzservice/`.

### Step 1 — Create experiment in Splitz dashboard
- Experiment name: `soundbox_notification_only`
- Evaluated per `org_code`
- JnK Bank org code → `on`; Indian Bank org code → `on`; all others → `off`
- Adding a new bank = enabling their org code in the experiment, no code deploy

### Step 2 — Add experiment constant + helper in qr-codes

`internal/qrcodes/constants/soundbox_experiments.go`:
```go
package constants

// ExperimentSoundboxNotificationOnly is the Splitz experiment that
// identifies merchants for UC-1 (notification-only) processing.
const ExperimentSoundboxNotificationOnly = "soundbox_notification_only"
```

`internal/qrcodes/service/uc_classifier.go`:
```go
// IsSoundboxMerchant evaluates the Splitz experiment for the given orgCode.
// Returns false on error (fail closed — unknown org codes follow standard flow).
func IsSoundboxMerchant(ctx context.Context, splitzClient splitzservice.ClientImplementor, orgCode string) bool {
    resp, err := splitzClient.EvaluateExperiment(ctx, &splitzservice.EvaluateRequest{
        ID:           orgCode,
        ExperimentID: splitzClient.GetExperimentID(constants.ExperimentSoundboxNotificationOnly),
    })
    if err != nil {
        boot.Logger(ctx).Warn("splitz evaluation failed, defaulting to standard flow",
            "org_code", orgCode, "error", err.Error(),
        )
        return false
    }
    return resp.IsExperimentEnabled()
}
```

**Test:** Mock `splitzservice.ClientImplementor`; verify `true` when experiment returns `"on"` for a known org code, `false` when `"off"`, `false` on Splitz error (fail-closed).

---

## P1-2: Pass `category="soundbox"` on UPS intent creation

**Repos:** `qr-codes`, `payments-upi` (prerequisite proto change)

### P1-2a: payments-upi proto prerequisite

`category` does **not exist** in UPS today. Before qr-codes can pass it, payments-upi needs:

1. Add `string category = 15` to `CreateIntentRequest` in `proto/paymentsupi/intent/v1/intent_api.proto`
2. Store category on the Intent entity — add to `metadata` JSON or a new column in the intents table migration
3. Return category in preprocess response — add to `proto/paymentsupi/preprocess/v1/preprocess_api.proto` so PGR can read it

This is a **prerequisite for P1-3** (PGR routing). Coordinate with the UPS/CPS team.

### P1-2b: qr-codes — QRCreationProcessor pattern

Introduce a `QRCreationProcessor` interface so UC-specific intent creation logic lives in its own type, not in a shared if/else block.

`internal/qrcodes/service/qr_creation_processor.go`:
```go
// QRCreationProcessor handles UC-specific steps during QR code creation.
type QRCreationProcessor interface {
    // SetIntentCategory populates (or skips) the category field on the UPS request.
    // org_code selection already resolved in NewQRCreationProcessor — no params needed.
    SetIntentCategory(ctx context.Context, req *ups.CreateIntentRequest) error
}

// SoundboxQRCreationProcessor sets category="soundbox" — selected when org_code is UC-1.
type SoundboxQRCreationProcessor struct{}

func (p *SoundboxQRCreationProcessor) SetIntentCategory(_ context.Context, req *ups.CreateIntentRequest) error {
    req.Category = "soundbox"
    return nil
}

// StandardQRCreationProcessor is a no-op — existing flow is unchanged.
type StandardQRCreationProcessor struct{}

func (p *StandardQRCreationProcessor) SetIntentCategory(_ context.Context, _ *ups.CreateIntentRequest) error {
    return nil
}

// NewQRCreationProcessor selects the correct processor by evaluating the Splitz experiment.
func NewQRCreationProcessor(ctx context.Context, splitzClient splitzservice.ClientImplementor, orgCode string) QRCreationProcessor {
    if IsSoundboxMerchant(ctx, splitzClient, orgCode) {
        return &SoundboxQRCreationProcessor{}
    }
    return &StandardQRCreationProcessor{}
}
```

In the QR creation service, call the processor before the UPS intent creation:
```go
proc := NewQRCreationProcessor(ctx, s.splitzClient, orgCode)
if err := proc.SetIntentCategory(ctx, intentReq); err != nil {
    return nil, err
}
resp, err := s.upsClient.CreateIntent(ctx, intentReq)
```

**Test:** Mock `splitzClient`; verify `SoundboxQRCreationProcessor` sets `category="soundbox"`; verify `StandardQRCreationProcessor` leaves it unset. Test `NewQRCreationProcessor` returns the correct type.

---

## P1-3: PGR soundbox routing

**Repo:** `pg-router`

**Prerequisite:** P1-2a (payments-upi preprocess response must expose `intent.category`) must be deployed first.

### Current state

PGR calls CPS via a single unified call (`CpsServiceClient.CPSService.CallbackPaymentCPS(...)` in `internal/payments/core/callback.go`). There is no Splitz check, no category routing, no QRMS forwarding. This is entirely new work.

### What to build

After the CPS call returns, two sequential checks gate the soundbox path:

```go
// Gate 1: PGR-level kill switch — is the optimization enabled at all?
// pg-router already has a Splitz client at internal/splitz/service.go
if isSoundboxOptimizationEnabled(ctx, s.splitzService) &&
    cpsResp.Intent.Category == "soundbox" {
    // Gate 2 passed: category confirmed soundbox
    return forwardToQRMS(ctx, cpsResp, rawCallback)
}
// else: existing UPS payment flow — NO change
```

```go
// isSoundboxOptimizationEnabled evaluates the PGR-level kill switch.
// Fail closed: if Splitz errors, returns false → traffic stays on existing UPS flow.
func isSoundboxOptimizationEnabled(ctx context.Context, splitzSvc splitz.Service) bool {
    variant, err := splitzSvc.Evaluate(ctx, splitz.EvaluateRequest{
        ID:           "soundbox_callback_optimization", // stable global ID
        ExperimentID: splitzSvc.GetExperimentID("soundbox_callback_optimization"),
    }, &splitz.Variant{Name: "off"}) // default = off (safe)
    if err != nil {
        boot.Logger(ctx).Warn("splitz evaluation failed for soundbox kill switch, defaulting off",
            "error", err.Error(),
        )
        return false
    }
    return variant.Name == "on"
}
```

`forwardToQRMS` builds a `CallbackPayload` JSON body from CPS response fields (PaymentID, QrID, DeviceID, AmountPaise, Gateway, MerchantID, Timestamp) and POSTs to the qr-codes URL. PGR must return 200 to gateway regardless of QRMS response (no retry).

### Config additions required

```toml
# config/pg-router/default.toml
[qrms]
  base_url = "http://qr-codes-internal"
```

Add Splitz experiment `soundbox_callback_optimization` to pg-router's experiment list in config (alongside existing experiments).

### Test
- Kill switch **off** (Splitz returns `"off"`) + `intent.category == "soundbox"` → **existing UPS flow** (QRMS NOT called)
- Kill switch **on** + `intent.category == "soundbox"` → QRMS called, existing UPS flow skipped
- Kill switch **on** + `intent.category == ""` → existing UPS flow, QRMS not called
- Kill switch **on** + QRMS returns non-2xx → PGR returns 200 to gateway (no retry)
- Splitz errors → kill switch treated as **off** → existing UPS flow (fail-closed)

---

## P1-4: AuditLog DynamoDB table + model + repo

**Repo:** `qr-codes`

qr-codes uses DynamoDB via `goutils/kvstore`. No SQL migrations exist.

1. **Schema JSON:** Create `config/qr-codes/dynamo/dynamo_soundbox_audit_logs.json` — use schema from `domain/db-schema.md`
2. **Schema loader:** Add `getSoundboxAuditLogKVStoreSchemaFilePath(env string)` to `internal/dynamoutils/dynamoutils.go` following the `getQrPaymentKVStoreSchemaFilePath` pattern
3. **Model:** Create `internal/qrcodes/model/audit_log.go` — use the struct from `domain/db-schema.md` (all `kvstore:` tags, all timestamps as `int64`)
4. **Repository interface + kvstore implementation:**

```go
type AuditLogRepository interface {
    // Create uses conditional put (attribute_not_exists(pk)) for idempotency.
    // Returns ErrDuplicatePayment if payment_id already exists.
    Create(ctx context.Context, log *model.AuditLog) error
}
```

**Test:** Mock repo; verify `Create` is called with correct fields on happy path and on ezetap failure. Verify duplicate `payment_id` returns the expected error without creating a second record.

---

## P1-5: EzetapService config

**Repo:** `qr-codes`

Add to `internal/config/config.go` (follow existing three-struct pattern from `code-patterns/config.md`):

```go
type EzetapService struct {
    Config   EzetapServiceConfig   `mapstructure:"config"`
    Auth     EzetapServiceAuth     `mapstructure:"auth"`
    ConnPool EzetapServiceConnPool `mapstructure:"conn_pool"`
}
type EzetapServiceConfig struct {
    BaseURL string `mapstructure:"base_url"`
    Mock    bool   `mapstructure:"mock"`
}
type EzetapServiceAuth struct {
    HmacSecret string `mapstructure:"hmac_secret"`
}
type EzetapServiceConnPool struct {
    Timeout            int `mapstructure:"timeout"`
    KeepAliveTimeout   int `mapstructure:"keep_alive_timeout"`
    MaxIdleConnections int `mapstructure:"max_idle_connections"`
}
```

Add to `Config` struct: `EzetapService EzetapService \`mapstructure:"ezetap_service"\``

Add to `config/qr-codes/default.toml`:
```toml
[ezetap_service]
  [ezetap_service.config]
    base_url = "http://eze-server-internal"
    mock = false
  [ezetap_service.auth]
    hmac_secret = ""
  [ezetap_service.conn_pool]
    timeout = 5000
    keep_alive_timeout = 30000
    max_idle_connections = 10
```

**Test:** Load config; assert `EzetapService.Config.BaseURL` and `Auth.HmacSecret` are populated.

---

## P1-6: Ezetap HTTP client

**Repo:** `qr-codes`

Create `internal/qrcodes/clients/ezetap/client.go`:

```go
type EzetapClient interface {
    SendNotification(ctx context.Context, req *NotificationRequest) error
}

type NotificationRequest struct {
    DeviceID    string
    AmountPaise int64
    PaymentID   string
    Timestamp   int64
}
```

- HMAC-SHA256 over `payment_id + ":" + amount + ":" + device_id + ":" + timestamp` using `HmacSecret`
- Header: `X-Razorpay-Signature`
- Return error on non-2xx; caller handles as best-effort

Implement HMAC-SHA256 using Go's `crypto/hmac` + `crypto/sha256` + `encoding/hex` — no reference file exists in Go repos.

**Test:** Mock HTTP server; verify HMAC header present, correct payload.

---

## P1-7: Soundbox callback processor + HTTP handler

**Repo:** `qr-codes`

Two files: a `CallbackProcessor` that owns all soundbox-specific logic, and a thin HTTP handler that dispatches to it.

### P1-7a: CallbackProcessor interface + SoundboxCallbackProcessor

`internal/qrcodes/service/callback_processor.go`:

```go
// CallbackProcessor owns the full processing logic for a payment callback.
// Each UC gets its own implementation — no if/else in shared code.
type CallbackProcessor interface {
    Process(ctx context.Context, payload *CallbackPayload) error
}

// CallbackPayload is the normalized payload forwarded by PGR.
// All fields are populated from the CPS preprocess response + raw callback.
type CallbackPayload struct {
    PaymentID   string `json:"payment_id"`
    AmountPaise int64  `json:"amount_paise"`
    DeviceID    string `json:"device_id"`
    QrID        string `json:"qr_id"`
    MerchantID  string `json:"merchant_id"`
    Gateway     string `json:"gateway"`     // e.g. "jnk_bank" — for metrics labelling
    Timestamp   int64  `json:"timestamp"`
}

// SoundboxCallbackProcessor handles UC-1 (notification-only) callbacks.
// It owns: idempotency gate → counter update → notification → audit log.
// It deliberately does NOT call UPS payment processing, Stork, or EZT writes.
type SoundboxCallbackProcessor struct {
    auditRepo    AuditLogRepository
    counterRepo  QrCodeRepository
    ezetapClient ezetap.EzetapClient
}

func NewSoundboxCallbackProcessor(
    auditRepo    AuditLogRepository,
    counterRepo  QrCodeRepository,
    ezetapClient ezetap.EzetapClient,
) *SoundboxCallbackProcessor {
    return &SoundboxCallbackProcessor{...}
}

func (p *SoundboxCallbackProcessor) Process(ctx context.Context, payload *CallbackPayload) error {
    // 1. Idempotency gate (DynamoDB conditional put)
    // 2. Update QR code counters
    // 3. Send notification to eze-server
    // 4. Write audit log synchronously based on response
    // Returns nil — handler always returns 200; errors are logged + audited
}
```

### P1-7b: Proto definition + gRPC-Gateway route + server.go implementation

The soundbox callback endpoint goes through **gRPC-Gateway** like all other internal qr-codes endpoints — NOT `mux.HandlePath`. The route is auto-registered once the proto rpc is added.

#### Step 1 — Create proto message file

`proto/qr_code/v1/soundbox.proto`:
```protobuf
syntax = "proto3";

package rzp.qr_code.v1;

option go_package = "qr_code/v1;qr_code";

// SoundboxCallbackRequest is the payload PGR forwards for UC-1 (notification-only) callbacks.
message SoundboxCallbackRequest {
  string payment_id   = 1; // unique per payment; used as idempotency key
  int64  amount_paise = 2;
  string device_id    = 3; // soundbox device identifier
  string qr_id        = 4; // qr_codes entity ID
  string merchant_id  = 5;
  string gateway      = 6; // e.g. "jnk_bank" — for metrics labelling
  int64  timestamp    = 7;
}

// SoundboxCallbackResponse is always empty — PGR only cares about HTTP 200.
message SoundboxCallbackResponse {}
```

#### Step 2 — Add rpc to service proto

`proto/qr_code/v1/qr_code_service.proto` — add after the last existing rpc:
```protobuf
// ProcessSoundboxCallback handles UC-1 payment callbacks forwarded by PGR.
// Always returns 200 — PGR must not retry regardless of downstream outcome.
rpc ProcessSoundboxCallback(SoundboxCallbackRequest) returns (SoundboxCallbackResponse) {
  option (google.api.http) = {
    post: "/v1/internal/soundbox/callback",
    body: "*"
  };
}
```

#### Step 3 — Regenerate

```bash
buf generate        # from qr-codes repo root
```

Generates into `rpc/qr_code/v1/`: `soundbox.pb.go` + updated gateway + grpc stubs.

#### Step 4 — Sync to shared proto repo

Copy updated files to `/Users/akash.mukherjee/go/src/github.com/razorpay/proto/qr_code/v1/`:
- `soundbox.proto` (new)
- `qr_code_service.proto` (updated)

#### Step 5 — Implement in server.go

`internal/qrcodes/server.go`:
```go
// ProcessSoundboxCallback handles UC-1 payment callbacks forwarded by PGR.
// Delegates all processing to SoundboxCallbackProcessor.
// Always returns success (HTTP 200) — PGR must not retry.
func (s *Server) ProcessSoundboxCallback(
    ctx context.Context,
    req *qrcodesv1.SoundboxCallbackRequest,
) (*qrcodesv1.SoundboxCallbackResponse, error) {
    payload := &service.CallbackPayload{
        PaymentID:   req.PaymentId,
        AmountPaise: req.AmountPaise,
        DeviceID:    req.DeviceId,
        QrID:        req.QrId,
        MerchantID:  req.MerchantId,
        Gateway:     req.Gateway,
        Timestamp:   req.Timestamp,
    }

    if err := s.soundboxProcessor.Process(ctx, payload); err != nil {
        // Log but never return error — gRPC error → non-200 HTTP → PGR retry
        boot.Logger(ctx).Error("soundbox callback processor error",
            "error", err.Error(), "payment_id", payload.PaymentID,
        )
    }

    return &qrcodesv1.SoundboxCallbackResponse{}, nil
}
```

Inject `soundboxProcessor service.CallbackProcessor` into the `Server` struct and wire it in `boot.go`.

**No changes to `handler.go`** — `RegisterQrCodeServiceHandlerFromEndpoint()` auto-registers the new route.

**Tests:**
- Server method: valid req → processor.Process called, returns empty response + nil error
- Server method: processor returns error → still returns empty response + nil error (absorbs)
- Integration: POST `/v1/internal/soundbox/callback` → 200

---

## P1-8: OTel metrics

**Repo:** `qr-codes`

Add to `internal/metrics/metrics.go` (follow pattern in `code-patterns/metrics.md`):

```go
var SoundboxCallbackTotal        metric.Int64Counter      // labels: gateway, status
var SoundboxNotificationTotal    metric.Int64Counter      // labels: gateway, channel, status
var SoundboxNotificationLatencyMs metric.Float64Histogram // labels: gateway, channel
var SoundboxCounterUpdateTotal   metric.Int64Counter      // labels: gateway, status
```

Add `RecordSoundboxCallback`, `RecordSoundboxNotification`, `RecordSoundboxCounterUpdate` helpers.

**Test:** Verify all metrics initialize without panic; `Record*` functions don't fail.

---

## P1-9: ~~eze-server DB write suppression~~ — NOT IN PHASE 1 SCOPE

> **Removed.** UC-1 bank callbacks (JnK Bank, Indian Bank) flow exclusively through PGR → QRMS. Eze-server never receives raw payment callbacks for these banks — it only receives HMAC-signed notification requests from QRMS (P1-6). There are no EZT DB writes to suppress.
>
> If a future bank is onboarded with a direct eze-server callback path, this task should be revisited then.

---

## P1-10: `payments-upi` safety-net early-exit

**Repo:** `payments-upi`

`Category` field does **not exist** on `PaymentActionRequest` or its proto. Add it first:

1. Add `string category = 11` to `proto/paymentsupi/payments/v1/callback.proto` → `CallbackRequest`
2. Add `Category string` to `PaymentActionRequest` struct in `internal/app/payment/request/request.go`
3. Wire from gRPC-generated struct to `PaymentActionRequest` in the request mapping layer
4. Add early-exit in `internal/app/payment/processor/callback.go`:

```go
if req.Category == "soundbox" {
    // PGR should never send this to UPS, but guard against routing bugs
    return &ProcessCallbackResponse{Success: true}, nil
}
```

**Test:** `category == "soundbox"` → returns success without calling any downstream service. `category == ""` or `category == "upi"` → normal processing continues.

---

## P1-11: Dashboards + Alerts

**Metrics to dashboard:**
- `soundbox_callback_handled_total` by gateway, status
- `soundbox_notification_total` by channel, status
- `soundbox_notification_latency_ms` p50/p95/p99
- `soundbox_counter_update_total` by gateway, status

**Alerts:**
- Notification failure rate >1% for 5 minutes
- Counter update failure rate >0.1%
- Notification latency p99 >5s

---

## Dependency Order

```
P1-2a (payments-upi: add category proto + preprocess response)  ← PREREQUISITE for P1-2b and P1-3
  │
  ├─ P1-2b (qr-codes: pass category on intent creation)
  │    └─ requires P1-1 (org code constants)
  │
  └─ P1-3 (PGR routing) ← deploy AFTER P1-7 is live in qr-codes

P1-1 (org code constants)
  ├─ P1-5 (config)
  │    └─ P1-6 (ezetap client)
  │         └─ P1-7 (callback handler) ←── P1-4 (audit DynamoDB model/repo)
  │              └─ P1-8 (metrics)

P1-10 (payments-upi safety net) — depends on P1-2a proto changes
P1-11 (dashboards) — last
```

## Deploy Order (see `operations/backward-compat.md`)

1. `payments-upi` (P1-2a: proto + preprocess response changes)
2. `qr-codes` (P1-1, P1-2b, P1-4, P1-5, P1-6, P1-7, P1-8)
3. `payments-upi` (P1-10: safety net — wire proto field to request struct)
4. `pg-router` (P1-3) ← **activation step, deploy last** (start with kill switch OFF, then flip ON)
