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
	"os"

	"github.com/mycophonic/saprobe-alac"
)

// decodeSaprobe decodes an encoded file using the saprobe (pure Go) decoder.
func decodeSaprobe(path string) ([]byte, alac.PCMFormat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, alac.PCMFormat{}, err
	}
	defer f.Close()

	return alac.Decode(f)
}
