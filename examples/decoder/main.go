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

// decoder decodes an ALAC M4A file to WAV or raw PCM on stdout.
//
// Usage:
//
//	decoder [-format wav|pcm] <input.m4a | ->
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"

	alac "github.com/mycophonic/saprobe-alac"
)

const formatWAV = "wav"

// version is the build's stamp: the build recipe sets it from `git describe`
// through -X main.version; a plain `go build` leaves "dev". gochecknoglobals
// exempts the name for this use.
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	format := flag.String("format", formatWAV, "output format: wav or pcm")

	flag.Usage = func() { //nolint:reassign // flag.Usage is the package's documented hook for a custom usage message
		fmt.Fprintf(os.Stderr, "Usage: %s [-format wav|pcm] <input.m4a | ->\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showVersion {
		fmt.Fprintln(os.Stdout, "decoder", version)
		os.Exit(0)
	}

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	if *format != formatWAV && *format != "pcm" {
		fmt.Fprintf(os.Stderr, "unknown format %q (use wav or pcm)\n", *format)
		os.Exit(1)
	}

	os.Exit(run(*format, flag.Arg(0)))
}

func run(format, inputPath string) int {
	reader, cleanup, err := openInput(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)

		return 1
	}

	defer cleanup()

	dec, decErr := alac.NewDecoder(reader)
	if decErr != nil {
		fmt.Fprintf(os.Stderr, "decode: %v\n", decErr)

		return 1
	}

	pcmFormat := dec.Format()

	fmt.Fprintf(os.Stderr, "%d Hz, %d-bit, %d ch\n",
		pcmFormat.SampleRate, pcmFormat.BitDepth, pcmFormat.Channels)

	if format == formatWAV {
		// WAV header requires total data size upfront.
		pcm, err := io.ReadAll(dec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "decode: %v\n", err)

			return 1
		}

		if err := writeWAV(os.Stdout, pcm, pcmFormat); err != nil {
			fmt.Fprintf(os.Stderr, "write: %v\n", err)

			return 1
		}
	} else {
		if _, err := io.Copy(os.Stdout, dec); err != nil {
			fmt.Fprintf(os.Stderr, "write: %v\n", err)

			return 1
		}
	}

	return 0
}

// openInput returns a ReadSeeker for the given path, or buffers stdin when path is "-".
func openInput(path string) (io.ReadSeeker, func(), error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, func() {}, fmt.Errorf("reading stdin: %w", err)
		}

		return bytes.NewReader(data), func() {}, nil
	}

	f, err := os.Open(path) // #nosec G304 -- the path is the command line's argument
	if err != nil {
		return nil, func() {}, fmt.Errorf("opening %s: %w", path, err)
	}

	return f, func() { _ = f.Close() }, nil
}

// The canonical PCM WAV header: the RIFF chunk header (8), the "WAVE" form
// tag (4), the fmt chunk (24) and the data chunk header (8). The RIFF size
// field counts the bytes after itself: the file minus the RIFF chunk header.
const (
	wavHeaderSize       = 44
	riffChunkHeaderSize = 8
)

// errWAVLimit reports PCM that the 32-bit size fields of a WAV header cannot describe.
var errWAVLimit = errors.New("PCM exceeds the WAV format's 4 GiB limit")

// writeWAV writes a standard PCM WAV to writer.
func writeWAV(writer io.Writer, pcm []byte, format alac.PCMFormat) error {
	bytesPerSample := format.BitDepth / 8
	blockAlign := format.Channels * bytesPerSample
	byteRate := format.SampleRate * blockAlign
	dataSize := len(pcm)

	// The RIFF and data sizes and the byte rate are 32-bit fields.
	if int64(dataSize) > math.MaxUint32-(wavHeaderSize-riffChunkHeaderSize) || int64(byteRate) > math.MaxUint32 {
		return fmt.Errorf("%w: %d bytes at %d bytes/s", errWAVLimit, dataSize, byteRate)
	}

	var hdr [wavHeaderSize]byte

	copy(hdr[0:4], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(wavHeaderSize-riffChunkHeaderSize+dataSize))
	copy(hdr[8:12], "WAVE")

	// The fmt chunk: the format is the cookie's (NumChannels 8-bit, SampleRate 32-bit, BitDepth 16 to 32).
	copy(hdr[12:16], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:20], 16)
	binary.LittleEndian.PutUint16(hdr[20:22], 1)                         // PCM
	binary.LittleEndian.PutUint16(hdr[22:24], uint16(format.Channels))   // #nosec G115 -- an 8-bit cookie field
	binary.LittleEndian.PutUint32(hdr[24:28], uint32(format.SampleRate)) // #nosec G115 -- a 32-bit cookie field
	// #nosec G115 -- checked above, and unsigned at source
	binary.LittleEndian.PutUint32(hdr[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(hdr[32:34], uint16(blockAlign))      // #nosec G115 -- 8-bit channels × 4 bytes
	binary.LittleEndian.PutUint16(hdr[34:36], uint16(format.BitDepth)) // #nosec G115 -- 16 to 32 (NewPacketDecoder)

	copy(hdr[36:40], "data")
	binary.LittleEndian.PutUint32(hdr[40:44], uint32(dataSize))

	if _, err := writer.Write(hdr[:]); err != nil {
		return fmt.Errorf("writing WAV header: %w", err)
	}

	if _, err := writer.Write(pcm); err != nil {
		return fmt.Errorf("writing WAV data: %w", err)
	}

	return nil
}
