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
	"io"
	"os"

	alac "github.com/mycophonic/saprobe-alac"
)

// decode decodes ALAC from an io.ReadSeeker using NewDecoder + io.ReadAll.
// The error tests only look at the PCM and the error; decodeSaprobe also
// returns the format.
func decode(reader io.ReadSeeker) ([]byte, error) {
	dec, err := alac.NewDecoder(reader)
	if err != nil {
		return nil, err
	}

	pcm, readErr := io.ReadAll(dec)
	if readErr != nil {
		return nil, readErr
	}

	return pcm, nil
}

// decodeSaprobe decodes an encoded file using the saprobe (pure Go) decoder.
func decodeSaprobe(path string) ([]byte, alac.PCMFormat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, alac.PCMFormat{}, err
	}
	defer func() { _ = f.Close() }() // opened read-only: Close has nothing to flush and nothing to report

	dec, decErr := alac.NewDecoder(f)
	if decErr != nil {
		return nil, alac.PCMFormat{}, decErr
	}

	pcm, readErr := io.ReadAll(dec)
	if readErr != nil {
		return nil, alac.PCMFormat{}, readErr
	}

	return pcm, dec.Format(), nil
}
