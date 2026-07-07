# Phase 2 Implementation Tasks

> **STATUS: PLACEHOLDER** — Phase 2 is scoped but not yet ready to implement.
> This file will be detailed once Phase 1 is stable in production.

---

## Phase 2 Goals (from spec)

1. Remove UPS completely from the SQR flow, including intent string generation
2. Deprecate COMX/EPN as the notification delivery mechanism
3. Introduce AWS IoT as the primary notification channel for devices
4. Migrate existing gateways (Axis, HDFC, etc.) to the notification-only flow

---

## High-Level Tasks (to be detailed)

| Task | Description | Repo |
|------|-------------|------|
| P2-1 | Define `CallbackEvent` proto | `integrations-upi` |
| P2-2 | Kafka consumer in `qr-codes` | `qr-codes` |
| P2-3 | Remove UPS from SQR intent generation | `qr-codes`, `payments-upi` |
| P2-4 | AWS IoT client in `qr-codes` | `qr-codes` |
| P2-5 | AWS IoT handler in `eze-server` | `eze-server` |
| P2-6 | Migrate `integrations-upi` gateway handlers to queue | `integrations-upi` |
| P2-7 | Deprecate COMX/EPN notification path | `eze-server` |

---

## Known Design Decisions for Phase 2

- `CallbackEvent` proto will be owned by `integrations-upi` (producer); `qr-codes` generates consumer stubs
- AWS IoT uses MQTT; `delivery_channel` in `audit_log` switches from `comx` → `iot`
- Existing gateway migration will be one gateway at a time with monitoring between each

---

> **To fill in:** When Phase 2 is ready, replace this file with the same detailed format as `phase1-tasks.md` — task by task with code specs, test requirements, and a dependency graph.
