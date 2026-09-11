# Bundled Noto color emoji

128px PNGs from [googlefonts/noto-emoji](https://github.com/googlefonts/noto-emoji/tree/8998f5dd683424a73e2314a8c1f1e359c19e8742).
Copyright Google Inc. and contributors; image licensing follows the upstream
README (retained in the archive): Apache-2.0 for most image resources, with
region flags as described below. The archive retains the upstream root LICENSE
(OFL) and includes the applicable `LICENSE-APACHE-2.0.txt` and AUTHORS. Apache
license text is from https://www.apache.org/licenses/LICENSE-2.0.txt.
Upstream describes the region flags as public domain or exempt from copyright;
upstream notices and aliases are retained in the archive. Country/subdivision
flag sources are resized onto transparent 128px squares, preserving their aspect
ratios; the other PNG artwork is unmodified.

- Commit: `8998f5dd683424a73e2314a8c1f1e359c19e8742`
- PNG count: 3985
- Archive SHA-256: `eeb50e440f905a24b44e9bed47d5311efaba3e956387ba118edc593935b8584f`

The daemon embeds the archive, decodes only images used in each render, and makes
no external image requests. Regenerate from this directory with
`python3 update.py 8998f5dd683424a73e2314a8c1f1e359c19e8742`. Requires Python 3 and the project Go toolchain; downloads source data only.
