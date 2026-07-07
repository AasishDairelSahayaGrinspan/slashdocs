# Non-Functional Requirements

| NFR | Target |
|-----|--------|
| Notification delivery rate | > 99.9% |
| e2e notification latency p95 | < 3 seconds |
| e2e notification latency p99 | < 5 seconds |
| Throughput | 4M+ txns/month; handles 10x burst |
| DB writes reduction | ~9–12M writes/month eliminated |
| Zero regression | Existing gateway txns fully unaffected |
| Idempotency | 100% — duplicate payment_id never double-notifies (DynamoDB conditional put) |
| Handler availability | 99.9% uptime (SLA matches qr-codes service) |
