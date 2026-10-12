#!/usr/bin/env python3
"""Explicit maintainer operation: resolve RPM inputs and write reviewed hash locks.

Requires podman and network access. Does not run as part of image builds.
Set SSL_CERT_FILE to a trusted CA bundle if your network requires one.
"""
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parent
BUILDER = ('registry.access.redhat.com/hi/python@sha256:'
           '0da78c701c368decae8777dee35387d07456c639278a4ea9f9dbfa56dc675ee7')
COMMON = ['bash', 'coreutils', 'git-core', 'curl', 'jq', 'tar', 'gzip', 'unzip',
          'findutils', 'diffutils', 'patch', 'which', 'ca-certificates', 'python3', 'libstdc++']
FULL = ['rust', 'cargo', 'clippy', 'rustfmt', 'rust-src', 'gcc', 'gcc-c++', 'make',
        'pkgconf-pkg-config', 'python3-devel', 'openssl-devel', 'nodejs24', 'nodejs24-npm']
NS = {'r': 'http://linux.duke.edu/metadata/repo', 'c': 'http://linux.duke.edu/metadata/common'}


def read(url):
    with urllib.request.urlopen(url, timeout=120) as response:
        return response.read()


def main():
    for arch, target in [('x86_64', 'amd64'), ('aarch64', 'arm64')]:
        resolved = {}
        for variant in ['slim', 'full']:
            args = ['podman', 'run', '--rm', '--user', '0']
            ca_args = []
            with tempfile.TemporaryDirectory() as directory:
                if ca := os.environ.get('SSL_CERT_FILE'):
                    bundle = Path(directory) / 'ca.pem'
                    bundle.write_bytes(Path(ca).read_bytes())
                    args += ['-v', f'{bundle}:/locks/ca.pem:ro,Z']
                    ca_args = ['--setopt=sslcacert=/locks/ca.pem']
                args += ['--entrypoint', '/usr/sbin/dnf', BUILDER, f'--forcearch={arch}',
                         *ca_args, '--setopt=install_weak_deps=False', 'download',
                         '--resolve', '--alldeps', '--url', *COMMON]
                if variant == 'full':
                    args += FULL
                urls = subprocess.check_output(args, text=True).splitlines()
            if not urls or any(not u.startswith('https://packages.redhat.com/') for u in urls):
                raise ValueError('Expected Red Hat RPM URLs from dnf')
            resolved[variant] = urls
        base = f'https://packages.redhat.com/api/pulp-content/public-hummingbird/{arch}/'
        metadata = ET.fromstring(read(base + 'repodata/repomd.xml'))
        primary = metadata.find("r:data[@type='primary']", NS)
        checksum = primary.find('r:checksum', NS)
        raw = read(urllib.parse.urljoin(base, primary.find('r:location', NS).get('href')))
        if checksum.get('type') != 'sha256' or hashlib.sha256(raw).hexdigest() != checksum.text:
            raise ValueError('Repository metadata checksum mismatch')
        wanted = set(sum(resolved.values(), []))
        found = {}
        for _, element in ET.iterparse(io.BytesIO(gzip.decompress(raw)), events=('end',)):
            if element.tag != '{http://linux.duke.edu/metadata/common}package':
                continue
            url = urllib.parse.urljoin(base, element.find('c:location', NS).get('href'))
            if url in wanted:
                checksum = element.find('c:checksum', NS)
                if checksum.get('type') != 'sha256':
                    raise ValueError('Expected SHA-256 RPM checksum')
                found[url] = {'url': url, 'sha256': checksum.text}
            element.clear()
        if not wanted <= found.keys():
            raise ValueError('Metadata changed during resolution; retry regeneration')
        for variant, urls in resolved.items():
            path = ROOT / 'locks' / f'{variant}-{target}.json'
            path.write_text(json.dumps([found[u] for u in sorted(urls)], indent=2) + '\n')
            print(f'{path.name}: {len(urls)} pinned packages')


if __name__ == '__main__':
    main()
