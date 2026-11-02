"""Package built binaries together with project and dependency notices."""
import argparse
from pathlib import Path
import zipfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--name', required=True)
    args = parser.parse_args()
    if Path(args.name).name != args.name or args.name in {'.', '..'}:
        parser.error('name must be a single filename component')
    root = Path(__file__).resolve().parents[1]
    binaries = sorted((root / 'build/package').glob('*'))
    if not binaries or any(not p.is_file() for p in binaries):
        parser.error('build/package must contain binaries')
    output = root / 'dist' / f'{args.name}.zip'
    output.parent.mkdir(exist_ok=True)
    notices = [root / name for name in ('LICENSE', 'README.md', 'README.en.md', 'BUILD.md')]
    notices.extend(sorted((root / 'licenses').glob('*.txt')))
    with zipfile.ZipFile(output, 'w', compression=zipfile.ZIP_DEFLATED) as archive:
        for path in binaries:
            archive.write(path, path.name)
        for path in notices:
            archive.write(path, path.relative_to(root))
    print(output)


if __name__ == '__main__':
    main()
