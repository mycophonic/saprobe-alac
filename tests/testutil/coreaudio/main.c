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

// alac-coreaudio: ALAC encoder/decoder using macOS CoreAudio (AudioToolbox).
//
// Usage:
//   alac-coreaudio decode [input] [output]
//   alac-coreaudio encode [--sample-rate N] [--bit-depth N] [--channels N] [input] [output]
//
// Use "-" for stdin/stdout.

#include <AudioToolbox/AudioToolbox.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

// ---------------------------------------------------------------------------
// Memory-backed reader for AudioFileOpenWithCallbacks.
// ---------------------------------------------------------------------------

typedef struct {
    const char *data;
    int64_t     size;
} mem_reader;

static OSStatus mem_read_proc(
    void   *inClientData,
    SInt64  inPosition,
    UInt32  requestCount,
    void   *buffer,
    UInt32 *actualCount
) {
    mem_reader *r = (mem_reader *)inClientData;
    if (inPosition >= r->size) {
        *actualCount = 0;
        return noErr;
    }
    int64_t available = r->size - inPosition;
    UInt32 toRead = requestCount;
    if ((int64_t)toRead > available) {
        toRead = (UInt32)available;
    }
    memcpy(buffer, r->data + inPosition, toRead);
    *actualCount = toRead;
    return noErr;
}

static SInt64 mem_get_size_proc(void *inClientData) {
    mem_reader *r = (mem_reader *)inClientData;
    return (SInt64)r->size;
}

// ---------------------------------------------------------------------------
// Read entire file (or stdin) into a malloc'd buffer.
// ---------------------------------------------------------------------------

static char *read_all(const char *path, int64_t *out_size) {
    FILE *f;
    if (strcmp(path, "-") == 0) {
        f = stdin;
    } else {
        f = fopen(path, "rb");
        if (!f) {
            fprintf(stderr, "error: cannot open '%s': %s\n", path, strerror(errno));
            return NULL;
        }
    }

    size_t cap = 1024 * 1024;
    size_t len = 0;
    char *buf = malloc(cap);
    if (!buf) {
        fprintf(stderr, "error: out of memory\n");
        if (f != stdin) fclose(f);
        return NULL;
    }

    for (;;) {
        size_t n = fread(buf + len, 1, cap - len, f);
        len += n;
        if (n == 0) break;
        if (len == cap) {
            cap *= 2;
            char *tmp = realloc(buf, cap);
            if (!tmp) {
                fprintf(stderr, "error: out of memory\n");
                free(buf);
                if (f != stdin) fclose(f);
                return NULL;
            }
            buf = tmp;
        }
    }

    if (f != stdin) fclose(f);
    *out_size = (int64_t)len;
    return buf;
}

// ---------------------------------------------------------------------------
// WAV header parsing.
// ---------------------------------------------------------------------------

static int parse_wav_header(
    const char *data, int64_t size,
    int *sample_rate, int *bit_depth, int *channels, int64_t *pcm_offset, int64_t *pcm_size
) {
    const unsigned char *d = (const unsigned char *)data;

    if (size < 44) return -1;
    if (memcmp(d, "RIFF", 4) != 0) return -1;
    if (memcmp(d + 8, "WAVE", 4) != 0) return -1;

    // Walk chunks to find "fmt " and "data".
    int64_t pos = 12;
    int found_fmt = 0;
    int found_data = 0;
    while (pos + 8 <= size) {
        uint32_t chunk_size = (uint32_t)d[pos+4]
            | ((uint32_t)d[pos+5] << 8)
            | ((uint32_t)d[pos+6] << 16)
            | ((uint32_t)d[pos+7] << 24);

        if (memcmp(d + pos, "fmt ", 4) == 0 && chunk_size >= 16) {
            uint16_t format = (uint16_t)d[pos+8] | ((uint16_t)d[pos+9] << 8);
            if (format != 1) { // PCM only
                fprintf(stderr, "error: WAV format %u not supported (PCM only)\n", format);
                return -1;
            }
            *channels    = (int)((uint16_t)d[pos+10] | ((uint16_t)d[pos+11] << 8));
            *sample_rate = (int)((uint32_t)d[pos+12]
                | ((uint32_t)d[pos+13] << 8)
                | ((uint32_t)d[pos+14] << 16)
                | ((uint32_t)d[pos+15] << 24));
            *bit_depth   = (int)((uint16_t)d[pos+22] | ((uint16_t)d[pos+23] << 8));
            found_fmt = 1;
        }

        if (memcmp(d + pos, "data", 4) == 0) {
            *pcm_offset = pos + 8;
            *pcm_size   = (int64_t)chunk_size;
            found_data = 1;
        }

        if (found_fmt && found_data) return 0;

        pos += 8 + chunk_size;
        if (chunk_size & 1) pos++; // WAV chunks are 2-byte aligned
    }

    return -1; // missing fmt or data chunk
}

static int is_wav(const char *data, int64_t size) {
    return size >= 12 && memcmp(data, "RIFF", 4) == 0 && memcmp(data + 8, "WAVE", 4) == 0;
}

// ---------------------------------------------------------------------------
// Decode: ALAC container → raw PCM.
// ---------------------------------------------------------------------------

static int do_decode(const char *input_path, const char *output_path) {
    int64_t input_size = 0;
    char *input_data = read_all(input_path, &input_size);
    if (!input_data) return 1;

    mem_reader reader = { .data = input_data, .size = input_size };

    AudioFileID audioFile = NULL;
    OSStatus status = AudioFileOpenWithCallbacks(
        &reader, mem_read_proc, NULL, mem_get_size_proc, NULL,
        0, // auto-detect container type
        &audioFile
    );
    if (status != noErr) {
        fprintf(stderr, "error: AudioFileOpenWithCallbacks failed (OSStatus %d)\n", (int)status);
        free(input_data);
        return 1;
    }

    ExtAudioFileRef extFile = NULL;
    status = ExtAudioFileWrapAudioFileID(audioFile, false, &extFile);
    if (status != noErr) {
        fprintf(stderr, "error: ExtAudioFileWrapAudioFileID failed (OSStatus %d)\n", (int)status);
        AudioFileClose(audioFile);
        free(input_data);
        return 1;
    }

    // Query source format.
    AudioStreamBasicDescription srcFormat;
    UInt32 propSize = sizeof(srcFormat);
    status = ExtAudioFileGetProperty(extFile, kExtAudioFileProperty_FileDataFormat, &propSize, &srcFormat);
    if (status != noErr) {
        fprintf(stderr, "error: cannot read source format (OSStatus %d)\n", (int)status);
        ExtAudioFileDispose(extFile);
        AudioFileClose(audioFile);
        free(input_data);
        return 1;
    }

    // Determine output bit depth from source.
    // For compressed formats (ALAC), mBitsPerChannel is 0 in the file data format.
    // ALAC stores the source bit depth in mFormatFlags:
    //   1 = 16-bit, 2 = 20-bit, 3 = 24-bit, 4 = 32-bit.
    UInt32 outBitsPerChannel = srcFormat.mBitsPerChannel;
    if (outBitsPerChannel == 0 && srcFormat.mFormatID == kAudioFormatAppleLossless) {
        switch (srcFormat.mFormatFlags) {
        case kAppleLosslessFormatFlag_16BitSourceData: outBitsPerChannel = 16; break;
        case kAppleLosslessFormatFlag_20BitSourceData: outBitsPerChannel = 20; break;
        case kAppleLosslessFormatFlag_24BitSourceData: outBitsPerChannel = 24; break;
        case kAppleLosslessFormatFlag_32BitSourceData: outBitsPerChannel = 32; break;
        default: outBitsPerChannel = 16; break; // fallback
        }
    }
    if (outBitsPerChannel == 0) outBitsPerChannel = 16; // fallback for non-ALAC

    // CoreAudio outputs PCM at byte-aligned depths. 20-bit source is output as 24-bit.
    UInt32 clientBitsPerChannel = outBitsPerChannel;
    if (clientBitsPerChannel == 20) clientBitsPerChannel = 24;
    UInt32 bytesPerSample = clientBitsPerChannel / 8;

    // Set client format: interleaved signed LE PCM at source bit depth.
    AudioStreamBasicDescription clientFormat;
    memset(&clientFormat, 0, sizeof(clientFormat));
    clientFormat.mSampleRate       = srcFormat.mSampleRate;
    clientFormat.mFormatID         = kAudioFormatLinearPCM;
    clientFormat.mFormatFlags      = kAudioFormatFlagIsSignedInteger | kAudioFormatFlagIsPacked;
    clientFormat.mBitsPerChannel   = clientBitsPerChannel;
    clientFormat.mChannelsPerFrame = srcFormat.mChannelsPerFrame;
    clientFormat.mBytesPerFrame    = bytesPerSample * srcFormat.mChannelsPerFrame;
    clientFormat.mFramesPerPacket  = 1;
    clientFormat.mBytesPerPacket   = clientFormat.mBytesPerFrame;

    status = ExtAudioFileSetProperty(extFile, kExtAudioFileProperty_ClientDataFormat, sizeof(clientFormat), &clientFormat);
    if (status != noErr) {
        fprintf(stderr, "error: cannot set client format (OSStatus %d)\n", (int)status);
        ExtAudioFileDispose(extFile);
        AudioFileClose(audioFile);
        free(input_data);
        return 1;
    }

    // Total frame count.
    SInt64 totalFrames = 0;
    propSize = sizeof(totalFrames);
    status = ExtAudioFileGetProperty(extFile, kExtAudioFileProperty_FileLengthFrames, &propSize, &totalFrames);
    if (status != noErr || totalFrames <= 0) {
        fprintf(stderr, "error: cannot determine frame count (OSStatus %d, frames %lld)\n", (int)status, totalFrames);
        ExtAudioFileDispose(extFile);
        AudioFileClose(audioFile);
        free(input_data);
        return 1;
    }

    // Print format info to stderr.
    fprintf(stderr, "sample_rate=%d bit_depth=%u channels=%u frames=%lld\n",
        (int)srcFormat.mSampleRate, outBitsPerChannel, srcFormat.mChannelsPerFrame, totalFrames);

    // Open output.
    FILE *out;
    if (strcmp(output_path, "-") == 0) {
        out = stdout;
    } else {
        out = fopen(output_path, "wb");
        if (!out) {
            fprintf(stderr, "error: cannot open output '%s': %s\n", output_path, strerror(errno));
            ExtAudioFileDispose(extFile);
            AudioFileClose(audioFile);
            free(input_data);
            return 1;
        }
    }

    // Decode loop.
    const UInt32 framesPerRead = 4096;
    char *readBuf = malloc(framesPerRead * clientFormat.mBytesPerFrame);
    if (!readBuf) {
        fprintf(stderr, "error: out of memory\n");
        if (out != stdout) fclose(out);
        ExtAudioFileDispose(extFile);
        AudioFileClose(audioFile);
        free(input_data);
        return 1;
    }

    int ret = 0;
    SInt64 framesDecoded = 0;
    while (framesDecoded < totalFrames) {
        UInt32 frameCount = framesPerRead;

        AudioBufferList bufList;
        bufList.mNumberBuffers = 1;
        bufList.mBuffers[0].mNumberChannels = srcFormat.mChannelsPerFrame;
        bufList.mBuffers[0].mDataByteSize   = frameCount * clientFormat.mBytesPerFrame;
        bufList.mBuffers[0].mData           = readBuf;

        status = ExtAudioFileRead(extFile, &frameCount, &bufList);
        if (status != noErr) {
            fprintf(stderr, "error: ExtAudioFileRead failed (OSStatus %d)\n", (int)status);
            ret = 1;
            break;
        }
        if (frameCount == 0) break;

        size_t bytes = frameCount * clientFormat.mBytesPerFrame;
        if (fwrite(readBuf, 1, bytes, out) != bytes) {
            fprintf(stderr, "error: write failed\n");
            ret = 1;
            break;
        }
        framesDecoded += frameCount;
    }

    free(readBuf);
    if (out != stdout) fclose(out);
    ExtAudioFileDispose(extFile);
    AudioFileClose(audioFile);
    free(input_data);
    return ret;
}

// ---------------------------------------------------------------------------
// Encode: raw PCM or WAV → ALAC M4A.
// ---------------------------------------------------------------------------

static int do_encode(
    const char *input_path, const char *output_path,
    int sample_rate, int bit_depth, int channels
) {
    int64_t input_size = 0;
    char *input_data = read_all(input_path, &input_size);
    if (!input_data) return 1;

    const char *pcm_data = input_data;
    int64_t pcm_size = input_size;

    // Auto-detect WAV.
    if (is_wav(input_data, input_size)) {
        int wav_sr, wav_bd, wav_ch;
        int64_t wav_offset, wav_pcm_size;
        if (parse_wav_header(input_data, input_size, &wav_sr, &wav_bd, &wav_ch, &wav_offset, &wav_pcm_size) != 0) {
            fprintf(stderr, "error: invalid WAV file\n");
            free(input_data);
            return 1;
        }
        // WAV parameters override flags (WAV is self-describing).
        sample_rate = wav_sr;
        bit_depth   = wav_bd;
        channels    = wav_ch;
        pcm_data    = input_data + wav_offset;
        pcm_size    = wav_pcm_size;
        if (wav_offset + wav_pcm_size > input_size) {
            pcm_size = input_size - wav_offset;
        }
        fprintf(stderr, "WAV detected: sample_rate=%d bit_depth=%d channels=%d\n", sample_rate, bit_depth, channels);
    }

    // Validate parameters.
    if (sample_rate <= 0 || bit_depth <= 0 || channels <= 0) {
        fprintf(stderr, "error: --sample-rate, --bit-depth, and --channels are required for raw PCM input\n");
        free(input_data);
        return 1;
    }

    UInt32 bytesPerSample = (UInt32)bit_depth / 8;

    // Source format: interleaved signed LE PCM.
    AudioStreamBasicDescription srcFormat;
    memset(&srcFormat, 0, sizeof(srcFormat));
    srcFormat.mSampleRate       = (Float64)sample_rate;
    srcFormat.mFormatID         = kAudioFormatLinearPCM;
    srcFormat.mFormatFlags      = kAudioFormatFlagIsSignedInteger | kAudioFormatFlagIsPacked;
    srcFormat.mBitsPerChannel   = (UInt32)bit_depth;
    srcFormat.mChannelsPerFrame = (UInt32)channels;
    srcFormat.mBytesPerFrame    = bytesPerSample * (UInt32)channels;
    srcFormat.mFramesPerPacket  = 1;
    srcFormat.mBytesPerPacket   = srcFormat.mBytesPerFrame;

    // Destination format: ALAC.
    AudioStreamBasicDescription dstFormat;
    memset(&dstFormat, 0, sizeof(dstFormat));
    dstFormat.mSampleRate       = (Float64)sample_rate;
    dstFormat.mFormatID         = kAudioFormatAppleLossless;
    dstFormat.mChannelsPerFrame = (UInt32)channels;
    // mBytesPerPacket, mFramesPerPacket, mBytesPerFrame: set to 0 for VBR codec.
    // mBitsPerChannel encodes the source depth for ALAC.
    dstFormat.mBitsPerChannel   = (UInt32)bit_depth;

    // Stdout not supported for encode (CoreAudio needs a file URL).
    if (strcmp(output_path, "-") == 0) {
        fprintf(stderr, "error: encode to stdout is not supported (CoreAudio requires a file path)\n");
        free(input_data);
        return 1;
    }

    // Create output URL.
    CFStringRef pathStr = CFStringCreateWithCString(kCFAllocatorDefault, output_path, kCFStringEncodingUTF8);
    if (!pathStr) {
        fprintf(stderr, "error: invalid output path\n");
        free(input_data);
        return 1;
    }
    CFURLRef outputURL = CFURLCreateWithFileSystemPath(kCFAllocatorDefault, pathStr, kCFURLPOSIXPathStyle, false);
    CFRelease(pathStr);
    if (!outputURL) {
        fprintf(stderr, "error: cannot create output URL\n");
        free(input_data);
        return 1;
    }

    ExtAudioFileRef extFile = NULL;
    OSStatus status = ExtAudioFileCreateWithURL(
        outputURL, kAudioFileM4AType, &dstFormat, NULL,
        kAudioFileFlags_EraseFile, &extFile
    );
    CFRelease(outputURL);
    if (status != noErr) {
        fprintf(stderr, "error: ExtAudioFileCreateWithURL failed (OSStatus %d)\n", (int)status);
        free(input_data);
        return 1;
    }

    status = ExtAudioFileSetProperty(extFile, kExtAudioFileProperty_ClientDataFormat, sizeof(srcFormat), &srcFormat);
    if (status != noErr) {
        fprintf(stderr, "error: cannot set client format (OSStatus %d)\n", (int)status);
        ExtAudioFileDispose(extFile);
        free(input_data);
        return 1;
    }

    // Encode loop.
    const UInt32 framesPerWrite = 4096;
    int64_t totalFrames = pcm_size / srcFormat.mBytesPerFrame;
    int64_t framesWritten = 0;
    int ret = 0;

    fprintf(stderr, "encoding: sample_rate=%d bit_depth=%d channels=%d frames=%lld\n",
        sample_rate, bit_depth, channels, totalFrames);

    while (framesWritten < totalFrames) {
        UInt32 frameCount = framesPerWrite;
        int64_t remaining = totalFrames - framesWritten;
        if ((int64_t)frameCount > remaining) {
            frameCount = (UInt32)remaining;
        }

        AudioBufferList bufList;
        bufList.mNumberBuffers = 1;
        bufList.mBuffers[0].mNumberChannels = (UInt32)channels;
        bufList.mBuffers[0].mDataByteSize   = frameCount * srcFormat.mBytesPerFrame;
        bufList.mBuffers[0].mData           = (void *)(pcm_data + framesWritten * srcFormat.mBytesPerFrame);

        status = ExtAudioFileWrite(extFile, frameCount, &bufList);
        if (status != noErr) {
            fprintf(stderr, "error: ExtAudioFileWrite failed (OSStatus %d)\n", (int)status);
            ret = 1;
            break;
        }
        framesWritten += frameCount;
    }

    ExtAudioFileDispose(extFile);
    free(input_data);
    return ret;
}

// ---------------------------------------------------------------------------
// CLI argument parsing.
// ---------------------------------------------------------------------------

static void usage(void) {
    fprintf(stderr,
        "Usage:\n"
        "  alac-coreaudio decode <input> <output>\n"
        "  alac-coreaudio encode [--sample-rate N] [--bit-depth N] [--channels N] <input> <output>\n"
        "\n"
        "Use \"-\" for stdin (input) or stdout (output).\n"
        "Encode: WAV input is auto-detected; raw PCM requires all three flags.\n"
        "Decode: format metadata is printed to stderr.\n"
    );
}

int main(int argc, char *argv[]) {
    if (argc < 2) {
        usage();
        return 1;
    }

    if (strcmp(argv[1], "decode") == 0) {
        if (argc != 4) {
            fprintf(stderr, "error: decode requires exactly 2 arguments: <input> <output>\n");
            usage();
            return 1;
        }
        return do_decode(argv[2], argv[3]);
    }

    if (strcmp(argv[1], "encode") == 0) {
        int sample_rate = 0;
        int bit_depth   = 0;
        int channels    = 0;
        int argi = 2;

        // Parse optional flags.
        while (argi < argc && argv[argi][0] == '-' && strcmp(argv[argi], "-") != 0) {
            if (strcmp(argv[argi], "--sample-rate") == 0 && argi + 1 < argc) {
                sample_rate = atoi(argv[++argi]);
            } else if (strcmp(argv[argi], "--bit-depth") == 0 && argi + 1 < argc) {
                bit_depth = atoi(argv[++argi]);
            } else if (strcmp(argv[argi], "--channels") == 0 && argi + 1 < argc) {
                channels = atoi(argv[++argi]);
            } else {
                fprintf(stderr, "error: unknown flag '%s'\n", argv[argi]);
                usage();
                return 1;
            }
            argi++;
        }

        if (argc - argi != 2) {
            fprintf(stderr, "error: encode requires exactly 2 positional arguments: <input> <output>\n");
            usage();
            return 1;
        }

        return do_encode(argv[argi], argv[argi + 1], sample_rate, bit_depth, channels);
    }

    fprintf(stderr, "error: unknown command '%s'\n", argv[1]);
    usage();
    return 1;
}
