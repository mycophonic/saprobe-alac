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

package alac

import "fmt"

// BytesPerSample returns the number of bytes needed to store one sample at
// the given bit depth. Only ALAC-supported depths (16, 20, 24, 32) are valid.
func BytesPerSample(depth uint8) int {
	switch depth {
	case 16:
		return 2
	case 20, 24:
		return 3
	case 32:
		return 4
	default:
		panic(fmt.Sprintf("alac: BytesPerSample called with unsupported bit depth %d", depth))
	}
}
