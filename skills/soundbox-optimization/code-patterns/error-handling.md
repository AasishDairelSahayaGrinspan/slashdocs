# Error Handling Patterns

Source: `internal/errors/errors.go` and service layer in `qr-codes`

---

## Error Classes

```go
import goutilserrors "github.com/razorpay/goutils/errors"

var (
    ErrValidationFailure   = goutilserrors.NewClass("validation_failure", SeverityLow, ValidationFailure)
    ErrInternalServerError = goutilserrors.NewClass("internal_error", SeverityHigh, Recoverable)
    ErrBadRequestError     = goutilserrors.NewClass("bad_request", SeverityLow, BadRequest)
    ErrNotFound            = goutilserrors.NewClass("not_found", SeverityLow, BadRequest)
    ErrEntityAlreadyExists = goutilserrors.NewClass("already_exists", SeverityLow, BadRequest)
    ErrDuplicateRequest    = goutilserrors.NewClass("duplicate_request", SeverityLow, BadRequest)
    ErrContextCancelled    = goutilserrors.NewClass("CONTEXT_CANCELLED", SeverityHigh, Base)
)
```

Use existing classes from `internal/errors/errors.go`. Add new classes only if none fit.

---

## Creating and Wrapping Errors

```go
// New error with code
return nil, errpkg.ErrNotFound.
    New(errpkg.CodeQRCodeNotFound).
    WithPublicDescriptionParams("QR code not found for device").
    Wrap(err)

// Validation failure
return nil, errpkg.ErrValidationFailure.
    New(errpkg.CodeBadRequestError).
    WithPublicDescriptionParams("payment_id is required")

// Internal error wrapping upstream
return nil, errpkg.ErrInternalServerError.
    New(errpkg.CodeInternalServerError).
    Wrap(fmt.Errorf("ezetap client failed: %w", err))
```

---

## Critical Operations (fail-fast)

Fail the entire operation when a critical dependency fails:

```go
// Counter update failure (e.g., QR code is closed)
if err := repo.AtomicIncrementPaymentCounters(ctx, qrID, now, amountPaise); err != nil {
    boot.Logger(ctx).Error("failed to update QR code payment counters",
        "error", err.Error(),
        "qr_id", qrID,
        "payment_id", paymentID,
    )
    // best-effort: write audit log with status=failed, return 200 to PGR
    return errpkg.ErrInternalServerError.
        New(errpkg.CodeInternalServerError).
        Wrap(err)
}
```

---

## Best-Effort Operations (continue on failure)

Log a warning and continue when a non-critical operation fails:

```go
err := s.ezetapClient.SendNotification(ctx, notifReq)
if err != nil {
    boot.Logger(ctx).Warn("failed to deliver soundbox notification (best-effort)",
        "error", err.Error(),
        "device_id", notifReq.DeviceID,
        "payment_id", notifReq.PaymentID,
    )
    // continue — do not return error
}
```

---

## gRPC Error Wrapping

When returning errors from gRPC server handlers:

```go
import qrerrors "github.com/razorpay/qr-codes/internal/errors"

return nil, qrerrors.WrapError(err.(goutilserrors.IError))
```

---

## HTTP Handler Error Response

For the callback handler returning to PGR, always return 200 (see business rules):

```go
func writeError(w http.ResponseWriter, msg string, code int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// In handler: never return 5xx to PGR
// Even on internal failure, return 200 with a logged warning
```
