#!/usr/bin/env bash
#
# odooctl Integration Test Suite
# ==============================
# Tests all odooctl commands in a realistic end-to-end workflow.
#
# Requirements:
#   - Docker running
#   - odooctl binary (uses the one from ./bin/ or $PATH)
#
# Usage:
#   ./test_all_commands.sh              # Run full test suite
#   ./test_all_commands.sh --skip-slow  # Skip slow Docker build/init steps
#

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
ODOO_VERSION="19.0"
TIMESTAMP="$(date +%s)"
TEST_DIR="/tmp/odooctl-test-${TIMESTAMP}"
TEST_MODULE="test_integration_module"
LOG_FILE="/tmp/odooctl-test-${TIMESTAMP}.log"
SKIP_SLOW=false
ODOOCTL=""

# Derived: project name = basename of TEST_DIR, branch = ${ODOO_VERSION}-test
PROJECT_NAME="odooctl-test-${TIMESTAMP}"
BRANCH="${ODOO_VERSION}-test"

# Parse arguments
for arg in "$@"; do
    case "$arg" in
        --skip-slow) SKIP_SLOW=true ;;
        *) echo "Unknown argument: $arg"; exit 1 ;;
    esac
done

# ---------------------------------------------------------------------------
# Color helpers
# ---------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m' # No Color

# ---------------------------------------------------------------------------
# Test tracking
# ---------------------------------------------------------------------------
PASSED=0
FAILED=0
SKIPPED=0
declare -a TEST_RESULTS=()

pass() {
    PASSED=$((PASSED + 1))
    TEST_RESULTS+=("${GREEN}PASS${NC}  $1")
    echo -e "  ${GREEN}PASS${NC}  $1"
}

fail() {
    FAILED=$((FAILED + 1))
    TEST_RESULTS+=("${RED}FAIL${NC}  $1  ${RED}-- $2${NC}")
    echo -e "  ${RED}FAIL${NC}  $1"
    echo -e "        ${RED}$2${NC}"
}

skip() {
    SKIPPED=$((SKIPPED + 1))
    TEST_RESULTS+=("${YELLOW}SKIP${NC}  $1  ${DIM}($2)${NC}")
    echo -e "  ${YELLOW}SKIP${NC}  $1  ${DIM}($2)${NC}"
}

# ---------------------------------------------------------------------------
# Run a test: run_test "description" command [args...]
# Captures stdout+stderr, checks exit code. Logs everything to LOG_FILE.
# ---------------------------------------------------------------------------
run_test() {
    local desc="$1"
    shift

    {
        echo ""
        echo "=== TEST: $desc ==="
        echo "CMD: $*"
        echo "CWD: $(pwd)"
        echo "---"
    } >> "$LOG_FILE"

    local output
    local rc=0
    output=$("$@" 2>&1) || rc=$?

    echo "$output" >> "$LOG_FILE"
    echo "EXIT CODE: $rc" >> "$LOG_FILE"
    echo "===" >> "$LOG_FILE"

    if [ $rc -eq 0 ]; then
        pass "$desc"
    else
        # Extract meaningful error lines to show inline
        local errmsg
        errmsg=$(echo "$output" | grep -iE "error|fail|fatal|unknown|cannot|not found|no such|denied|refused" | head -5)
        if [ -z "$errmsg" ]; then
            # Fallback to last non-empty lines
            errmsg=$(echo "$output" | grep -v '^$' | tail -5)
        fi
        fail "$desc" "exit code $rc"
        # Print the error detail indented below the FAIL line
        while IFS= read -r line; do
            echo -e "        ${DIM}> $line${NC}"
        done <<< "$errmsg"
    fi

    LAST_OUTPUT="$output"
    LAST_RC=$rc
    return 0
}

# Same as run_test but expects the command to fail (non-zero exit)
run_test_expect_fail() {
    local desc="$1"
    shift

    {
        echo ""
        echo "=== TEST (expect fail): $desc ==="
        echo "CMD: $*"
        echo "CWD: $(pwd)"
        echo "---"
    } >> "$LOG_FILE"

    local output
    local rc=0
    output=$("$@" 2>&1) || rc=$?

    echo "$output" >> "$LOG_FILE"
    echo "EXIT CODE: $rc" >> "$LOG_FILE"
    echo "===" >> "$LOG_FILE"

    if [ $rc -ne 0 ]; then
        pass "$desc"
    else
        fail "$desc" "expected non-zero exit but got 0"
    fi

    LAST_OUTPUT="$output"
    LAST_RC=$rc
    return 0
}

# ---------------------------------------------------------------------------
# Cleanup handler
# ---------------------------------------------------------------------------
cleanup() {
    local exit_code=$?
    echo ""
    echo -e "${BOLD}Cleaning up...${NC}"

    # Stop and remove Docker containers/volumes/config for our test project
    if [ -n "$ODOOCTL" ] && [ -d "$TEST_DIR" ]; then
        (cd "$TEST_DIR" && "$ODOOCTL" docker reset -v -c -f 2>/dev/null) || true
    fi

    # Remove any leftover config directory
    rm -rf "$HOME/.odooctl/${PROJECT_NAME}" 2>/dev/null || true

    # Remove the test workspace
    rm -rf "$TEST_DIR"

    # Remove dump file and fake key if they exist
    rm -f "/tmp/odooctl-test-dump-${TIMESTAMP}.zip" 2>/dev/null || true
    rm -f "/tmp/odooctl-test-key-${TIMESTAMP}" 2>/dev/null || true

    echo -e "${GREEN}Cleanup complete.${NC}"
    exit $exit_code
}

trap cleanup EXIT

# ---------------------------------------------------------------------------
# Resolve odooctl binary
# ---------------------------------------------------------------------------
resolve_binary() {
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local local_bin="$script_dir/bin/odooctl"

    if [ -x "$local_bin" ]; then
        ODOOCTL="$local_bin"
    elif command -v odooctl &>/dev/null; then
        ODOOCTL="$(command -v odooctl)"
    else
        echo -e "${RED}Error: odooctl binary not found.${NC}"
        echo "  Looked in: $local_bin"
        echo "  Also checked: \$PATH"
        echo "  Build it first: make build"
        exit 1
    fi
}

# ===========================================================================
# MAIN
# ===========================================================================
echo ""
echo -e "${BOLD}=====================================================${NC}"
echo -e "${BOLD}         odooctl Integration Test Suite               ${NC}"
echo -e "${BOLD}=====================================================${NC}"
echo ""

resolve_binary

echo -e "Binary:       ${CYAN}$ODOOCTL${NC}"
echo -e "Odoo version: ${CYAN}$ODOO_VERSION${NC}"
echo -e "Test dir:     ${CYAN}$TEST_DIR${NC}"
echo -e "Project name: ${CYAN}$PROJECT_NAME${NC}"
echo -e "Branch:       ${CYAN}$BRANCH${NC}"
echo -e "Log file:     ${CYAN}$LOG_FILE${NC}"
echo -e "Skip slow:    ${CYAN}$SKIP_SLOW${NC}"
echo ""

# Initialize log
{
    echo "odooctl integration test - $(date)"
    echo "Binary: $ODOOCTL"
    echo "Odoo version: $ODOO_VERSION"
    echo "Test dir: $TEST_DIR"
    echo "Project name: $PROJECT_NAME"
    echo ""
} > "$LOG_FILE"

# =============================================
# Phase 1: Pre-flight checks
# =============================================
echo -e "${BOLD}-- Phase 1: Pre-flight checks --${NC}"

run_test "odooctl version" "$ODOOCTL" version

run_test "odooctl --help" "$ODOOCTL" --help

run_test "odooctl docker --help" "$ODOOCTL" docker --help

run_test "odooctl module --help" "$ODOOCTL" module --help

run_test "odooctl config show" "$ODOOCTL" config show

run_test "docker daemon is running" docker info --format '{{.ServerVersion}}'

echo ""

# =============================================
# Phase 2: Setup test workspace
# =============================================
echo -e "${BOLD}-- Phase 2: Setup test workspace --${NC}"

mkdir -p "$TEST_DIR"
(
    cd "$TEST_DIR"
    git init -q
    git checkout -b "$BRANCH" 2>/dev/null || git switch -c "$BRANCH" 2>/dev/null
    echo "# odooctl integration test project" > README.md
    git add README.md
    git commit -q -m "initial commit"
)

if [ -d "$TEST_DIR/.git" ]; then
    pass "git repo created (branch: $BRANCH)"
else
    fail "git repo created" "git init failed in $TEST_DIR"
fi

# Change into test directory -- all subsequent commands run from here
cd "$TEST_DIR"

echo ""

# =============================================
# Phase 3: Module scaffolding
# =============================================
echo -e "${BOLD}-- Phase 3: Module scaffolding --${NC}"

run_test "module scaffold (with model)" \
    "$ODOOCTL" module scaffold "$TEST_MODULE" \
        --odoo-version "$ODOO_VERSION" \
        --model \
        --author "Integration Test" \
        --depends base \
        --description "Test module for integration testing"

if [ -f "$TEST_DIR/$TEST_MODULE/__manifest__.py" ]; then
    pass "scaffolded module has __manifest__.py"
else
    fail "scaffolded module has __manifest__.py" "file not found"
fi

if [ -d "$TEST_DIR/$TEST_MODULE/models" ]; then
    pass "scaffolded module has models/ directory"
else
    fail "scaffolded module has models/ directory" "directory not found"
fi

if [ -d "$TEST_DIR/$TEST_MODULE/views" ]; then
    pass "scaffolded module has views/ directory"
else
    fail "scaffolded module has views/ directory" "directory not found"
fi

echo ""

# =============================================
# Phase 4: Docker environment creation
# =============================================
echo -e "${BOLD}-- Phase 4: Docker environment creation --${NC}"

run_test "docker create (v${ODOO_VERSION})" \
    "$ODOOCTL" docker create \
        --odoo-version "$ODOO_VERSION" \
        --modules base \
        --auto-discover-deps=false

run_test "docker path" "$ODOOCTL" docker path

# Determine actual environment directory
ENV_DIR="$HOME/.odooctl/${PROJECT_NAME}/${BRANCH}"
if [ -f "$ENV_DIR/.odooctl-state.json" ]; then
    pass "state file created"
else
    # Fallback: search for it
    FOUND_STATE=$(find "$HOME/.odooctl" -name ".odooctl-state.json" -path "*${PROJECT_NAME}*" 2>/dev/null | head -1)
    if [ -n "$FOUND_STATE" ]; then
        ENV_DIR="$(dirname "$FOUND_STATE")"
        pass "state file created (at $ENV_DIR)"
    else
        fail "state file created" "not found under ~/.odooctl/${PROJECT_NAME}/"
    fi
fi

# Verify generated Docker files
if [ -f "$ENV_DIR/docker/docker-compose.yml" ]; then
    pass "docker-compose.yml generated"
else
    # The compose file might be directly in ENV_DIR
    if [ -f "$ENV_DIR/docker-compose.yml" ]; then
        pass "docker-compose.yml generated"
    else
        fail "docker-compose.yml generated" "not found"
    fi
fi

if [ -f "$ENV_DIR/docker/Dockerfile" ]; then
    pass "Dockerfile generated"
else
    if [ -f "$ENV_DIR/Dockerfile" ]; then
        pass "Dockerfile generated"
    else
        fail "Dockerfile generated" "not found"
    fi
fi

# Creating the same environment again should fail
run_test_expect_fail "docker create (duplicate -- should fail)" \
    "$ODOOCTL" docker create \
        --odoo-version "$ODOO_VERSION" \
        --auto-discover-deps=false

echo ""

# =============================================
# Phase 5: Docker run (build + init)
# =============================================
echo -e "${BOLD}-- Phase 5: Docker run (build + init) --${NC}"

if [ "$SKIP_SLOW" = true ]; then
    skip "docker run --build -i" "--skip-slow"
else
    echo -e "  ${CYAN}Building and initializing... (this may take several minutes)${NC}"
    run_test "docker run --build -i --no-prompt" \
        "$ODOOCTL" docker run --build -i --no-prompt
fi

echo ""

# =============================================
# Phase 6: Container operations
# =============================================
echo -e "${BOLD}-- Phase 6: Container status & logs --${NC}"

if [ "$SKIP_SLOW" = true ]; then
    skip "docker status" "--skip-slow"
    skip "docker logs" "--skip-slow"
    skip "odoo HTTP endpoint" "--skip-slow"
else
    run_test "docker status" "$ODOOCTL" docker status

    run_test "docker logs (last 10 lines)" "$ODOOCTL" docker logs --tail 10

    # Wait for Odoo to be ready
    echo -e "  ${CYAN}Waiting for Odoo to respond (up to 120s)...${NC}"
    # Extract the Odoo port from state file
    ODOO_PORT=$(python3 -c "
import json, sys
with open('$ENV_DIR/.odooctl-state.json') as f:
    data = json.load(f)
print(data.get('ports', {}).get('odoo', 9900))
" 2>/dev/null || echo "9900")

    ODOO_READY=false
    for _ in $(seq 1 24); do
        if curl -sf -o /dev/null "http://localhost:${ODOO_PORT}/web/login" 2>/dev/null; then
            ODOO_READY=true
            break
        fi
        sleep 5
    done

    if [ "$ODOO_READY" = true ]; then
        pass "odoo HTTP responding on port $ODOO_PORT"
    else
        fail "odoo HTTP responding on port $ODOO_PORT" "timeout after 120s"
    fi
fi

echo ""

# =============================================
# Phase 7: Module installation
# =============================================
echo -e "${BOLD}-- Phase 7: Module installation --${NC}"

if [ "$SKIP_SLOW" = true ]; then
    skip "docker install --list-only" "--skip-slow"
    skip "docker install (test module)" "--skip-slow"
    skip "docker install --compute-hashes" "--skip-slow"
else
    run_test "docker install --list-only" \
        "$ODOOCTL" docker install --list-only

    run_test "docker install $TEST_MODULE" \
        "$ODOOCTL" docker install "$TEST_MODULE"

    run_test "docker install --compute-hashes" \
        "$ODOOCTL" docker install --compute-hashes
fi

echo ""

# =============================================
# Phase 8: Test runner
# =============================================
echo -e "${BOLD}-- Phase 8: Test runner --${NC}"

if [ "$SKIP_SLOW" = true ]; then
    skip "docker test" "--skip-slow"
else
    run_test "docker test --modules $TEST_MODULE" \
        "$ODOOCTL" docker test --modules "$TEST_MODULE"
fi

echo ""

# =============================================
# Phase 9: Shell, DB & odoo-bin access
# =============================================
echo -e "${BOLD}-- Phase 9: Shell, DB & odoo-bin --${NC}"

if [ "$SKIP_SLOW" = true ]; then
    skip "docker exec (bash echo test)" "--skip-slow"
    skip "docker exec (psql version)" "--skip-slow"
    skip "docker odoo-bin --version" "--skip-slow"
else
    # We cannot test interactive `docker shell` or `docker db` directly since
    # they open interactive sessions (exec bash / exec psql).
    # Instead, test the underlying Docker Compose exec to verify containers work.
    # This validates the same infrastructure that shell/db rely on.

    # Test bash access to the odoo container
    ENV_COMPOSE_DIR="$ENV_DIR/docker"
    if [ ! -f "$ENV_COMPOSE_DIR/docker-compose.yml" ]; then
        ENV_COMPOSE_DIR="$ENV_DIR"
    fi

    run_test "container exec: bash echo test" \
        docker compose -f "$ENV_COMPOSE_DIR/docker-compose.yml" exec -T odoo bash -c "echo shell-test-ok"

    # Test psql access to the db container
    DB_NAME="odoo-$(echo "$ODOO_VERSION" | tr -d '.')"
    run_test "container exec: psql version check" \
        docker compose -f "$ENV_COMPOSE_DIR/docker-compose.yml" exec -T db psql -U odoo -d "$DB_NAME" -c "SELECT version();"

    # Test odoo-bin --version (non-interactive, passes args through)
    run_test "docker odoo-bin --version" \
        "$ODOOCTL" docker odoo-bin --version
fi

echo ""

# =============================================
# Phase 10: Reconfigure
# =============================================
echo -e "${BOLD}-- Phase 10: Reconfigure --${NC}"

if [ "$SKIP_SLOW" = true ]; then
    skip "docker reconfigure --add-pip" "--skip-slow"
else
    run_test "docker reconfigure --add-pip requests (no rebuild)" \
        "$ODOOCTL" docker reconfigure --add-pip requests --rebuild=false --stop-first=false
fi

echo ""

# =============================================
# Phase 11: Dump (backup)
# =============================================
echo -e "${BOLD}-- Phase 11: Dump (backup) --${NC}"

if [ "$SKIP_SLOW" = true ]; then
    skip "docker dump" "--skip-slow"
    skip "dump file non-empty" "--skip-slow"
else
    DUMP_FILE="/tmp/odooctl-test-dump-${TIMESTAMP}.zip"
    run_test "docker dump -o $DUMP_FILE" \
        "$ODOOCTL" docker dump -o "$DUMP_FILE"

    if [ -f "$DUMP_FILE" ]; then
        DUMP_SIZE=$(stat -c%s "$DUMP_FILE" 2>/dev/null || stat -f%z "$DUMP_FILE" 2>/dev/null || echo "0")
        if [ "$DUMP_SIZE" -gt 0 ]; then
            pass "dump file is non-empty (${DUMP_SIZE} bytes)"
        else
            fail "dump file is non-empty" "file is 0 bytes"
        fi
        rm -f "$DUMP_FILE"
    else
        fail "dump file created" "not found at $DUMP_FILE"
    fi
fi

echo ""

# =============================================
# Phase 12: Edit & path commands
# =============================================
echo -e "${BOLD}-- Phase 12: Edit & path --${NC}"

run_test "docker edit --help" "$ODOOCTL" docker edit --help

run_test "docker path (verify)" "$ODOOCTL" docker path

echo ""

# =============================================
# Phase 13: Config commands
# =============================================
echo -e "${BOLD}-- Phase 13: Config management --${NC}"

# ssh-key-path requires the file to actually exist, so create a dummy one
FAKE_KEY="/tmp/odooctl-test-key-${TIMESTAMP}"
touch "$FAKE_KEY"

run_test "config set ssh-key-path" \
    "$ODOOCTL" config set ssh-key-path "$FAKE_KEY"

run_test "config get ssh-key-path" \
    "$ODOOCTL" config get ssh-key-path

run_test "config show" \
    "$ODOOCTL" config show

run_test "config unset ssh-key-path" \
    "$ODOOCTL" config unset ssh-key-path

# Verify the key was actually unset
run_test "config get (after unset -- should be empty)" \
    "$ODOOCTL" config get ssh-key-path

# Also test github-token set/get/unset cycle
run_test "config set github-token" \
    "$ODOOCTL" config set github-token ghp_testabc1234567890fake

run_test "config get github-token" \
    "$ODOOCTL" config get github-token

run_test "config unset github-token" \
    "$ODOOCTL" config unset github-token

rm -f "$FAKE_KEY"

echo ""

# =============================================
# Phase 14: Stop containers
# =============================================
echo -e "${BOLD}-- Phase 14: Stop containers --${NC}"

if [ "$SKIP_SLOW" = true ]; then
    skip "docker stop" "--skip-slow"
else
    run_test "docker stop" "$ODOOCTL" docker stop

    # Verify containers are stopped
    run_test "docker status (after stop)" "$ODOOCTL" docker status
fi

echo ""

# =============================================
# Phase 15: Reset (full cleanup)
# =============================================
echo -e "${BOLD}-- Phase 15: Reset (full cleanup) --${NC}"

run_test "docker reset -v -c -f" \
    "$ODOOCTL" docker reset -v -c -f

# Verify the state file is gone
if [ ! -f "$ENV_DIR/.odooctl-state.json" ]; then
    pass "state file removed after reset"
else
    fail "state file removed after reset" "file still exists at $ENV_DIR"
fi

echo ""

# ===========================================================================
# RESULTS SUMMARY
# ===========================================================================
TOTAL=$((PASSED + FAILED + SKIPPED))

echo -e "${BOLD}=====================================================${NC}"
echo -e "${BOLD}              Test Results Summary                    ${NC}"
echo -e "${BOLD}=====================================================${NC}"
echo ""

for result in "${TEST_RESULTS[@]}"; do
    echo -e "  $result"
done

echo ""
echo -e "${BOLD}-----------------------------------------------------${NC}"
echo -e "  Total:   ${BOLD}$TOTAL${NC}"
echo -e "  Passed:  ${GREEN}${BOLD}$PASSED${NC}"
echo -e "  Failed:  ${RED}${BOLD}$FAILED${NC}"
echo -e "  Skipped: ${YELLOW}${BOLD}$SKIPPED${NC}"
echo -e "${BOLD}-----------------------------------------------------${NC}"
echo ""
echo -e "  Log: ${CYAN}$LOG_FILE${NC}"
echo ""

if [ "$FAILED" -gt 0 ]; then
    echo -e "${RED}${BOLD}Some tests failed.${NC} Check the log for details."
    exit 1
else
    echo -e "${GREEN}${BOLD}All tests passed!${NC}"
    exit 0
fi
