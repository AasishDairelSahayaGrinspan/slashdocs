# HTTP Handler Patterns

Source: `internal/qrcodes/handler.go` and `internal/qrcodes/handler_test.go` in `qr-codes`

---

## Route Registration

The soundbox callback endpoint is registered via **gRPC-Gateway**, not `mux.HandlePath`. All qr-codes internal endpoints follow this pattern.

**Proto defines the route** (`proto/qr_code/v1/qr_code_service.proto`):
```protobuf
rpc ProcessSoundboxCallback(SoundboxCallbackRequest) returns (SoundboxCallbackResponse) {
  option (google.api.http) = {
    post: "/v1/internal/soundbox/callback",
    body: "*"
  };
}
```

`handler.go` does **not** need modification — `RegisterQrCodeServiceHandlerFromEndpoint()` auto-registers all proto-defined routes including the new one:

```go
// handler.go — already present, unchanged
if err := qrcodesv1.RegisterQrCodeServiceHandlerFromEndpoint(ctx, mux, address, opts); err != nil {
    return err
}
```

`mux.HandlePath` is only used for binary/non-JSON responses (QR image downloads). Never use it for business logic endpoints.

---

## Server Method Pattern (gRPC-Gateway dispatch)

The gRPC server method in `internal/qrcodes/server.go` is the entry point. It converts the proto request to `CallbackPayload` and delegates to the processor.

Add `soundboxProcessor service.CallbackProcessor` to the existing `Server` struct. Inject it in `boot.go`:

```go
// boot.go
soundboxProcessor := service.NewSoundboxCallbackProcessor(auditRepo, counterRepo, ezetapClient)
server := qrcodes.NewServer(..., soundboxProcessor)
```

---

## Server Method Implementation (thin dispatch)

```go
// internal/qrcodes/server.go — implement the generated interface method
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

    // Delegate all UC logic to the processor
    // Never return a gRPC error — that maps to non-200 HTTP → PGR would retry
    if err := s.soundboxProcessor.Process(ctx, payload); err != nil {
        boot.Logger(ctx).Error("soundbox callback processor error",
            "error", err.Error(), "payment_id", payload.PaymentID,
        )
    }

    return &qrcodesv1.SoundboxCallbackResponse{}, nil
}
```

---

## SoundboxCallbackProcessor (owns all UC-1 logic)

```go
// SoundboxCallbackProcessor handles UC-1 (notification-only) callbacks.
// It deliberately does NOT call UPS payment processing, Stork, or EZT DB writes.
type SoundboxCallbackProcessor struct {
    auditRepo    AuditLogRepository
    counterRepo  QrCodeRepository
    ezetapClient ezetap.EzetapClient
}

func (p *SoundboxCallbackProcessor) Process(ctx context.Context, payload *CallbackPayload) error {
    now := time.Now().Unix()

    // 1. Idempotency gate: DynamoDB conditional put — no Redis
    auditLog := &model.AuditLog{
        ID:              uniqueid.New(auditLogEntityPrefix),
        PaymentID:       payload.PaymentID,
        QrCodeID:        payload.QrID,
        MerchantID:      payload.MerchantID,
        DeviceID:        payload.DeviceID,
        AmountPaise:     payload.AmountPaise,
        DeliveryChannel: model.DeliveryChannelCOMX, // Phase 1: always CommX
        UseCase:         model.UseCaseNotificationOnly,
        AttemptedAt:     now,
        // OrgCode not available in PGR callback payload — optional field, left empty
    }
    if err := p.auditRepo.Create(ctx, auditLog); err != nil {
        if errors.Is(err, auditrepo.ErrDuplicatePayment) {
            boot.Logger(ctx).Info("duplicate payment_id, skipping", "payment_id", payload.PaymentID)
            return nil
        }
        return err
    }

    // 2. Update QR code counters (existing method — kvdynamo_qr_code_repo.go:440)
    if err := p.counterRepo.AtomicIncrementPaymentCounters(ctx, payload.QrID, now, payload.AmountPaise); err != nil {
        boot.Logger(ctx).Error("counter update failed", "error", err.Error(), "qr_id", payload.QrID)
        metrics.RecordSoundboxCounterUpdate(ctx, payload.Gateway, "failed")
        p.writeAuditStatus(ctx, auditLog, model.AuditStatusFailed, err.Error(), 0)
        return nil // handler still returns 200
    }
    metrics.RecordSoundboxCounterUpdate(ctx, payload.Gateway, "success")

    // 3. Send notification to eze-server (best-effort)
    start := time.Now()
    notifErr := p.ezetapClient.SendNotification(ctx, &ezetap.NotificationRequest{
        DeviceID:    payload.DeviceID,
        AmountPaise: payload.AmountPaise,
        PaymentID:   payload.PaymentID,
        Timestamp:   now,
    })
    latencyMs := time.Since(start).Milliseconds()

    // 4. Write audit log synchronously based on eze-server response
    if notifErr != nil {
        boot.Logger(ctx).Warn("notification delivery failed (best-effort)",
            "error", notifErr.Error(), "payment_id", payload.PaymentID)
        metrics.RecordSoundboxNotification(ctx, payload.Gateway, "comx", "failed", float64(latencyMs))
        p.writeAuditStatus(ctx, auditLog, model.AuditStatusFailed, notifErr.Error(), latencyMs)
    } else {
        metrics.RecordSoundboxNotification(ctx, payload.Gateway, "comx", "success", float64(latencyMs))
        p.writeAuditStatus(ctx, auditLog, model.AuditStatusDelivered, "", latencyMs)
    }
    return nil
}
```

---

## CallbackPayload Struct

This is the JSON body PGR POSTs to `POST /internal/v1/soundbox/callback`. It is also the struct `SoundboxCallbackProcessor.Process()` operates on — one struct throughout, no aliasing.

```go
// internal/qrcodes/service/callback_processor.go
// CallbackPayload is the normalized payload forwarded by PGR.
// PGR populates all fields from the CPS preprocess response + raw callback.
type CallbackPayload struct {
    PaymentID   string `json:"payment_id"`   // unique per payment (idempotency key)
    AmountPaise int64  `json:"amount_paise"`
    DeviceID    string `json:"device_id"`    // soundbox device identifier
    QrID        string `json:"qr_id"`        // qr_codes entity ID
    MerchantID  string `json:"merchant_id"`
    Gateway     string `json:"gateway"`      // e.g. "jnk_bank", "indian_bank" (for metrics)
    Timestamp   int64  `json:"timestamp"`
}
```

`OrgCode` is NOT included in the callback payload — it is an optional audit field not available from PGR at callback time. The `org_code` column in `soundbox_audit_logs` is optional and left empty in Phase 1.

---

## writeAuditStatus Helper

```go
// writeAuditStatus updates the audit log entry synchronously after the eze-server response.
// Called inside SoundboxCallbackProcessor.Process() — not a goroutine.
func (p *SoundboxCallbackProcessor) writeAuditStatus(
    ctx context.Context,
    log *model.AuditLog,
    status, failureReason string,
    latencyMs int64,
) {
    log.Status = status
    log.FailureReason = failureReason
    if status == model.AuditStatusDelivered {
        deliveredAt := time.Now().Unix()
        log.DeliveredAt = &deliveredAt
        log.LatencyMs   = &latencyMs
    } else {
        log.ErrorCode = failureReason // reuse failure text as error code for now
    }
    if err := p.auditRepo.Update(ctx, log); err != nil {
        boot.Logger(ctx).Error("failed to update audit log status",
            "error", err.Error(), "payment_id", log.PaymentID,
        )
        // non-fatal — we already returned 200 to PGR
    }
}
```

---

## Testing Pattern

**Server method tests** verify proto dispatch — mock the `CallbackProcessor`:

```go
func TestProcessSoundboxCallback_DispatchesToProcessor(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockProcessor := mocks.NewMockCallbackProcessor(ctrl)
    server := qrcodes.NewServer(..., mockProcessor)

    req := &qrcodesv1.SoundboxCallbackRequest{
        PaymentId:   "PAY123",
        AmountPaise: 10000,
        DeviceId:    "DEV456",
        QrId:        "QR789",
        MerchantId:  "MID001",
        Gateway:     "jnk_bank",
    }

    mockProcessor.EXPECT().
        Process(gomock.Any(), gomock.MatchedBy(func(p *service.CallbackPayload) bool {
            return p.PaymentID == "PAY123" && p.Gateway == "jnk_bank"
        })).
        Return(nil)

    resp, err := server.ProcessSoundboxCallback(context.Background(), req)

    assert.NoError(t, err)
    assert.NotNil(t, resp) // always returns empty response
}

func TestProcessSoundboxCallback_ProcessorError_StillReturnsSuccess(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockProcessor := mocks.NewMockCallbackProcessor(ctrl)
    server := qrcodes.NewServer(..., mockProcessor)

    req := &qrcodesv1.SoundboxCallbackRequest{PaymentId: "PAY123", ...}

    // Even if processor errors, server must return nil gRPC error → HTTP 200
    mockProcessor.EXPECT().Process(gomock.Any(), gomock.Any()).
        Return(errors.New("ezetap timeout"))

    resp, err := server.ProcessSoundboxCallback(context.Background(), req)

    assert.NoError(t, err) // nil error → HTTP 200 → PGR does not retry
    assert.NotNil(t, resp)
}
```

**Processor tests** (see `testing/patterns.md`) test all UC-1 logic in isolation.
