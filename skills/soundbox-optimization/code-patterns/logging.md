# Logging Patterns

Source: `internal/qrcodes/service/` in `qr-codes`

---

## Logger Access

```go
import "github.com/razorpay/foundation/boot"

boot.Logger(ctx).Info(...)
boot.Logger(ctx).Warn(...)
boot.Logger(ctx).Error(...)
```

Always pass `ctx` — it carries trace/request ID for correlation.

---

## Log Level Guidelines

| Level | When to use |
|-------|-------------|
| `Info` | Successful operations, state transitions |
| `Warn` | Best-effort failures, degraded but continuing |
| `Error` | Operation-blocking failures, unexpected states |
| `Debug` | DB queries, detailed internal state (not prod-visible by default) |

---

## Message Casing Convention

- **Info:** Sentence case — `"QR code created successfully"`
- **Warn/Error:** lowercase — `"failed to deliver notification"`, `"ezetap client returned non-2xx"`

---

## Structured Fields

Always include relevant IDs as key-value pairs:

```go
boot.Logger(ctx).Info("soundbox callback processed",
    "payment_id", paymentID,
    "device_id", deviceID,
    "qr_code_id", qrCodeID,
    "gateway", gateway,
    "latency_ms", latencyMs,
)

boot.Logger(ctx).Warn("failed to deliver soundbox notification (best-effort)",
    "error", err.Error(),
    "payment_id", paymentID,
    "device_id", deviceID,
    "gateway", gateway,
)

boot.Logger(ctx).Error("failed to create audit log entry",
    "error", err.Error(),
    "payment_id", paymentID,
)
```

---

## What NOT to Log

- Do not log `hmac_secret` or any credential
- Do not log full request body at Info level (use Debug)
- Do not duplicate the same field twice in one log call
