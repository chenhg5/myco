#!/usr/bin/env bash
# Minimal LGPL FFmpeg: no bundled GPL encoders, players or network inputs.
set -euo pipefail
out="$(pwd)/component-out"
mkdir -p "$out"
source_dir="$(pwd)/ffmpeg-source"
if [[ ! -d "$source_dir/.git" ]]; then
  git init "$source_dir"
  git -C "$source_dir" remote add origin https://github.com/FFmpeg/FFmpeg.git
fi
revision=f893221c8d89cb798b829bebe71d55e1a3f242fd # FFmpeg n7.1.2
 git -C "$source_dir" fetch --depth=1 origin "$revision"
 git -C "$source_dir" checkout --detach FETCH_HEAD
[[ "$(git -C "$source_dir" rev-parse HEAD)" == "$revision" ]]
cd "$source_dir"
args=(--disable-everything --disable-autodetect --disable-doc --disable-debug
 --disable-shared --enable-static --disable-programs --enable-ffmpeg
 --enable-avdevice --enable-avfilter --enable-swscale --enable-network
 --enable-protocol=file,udp,rtp,pipe --enable-demuxer=rawvideo --enable-muxer=rtp,image2pipe --enable-decoder=rawvideo,bmp
 --enable-encoder=bmp --enable-filter=scale,format,fps,null,select --enable-bsf=h264_mp4toannexb
 --enable-parser=h264 --disable-x86asm)
binary=ffmpeg
case "${RUNNER_OS:-$(uname -s)}" in
 macOS|Darwin) args+=(--enable-indev=avfoundation --enable-encoder=h264_videotoolbox --enable-avfoundation --enable-videotoolbox) ;;
 Windows|MINGW*|MSYS*) binary=ffmpeg.exe; args+=(--enable-indev=gdigrab --enable-encoder=h264_mf --enable-mediafoundation --extra-ldflags=-static) ;;
 *) echo 'Only macOS and Windows are supported' >&2; exit 1 ;;
esac
./configure "${args[@]}"
make -j4 "$binary"
cp "$binary" "$out/"
cp COPYING.LGPLv2.1 "$out/LICENSE-FFmpeg.txt"
printf '%s\n' 'FFmpeg n7.1.2, LGPL-2.1-or-later, https://ffmpeg.org/' \
 'Source and build configuration: https://github.com/chenhg5/myco/tree/main/components/remote/scripts/build-ffmpeg.sh' \
 "Source revision: $revision" > "$out/NOTICE-FFmpeg.txt"
