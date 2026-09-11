#!/usr/bin/env python3
"""Rebuild the bundled Noto PNG archive from one pinned upstream commit."""
import hashlib
import io
import json
from pathlib import Path
import sys
import subprocess
import tarfile
import urllib.request
import zipfile

commit = sys.argv[1]
if len(commit) != 40 or any(c not in '0123456789abcdef' for c in commit):
    raise SystemExit('Pass a full upstream commit SHA')
url = f'https://codeload.github.com/googlefonts/noto-emoji/tar.gz/{commit}'
with urllib.request.urlopen(url, timeout=120) as response:
    archive = response.read()
dest = Path(__file__).resolve().parent
files = {}
with tarfile.open(fileobj=io.BytesIO(archive), mode='r:gz') as source:
    for member in source.getmembers():
        path = member.name.split('/', 1)[-1]
        if not member.isfile():
            continue
        if path.startswith('png/128/emoji_u') and path.endswith('.png'):
            files[path.rsplit('/', 1)[-1]] = source.extractfile(member).read()
        elif path.startswith('third_party/region-flags/png/') and path.endswith('.png'):
            code = path.rsplit('/', 1)[-1][:-4]
            if len(code) == 2 and code.isalpha() and code.isupper():
                sequence = '_'.join(f'{0x1f1e6 + ord(c) - ord("A"):x}' for c in code)
            elif code in ('GB-ENG', 'GB-SCT', 'GB-WLS'):
                sequence = '1f3f4_' + '_'.join(f'{0xe0000+ord(c):x}' for c in code.lower().replace('-', '')) + '_e007f'
            else:
                continue
            files[f'emoji_u{sequence}.png'] = source.extractfile(member).read()
        elif path == 'README.md':
            files['UPSTREAM-README.md'] = source.extractfile(member).read()
        elif path in ('LICENSE', 'AUTHORS', 'NOTICE', 'emoji_aliases.txt', 'third_party/region-flags/LICENSE', 'third_party/region-flags/AUTHORS', 'third_party/region-flags/README.md', 'third_party/region-flags/README.third_party'):
            files[path.replace('/', '-')] = source.extractfile(member).read()
apache = (dest / 'LICENSE-APACHE-2.0.txt').read_bytes()
if hashlib.sha256(apache).hexdigest() != 'cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30':
    raise SystemExit('Unexpected Apache-2.0 license contents')
files['LICENSE-APACHE-2.0.txt'] = apache
if len(files) < 3000:
    raise SystemExit(f'Incomplete asset set: {len(files)}')
raw = dest / 'noto-emoji-raw.zip'
with zipfile.ZipFile(raw, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=9) as out:
    for name, data in sorted(files.items()):
        info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
        info.compress_type = zipfile.ZIP_DEFLATED
        info.external_attr = 0o644 << 16
        out.writestr(info, data)
packed = dest / 'noto-emoji-new.zip'
subprocess.run(['go', 'run', str(dest/'pack.go'), str(raw), str(packed)], check=True)
packed.replace(dest/'noto-emoji.zip')
raw.unlink()
(dest / 'LICENSE').write_text('\n'.join(line.rstrip() for line in files['LICENSE'].decode().splitlines()) + '\n')
(dest / 'AUTHORS').write_bytes(files['AUTHORS'])
checksum = hashlib.sha256((dest / 'noto-emoji.zip').read_bytes()).hexdigest()
(dest / 'README.md').write_text(f'''# Bundled Noto color emoji

128px PNGs from [googlefonts/noto-emoji](https://github.com/googlefonts/noto-emoji/tree/{commit}).
Copyright Google Inc. and contributors; image licensing follows the upstream
README (retained in the archive): Apache-2.0 for most image resources, with
region flags as described below. The archive retains the upstream root LICENSE
(OFL) and includes the applicable `LICENSE-APACHE-2.0.txt` and AUTHORS. Apache
license text is from https://www.apache.org/licenses/LICENSE-2.0.txt.
Upstream describes the region flags as public domain or exempt from copyright;
upstream notices and aliases are retained in the archive. Country/subdivision
flag sources are resized onto transparent 128px squares, preserving their aspect
ratios; the other PNG artwork is unmodified.

- Commit: `{commit}`
- PNG count: {sum(name.endswith('.png') for name in files)}
- Archive SHA-256: `{checksum}`

The daemon embeds the archive, decodes only images used in each render, and makes
no external image requests. Regenerate from this directory with
`python3 update.py {commit}`. Requires Python 3 and the project Go toolchain; downloads source data only.
''')
print(json.dumps({'commit': commit, 'pngs': sum(n.endswith('.png') for n in files), 'archive_bytes': (dest/'noto-emoji.zip').stat().st_size}), flush=True)
