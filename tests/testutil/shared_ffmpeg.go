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
	"io"
	"os"
	"testing"
	"time"

	"github.com/mycophonic/agar/pkg/agar"
)

// BenchDecodeFFmpeg benchmarks ffmpeg decoding of an encoded file.
func BenchDecodeFFmpeg(t *testing.T, format agar.BenchFormat, opts agar.BenchOptions, srcPath string) agar.BenchResult {
	t.Helper()

	opts = opts.WithDefaults()

	durations := make([]time.Duration, opts.Iterations)

	for iter := range opts.Iterations {
		start := time.Now()

		agar.FFmpegDecode(t, agar.FFmpegDecodeOptions{
			Src:      srcPath,
			BitDepth: format.BitDepth,
			Channels: format.Channels,
			Stdout:   io.Discard,
		})

		durations[iter] = time.Since(start)
	}

	info, err := os.Stat(srcPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	return agar.ComputeResult(format, "ffmpeg", "decode", durations, int(info.Size()))
}
