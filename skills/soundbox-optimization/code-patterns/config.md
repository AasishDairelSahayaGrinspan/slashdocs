# Config Patterns

Source: `internal/config/config.go` in `qr-codes`

---

## Service Config Pattern

Every new external service follows this three-struct pattern with `mapstructure` tags:

```go
type EzetapService struct {
    Config   EzetapServiceConfig   `mapstructure:"config"`
    Auth     EzetapServiceAuth     `mapstructure:"auth"`
    ConnPool EzetapServiceConnPool `mapstructure:"conn_pool"`
}

type EzetapServiceConfig struct {
    BaseURL string `mapstructure:"base_url"`
    Mock    bool   `mapstructure:"mock"`
}

type EzetapServiceAuth struct {
    HmacSecret string `mapstructure:"hmac_secret"`
}

type EzetapServiceConnPool struct {
    Timeout            int `mapstructure:"timeout"`
    KeepAliveTimeout   int `mapstructure:"keep_alive_timeout"`
    MaxIdleConnections int `mapstructure:"max_idle_connections"`
    RetryCount         int `mapstructure:"retry_count"`
}
```

Add to the top-level `Config` struct:
```go
type Config struct {
    // ... existing fields ...
    EzetapService EzetapService `mapstructure:"ezetap_service"`
}
```

---

## TOML Config File

`config/qr-codes/default.toml`:
```toml
[ezetap_service]
  [ezetap_service.config]
    base_url = "http://eze-server-internal"
    mock = false
  [ezetap_service.auth]
    hmac_secret = ""          # populated via environment variable or secrets manager in non-dev envs
  [ezetap_service.conn_pool]
    timeout = 5000            # ms
    keep_alive_timeout = 30000
    max_idle_connections = 10
    retry_count = 1
```

---

## Mock Mode

When `mock = true`, the service client returns a no-op/success response without making real HTTP calls. This is used in local dev and tests.

```go
func NewEzetapClient(cfg config.EzetapService) EzetapClient {
    if cfg.Config.Mock {
        return &MockEzetapClient{}
    }
    return &HTTPEzetapClient{
        baseURL:    cfg.Config.BaseURL,
        hmacSecret: cfg.Auth.HmacSecret,
        // configure conn pool from cfg.ConnPool
    }
}
```

---

## Accessing Config in Boot

Config is loaded once at startup and passed via dependency injection:

```go
// internal/boot/boot.go (or similar)
ezetapClient := ezetap.NewEzetapClient(cfg.EzetapService)
```

Never read config directly from viper in business logic — always access via the injected `Config` struct.
