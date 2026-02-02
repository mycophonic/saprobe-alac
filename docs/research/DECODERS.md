# ALAC Decoders: Prevalence and Technical Specifications

## Summary

**Unlike encoders, ALAC decoders are essentially uniform**: all major implementations support the full ALAC specification (16/20/24/32-bit, 1-8 channels, up to 384 kHz). There are no significant capability gaps between decoders.

---

## Decoder Comparison Table

| Rank | Decoder | Bit Depth | Sample Rate | Channels | Platform | Notes |
|------|---------|-----------|-------------|----------|----------|-------|
| 1 | **Apple CoreAudio / AudioToolbox** | 16, 20, 24, 32 | 1 Hz – 384 kHz | 1–8 | macOS, iOS, Windows | Proprietary system framework. Powers Apple Music, iTunes, QuickTime. |
| 2 | **Apple Reference (open-source)** | 16, 20, 24, 32 | 1 Hz – 384 kHz | 1–8 | Cross-platform | Apache 2.0 license (Oct 2011). github.com/macosforge/alac. Basis for refalac, Rockbox, and many other implementations. |
| 3 | **FFmpeg (libavcodec)** | 16, 20, 24, 32 | Any | 1–8 | Cross-platform | Independent implementation. **Full spec support** (unlike encoder). Powers VLC, Plex, Kodi, mpv, MPlayer, Jellyfin, and most media software. |
| 4 | **Windows 10/11 Media Foundation** | 16, 20, 24, 32 | 1 Hz – 384 kHz | 1–8 | Windows | Native OS codec since Windows 10. Used by Windows Media Player, Groove, etc. |
| 5 | **foobar2000 (foo_input_alac)** | 16, 20, 24, 32 | Any | 1–8 | Windows | Plugin-based. Full spec support. |
| 6 | **Rockbox** | 16, 20, 24, 32 | Any | 1–2 | Embedded/portable | Uses Apple open-source code, ARM-optimized. Stereo only (hardware limitation). ~4x more CPU than FLAC. |

---

## Historical Note

**David Hammerton's reverse-engineered decoder (March 2005)** was the first open-source ALAC decoder, created by analyzing Apple's bitstream format. It enabled ALAC playback on non-Apple platforms for 6+ years before Apple open-sourced their implementation. Now obsolete — superseded by Apple's official open-source release in October 2011.

---

## Key Differences from Encoders

| Aspect | Encoders | Decoders |
|--------|----------|----------|
| 20-bit support | **Only Apple-based** | All implementations |
| 32-bit support | **Only Apple-based** | All implementations |
| Capability gaps | Significant (FFmpeg limited) | None (all full-spec) |
| Recommendation | Choose carefully | Any will work |

---

## Why Decoders Are Uniform

1. **Simpler problem**: Decoding just reads what's in the file; encoding must make compression decisions
2. **Open-source reference**: Apple released decoder source in 2011; everyone uses it or reimplements it faithfully
3. **Interoperability requirement**: A decoder that can't play valid files is useless

---

## Platform-Specific Notes

### FFmpeg-based players (VLC, mpv, Plex, Kodi, etc.)
- **Full 16/20/24/32-bit decoding** — no limitations
- Asymmetry: can decode 32-bit ALAC but cannot create it
- Warning: Converting ALAC to WAV with `ffmpeg -i input.m4a output.wav` **silently truncates to 16-bit**. Use `-c:a pcm_s24le` or `-c:a pcm_s32le` explicitly.

### Windows 10/11
- Native ALAC support added in Windows 10
- Works in Windows Media Player, Movies & TV, and any app using Media Foundation
- Full spec support

### Rockbox (portable players)
- Full bit-depth support
- Limited to stereo (hardware constraint, not codec)
- ~4x CPU usage vs FLAC (battery impact on low-power devices)

### foobar2000
- Requires `foo_input_alac` component (included in Free Encoder Pack)
- Full spec decoding
- Encoder component uses refalac (Apple reference)

---

## Practical Implications

**For playback**: Use any modern player. They all work.

**For transcoding**: Be aware that FFmpeg can decode 32-bit ALAC perfectly but cannot re-encode it losslessly back to ALAC (will truncate to 24-bit). If round-trip fidelity matters for 32-bit content, use Apple-based tools.

---

## ALAC Format Specification (Reference)

| Parameter | Specification |
|-----------|---------------|
| Bit depths | 16, 20, 24, 32 (integer PCM) |
| Sample rates | 1 – 384,000 Hz |
| Channels | 1 – 8 |
| Container formats | M4A (MP4), CAF |
| CPU efficiency | ~4x more than FLAC (Rockbox measurements) |