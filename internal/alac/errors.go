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

// ALAC decoder error sentinels.
//
//revive:disable:exported
var (
	ErrInvalidCookie      = errors.New("alac: invalid magic cookie")
	ErrUnsupportedVersion = errors.New("alac: unsupported compatible version")
	ErrUnsupportedElement = errors.New("alac: unsupported element type (CCE/PCE)")
	ErrInvalidHeader      = errors.New("alac: invalid frame header")
	ErrInvalidShift       = errors.New("alac: invalid bytesShifted value")
	ErrBitstreamOverrun   = errors.New("alac: bitstream overrun")
	ErrSampleOverrun      = errors.New("alac: sample count exceeds buffer")
	ErrBitDepth           = errors.New("alac: unsupported bit depth")
)
