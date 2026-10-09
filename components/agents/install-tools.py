#!/usr/bin/env python3
"""Download only reviewed, SHA-256 locked inputs; never run vendor installers."""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.request


def download(item, directory):
    target = directory / item['url'].rsplit('/', 1)[-1]
    for attempt in range(3):
        digest = hashlib.sha256()
        try:
            with urllib.request.urlopen(item['url'], timeout=60) as response, target.open('wb') as output:
                while chunk := response.read(1024 * 1024):
                    output.write(chunk)
                    digest.update(chunk)
            break
        except (urllib.error.URLError, TimeoutError, ConnectionError) as error:
            if isinstance(error, urllib.error.HTTPError) and error.code != 429 and error.code < 500:
                raise
            if attempt == 2:
                raise RuntimeError('Download failed: ' + item['url']) from error
            print(f'Retrying {target.name}: {error}', flush=True)
            time.sleep(2 ** attempt)
    if digest.hexdigest() != item['sha256']:
        target.unlink()
        raise ValueError('SHA-256 mismatch: ' + item['url'])
    print('Verified ' + target.name, flush=True)
    return target


def main():
    mode, lock, destination = sys.argv[1:]
    items = json.loads(Path(lock).read_text())
    destination = Path(destination)
    destination.mkdir(parents=True, exist_ok=True)
    if mode == 'rpms':
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            list(pool.map(lambda item: download(item, destination), items))
        return
    if mode != 'tools':
        raise ValueError('Expected rpms or tools mode')
    for item in items:
        with tempfile.TemporaryDirectory() as temporary:
            archive = download(item, Path(temporary))
            output = destination / item['name']
            if item.get('directory'):
                with tarfile.open(archive) as tar:
                    tar.extractall(destination / item['directory'], filter='data')
                continue
            if item.get('member'):
                with tarfile.open(archive) as tar:
                    member = tar.getmember(item['member'])
                    if not member.isfile():
                        raise ValueError('Expected regular executable: ' + member.name)
                    with tar.extractfile(member) as source, output.open('wb') as target:
                        while chunk := source.read(1024 * 1024):
                            target.write(chunk)
            else:
                archive.replace(output)
            os.chmod(output, 0o755)


if __name__ == '__main__':
    main()
