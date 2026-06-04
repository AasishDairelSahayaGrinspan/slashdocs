# Repo Boundaries

## System Identity

go-foundation-v2 is a **Go microservice template** — a starting point that developers clone and rename to build new Razorpay microservices. It provides a working User service that demonstrates every convention: layer structure, error handling, proto workflow, config management, and build system.

## What This Repo Owns

- **Service template structure**: `cmd/`, `internal/<domain>/`, `config/`, `proto/`, `migrations/` layout
- **Example User domain**: `internal/user/` — server, handler, service, repo, model — as a reference implementation
- **Foundation framework wiring**: `cmd/user/main.go` demonstrates how to bootstrap `foundation.NewServer`, wire deps, and call `server.Start`
- **Error handling package**: `internal/errors/` — error class definitions, codes, `WrapError`, `ToProtoPublicError`
- **Custom config pattern**: `internal/config/config.go` — `AppConfig` struct with `mapstructure` tags and `Validate()`
- **Proto source files**: `proto/go_foundation_v2/` — committed for reproducibility; defines the gRPC API contract
- **Build system**: `Makefile` + `scripts/` — local and Docker builds, proto lifecycle, service rename
- **Database migrations**: `migrations/` — SQL migration files managed by `golang-migrate`

## What This Repo Does NOT Own

- **Foundation framework internals**: The Foundation library (`github.com/razorpay/foundation`) is an external dependency. Its source is NOT here. Do not look for interceptor middleware, health check logic, telemetry initialization, or gRPC server setup code in this repo.
- **Generated proto code (`rpc/`)**: This is regenerated from `proto/` via `make proto-generate` and is excluded from git. Any `rpc/` imports only exist after generation.
- **Proto source of truth**: The upstream proto definitions live in `github.com/razorpay/proto`. `make proto-fetch` pulls them here.
- **spine/GORM internals**: `github.com/razorpay/goutils/spine` owns `SoftDeletableModel`, `Repo`, `GetDBError`, `UniqueConstraintViolation`, `RecordNotFound`. This repo only uses them.
- **Authentication/JWT logic**: Handled by Foundation interceptors (`[grpc_server.interceptor.passport]` in TOML). No auth code lives here.
- **Error mapping module**: `github.com/razorpay/error-mapping-module` maps error codes to customer-facing messages. This repo registers service names in config but does not own the mapping logic.

## Key Constraints and Invariants

- **Module path**: `github.com/razorpay/go-foundation-v2`. After `make rename`, all import paths change to the new module name.
- **Go version**: 1.25+
- **Dual transport always**: Every RPC method gets both gRPC registration (`GRPCHandler`) and HTTP gateway registration (`HTTPHandler`). Registering only one is not the pattern.
- **Soft deletes always**: Domain models embed `spine.SoftDeletableModel` — hard deletes are not used; `deleted_at` is a nullable bigint.
- **DB timestamps as bigints**: `created_at`, `updated_at`, `deleted_at` are Unix epoch integers, not SQL `TIMESTAMP` columns.
- **Proto files are committed, generated code is not**: `proto/` is in git; `rpc/` is gitignored. This is intentional for reproducibility.

## Subsystem Layout

```
cmd/<service>/main.go         Bootstrap: Foundation wiring only
internal/<domain>/
  server.go                   gRPC handler: validation + service delegation + dual error return
  handler.go                  Registration: GRPCHandler + HTTPHandler
  service/
    service.go                Business logic: sentinel error translation
    repo.go                   Repo interface (for mock injection)
  repo/repo.go                Data access: spine.Repo + GORM
  model/model.go              Domain model: spine.SoftDeletableModel + ToProto/FromProto
internal/errors/              Error taxonomy: classes, codes, WrapError, ToProtoPublicError
internal/config/              Custom config: AppConfig + Validate()
config/<service>/             TOML config files (default.toml, dev.toml, route_config.toml)
proto/<module>/<entity>/v1/   Proto source (COMMITTED)
rpc/                          Generated Go code (NOT committed, gitignored)
migrations/                   SQL migrations (golang-migrate format)
```

## How This Differs from go-foundation (v1)

go-foundation (`github.com/razorpay/foundation`) is the **library** that this template depends on. go-foundation-v2 is a **service template** that consumes that library. Key differences:

| Aspect | go-foundation (library) | go-foundation-v2 (template) |
|--------|------------------------|----------------------------|
| Role | Framework/library | Service starter template |
| Import | `github.com/razorpay/foundation` | `github.com/razorpay/go-foundation-v2` |
| What it provides | Server lifecycle, interceptors, telemetry, DB pools | Wiring patterns, example domain, error package, build system |
| Deployed? | No (library) | Yes (template for deployable services) |
| Versioned API? | Yes (library releases) | No (clone and rename) |

The "v2" in the name refers to a second-generation template design (using Foundation library + DDD patterns), not a v2 of the Foundation library itself.
