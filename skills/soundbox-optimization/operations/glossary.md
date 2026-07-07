# Glossary

| Term | Meaning |
|------|---------|
| SQR | Static QR code |
| UPS | Unified Payment Service — Razorpay's internal payment processing service |
| QRMS | QR Microservice — the `qr-codes` repo |
| PGR | PG Router — routes callbacks to appropriate processors; reads `intent.category` for soundbox routing |
| Mozart | Gateway adapter layer inside UPS — handles encryption/decryption and intent resolution |
| EZT / eze-server | Ezetap server — validates HMAC, resolves device, delivers audio notification via CommX or AWS IoT |
| CommX / EPN | Existing SoundBox notification channel (Kafka-based); used in Phase 1 |
| AWS IoT | MQTT-based notification channel — Phase 2 replacement for CommX |
| **PaymentCallback** | Domain entity: raw event from UPI PSP via gateway (payment_id, amount, device_id, qr_id) |
| **CPS** | Callback Payment Service — Razorpay's payment callback orchestrator used by PGR; abstracts UPS Decrypt + Preprocess into a single unified call (`CallbackPaymentCPS`) |
| **AuditLog** | Domain entity: immutable delivery record per notification attempt; replaces EZT transaction records for UC-1 |
| **DeviceMapping** | Domain entity: lookup resolving SoundBox device ID → notification endpoint (CommX vs IoT) |
| **CallbackEvent** | Domain entity: message schema for Callback Processing Queue (Phase 2 only) |
| Callback Processing Queue | Kafka/Metro topic — `integrations-upi` publishes, `qr-codes` consumes (Phase 2) |
| `soundbox` | The `category` value on UPS intent identifying notification-only SoundBox merchant flow |
| Org code | Merchant organisation code used to determine UC classification at QR creation time |
| UC-1 | Notification-only use case — bank owns transaction record; Razorpay delivers QR counter update + audio alert |
| UC-2 | Full history use case — Razorpay owns transaction record; refunds and reports available |
| HMAC | Hash-based Message Authentication Code — authenticates QRMS→eze-server notification requests |
| WDA | Write-Designed Aggregates — TiDB query service used for device-to-QR lookups |
| KVStore | DynamoDB abstraction layer in `goutils/kvstore` |
| Stork | Razorpay's webhook dispatch service (skipped for UC-1) |
