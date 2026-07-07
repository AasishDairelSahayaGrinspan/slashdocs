# OTel Metrics Patterns

Source: `internal/metrics/metrics.go` in `qr-codes`

---

## Import

```go
import "go.opentelemetry.io/otel/metric"
```

---

## Initialization Pattern

All metrics are declared as package-level vars and initialized once via `sync.Once` in `Init()`:

```go
var (
    SoundboxCallbackTotal         metric.Int64Counter      // labels: status
    SoundboxNotificationTotal     metric.Int64Counter      // labels: channel, status
    SoundboxNotificationLatencyMs metric.Float64Histogram  // labels: channel
    SoundboxCounterUpdateTotal    metric.Int64Counter      // labels: status
)

func Init(meter metric.Meter) {
    // existing metrics init...

    SoundboxCallbackTotal, _ = meter.Int64Counter(
        "soundbox_callback_handled_total",
        metric.WithDescription("Total soundbox callbacks received from PGR"),
    )
    SoundboxNotificationTotal, _ = meter.Int64Counter(
        "soundbox_notification_total",
        metric.WithDescription("Total soundbox notification delivery attempts"),
    )
    SoundboxNotificationLatencyMs, _ = meter.Float64Histogram(
        "soundbox_notification_latency_ms",
        metric.WithDescription("Soundbox notification delivery latency in milliseconds"),
        // Buckets aligned with SLO targets: p95 < 3s, p99 < 5s (see operations/nfrs.md)
        metric.WithExplicitBucketBoundaries(1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 20000),
    )
    SoundboxCounterUpdateTotal, _ = meter.Int64Counter(
        "soundbox_counter_update_total",
        metric.WithDescription("Total QR code counter update attempts for soundbox callbacks"),
    )
}
```

---

## Label Constants

Add alongside existing label constants:

```go
const (
    // existing
    ClientLabel      = "client"
    ProviderLabel    = "provider"
    // new for soundbox
    GatewayLabel     = "gateway"
    ChannelLabel     = "channel"
    StatusLabel      = "status"
)
```

---

## RecordXxx Helper Pattern

```go
func RecordSoundboxCallback(ctx context.Context, gateway, status string) {
    SoundboxCallbackTotal.Add(ctx, 1,
        metric.WithAttributes(
            attribute.String(GatewayLabel, gateway),
            attribute.String(StatusLabel, status),
        ),
    )
}

func RecordSoundboxCounterUpdate(ctx context.Context, gateway, status string) {
    SoundboxCounterUpdateTotal.Add(ctx, 1,
        metric.WithAttributes(
            attribute.String(GatewayLabel, gateway),
            attribute.String(StatusLabel, status),
        ),
    )
}

func RecordSoundboxNotification(ctx context.Context, gateway, channel, status string, latencyMs float64) {
    SoundboxNotificationTotal.Add(ctx, 1,
        metric.WithAttributes(
            attribute.String(GatewayLabel, gateway),
            attribute.String(ChannelLabel, channel),
            attribute.String(StatusLabel, status),
        ),
    )
    SoundboxNotificationLatencyMs.Record(ctx, latencyMs,
        metric.WithAttributes(
            attribute.String(GatewayLabel, gateway),
            attribute.String(ChannelLabel, channel),
        ),
    )
}
```

---

## Usage in Handler

```go
start := time.Now()

err := s.ezetapClient.SendNotification(ctx, req)
latencyMs := float64(time.Since(start).Milliseconds())

status := "success"
if err != nil {
    status = "failed"
}

metrics.RecordSoundboxCounterUpdate(ctx, "success") // after AtomicIncrementPaymentCounters
metrics.RecordSoundboxNotification(ctx, "comx", status, latencyMs) // Phase 1: channel always "comx"
```

---

## Latency Buckets

Use the same buckets as the rest of the service:
```go
[]float64{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 20000}
```
