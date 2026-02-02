/*
   Copyright Mycophonic.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package testutil

import (
	"testing"
	"time"

	"github.com/mycophonic/agar/pkg/agar"
)

// BenchFormat describes an audio format configuration for benchmarking.
type BenchFormat = agar.BenchFormat

// BenchOptions controls benchmark execution parameters.
type BenchOptions = agar.BenchOptions

// BenchResult holds timing statistics for a single benchmark run.
type BenchResult = agar.BenchResult

// Re-export benchmark constants.
const (
	DefaultBenchIterations = agar.DefaultBenchIterations
	DefaultBenchDuration   = agar.DefaultBenchDuration
)

// ComputeResult calculates timing statistics from a set of measured durations.
func ComputeResult(
	format BenchFormat, tool, operation string, durations []time.Duration, pcmSize int,
) BenchResult {
	return agar.ComputeResult(format, tool, operation, durations, pcmSize)
}

// PrintResults displays benchmark results in a formatted table.
func PrintResults(t *testing.T, opts BenchOptions, results []BenchResult) {
	t.Helper()

	agar.PrintResults(t, opts, results)
}
