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
"os"
"testing"

"github.com/mycophonic/agar/pkg/agar"
"github.com/mycophonic/agar/pkg/coreaudio"
)

const coreAudioBinary = "alac-coreaudio"

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

// CoreAudioBinary returns a coreaudio.Codec backed by the alac-coreaudio binary.
// It fatals if the binary is not available.
func CoreAudioBinary(t *testing.T) coreaudio.Codec {
t.Helper()

path := CoreAudioPath(t)
if path == "" {
t.Fatal("alac-coreaudio binary not available")
}

return coreaudio.NewBinary(path)
}

// CoreAudioCGO returns a coreaudio.Codec backed by CGO AudioToolbox.
// It skips the test if CGO is not available.
func CoreAudioCGO(t *testing.T) coreaudio.Codec {
t.Helper()

codec := coreaudio.NewCGO()
if !codec.Available() {
t.Skip("CoreAudio CGO not available on this platform")
}

return codec
}

// CoreAudioEncode encodes PCM to ALAC M4A using the alac-coreaudio binary.
// It writes the result to the specified output path.
func CoreAudioEncode(t *testing.T, pcm []byte, format coreaudio.Format, outputPath string) {
t.Helper()

codec := CoreAudioBinary(t)

m4a, err := codec.Encode(pcm, format)
if err != nil {
t.Fatalf("coreaudio encode: %v", err)
}

if err := os.WriteFile(outputPath, m4a, 0o600); err != nil {
t.Fatalf("write encoded file: %v", err)
}
}
