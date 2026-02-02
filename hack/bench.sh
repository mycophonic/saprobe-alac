#!/usr/bin/env bash

#   Copyright Mycophonic.

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.

set -euo pipefail

cd "$(dirname "$0")/.."

TEST_NAME="${1:?Usage: hack/bench.sh <TestName> [file] [ENV_VAR]}"
BENCH_FILE="${2:-}"
ENV_VAR="${3:-BENCH_FILE}"

PROF_DIR=$(mktemp -d)
trap 'rm -rf "$PROF_DIR"' EXIT

CPU_PROF="$PROF_DIR/cpu.prof"
MEM_PROF="$PROF_DIR/mem.prof"

if [[ -n "$BENCH_FILE" ]]; then
  echo "Running $TEST_NAME on: $BENCH_FILE ..."
  export "$ENV_VAR=$BENCH_FILE"
else
  echo "Running $TEST_NAME (synthetic) ..."
fi

echo ""

go test ./tests/ \
  -run "$TEST_NAME" \
  -count=1 \
  -v \
  -cpuprofile "$CPU_PROF" \
  -memprofile "$MEM_PROF" \
  2>&1

echo ""
echo "================================================================================"
echo "CPU Profile (top 20)"
echo "================================================================================"
go tool pprof -top -nodecount=20 "$CPU_PROF"

echo ""
echo "================================================================================"
echo "Memory Profile — alloc_space (top 20)"
echo "================================================================================"
go tool pprof -top -nodecount=20 -alloc_space "$MEM_PROF"

echo ""
echo "================================================================================"
echo "Memory Profile — inuse_space (top 20)"
echo "================================================================================"
go tool pprof -top -nodecount=20 -inuse_space "$MEM_PROF"

DOCS_DIR="docs"

echo ""
echo "================================================================================"
echo "Generating call graph diagrams in $DOCS_DIR/"
echo "================================================================================"
go tool pprof -png -nodecount=20 "$CPU_PROF" > "$DOCS_DIR/decode_cpu.png" 2>/dev/null \
  && echo "  $DOCS_DIR/decode_cpu.png" \
  || echo "  (skipped CPU PNG: graphviz not installed)"
go tool pprof -png -nodecount=20 -alloc_space "$MEM_PROF" > "$DOCS_DIR/decode_alloc.png" 2>/dev/null \
  && echo "  $DOCS_DIR/decode_alloc.png" \
  || echo "  (skipped alloc PNG: graphviz not installed)"
