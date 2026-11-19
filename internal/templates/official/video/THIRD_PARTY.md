# Third-party code in 视频剪辑

- [mediabunny](https://github.com/Vanilagy/mediabunny) 1.66.1, MPL-2.0: reading
  and writing mp4, encoding with WebCodecs. Unmodified; its source is at the link.
- [@mediabunny/aac-encoder](https://github.com/Vanilagy/mediabunny/tree/main/packages/aac-encoder)
  1.66.1, MPL-2.0, loaded only in browsers without an AAC or Opus encoder. It
  contains FFmpeg's AAC encoder built to WebAssembly; FFmpeg is LGPL-2.1 or
  later (no GPL, version 3 or nonfree parts). The FFmpeg source and the steps
  to rebuild it, or to use a modified FFmpeg, are in that package's README.
- [lucide](https://lucide.dev) icons, ISC; [React](https://react.dev), MIT.
