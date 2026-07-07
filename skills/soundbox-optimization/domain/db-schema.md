# DB Schema

## `soundbox_audit_logs` Table (DynamoDB)

Replaces EZT transaction records for UC-1 merchants. Append-only. Primary observability source for notification delivery per spec.

qr-codes uses `aws-sdk-go-v2` + `goutils/kvstore` abstraction. No SQL migrations. Tables are defined via JSON schema files at `config/qr-codes/dynamo/`.

### DynamoDB Schema JSON

`config/qr-codes/dynamo/dynamo_soundbox_audit_logs.json`:

```json
{
  "metadata": {"name": "kv_soundbox_audit_log_schema"},
  "fields": [
    { "name": "id",               "type": "string" },
    { "name": "payment_id",       "type": "string" },
    { "name": "qr_code_id",       "type": "string" },
    { "name": "merchant_id",      "type": "string" },
    { "name": "org_code",         "type": "string" },
    { "name": "device_id",        "type": "string" },
    { "name": "amount_paise",     "type": "int"    },
    { "name": "delivery_channel", "type": "string" },
    { "name": "status",           "type": "string" },
    { "name": "failure_reason",   "type": "string" },
    { "name": "use_case",         "type": "string" },
    { "name": "attempted_at",     "type": "int"    },
    { "name": "delivered_at",     "type": "int"    },
    { "name": "latency_ms",       "type": "int"    },
    { "name": "error_code",       "type": "string" }
  ],
  "destinations": [
    {
      "name": "dynamo_soundbox_audit_log_entity",
      "dialect": "dynamodb",
      "table": "soundbox_audit_logs",
      "pk": {
        "name": "pk",
        "type": "string",
        "prefix": "live#sbaudit#",
        "identifier": "#sbaudit#",
        "fields": ["payment_id"]
      },
      "sk": {
        "name": "sk",
        "type": "string",
        "fields": ["attempted_at"]
      },
      "columns": [
        { "name": "id",         "fields": ["id"],               "type": "string", "optional": false },
        { "name": "qr_id",      "fields": ["qr_code_id"],       "type": "string", "optional": true  },
        { "name": "mrch_id",    "fields": ["merchant_id"],       "type": "string", "optional": true  },
        { "name": "org_cd",     "fields": ["org_code"],          "type": "string", "optional": true  },
        { "name": "dev_id",     "fields": ["device_id"],         "type": "string", "optional": true  },
        { "name": "amt",        "fields": ["amount_paise"],      "type": "int",    "optional": false },
        { "name": "dlv_ch",     "fields": ["delivery_channel"],  "type": "string", "optional": false },
        { "name": "status",     "fields": ["status"],            "type": "string", "optional": false },
        { "name": "fail_rsn",   "fields": ["failure_reason"],    "type": "string", "optional": true  },
        { "name": "use_case",   "fields": ["use_case"],          "type": "string", "optional": false },
        { "name": "dlvd_at",    "fields": ["delivered_at"],      "type": "int",    "optional": true  },
        { "name": "lat_ms",     "fields": ["latency_ms"],        "type": "int",    "optional": true  },
        { "name": "err_cd",     "fields": ["error_code"],        "type": "string", "optional": true  }
      ]
    }
  ]
}
```

### Key Design Decisions

| Concern | Decision |
|---------|----------|
| **Partition key** | `payment_id` — unique per payment; drives DynamoDB conditional-put idempotency |
| **Sort key** | `attempted_at` (Unix int64) |
| **Idempotency** | Conditional put with `attribute_not_exists(pk)` — no Redis dependency |
| **GSIs** | None in Phase 1 — primary access is by `payment_id` only |
| **Timestamps** | `int64` Unix seconds, not `time.Time` (consistent with QrCode model) |

---

## Go Model

`internal/qrcodes/model/audit_log.go`:

```go
package model

const (
    AuditStatusDelivered = "delivered"
    AuditStatusFailed    = "failed"

    DeliveryChannelCOMX = "comx"
    DeliveryChannelIoT  = "iot"   // Phase 2

    UseCaseNotificationOnly = "notification_only"

    auditLogTableName   = "soundbox_audit_logs"
    auditLogEntityName  = "soundbox_audit_log"
    auditLogEntityPrefix = "sbaudit_"
)

// AuditLog records each notification delivery attempt for UC-1 callbacks.
// Written synchronously after eze-server responds (success or failure).
type AuditLog struct {
    ID              string  `json:"id"               kvstore:"id"`
    PaymentID       string  `json:"payment_id"       kvstore:"payment_id"`
    QrCodeID        string  `json:"qr_code_id"       kvstore:"qr_code_id"`
    MerchantID      string  `json:"merchant_id"      kvstore:"merchant_id"`
    OrgCode         string  `json:"org_code"         kvstore:"org_code"`
    DeviceID        string  `json:"device_id"        kvstore:"device_id"`
    AmountPaise     int64   `json:"amount_paise"     kvstore:"amount_paise"`
    DeliveryChannel string  `json:"delivery_channel" kvstore:"delivery_channel"`
    Status          string  `json:"status"           kvstore:"status"`
    FailureReason   string  `json:"failure_reason"   kvstore:"failure_reason"`
    UseCase         string  `json:"use_case"         kvstore:"use_case"`
    AttemptedAt     int64   `json:"attempted_at"     kvstore:"attempted_at"`
    DeliveredAt     *int64  `json:"delivered_at"     kvstore:"delivered_at"`
    LatencyMs       *int64  `json:"latency_ms"       kvstore:"latency_ms"`
    ErrorCode       string  `json:"error_code"       kvstore:"error_code"`
}

func (a *AuditLog) TableName() string { return auditLogTableName }
```

> Note: Use `uniqueid.New()` (from `goutils/uniqueid`) for `ID` generation, prefixed with `auditLogEntityPrefix`. Timestamps are Unix seconds: `time.Now().Unix()`.

---

## Repository Interface

```go
type AuditLogRepository interface {
    // Create writes a new audit log entry.
    // Uses DynamoDB conditional put (attribute_not_exists(pk)) for idempotency.
    // Returns ErrDuplicatePayment if payment_id already exists.
    Create(ctx context.Context, log *model.AuditLog) error
}
```

---

## QR Code Counter Fields (existing model, no schema change)

The existing `QrCode` entity in DynamoDB already has these fields — updated by `AtomicIncrementPaymentCounters`:

| Field | kvstore tag | Update |
|-------|-------------|--------|
| `PaymentsReceivedCount` | `payments_received_count` | `+= 1` per callback |
| `PaymentsAmountReceived` | `payments_amount_received` | `+= amount_paise` per callback |

**Existing method:** `AtomicIncrementPaymentCounters(ctx, qrCodeID, createdAt, amountPaise int64)` at
`internal/qrcodes/dynamorepo/kvdynamo_qr_code_repo.go:440`

Uses `ADD` expression with condition `attribute_exists(id) AND status <> 'closed'`.
If QR is closed, condition fails → treat as error → write audit log with `status=failed`.

---

## Phase 2 Schema Changes (PLACEHOLDER)

> To be defined when Phase 2 is scoped.
> Likely: `delivery_channel` values expand to include `iot`; `CallbackEvent` queue schema defined in proto.
