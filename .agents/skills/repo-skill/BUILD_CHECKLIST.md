# Build Checklist

Agent-readiness knowledge base for `github.com/razorpay/go-foundation-v2`.

- [x] `SKILL.md` — orchestrator: load-when table, key gotchas
- [x] `core/boundaries.md` — repo scope, what it owns vs not, subsystem layout, v1 vs v2 distinction
- [x] `core/architecture.md` — layer structure, DI wiring, handler registration, ports
- [x] `core/foundation-framework.md` — Foundation bootstrap, Container deps, start options, config sections
- [x] `core/proto-grpc-workflow.md` — proto layout, buf commands, make targets, why proto/ committed
- [x] `core/error-handling.md` — error classes, codes, WrapError, ToProtoPublicError, dual return pattern
- [x] `core/build-system.md` — Makefile variables, service rename, build commands, CI/CD
- [x] `modules/domain/model-patterns.md` — spine.SoftDeletableModel, TableName/EntityName/EntityPrefix, ToProto/FromProto, public ID format
- [x] `modules/technical/database-patterns.md` — spine.Repo, DBInstance(ctx), GetDBError, sentinel errors, dual DB setup
- [x] `modules/technical/config-management.md` — AppConfig, mapstructure tags, Validate(), Foundation-managed sections
- [x] `modules/technical/testing-patterns.md` — test file convention, Repo interface for mocking, testify, test naming
