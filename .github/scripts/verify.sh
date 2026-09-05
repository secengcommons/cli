#!/usr/bin/env bash
set -euo pipefail

repository_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
readonly repository_root
readonly max_fuzz_targets=1024
export GOWORK=off

step() {
  local name=$1
  shift
  printf '\n==> %s\n' "$name"
  "$@"
}

tool_path() (
  cd -- "$repository_root/tools"
  go tool -n "$1"
)

require_toolchain() {
  local actual required
  required=$(awk '$1 == "toolchain" { sub(/^go/, "", $2); print $2; exit }' "$repository_root/go.mod")
  actual=$(go env GOVERSION)
  actual=${actual#go}
  if [[ -z "$required" || "$actual" != "$required" ]]; then
    printf 'Go toolchain mismatch: required %s, found %s\n' "${required:-unknown}" "${actual:-unknown}" >&2
    return 1
  fi
}

require_empty_module_graph() {
  local modules
  modules=$(cd -- "$repository_root" && go list -m all)
  if [[ "$modules" != 'github.com/secengcommons/cli' ]]; then
    printf 'Production module graph contains external modules:\n%s\n' "$modules" >&2
    return 1
  fi
}

require_clean_go_fix() {
  local difference
  difference=$(cd -- "$repository_root" && go fix -diff ./...)
  if [[ -n "$difference" ]]; then
    printf '%s\n' "$difference"
    printf 'Go source requires modernisation\n' >&2
    return 1
  fi
}

require_complete_coverage() {
  local profile=$1
  awk '
    NR == 1 { if ($0 != "mode: atomic") exit 2; next }
    NF != 3 { exit 2 }
    {
      key = $1
      if (key in statements && statements[key] != $2) exit 2
      statements[key] = $2
      counts[key] += $3
    }
    END {
      for (key in statements) {
        total += statements[key]
        if (counts[key] == 0) missed += statements[key]
      }
      if (NR < 2 || total == 0) exit 2
      if (missed != 0) exit 1
    }
  ' "$profile"
}

coverage_self_test() (
  local covered uncovered
  covered=$(mktemp "${TMPDIR:-/tmp}/secengcommons-cli-covered.XXXXXX")
  uncovered=$(mktemp "${TMPDIR:-/tmp}/secengcommons-cli-uncovered.XXXXXX")
  trap 'rm -f -- "$covered" "$uncovered"' EXIT
  printf 'mode: atomic\nprobe.go:1.1,1.2 1 1\n' >"$covered"
  printf 'mode: atomic\nprobe.go:1.1,1.2 1 0\n' >"$uncovered"
  require_complete_coverage "$covered"
  if require_complete_coverage "$uncovered"; then
    printf 'Coverage check accepted an uncovered statement\n' >&2
    return 1
  fi
)

modernisation_self_test() (
  local fixture linter output
  fixture=$(mktemp -d "${TMPDIR:-/tmp}/secengcommons-cli-modernisation.XXXXXX")
  case "$fixture" in
    "${TMPDIR:-/tmp}"/secengcommons-cli-modernisation.*) ;;
    *) printf 'Unsafe modernisation directory: %s\n' "$fixture" >&2; return 1 ;;
  esac
  trap 'rm -rf -- "$fixture"' EXIT
  cat >"$fixture/go.mod" <<'EOF'
module modernisation.test/probe

go 1.26.0
EOF
  cat >"$fixture/probe.go" <<'EOF'
package probe

func boundedIndex(index int, values []int) int {
	maximum := index
	if len(values) < index {
		maximum = len(values)
	}
	return maximum
}
EOF
  output=$fixture/result.out
  if (cd -- "$fixture" && go fix -diff ./...) >"$output" 2>&1; then
    printf 'Go fix accepted legacy source\n' >&2
    return 1
  fi
  grep -Fq 'min(len(values), index)' "$output"
  linter=$(tool_path golangci-lint)
  if (cd -- "$fixture" && "$linter" run --config "$repository_root/.golangci.yml" ./...) >"$output" 2>&1; then
	printf 'Linter accepted legacy source\n' >&2
    return 1
  fi
  grep -Fq '(modernize)' "$output"
)

tool_packages() {
  go -C "$repository_root/tools" mod edit -json | awk '
    $0 == "\t\"Tool\": [" { found = 1; tools = 1; next }
    tools && $0 == "\t]," { complete = 1; tools = 0; next }
    tools && match($0, /^\t\t\t"Path": "[A-Za-z0-9._~\/-]+"$/) {
      path = substr($0, 13, length($0) - 13)
      if (length(path) > 256 || path <= previous) exit 2
      print path
      previous = path
      count++
    }
    END { if (!found || !complete || count < 1 || count > 64) exit 2 }
  '
}

scan_tool_vulnerabilities() (
  local inventory package scanner=$1
  local -a packages
  packages=()
  inventory=$(mktemp "${TMPDIR:-/tmp}/secengcommons-cli-tools.XXXXXX")
  trap 'rm -f -- "$inventory"' EXIT
  tool_packages >"$inventory"
  while IFS= read -r package; do
    [[ -n "$package" ]] || return 1
    packages[${#packages[@]}]=$package
  done <"$inventory"
  ((${#packages[@]} > 0))
  cd -- "$repository_root/tools"
  "$scanner" "${packages[@]}"
)

tool_scan_self_test() (
  local expected fixture receipt temporary
  temporary=$(mktemp -d "${TMPDIR:-/tmp}/secengcommons-cli-tool-scan.XXXXXX")
  case "$temporary" in
    "${TMPDIR:-/tmp}"/secengcommons-cli-tool-scan.*) ;;
    *) printf 'Unsafe tool scan directory: %s\n' "$temporary" >&2; return 1 ;;
  esac
  trap 'rm -rf -- "$temporary"' EXIT
  expected=$temporary/expected
  fixture=$temporary/scanner
  receipt=$temporary/receipt
  tool_packages >"$expected"
  cat >"$fixture" <<'EOF'
#!/bin/sh
printf '%s\n' "$@" >"${TOOL_SCAN_RECEIPT:?}"
EOF
  chmod 0700 "$fixture"
  TOOL_SCAN_RECEIPT="$receipt" scan_tool_vulnerabilities "$fixture"
  cmp -- "$expected" "$receipt"
)

run_static() {
  local golangci govulncheck shellcheck
  cd -- "$repository_root"
  require_toolchain
  golangci=$(tool_path golangci-lint)
  govulncheck=$(tool_path govulncheck)
  shellcheck=$(tool_path shellcheck)
  [[ -n "$golangci" && -n "$govulncheck" && -n "$shellcheck" ]]
  step 'Module Tidy' go mod tidy -diff
  step 'Module Verification' go mod verify
  step 'Tool Module Tidy' go -C tools mod tidy -diff
  step 'Tool Module Verification' go -C tools mod verify
  step 'Production Module Graph' require_empty_module_graph
  step 'Go Fix' require_clean_go_fix
  step 'Modernisation Self-Test' modernisation_self_test
  step 'Linter Configuration' "$golangci" config verify
  step 'Formatting' "$golangci" fmt --diff
  step 'Go Vet' go vet ./...
  step 'Go Lint' "$golangci" run
  step 'Production Vulnerabilities' "$govulncheck" ./...
  step 'Tool Scan Self-Test' tool_scan_self_test
  step 'Tool Vulnerabilities' scan_tool_vulnerabilities "$govulncheck"
  step 'Shell Syntax' bash -n .github/scripts/verify.sh
  step 'Shell Analysis' "$shellcheck" .github/scripts/verify.sh
  step 'Build' go build -trimpath ./...
}

run_tests() (
  local profile
  cd -- "$repository_root"
  profile=$repository_root/coverage.out
  rm -f -- "$profile"
  trap 'rm -f -- "$profile"' EXIT
  step 'Coverage Self-Test' coverage_self_test
  step 'Tests and Coverage' go test -count=1 -shuffle=on -covermode=atomic -coverprofile="$profile" ./...
  step 'Coverage Report' go tool cover -func="$profile"
  step 'Complete Coverage' require_complete_coverage "$profile"
  step 'Race' go test -race -count=1 -shuffle=on ./...
)

discover_fuzz_targets() {
  local count=0 inventory=$1 package package_output target target_output
  : >"$inventory"
  package_output=$(cd -- "$repository_root" && go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...)
  while IFS= read -r package; do
    [[ -n "$package" ]] || continue
    target_output=$(cd -- "$repository_root" && go test -list '^Fuzz[A-Za-z0-9_]+$' "$package")
    while IFS= read -r target; do
      [[ "$target" =~ ^Fuzz[A-Za-z0-9_]+$ ]] || continue
      if ((count >= max_fuzz_targets)); then
        printf 'Fuzz target inventory exceeds %d entries\n' "$max_fuzz_targets" >&2
        return 1
      fi
      printf '%s\t%s\n' "$package" "$target" >>"$inventory"
      count=$((count + 1))
    done <<<"$target_output"
  done <<<"$package_output"
  if ((count == 0)); then
    printf 'No fuzz targets were discovered\n' >&2
    return 1
  fi
}

run_fuzz() (
  local completed=0 extra inventory package target
  cd -- "$repository_root"
  inventory=$(mktemp "${TMPDIR:-/tmp}/secengcommons-cli-fuzz.XXXXXX")
  trap 'rm -f -- "$inventory"' EXIT
  discover_fuzz_targets "$inventory"
  while IFS=$'\t' read -r package target extra; do
    [[ -n "$package" && -n "$target" && -z "$extra" ]] || return 1
    step "Fuzz $package/$target" go test -run '^$' \
      -fuzz "^$target$" -fuzztime="${FUZZTIME:-100000x}" "$package"
    completed=$((completed + 1))
  done <"$inventory"
  ((completed > 0))
)

run_benchmarks() {
  cd -- "$repository_root"
  go test -run '^$' -bench . -benchmem -benchtime="${BENCHTIME:-500ms}" -count="${BENCHSAMPLES:-5}" ./...
}

case "${1:-}" in
  all) run_static; run_tests; run_fuzz ;;
  static) run_static ;;
  test) run_tests ;;
  fuzz) run_fuzz ;;
  benchmark) run_benchmarks ;;
  self-test) coverage_self_test; modernisation_self_test; tool_scan_self_test ;;
  *) printf 'Usage: %s all|static|test|fuzz|benchmark|self-test\n' "${0##*/}" >&2; exit 2 ;;
esac
