# Key Decisions Log

| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-06-30 | Use Category/Source-based Routing | Custom merchant ref unreliable; category field on intent is deterministic post-UPS-preprocess |
| 2026-06-30 | `category="soundbox"` derived from org code | Bank-led merchants identified by org code, not gateway constant alone |
| 2026-07-06 | UC-1 classification via Splitz experiment, not hardcoded org codes | Onboarding a new bank must not require a code deploy; Splitz experiment `soundbox_notification_only` is evaluated per `org_code` at runtime — flip a flag, not a PR |
| 2026-07-06 | Processor pattern for QR creation and callback handling | No if/else in shared code; `SoundboxQRCreationProcessor` and `SoundboxCallbackProcessor` own their full logic independently; `StandardQRCreationProcessor` / existing flow is untouched; new UC types extend by adding a processor, not modifying existing ones |
| 2026-07-06 | PGR-level Splitz kill switch for the entire optimization | Two-layer safety: qr-codes Splitz gates per-org UC-1 classification; PGR Splitz experiment `soundbox_callback_optimization` gates whether PGR routes to QRMS at all — flip PGR flag to instantly revert all traffic to the existing UPS flow without a code deploy |
| 2026-07-06 | Skip logic lives inside the processor, not in shared code | eze-server DB write suppression, UPS payment processing skip — each is the absence of a call in the UC-1 processor, not a conditional in shared code |
| 2026-06-30 | UPS retained for intent creation in Phase 1 | No SDK/library access to intent module; removing UPS from intent gen is Phase 2 |
| 2026-06-30 | QRMS updates QR payment counters on callback | QR lifecycle tracking requires counter update even without full payment entity |
| 2026-06-30 | Phase 1 uses direct HTTP PGR→QRMS (no queue) | Simplest path for new gateway onboarding; Callback Processing Queue deferred to Phase 2 |
| 2026-06-30 | Phase 2 replaces PGR direct call with Callback Processing Queue | Queue decouples routing from PGR; enables existing gateway migration |
| 2026-06-30 | Phase 2 removes UPS from SQR flow entirely | Long-term cost goal; Phase 1 validates stability first |
| 2026-06-30 | CommX/EPN used in Phase 1; AWS IoT deferred to Phase 2 | AWS IoT migration is independent work stream; Phase 1 reuses existing delivery |
| 2026-06-30 | Remove `integrations-upi` from Phase 1 scope | Phase 1 uses existing gateway integrations; queue-based migration is Phase 2 |

---

## Updating This File

When a new plan or design decision arrives:
1. Add a new row — do not edit existing rows silently
2. If a decision is superseded, ~~strikethrough~~ the old row and add a new one with date
3. If a decision conflicts with architecture, flag for human review before implementing
