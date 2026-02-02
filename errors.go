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

import "errors"

// Public sentinel errors for consumer error matching.
var (
	// ErrConfig indicates an invalid or unsupported ALAC configuration
	// (bad magic cookie, unsupported version, unsupported bit depth).
	ErrConfig = errors.New("invalid configuration")

	// ErrNoTrack indicates no usable ALAC track was found in the container
	// (missing track, missing or malformed MP4 boxes).
	ErrNoTrack = errors.New("no track found")

	// ErrDecode indicates a failure during packet decoding
	// (bitstream overrun, invalid headers, unsupported elements).
	ErrDecode = errors.New("decode failed")
)
