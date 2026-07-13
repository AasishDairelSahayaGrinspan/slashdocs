# aidocs Grafana dashboard

This directory contains a portable Prometheus/Grafana dashboard for aidocs app and business metrics.

The MCP section tracks protocol and transport failures separately from tool
failures, plus per-tool call volume and latency. The two error-ratio stats are
intentionally distinct: rejected or invalid MCP requests count toward the
protocol/transport ratio, while an aidocs operation that returns a tool error
counts toward the tool ratio. This distinction matters because a valid MCP
response can report a tool error while still returning HTTP 200. Tool and method
labels are restricted to the registered, finite catalog; document IDs, users,
client names, and other unbounded values are never labels.

- `aidocs-dashboard.libsonnet` is the Grafonnet source of truth.
- `aidocs-dashboard.json` is the rendered dashboard for direct Grafana import or provisioning.
- `jsonnetfile.json` and `jsonnetfile.lock.json` pin Grafonnet dependencies.
- `vendor/` is intentionally ignored; regenerate it with `jb install`.

Render locally:

```bash
make dashboards
```

Verify the committed JSON is up to date:

```bash
make check-dashboards
```

CI runs `make check-dashboards` and fails if the rendered JSON differs from the committed JSON.

The dashboard uses a datasource variable named `DS_PROMETHEUS`, so it should work across internal, hosted, and self-hosted deployments after selecting the right Prometheus datasource in Grafana.
