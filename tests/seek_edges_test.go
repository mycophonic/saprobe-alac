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

package tests_test

import (
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mycophonic/agar/pkg/agar"

	alac "github.com/mycophonic/saprobe-alac"
)

// Seek at the edges of time: before the start lands at the start, at or
// past the end lands at the end and the next Read is EOF, the largest
// Duration there is lands at the end too, and a seek back after the end
// reads the stream again. Over a compressed stream the reference encoder
// wrote.
func TestSeekAtTheEdgesOfTime(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "source.raw")
	source := tonalPCM(8000, 16, 2)

	if err := os.WriteFile(src, source, 0o600); err != nil {
		t.Fatal(err)
	}

	enc := filepath.Join(dir, "encoded.m4a")
	agar.FFmpegEncode(t, agar.FFmpegEncodeOptions{
		Src: src, Dst: enc, BitDepth: 16, SampleRate: 8000, Channels: 2,
		CodecArgs: []string{"-c:a", "alac", "-sample_fmt", "s16p"},
	})

	data, err := os.ReadFile(enc)
	if err != nil {
		t.Fatal(err)
	}

	dec, err := alac.NewDecoder(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	whole, err := io.ReadAll(dec)
	if err != nil || !bytes.Equal(whole, source) {
		t.Fatalf("the stream is not its source: %d bytes, %v", len(whole), err)
	}

	duration := dec.Duration()

	// In sequence on one decoder, since each case starts where the last one
	// left the stream.
	for _, c := range []struct {
		name   string
		target time.Duration
		// atEnd: the next Read must be EOF; otherwise the landing is before
		// the end and reads on.
		atEnd bool
	}{
		{"before the start", -time.Second, false},
		{"the start", 0, false},
		{"the end", duration, true},
		{"past the end", duration + time.Hour, true},
		{"the largest duration", time.Duration(math.MaxInt64), true},
		{"one sample before the end", duration - time.Second/8000, false},
	} {
		landed, seekErr := dec.Seek(c.target)
		if seekErr != nil {
			t.Fatalf("%s: Seek(%v): %v", c.name, c.target, seekErr)
		}

		if landed < 0 || landed > duration {
			t.Fatalf("%s: Seek(%v) landed at %v, outside 0..%v", c.name, c.target, landed, duration)
		}

		if c.target <= 0 && landed != 0 {
			t.Fatalf("%s: Seek(%v) landed at %v, want the start", c.name, c.target, landed)
		}

		if c.atEnd && landed != duration {
			t.Fatalf("%s: Seek(%v) landed at %v, want the end %v", c.name, c.target, landed, duration)
		}

		n, readErr := dec.Read(make([]byte, 16))

		switch {
		case c.atEnd && (n != 0 || !errors.Is(readErr, io.EOF)):
			t.Fatalf("%s: after Seek(%v) to the end, Read gave %d bytes and %v", c.name, c.target, n, readErr)
		case !c.atEnd && (n == 0 || readErr != nil):
			t.Fatalf("%s: after Seek(%v) to %v, Read gave %d bytes and %v", c.name, c.target, landed, n, readErr)
		}
	}

	// Back to the start after the end: the whole stream again.
	if _, seekErr := dec.Seek(0); seekErr != nil {
		t.Fatal(seekErr)
	}

	again, readErr := io.ReadAll(dec)
	if readErr != nil || !bytes.Equal(again, source) {
		t.Fatalf("after a seek back from the end: %d bytes, %v", len(again), readErr)
	}
}
