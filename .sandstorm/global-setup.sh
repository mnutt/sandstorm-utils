#!/bin/bash
set -euo pipefail

# CAUTION: DO NOT MAKE CHANGES TO THIS FILE. The vagrant-spk upgradevm process will overwrite it.
# App-specific setup should be done in the setup.sh file.

CURL_OPTS="--silent --show-error"
echo localhost > /etc/hostname
hostname localhost

apt-mark hold grub-pc
apt-get update
apt-get upgrade -y

apt-get install -y curl gnupg

curl $CURL_OPTS https://install.sandstorm.io/ 2>&1 > /host-dot-sandstorm/caches/install.sh | cat

SANDSTORM_CURRENT_VERSION=$(curl $CURL_OPTS -f "https://install.sandstorm.io/dev?from=0&type=install")
SANDSTORM_PACKAGE="sandstorm-$SANDSTORM_CURRENT_VERSION.tar.xz"
if [[ ! -f /host-dot-sandstorm/caches/$SANDSTORM_PACKAGE ]] ; then
    echo -n "Downloading Sandstorm version ${SANDSTORM_CURRENT_VERSION}..."
    curl $CURL_OPTS --output "/host-dot-sandstorm/caches/$SANDSTORM_PACKAGE.partial" "https://dl.sandstorm.io/$SANDSTORM_PACKAGE" 2>&1 | cat
    mv "/host-dot-sandstorm/caches/$SANDSTORM_PACKAGE.partial" "/host-dot-sandstorm/caches/$SANDSTORM_PACKAGE"
    echo "...done."
fi
if [ ! -e /opt/sandstorm/latest/sandstorm ] ; then
    echo -n "Installing Sandstorm version ${SANDSTORM_CURRENT_VERSION}..."
    bash /host-dot-sandstorm/caches/install.sh -d -e -p 6090 "/host-dot-sandstorm/caches/$SANDSTORM_PACKAGE" >/dev/null
    echo "...done."
fi
modprobe ip_tables
usermod -a -G 'sandstorm' 'vagrant'
sudo sed --in-place='' \
        --expression='s/^BIND_IP=.*/BIND_IP=0.0.0.0/' \
        /opt/sandstorm/sandstorm.conf

echo 'ALLOW_LEGACY_RELAXED_CSP=false' >> /opt/sandstorm/sandstorm.conf

sudo service sandstorm restart
GATEWAY_IP=$(ip route  | grep ^default  | cut -d ' ' -f 3)
if nc -z "$GATEWAY_IP" 3142 ; then
    echo "Acquire::http::Proxy \"http://$GATEWAY_IP:3142\";" > /etc/apt/apt.conf.d/80httpproxy
fi
echo "APT::Acquire::Retries \"10\";" > /etc/apt/apt.conf.d/80sandstorm-retry
