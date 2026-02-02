# ALAC Encoders: Prevalence and Technical Specifications

## Overview

ALAC (Apple Lossless Audio Codec) supports up to **8 channels** at **16, 20, 24, and 32-bit** depth with sample rates
from **1 Hz to 384 kHz**.
However, not all encoders implement the full specification.

---

## Encoder Comparison Table

| Rank | Encoder                            | Bit Depth      | Sample Rate       | Channels   | Platform                               | Notes                                                                                                                                                                                              |
|------|------------------------------------|----------------|-------------------|------------|----------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 1    | **Apple CoreAudio / AudioToolbox** | 16, 20, 24, 32 | 1 Hz – 384 kHz    | 1–8        | macOS, iOS, Windows (via iTunes/Music) | Reference implementation. Used by Apple Music for lossless catalog. By far the dominant encoder by volume.                                                                                         |
| 2    | **FFmpeg (libavcodec)**            | 16, 24         | Any               | 1–8        | Cross-platform                         | **Cannot encode 20-bit or 32-bit** (20-bit input silently promoted to 24-bit; 32-bit lossy-converted to 24-bit). Prioritizes speed over compression ratio—produces ~5-10% larger files than Apple. |
| 3    | **qaac / refalac**                 | 16, 20, 24, 32 | 1 Hz – 384 kHz    | 1–8        | Windows                                | Command-line wrapper around Apple's CoreAudioToolbox.dll. `refalac` uses Apple's open-source reference encoder. Popular in audiophile/encoding communities.                                        |
| 4    | **XLD (X Lossless Decoder)**       | 16, 20, 24, 32 | 1 Hz – 384 kHz    | 1–8        | macOS                                  | Uses CoreAudio backend. Very popular for CD ripping among Mac users.                                                                                                                               |
| 5    | **dBpoweramp**                     | 16, 20, 24, 32 | 1 Hz – 384 kHz    | 1–8        | Windows, macOS                         | Commercial. Uses Apple encoder via CoreAudio.                                                                                                                                                      |
| 6    | **CUETools**                       | **16 only**    | **44.1 kHz only** | **2 only** | Windows                                | CD-focused tool. ALAC support limited to Red Book CD spec (16/44.1/stereo). Has known encoder bugs at higher compression levels.                                                                   |
| 7    | **fre:ac / other GUI converters**  | Varies         | Varies            | Varies     | Cross-platform                         | Typically wrap FFmpeg; inherit its limitations.                                                                                                                                                    |

---

## Detailed Notes

### Apple CoreAudio (Rank 1)
- **Usage**: Hundreds of millions of devices. Every Mac, iPhone, iPad, Apple TV.
- **Compression**: Best ratio among all encoders (~40-60% of original).
- **Integration**: Native to macOS/iOS; available on Windows via iTunes/Apple Music app.
- **Apple Music**: Entire lossless catalog (100M+ tracks) encoded with this.

### FFmpeg (Rank 2)
- **Usage**: Powers most online converters, media servers (Plex, Jellyfin, Kodi), and countless media pipelines.
- **Limitations**:
    - 20-bit input → encoded as 24-bit (wasteful but lossless)
    - 32-bit input → **lossy conversion to 24-bit** (data loss!)
- **Performance**: Faster encoding than Apple, but worse compression.
- **Decoding**: Full spec support (16/20/24/32-bit).

### qaac / refalac (Rank 3)
- **Usage**: Standard tool for Windows audiophiles; used by LameXP, foobar2000 plugins.
- **`qaac`**: Requires Apple DLLs (CoreAudioToolbox.dll, etc.)
- **`refalac`**: Standalone, uses Apple's Apache-licensed open-source code.
- **Quality**: Identical to Apple's encoder (same codebase).

### CUETools (Rank 6)
- **Usage**: Niche—CD ripping/verification community.
- **Limitation**: Hardcoded to CD spec only. From source code:
  ```csharp
  if (pcm.BitsPerSample != 16 || pcm.ChannelCount != 2 || pcm.SampleRate != 44100)
  ```
- **Known Issues**: Encoder bugs at compression levels 2-10 causing sample corruption on some albums.

---

## Summary: Which Encoder to Use?

| Use Case | Recommended Encoder |
|----------|---------------------|
| macOS user, any audio | **XLD** or **Apple Music app** |
| Windows user, hi-res audio | **qaac** or **refalac** |
| Cross-platform automation/scripting | **FFmpeg** (if 16/24-bit only) |
| CD ripping with verification | **CUETools** (16/44.1 only) |
| 32-bit audio | **Apple encoder only** (qaac/refalac/XLD) |
| Maximum compression | **Apple encoder** (any frontend) |
| Maximum speed | **FFmpeg** |

---

## ALAC Format Specification Reference

| Parameter | Specification |
|-----------|---------------|
| Bit depths | 16, 20, 24, 32 (integer PCM) |
| Sample rates | 1 – 384,000 Hz |
| Channels | 1 – 8 |
| Container formats | M4A (MP4), CAF |
| Frame size | Default 4096 samples; max 16,384 |
| Compression | ~40-60% of original (content-dependent) |
| DRM | None inherent; container may add |

### Channel Layouts (1-8 channels)

| Channels | Layout |
|----------|--------|
| 1 | Mono |
| 2 | Stereo (L, R) |
| 3 | MPEG 3.0 B (C, L, R) |
| 4 | MPEG 4.0 B (C, L, R, Cs) |
| 5 | MPEG 5.0 D (C, L, R, Ls, Rs) |
| 6 | MPEG 5.1 D (C, L, R, Ls, Rs, LFE) |
| 7 | AAC 6.1 (C, L, R, Ls, Rs, Cs, LFE) |
| 8 | MPEG 7.1 B (C, Lc, Rc, L, R, Ls, Rs, LFE) |