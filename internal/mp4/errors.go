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

package mp4

import "errors"

// MP4 container parsing error sentinels.
//
//revive:disable:exported
var (
	ErrNoALACTrack    = errors.New("mp4: no ALAC track found in container")
	ErrInvalidEntry   = errors.New("mp4: invalid ALAC sample entry")
	ErrInvalidBoxSize = errors.New("mp4: invalid box size")
	ErrNoChunkOffset  = errors.New("mp4: no chunk offset box (stco/co64)")
	ErrInvalidCo64    = errors.New("mp4: invalid co64 payload")
	ErrNoStsc         = errors.New("mp4: no stsc box")
	ErrInvalidStsc    = errors.New("mp4: invalid stsc payload")
	ErrNoStsz         = errors.New("mp4: no stsz box")
	ErrInvalidStsz    = errors.New("mp4: invalid stsz payload")
)
