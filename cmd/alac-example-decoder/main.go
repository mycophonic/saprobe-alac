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

// alac-example-decoder decodes an ALAC M4A file to WAV or raw PCM on stdout.
//
// Usage:
//
//	alac-example-decoder [-format wav|pcm] <input.m4a | ->
//
//nolint:gosec // Integer conversions are bounded by audio format constraints; file paths from CLI args.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/mycophonic/saprobe-alac"
	"github.com/mycophonic/saprobe-alac/version"
)

const formatWAV = "wav"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	format := flag.String("format", formatWAV, "output format: wav or pcm")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [-format wav|pcm] <input.m4a | ->\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showVersion {
		fmt.Fprintln(os.Stdout, version.String())
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

	pcm, pcmFormat, err := alac.Decode(reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decode: %v\n", err)

		return 1
	}

	fmt.Fprintf(os.Stderr, "%d Hz, %d-bit, %d ch, %d bytes PCM\n",
		pcmFormat.SampleRate, pcmFormat.BitDepth, pcmFormat.Channels, len(pcm))

	if format == formatWAV {
		err = writeWAV(os.Stdout, pcm, pcmFormat)
	} else {
		_, err = os.Stdout.Write(pcm)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)

		return 1
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

	f, err := os.Open(path)
	if err != nil {
		return nil, func() {}, fmt.Errorf("opening %s: %w", path, err)
	}

	return f, func() { _ = f.Close() }, nil
}

// writeWAV writes a standard PCM WAV to writer.
func writeWAV(writer io.Writer, pcm []byte, format alac.PCMFormat) error {
	bytesPerSample := format.BitDepth / 8
	blockAlign := format.Channels * bytesPerSample
	byteRate := format.SampleRate * blockAlign
	dataSize := len(pcm)

	var hdr [44]byte

	copy(hdr[0:4], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(36+dataSize))
	copy(hdr[8:12], "WAVE")

	copy(hdr[12:16], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:20], 16)
	binary.LittleEndian.PutUint16(hdr[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(hdr[22:24], uint16(format.Channels))
	binary.LittleEndian.PutUint32(hdr[24:28], uint32(format.SampleRate))
	binary.LittleEndian.PutUint32(hdr[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(hdr[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(hdr[34:36], uint16(format.BitDepth))

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
