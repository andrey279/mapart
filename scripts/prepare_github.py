"""Export publishable sources using .gitignore, without changing local Git data."""
import hashlib
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
DEST = ROOT / 'dist/github'
SECRET_PATTERNS = [
    rb'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----',
    rb'gh[pousr]_[A-Za-z0-9]{30,}',
    rb'github_pat_[A-Za-z0-9_]{40,}',
    rb'AKIA[0-9A-Z]{16}',
    rb'\b[0-9]{8,12}:[A-Za-z0-9_-]{35}\b',
]


def main():
    # An isolated Git directory also works when the workspace .git is absent or
    # read-only. Enumerate all files, including locally modified/untracked ones.
    with tempfile.TemporaryDirectory(prefix='mapart-export-') as temp:
        subprocess.run(['git', 'init', '--quiet', temp], check=True)
        raw = subprocess.check_output([
            'git', f'--git-dir={temp}/.git', f'--work-tree={ROOT}',
            'ls-files', '--others', '--exclude-standard', '-z',
        ], cwd=ROOT)
    names = sorted(name.decode('utf-8') for name in raw.split(b'\0') if name)
    if not names:
        raise SystemExit('No source files found')
    for name in names:
        path = ROOT / name
        if path.is_symlink() or not path.is_file():
            raise SystemExit(f'Not a regular source file: {name}')
        if path.stat().st_size >= 95 * 1024 * 1024:
            raise SystemExit(f'File too large for a source repository: {name}')
        data = path.read_bytes()
        if any(re.search(pattern, data) for pattern in SECRET_PATTERNS):
            raise SystemExit(f'Possible credential in {name}; export stopped')
    tree = DEST / 'mapartscript'
    if tree.exists():
        raise SystemExit(f'{tree} already exists; move it aside before exporting again')
    tree.mkdir(parents=True)
    manifest = []
    archive_path = DEST / 'mapartscript-github.zip'
    with zipfile.ZipFile(archive_path, 'w', compression=zipfile.ZIP_DEFLATED) as archive:
        for name in names:
            source, target = ROOT / name, tree / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(source, target)
            executable = name.endswith('.sh') or name == '.githooks/pre-commit'
            target.chmod(0o755 if executable else 0o644)
            manifest.append(f'{hashlib.sha256(target.read_bytes()).hexdigest()}  {name}\n')
            archive.write(target, Path('mapartscript') / name)
    (DEST / 'SOURCE_SHA256SUMS.txt').write_text(''.join(manifest), encoding='utf-8')
    digest = hashlib.sha256(archive_path.read_bytes()).hexdigest()
    (DEST / 'SHA256SUMS.txt').write_text(f'{digest}  {archive_path.name}\n', encoding='utf-8')
    print(f'{len(names)} source files, {sum((tree / n).stat().st_size for n in names):,} bytes')
    print(f'Sources: {tree}\nArchive: {archive_path}')
    print('Known credential patterns: no matches. Local images and builds preserved.')


if __name__ == '__main__':
    main()
