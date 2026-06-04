---
name: go-foundation-v2-repo-skill
description: Deep domain knowledge for the go-foundation-v2 service template. Provides architectural context about layer wiring, Foundation framework usage, proto/gRPC workflow, error handling, database patterns, and build system — enabling agents to correctly author changes without re-discovering conventions from scratch.
user-invocable: false
---

# go-foundation-v2 Repo Skill

## What This Skill Covers

go-foundation-v2 is a **production-ready Go microservice template** built on the Razorpay Foundation framework. It provides a working User service example demonstrating canonical DDD layering with gRPC+HTTP dual transport, PostgreSQL (primary + replica), proto-driven API contracts, and structured error handling.

## Load-When Table

| Sub-module | Load when... |
|-----------|-------------|
| `core/boundaries.md` | Starting any work in this repo; onboarding; understanding repo scope vs Foundation library scope |
| `core/architecture.md` | Adding/modifying layers, wiring dependencies, server startup, understanding cmd→server→service→repo→model flow |
| `core/foundation-framework.md` | Configuring Foundation server (`NewServer`, `Container`, `WithGRPCHandlers`, `WithHealthChecks`), telemetry, DB collection |
| `core/proto-grpc-workflow.md` | Working with `.proto` files, `buf` commands, HTTP annotations, gRPC/HTTP handler registration, `rpc/` generation |
| `core/error-handling.md` | Creating errors, error codes, gRPC status mapping, `WrapError`, `ToProtoPublicError`, dual return pattern |
| `core/build-system.md` | Makefile targets, service rename (`make rename`), Docker build, CI/CD, proto commands |
| `modules/domain/model-patterns.md` | Domain models, `spine.SoftDeletableModel`, `ToProto`/`FromProto`, `EntityPrefix`, public vs internal IDs |
| `modules/technical/database-patterns.md` | `spine.Repo`, GORM, `DBInstance(ctx)`, `GetDBError`, primary/replica DB, Repo interface |
| `modules/technical/config-management.md` | Custom config structs, TOML layout, `mapstructure` tags, `Validate()`, Foundation-managed sections |
| `modules/technical/testing-patterns.md` | Unit tests, mock injection via interfaces, testify patterns, test naming |

## Key Gotchas

1. **`rpc/` is never committed.** The `rpc/` directory (generated Go code from proto files) is in `.gitignore`. If `rpc/` imports fail locally, run `make proto-generate` first. Docker/CI does this automatically.

2. **Dual error return in every gRPC handler.** Always embed the error in the response proto AND return it as a gRPC status error: `return &Proto{Error: errpkg.ToProtoPublicError(err)}, errpkg.WrapError(err)`. Omitting either breaks HTTP or gRPC clients respectively.

3. **Service and repo layers return `errors.IError`, never `error`.** Using the standard `error` interface breaks the error class mapping and gRPC status code derivation.

4. **`r.database.DBInstance(ctx)`, not `r.database.Db`.** Using `Db` directly skips context propagation for tracing, timeouts, and transactions.

5. **Sentinel error checks belong in the service layer, not the repo.** The repo calls `spine.GetDBError(tx)` and returns; the service checks `errors.Is(err, spine.UniqueConstraintViolation)` and `errors.Is(err, spine.RecordNotFound)` to produce domain-specific errors.

6. **`mapstructure` tags on config structs, not `toml`.** Foundation uses viper/mapstructure internally — `toml` tags are ignored.

7. **`make rename NAME=<service>` does NOT update `.github/workflows/*.yaml` or `Dockerfile.*`.** These must be manually updated after rename.

8. **`protovalidate.New()` can fail — always check the error from `user.New()`.** The server constructor returns an error; callers must handle it (see `cmd/user/main.go`).
