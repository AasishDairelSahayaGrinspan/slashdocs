# Unit Test Patterns

Source: `internal/qrcodes/handler_test.go` and service tests in `qr-codes`

---

## Processor Test Suite (gomock + testify)

Processor tests exercise all UC-1 logic. Handler tests (in `code-patterns/http-handler.md`) only test dispatch — they mock the `CallbackProcessor` interface.

```go
package service_test

import (
    "context"
    "testing"

    "github.com/golang/mock/gomock"
    "github.com/stretchr/testify/suite"

    auditrepo "github.com/razorpay/qr-codes/internal/qrcodes/repo/auditlog"
    mocks     "github.com/razorpay/qr-codes/internal/qrcodes/mocks"
    model     "github.com/razorpay/qr-codes/internal/qrcodes/model"
    service   "github.com/razorpay/qr-codes/internal/qrcodes/service"
)

type SoundboxProcessorTestSuite struct {
    suite.Suite
    ctrl            *gomock.Controller
    ctx             context.Context
    processor       *service.SoundboxCallbackProcessor
    mockAuditRepo   *mocks.MockAuditLogRepository
    mockCounterRepo *mocks.MockQrCodeRepository
    mockEzetap      *mocks.MockEzetapClient
}

func (s *SoundboxProcessorTestSuite) SetupTest() {
    s.ctrl = gomock.NewController(s.T())
    s.ctx = context.Background()
    s.mockAuditRepo   = mocks.NewMockAuditLogRepository(s.ctrl)
    s.mockCounterRepo = mocks.NewMockQrCodeRepository(s.ctrl)
    s.mockEzetap      = mocks.NewMockEzetapClient(s.ctrl)
    s.processor = service.NewSoundboxCallbackProcessor(
        s.mockAuditRepo, s.mockCounterRepo, s.mockEzetap,
    )
}

func (s *SoundboxProcessorTestSuite) TearDownTest() {
    s.ctrl.Finish()
}

func TestSoundboxProcessorTestSuite(t *testing.T) {
    suite.Run(t, new(SoundboxProcessorTestSuite))
}

var validPayload = &service.CallbackPayload{
    PaymentID:   "PAY123",
    AmountPaise: 10000,
    DeviceID:    "DEV456",
    QrID:        "QR789",
    MerchantID:  "MID001",
    Gateway:     "jnk_bank",
}
```

---

## Happy Path Test

```go
func (s *SoundboxProcessorTestSuite) TestProcess_HappyPath() {
    // Idempotency gate: first-time write succeeds
    s.mockAuditRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
    // Counter update succeeds
    s.mockCounterRepo.EXPECT().
        AtomicIncrementPaymentCounters(gomock.Any(), "QR789", gomock.Any(), int64(10000)).
        Return(nil)
    // Notification sent successfully
    s.mockEzetap.EXPECT().SendNotification(gomock.Any(), gomock.Any()).Return(nil)
    // Audit log status updated to delivered
    s.mockAuditRepo.EXPECT().Update(gomock.Any(), gomock.MatchedBy(func(log *model.AuditLog) bool {
        return log.Status == model.AuditStatusDelivered
    })).Return(nil)

    err := s.processor.Process(s.ctx, validPayload)

    s.Require().NoError(err)
}
```

---

## Idempotency Test (DynamoDB Conditional Put)

`SoundboxCallbackProcessor` uses `AuditLog.Create` with `attribute_not_exists(pk)` as the idempotency gate — **no Redis**.

```go
func (s *SoundboxProcessorTestSuite) TestProcess_DuplicatePaymentID_Skips() {
    // Conditional put returns duplicate error
    s.mockAuditRepo.EXPECT().
        Create(gomock.Any(), gomock.Any()).
        Return(auditrepo.ErrDuplicatePayment)

    // Counter and ezetap must NOT be called on duplicate
    s.mockCounterRepo.EXPECT().
        AtomicIncrementPaymentCounters(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
        Times(0)
    s.mockEzetap.EXPECT().SendNotification(gomock.Any(), gomock.Any()).Times(0)

    err := s.processor.Process(s.ctx, validPayload)

    s.Require().NoError(err) // processor returns nil — handler returns 200
}
```

---

## Counter Update Failure (QR Closed)

```go
func (s *SoundboxProcessorTestSuite) TestProcess_CounterUpdateFails_AuditFailed() {
    s.mockAuditRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
    s.mockCounterRepo.EXPECT().
        AtomicIncrementPaymentCounters(gomock.Any(), "QR789", gomock.Any(), int64(10000)).
        Return(errors.New("conditional check failed: QR code is closed"))

    // Audit written with status=failed; ezetap NOT called
    s.mockAuditRepo.EXPECT().
        Update(gomock.Any(), gomock.MatchedBy(func(log *model.AuditLog) bool {
            return log.Status == model.AuditStatusFailed
        })).
        Return(nil)
    s.mockEzetap.EXPECT().SendNotification(gomock.Any(), gomock.Any()).Times(0)

    err := s.processor.Process(s.ctx, validPayload)

    s.Require().NoError(err)
}
```

---

## Ezetap Notification Failure

```go
func (s *SoundboxProcessorTestSuite) TestProcess_EzetapFails_AuditFailed() {
    s.mockAuditRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
    s.mockCounterRepo.EXPECT().
        AtomicIncrementPaymentCounters(gomock.Any(), "QR789", gomock.Any(), int64(10000)).
        Return(nil)
    s.mockEzetap.EXPECT().SendNotification(gomock.Any(), gomock.Any()).
        Return(errors.New("connection timeout"))

    // Audit written with status=failed, failure_reason set
    s.mockAuditRepo.EXPECT().
        Update(gomock.Any(), gomock.MatchedBy(func(log *model.AuditLog) bool {
            return log.Status == model.AuditStatusFailed && log.FailureReason != ""
        })).
        Return(nil)

    err := s.processor.Process(s.ctx, validPayload)

    s.Require().NoError(err) // processor returns nil — best-effort, handler returns 200
}
```

---

## Generating Mocks

```bash
# From qr-codes repo root
make mockgen
```

Or manually:
```bash
mockgen -source=internal/qrcodes/repo/interfaces.go \
        -destination=internal/qrcodes/mocks/mock_repo.go \
        -package=mocks
```

---

## IsSoundboxMerchant Tests (Splitz-based)

No hardcoded org code list — classification goes through the Splitz client.

```go
func TestIsSoundboxMerchant_ExperimentOn(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockSplitz := mocks.NewMockClientImplementor(ctrl)
    mockSplitz.EXPECT().GetExperimentID(constants.ExperimentSoundboxNotificationOnly).
        Return("exp-abc123")
    mockSplitz.EXPECT().EvaluateExperiment(gomock.Any(), &splitzservice.EvaluateRequest{
        ID:           "org_QtDz1D9Dl1LLsx",
        ExperimentID: "exp-abc123",
    }).Return(&splitzservice.EvaluateResponse{
        Response: splitzservice.VariantResponse{
            Variant: splitzservice.Variant{
                Variables: []splitzservice.Variable{{Key: "result", Value: "on"}},
            },
        },
    }, nil)

    got := service.IsSoundboxMerchant(context.Background(), mockSplitz, "org_QtDz1D9Dl1LLsx")
    assert.True(t, got)
}

func TestIsSoundboxMerchant_ExperimentOff(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockSplitz := mocks.NewMockClientImplementor(ctrl)
    mockSplitz.EXPECT().GetExperimentID(gomock.Any()).Return("exp-abc123")
    mockSplitz.EXPECT().EvaluateExperiment(gomock.Any(), gomock.Any()).Return(
        &splitzservice.EvaluateResponse{
            Response: splitzservice.VariantResponse{
                Variant: splitzservice.Variant{
                    Variables: []splitzservice.Variable{{Key: "result", Value: "off"}},
                },
            },
        }, nil)

    got := service.IsSoundboxMerchant(context.Background(), mockSplitz, "org_hdfc_xyz")
    assert.False(t, got)
}

func TestIsSoundboxMerchant_SplitzError_FailClosed(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockSplitz := mocks.NewMockClientImplementor(ctrl)
    mockSplitz.EXPECT().GetExperimentID(gomock.Any()).Return("exp-abc123")
    mockSplitz.EXPECT().EvaluateExperiment(gomock.Any(), gomock.Any()).
        Return(nil, errors.New("splitz timeout"))

    // Fail closed — unknown org follows standard flow
    got := service.IsSoundboxMerchant(context.Background(), mockSplitz, "org_unknown")
    assert.False(t, got)
}
```

---

## Telemetry in Tests

```go
import "github.com/razorpay/qr-codes/internal/testutil"

// Initialise no-op telemetry (required for handler tests that record OTel metrics)
testutil.NewTestTelemetry()
```
