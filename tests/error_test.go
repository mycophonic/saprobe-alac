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
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mycophonic/agar/pkg/agar"

	"github.com/mycophonic/saprobe-alac"
)

// encodeTestM4A generates a short M4A file for corruption tests.
func encodeTestM4A(t *testing.T) []byte {
	t.Helper()

	srcPCM := agar.GenerateWhiteNoise(44100, 16, 2, 1)
	tmpDir := t.TempDir()

	srcPath := filepath.Join(tmpDir, "source.raw")
	if err := os.WriteFile(srcPath, srcPCM, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	encPath := filepath.Join(tmpDir, "encoded.m4a")

	agar.FFmpegEncode(t, agar.FFmpegEncodeOptions{
		Src:        srcPath,
		Dst:        encPath,
		BitDepth:   16,
		SampleRate: 44100,
		Channels:   2,
		CodecArgs:  []string{"-c:a", "alac", "-sample_fmt", "s16p"},
		InputArgs:  []string{"-channel_layout", "stereo"},
	})

	data, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatalf("read encoded: %v", err)
	}

	return data
}

// findFourCC returns the offset of the first occurrence of a 4-byte FourCC
// in data, searching the type field (bytes 4-7 of each box header).
// Returns -1 if not found.
func findFourCC(data []byte, fourcc string) int {
	tag := []byte(fourcc)

	for i := 0; i+7 < len(data); i++ {
		if data[i+4] == tag[0] && data[i+5] == tag[1] && data[i+6] == tag[2] && data[i+7] == tag[3] {
			return i
		}
	}

	return -1
}

// --- ParseMagicCookie error tests ---

func TestParseMagicCookie_TooShort(t *testing.T) {
	t.Parallel()

	_, err := alac.ParseMagicCookie([]byte{0, 0, 0, 0})
	if err == nil {
		t.Fatal("expected error for short cookie")
	}

	if !errors.Is(err, alac.ErrConfig) {
		t.Fatalf("expected ErrConfig, got: %v", err)
	}
}

func TestParseMagicCookie_EmptyCookie(t *testing.T) {
	t.Parallel()

	_, err := alac.ParseMagicCookie(nil)
	if err == nil {
		t.Fatal("expected error for nil cookie")
	}

	if !errors.Is(err, alac.ErrConfig) {
		t.Fatalf("expected ErrConfig, got: %v", err)
	}
}

func TestParseMagicCookie_BadVersion(t *testing.T) {
	t.Parallel()

	// 24-byte cookie with compatible version = 99 at byte 4.
	cookie := make([]byte, 24)
	cookie[4] = 99

	_, err := alac.ParseMagicCookie(cookie)
	if err == nil {
		t.Fatal("expected error for bad version")
	}

	if !errors.Is(err, alac.ErrConfig) {
		t.Fatalf("expected ErrConfig, got: %v", err)
	}
}

// --- NewPacketDecoder error tests ---

func TestNewPacketDecoder_InvalidBitDepth(t *testing.T) {
	t.Parallel()

	_, err := alac.NewPacketDecoder(alac.PacketConfig{
		FrameLength: 4096,
		BitDepth:    13,
		NumChannels: 2,
		SampleRate:  44100,
	})
	if err == nil {
		t.Fatal("expected error for invalid bit depth")
	}

	if !errors.Is(err, alac.ErrConfig) {
		t.Fatalf("expected ErrConfig, got: %v", err)
	}
}

// --- NewDecoder / Decode error tests on corrupt M4A ---

func TestDecode_EmptyReader(t *testing.T) {
	t.Parallel()

	_, _, err := decode(bytes.NewReader(nil))
	if err == nil {
		t.Fatal("expected error for empty reader")
	}

	if !errors.Is(err, alac.ErrNoTrack) {
		t.Fatalf("expected ErrNoTrack, got: %v", err)
	}
}

func TestDecode_GarbageData(t *testing.T) {
	t.Parallel()

	garbage := bytes.Repeat([]byte{0xDE, 0xAD}, 1024)

	_, _, err := decode(bytes.NewReader(garbage))
	if err == nil {
		t.Fatal("expected error for garbage data")
	}

	if !errors.Is(err, alac.ErrNoTrack) {
		t.Fatalf("expected ErrNoTrack, got: %v", err)
	}
}

func TestDecode_TruncatedBeforeMoov(t *testing.T) {
	t.Parallel()

	data := encodeTestM4A(t)

	moovOff := findFourCC(data, "moov")
	if moovOff < 0 {
		t.Fatal("moov not found in test M4A")
	}

	// Truncate before moov box.
	truncated := data[:moovOff]

	_, _, err := decode(bytes.NewReader(truncated))
	if err == nil {
		t.Fatal("expected error for truncated file (before moov)")
	}

	if !errors.Is(err, alac.ErrNoTrack) {
		t.Fatalf("expected ErrNoTrack, got: %v", err)
	}
}

func TestDecode_TruncatedMoov(t *testing.T) {
	t.Parallel()

	data := encodeTestM4A(t)

	moovOff := findFourCC(data, "moov")
	if moovOff < 0 {
		t.Fatal("moov not found in test M4A")
	}

	// Read moov box size and truncate midway through it.
	moovSize := binary.BigEndian.Uint32(data[moovOff : moovOff+4])
	cutPoint := moovOff + int(moovSize)/2

	if cutPoint >= len(data) {
		cutPoint = moovOff + 16
	}

	truncated := data[:cutPoint]

	_, _, err := decode(bytes.NewReader(truncated))
	if err == nil {
		t.Fatal("expected error for truncated moov")
	}

	// Either ErrNoTrack (can't parse container) or ErrConfig (malformed cookie).
	if !errors.Is(err, alac.ErrNoTrack) && !errors.Is(err, alac.ErrConfig) {
		t.Fatalf("expected ErrNoTrack or ErrConfig, got: %v", err)
	}
}

func TestDecode_CorruptedStsd(t *testing.T) {
	t.Parallel()

	data := encodeTestM4A(t)

	stsdOff := findFourCC(data, "stsd")
	if stsdOff < 0 {
		t.Fatal("stsd not found in test M4A")
	}

	// Overwrite the stsd payload with garbage (after the 8-byte box header).
	corrupted := make([]byte, len(data))
	copy(corrupted, data)

	for i := stsdOff + 8; i < stsdOff+40 && i < len(corrupted); i++ {
		corrupted[i] = 0xFF
	}

	_, _, err := decode(bytes.NewReader(corrupted))
	if err == nil {
		t.Fatal("expected error for corrupted stsd")
	}

	// Container parse failure → ErrNoTrack, or malformed cookie → ErrConfig.
	if !errors.Is(err, alac.ErrNoTrack) && !errors.Is(err, alac.ErrConfig) {
		t.Fatalf("expected ErrNoTrack or ErrConfig, got: %v", err)
	}
}

func TestDecode_CorruptedALACCookie(t *testing.T) {
	t.Parallel()

	data := encodeTestM4A(t)

	// Find the 'alac' sample entry inside stsd. There are multiple 'alac'
	// FourCCs (box type and codec tag). Find stsd first, then search within it.
	stsdOff := findFourCC(data, "stsd")
	if stsdOff < 0 {
		t.Fatal("stsd not found in test M4A")
	}

	stsdSize := int(binary.BigEndian.Uint32(data[stsdOff : stsdOff+4]))

	// Search for 'alac' FourCC within stsd bounds.
	alacOff := -1

	for i := stsdOff + 8; i+7 < stsdOff+stsdSize; i++ {
		if data[i+4] == 'a' && data[i+5] == 'l' && data[i+6] == 'a' && data[i+7] == 'c' {
			alacOff = i

			break
		}
	}

	if alacOff < 0 {
		t.Fatal("alac entry not found within stsd")
	}

	// Corrupt the cookie area: set compatible version to 99.
	// The cookie is deep inside the alac sample entry. The ALACSpecificConfig
	// compatible version field is at offset 4 within the config.
	// Find it by looking for the second 'alac' FourCC (the codec-specific box).
	corrupted := make([]byte, len(data))
	copy(corrupted, data)

	// After the first 'alac' sample entry header, there's typically another
	// 'alac' box containing the actual config. Find it.
	innerAlacOff := -1

	for i := alacOff + 8; i+7 < stsdOff+stsdSize; i++ {
		if corrupted[i+4] == 'a' && corrupted[i+5] == 'l' && corrupted[i+6] == 'a' && corrupted[i+7] == 'c' {
			innerAlacOff = i

			break
		}
	}

	if innerAlacOff >= 0 {
		// Inner 'alac' box: [size:4][type:4][version:4][ALACSpecificConfig:24]
		// Config starts at innerAlacOff + 12. Compatible version is at config + 4.
		configStart := innerAlacOff + 12
		if configStart+5 < len(corrupted) {
			corrupted[configStart+4] = 99 // bad compatible version
		}
	} else {
		// Fallback: just corrupt bytes near the end of the alac entry.
		alacSize := int(binary.BigEndian.Uint32(data[alacOff : alacOff+4]))
		end := alacOff + alacSize

		if end-8 > alacOff && end-8 < len(corrupted) {
			corrupted[end-8] = 0xFF
			corrupted[end-7] = 0xFF
			corrupted[end-6] = 0xFF
			corrupted[end-5] = 0xFF
		}
	}

	_, _, err := decode(bytes.NewReader(corrupted))
	if err == nil {
		t.Fatal("expected error for corrupted ALAC cookie")
	}

	// Should be ErrConfig (bad version) or ErrNoTrack if parse failed entirely.
	if !errors.Is(err, alac.ErrConfig) && !errors.Is(err, alac.ErrNoTrack) {
		t.Fatalf("expected ErrConfig or ErrNoTrack, got: %v", err)
	}
}

func TestDecode_ZeroedStsz(t *testing.T) {
	t.Parallel()

	data := encodeTestM4A(t)

	stszOff := findFourCC(data, "stsz")
	if stszOff < 0 {
		t.Fatal("stsz not found in test M4A")
	}

	// stsz box layout: [size:4][type:4][version:1][flags:3][sampleSize:4][sampleCount:4][entries...]
	// Zero out sampleCount (at offset +12 from box start).
	corrupted := make([]byte, len(data))
	copy(corrupted, data)

	countOff := stszOff + 16
	if countOff+4 <= len(corrupted) {
		binary.BigEndian.PutUint32(corrupted[countOff:], 0)
	}

	// With zero samples, Decode should succeed but produce empty PCM.
	pcm, _, err := decode(bytes.NewReader(corrupted))
	if err != nil {
		// Also acceptable: an error during decode.
		return
	}

	if len(pcm) != 0 {
		t.Fatalf("expected empty PCM for zero stsz sample count, got %d bytes", len(pcm))
	}
}

func TestDecode_CorruptedMdat(t *testing.T) {
	t.Parallel()

	data := encodeTestM4A(t)

	mdatOff := findFourCC(data, "mdat")
	if mdatOff < 0 {
		t.Fatal("mdat not found in test M4A")
	}

	// Overwrite packet data inside mdat with garbage.
	corrupted := make([]byte, len(data))
	copy(corrupted, data)

	// Fill the first 512 bytes of mdat payload with garbage.
	for i := mdatOff + 8; i < mdatOff+520 && i < len(corrupted); i++ {
		corrupted[i] = 0xBA
	}

	_, _, err := decode(bytes.NewReader(corrupted))
	if err == nil {
		// Some corruptions may decode without error (garbage in → garbage out).
		// That's acceptable — ALAC doesn't have per-packet checksums.
		return
	}

	// If it does error, it should be ErrDecode.
	if !errors.Is(err, alac.ErrDecode) {
		t.Fatalf("expected ErrDecode, got: %v", err)
	}
}

func TestNewDecoder_EmptyReader(t *testing.T) {
	t.Parallel()

	_, err := alac.NewDecoder(bytes.NewReader(nil))
	if err == nil {
		t.Fatal("expected error for empty reader")
	}

	if !errors.Is(err, alac.ErrNoTrack) {
		t.Fatalf("expected ErrNoTrack, got: %v", err)
	}
}

func TestDecode_TruncatedPacket(t *testing.T) {
	t.Parallel()

	data := encodeTestM4A(t)

	mdatOff := findFourCC(data, "mdat")
	if mdatOff < 0 {
		t.Fatal("mdat not found in test M4A")
	}

	// Truncate the file partway through mdat.
	mdatSize := binary.BigEndian.Uint32(data[mdatOff : mdatOff+4])
	cutPoint := mdatOff + int(mdatSize)/2

	if cutPoint >= len(data) {
		cutPoint = mdatOff + 16
	}

	truncated := data[:cutPoint]

	_, _, err := decode(bytes.NewReader(truncated))
	if err == nil {
		// Might succeed if the sample table entries before the cut point
		// happen to be complete. Not guaranteed to fail.
		return
	}

	// Any error is acceptable — the decoder hit truncated data.
	t.Logf("got expected error: %v", err)
}
