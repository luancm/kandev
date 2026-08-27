# ADR-2026-08-20: Isolate Persistent Production-Built Staging Instances

**Status:** accepted
**Date:** 2026-08-20
**Area:** infra

## Context

Source-checkout development uses Vite and temporary state, which is useful for editing but produces a slow module-loading path and is not a durable environment for testing several tasks. A staging instance needs to keep its database and runtime state across restarts while remaining unable to mutate the user's main Kandev home or accidentally inherit its ports, task context, credentials, or debug profile.

## Decision

Provide `scripts/staging-instance` as the supported source-checkout operator wrapper. It builds the web application and synchronizes embedded production assets before starting the raw backend binary, leaving `KANDEV_WEB_INTERNAL_URL` unset so the backend serves the bundle directly.

The wrapper owns a separate persistent root, defaulting to `~/.kandev-staging`, with an independent `KANDEV_HOME_DIR`, SQLite database, process home, logs, PID file, temporary directory, standalone agentctl port, and agentctl execution-port range. The default backend port is `10102`; `10101` is reserved as the known main-instance port and is rejected. All requested ports are preflighted before startup.

The runtime starts with a clean environment. It keeps only the executable search path and locale plus explicit staging variables, so `KANDEV_TASK_ID`, parent database/port variables, credentials, and development selectors cannot leak into the child. The default provider mode enables mock agents and integration clients while retaining the production profile; real providers require an explicit `--real-providers` choice. Non-loopback binds enable Kandev authentication by default and unauthenticated remote binds require an explicit override.

The wrapper provides `start`, `stop`, `restart`, `rebuild`, `status`, `logs`, `build`, and `print-config`. Stop and rebuild preserve the staging root and database. Boot-time service-manager installation and database cloning are not part of this first operator surface.

## Consequences

Production-bundle browser startup avoids the Vite development module waterfall and the staging database remains available to later tasks. A clean child environment and runtime-state ownership lock prevent the common cross-instance corruption paths. The staging process must be rebuilt explicitly after source changes, and a staging run with real providers or host executors can still create external effects by deliberate user action.

The wrapper is source-checkout tooling rather than a new installed CLI command. Operators who need boot-time restart can place the wrapper behind their existing systemd or launchd controls after choosing an explicit staging root and environment.

## Alternatives Considered

1. Reuse `make dev`: rejected because it intentionally runs Vite, uses development defaults, and treats state as checkout-local rather than as a durable staging database.
2. Run a second `kandev start` directly: rejected because it does not provide a persistent staging lifecycle, clean environment boundary, provider safety defaults, or a guard for the known main port.
3. Install a second managed Kandev service: deferred because the current service installer does not reliably pin the requested backend port on every platform; the wrapper gives deterministic port and state isolation first.
