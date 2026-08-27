#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
SCRIPT="$ROOT_DIR/scripts/staging-instance"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/kandev-staging-instance-test.XXXXXX")"
fake_pid=""
cleanup() {
  if [[ -n "$fake_pid" ]]; then
    kill "$fake_pid" 2>/dev/null || true
    wait "$fake_pid" 2>/dev/null || true
  fi
  rm -rf "$TEST_ROOT"
}
trap cleanup EXIT

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

# Canonical protected roots before any mkdir/chmod. A symlink alias to HOME
# must be rejected without creating staging state in the protected target.
protected_home="$TEST_ROOT/protected-home"
home_alias="$TEST_ROOT/home-alias"
mkdir -p "$protected_home"
ln -s "$protected_home" "$home_alias"
if HOME="$protected_home" KANDEV_STAGING_ROOT="$home_alias" "$SCRIPT" print-config >/dev/null 2>&1; then
  echo "HOME symlink alias unexpectedly accepted" >&2
  exit 1
fi
[[ ! -e "$protected_home/kandev" ]]

# The main Kandev home and broad roots are protected after canonicalization as
# well, including aliases that would otherwise pass the string comparison.
main_home="$TEST_ROOT/main-home/.kandev"
main_alias="$TEST_ROOT/main-home-alias"
mkdir -p "$main_home"
ln -s "$main_home" "$main_alias"
if HOME="$TEST_ROOT/main-home" KANDEV_STAGING_ROOT="$main_alias" "$SCRIPT" print-config >/dev/null 2>&1; then
  echo "main Kandev home symlink alias unexpectedly accepted" >&2
  exit 1
fi

if KANDEV_STAGING_ROOT="$TEST_ROOT/broad-root" "$SCRIPT" print-config --root /tmp >/dev/null 2>&1; then
  echo "broad root unexpectedly accepted" >&2
  exit 1
fi

if KANDEV_STAGING_ROOT="$TEST_ROOT/overlap-root" "$SCRIPT" print-config --agentctl-port 11105 --agentctl-base 11100 --agentctl-max 11109 >/dev/null 2>&1; then
  echo "standalone agentctl port overlap unexpectedly accepted" >&2
  exit 1
fi

ipv6_output="$(KANDEV_STAGING_ROOT="$TEST_ROOT/ipv6-root" "$SCRIPT" print-config --host ::1 --port 11042 --agentctl-port 11043 --agentctl-base 11050 --agentctl-max 11059)"
grep -Fq "local URL:          http://[::1]:11042" <<<"$ipv6_output"

# A live process matching the old broad command check but lacking the
# launcher's persisted identity must be treated as a conflict, never signaled.
identity_root="$TEST_ROOT/identity-root"
fake_bin="$TEST_ROOT/fake-kandev"
mkdir -p "$identity_root/run" "$fake_bin"
cat > "$fake_bin/kandev" <<'EOF'
#!/usr/bin/env bash
while :; do sleep 1; done
EOF
chmod +x "$fake_bin/kandev"
"$fake_bin/kandev" __backend &
fake_pid=$!
printf '%s\n' "$fake_pid" > "$identity_root/run/backend.pid"
identity_output="$(KANDEV_STAGING_ROOT="$identity_root" "$SCRIPT" status 2>&1 || true)"
grep -Fq "status:             pid file conflict" <<<"$identity_output"
kill "$fake_pid" 2>/dev/null || true
wait "$fake_pid" 2>/dev/null || true
fake_pid=""

echo "staging-instance tests passed"
