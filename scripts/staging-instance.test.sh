#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
SCRIPT="$ROOT_DIR/scripts/staging-instance"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/kandev-staging-instance-test.XXXXXX")"
trap 'rm -rf "$TEST_ROOT"' EXIT

bash -n "$SCRIPT"

help_home="$TEST_ROOT/help-home"
mkdir -p "$help_home"
help_output="$(HOME="$help_home" "$SCRIPT" --help)"
grep -Fq "scripts/staging-instance start" <<<"$help_output"
[[ ! -e "$help_home/.kandev-staging" ]]

config_output="$(KANDEV_STAGING_ROOT="$TEST_ROOT/config-root" "$SCRIPT" print-config --port 11002 --agentctl-port 11003 --agentctl-base 11100 --agentctl-max 11109)"
grep -Fq "database:           $TEST_ROOT/config-root/kandev/data/kandev.db" <<<"$config_output"
grep -Fq "backend bind:       127.0.0.1:11002" <<<"$config_output"
grep -Fq "agentctl range:     11100-11109" <<<"$config_output"
grep -Fq "authentication:     off" <<<"$config_output"
grep -Fq "production assets:  embedded (KANDEV_WEB_INTERNAL_URL unset)" <<<"$config_output"

remote_output="$(KANDEV_STAGING_ROOT="$TEST_ROOT/remote-root" "$SCRIPT" print-config --remote --port 11012 --agentctl-port 11013 --agentctl-base 11110 --agentctl-max 11119)"
grep -Fq "backend bind:       0.0.0.0:11012" <<<"$remote_output"
grep -Fq "authentication:     on" <<<"$remote_output"

if KANDEV_STAGING_ROOT="$TEST_ROOT/unauthenticated-root" "$SCRIPT" print-config --host 0.0.0.0 --no-auth >/dev/null 2>&1; then
  echo "unauthenticated non-loopback bind unexpectedly succeeded" >&2
  exit 1
fi

if KANDEV_STAGING_ROOT="$TEST_ROOT/guard-root" "$SCRIPT" start --no-build --port 10101 >/dev/null 2>&1; then
  echo "main backend port guard unexpectedly passed" >&2
  exit 1
fi

echo "staging-instance tests passed"
