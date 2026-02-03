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
	"bytes"
	"context"
	"io"
	"os/exec"
	"testing"

	"github.com/mycophonic/agar/pkg/agar"
)

const coreAudioBinary = "alac-coreaudio"

// CoreAudioOptions configures an alac-coreaudio invocation.
type CoreAudioOptions struct {
	// Args are passed directly to the alac-coreaudio binary.
	Args []string
	// Stdin is connected to the command's standard input when non-nil.
	Stdin io.Reader
	// Stdout receives the command's standard output when non-nil.
	// When nil, stdout is captured and returned in CoreAudioResult.Stdout.
	Stdout io.Writer
	// Stderr receives the command's standard error when non-nil.
	// When nil, stderr is captured and returned in CoreAudioResult.Stderr.
	Stderr io.Writer
}

// CoreAudioResult holds captured output from an alac-coreaudio invocation.
type CoreAudioResult struct {
	// Stdout contains captured standard output, populated only when
	// CoreAudioOptions.Stdout was nil.
	Stdout []byte
	// Stderr contains captured standard error, populated only when
	// CoreAudioOptions.Stderr was nil.
	Stderr []byte
}

// CoreAudioPath returns the path to the alac-coreaudio binary, or an empty
// string if it is not available. Build it with: make alac-coreaudio.
func CoreAudioPath(t *testing.T) string {
	t.Helper()

	path, err := agar.LookFor(coreAudioBinary)
	if err != nil {
		t.Log("alac-coreaudio not found: run 'make alac-coreaudio' to enable CoreAudio tests")

		return ""
	}

	return path
}

// CoreAudio runs alac-coreaudio with the given options.
// It fatals the test if alac-coreaudio cannot be found or the command returns an error.
func CoreAudio(t *testing.T, opts CoreAudioOptions) CoreAudioResult {
	t.Helper()

	bin, err := agar.LookFor(coreAudioBinary)
	if err != nil {
		t.Log(coreAudioBinary + ": " + err.Error())
		t.FailNow()
	}

	//nolint:gosec // arguments are test-controlled
	cmd := exec.CommandContext(context.Background(), bin, opts.Args...)

	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	}

	var stdoutBuf bytes.Buffer

	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	} else {
		cmd.Stdout = &stdoutBuf
	}

	var stderrBuf bytes.Buffer

	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	} else {
		cmd.Stderr = &stderrBuf
	}

	if err := cmd.Run(); err != nil {
		t.Fatalf("alac-coreaudio: %v\n%s", err, stderrBuf.String())
	}

	return CoreAudioResult{
		Stdout: stdoutBuf.Bytes(),
		Stderr: stderrBuf.Bytes(),
	}
}
