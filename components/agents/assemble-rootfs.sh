#!/usr/bin/env bash
set -euo pipefail
variant=$1
arch=$2
# Download separately so vendor tools and RPMs never enter a runtime layer as archives.
python3 /inputs/install-tools.py rpms "/inputs/locks/${variant}-${arch}.json" /rpms
# No enabled repository: the complete dependency closure comes from the lock.
# Keep RPM signature verification as well as SHA-256 verification.
dnf --installroot=/rootfs --releasever=20251124 --disablerepo='*' \
    --setopt=install_weak_deps=False --setopt=localpkg_gpgcheck=True \
    --setopt=keepcache=False -y install --allowerasing /rpms/*.rpm
rm -rf /rootfs/var/cache/* /rootfs/var/log/* /rpms
# No system package manager in either final image. RPM inventory remains for scanners.
install -d -m 0755 /rootfs/usr/local/bin /rootfs/usr/share/hypershell/agent-runtime
cp /inputs/locks/*.json /rootfs/usr/share/hypershell/agent-runtime/
# OpenShift arbitrary UIDs use group 0 for writable application directories.
install -d -m 0770 /rootfs/home/agent /rootfs/sandbox
chown 10001:0 /rootfs/home/agent /rootfs/sandbox
printf "agent:x:10001:0:HyperShell agent:/home/agent:/bin/bash\n" >> /rootfs/etc/passwd
# Remove setuid/setgid modes inherited from OS packages.
find /rootfs -xdev -type f -perm /6000 -exec chmod a-s {} +
