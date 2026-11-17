#!/bin/sh
# Writes the AUR package tuimessenger-bin (PKGBUILD + .SRCINFO) for a release into DIR, e.g.
#   scripts/aur-pkgbuild.sh v0.1.3 dist/checksums.txt aur/
# The files are what gets pushed to ssh://aur@aur.archlinux.org/tuimessenger-bin.git.
set -eu

tag=$1
sums=$2
out=$3
version=${tag#v}
base="https://github.com/whyyor/tuimessenger/releases/download/$tag"

# sha prints the checksum of one release archive, failing if it's missing.
sha() {
	s=$(awk -v f="tuimessenger_${tag}_$1.tar.gz" '$2 == f { print $1 }' "$sums")
	if [ -z "$s" ]; then
		echo "no checksum for tuimessenger_${tag}_$1.tar.gz in $sums" >&2
		exit 1
	fi
	echo "$s"
}

amd64=$(sha linux_amd64)
arm64=$(sha linux_arm64)
desc="WhatsApp, Telegram, Slack and Instagram in one terminal app"
mkdir -p "$out"

cat >"$out/PKGBUILD" <<EOF
# Maintainer: whyyor <64630224+whyyor@users.noreply.github.com>
# Written by scripts/aur-pkgbuild.sh in whyyor/tuimessenger for every release; changes made here are overwritten.
pkgname=tuimessenger-bin
pkgver=$version
pkgrel=1
pkgdesc="$desc"
arch=('x86_64' 'aarch64')
url="https://github.com/whyyor/tuimessenger"
license=('AGPL-3.0-or-later')
depends=('glibc')
optdepends=('ffmpeg: images, stickers, GIFs and voice notes'
            'mpv: play videos and voice notes in the terminal'
            'yt-dlp: play video links with mpv'
            'libnotify: desktop notifications (notify-send)'
            'zenity: graphical file picker for attachments'
            'yazi: terminal file picker for attachments'
            'xdg-utils: open files and links')
provides=('tuimessenger')
conflicts=('tuimessenger')
options=('!strip' '!debug')
source_x86_64=("$base/tuimessenger_${tag}_linux_amd64.tar.gz")
source_aarch64=("$base/tuimessenger_${tag}_linux_arm64.tar.gz")
sha256sums_x86_64=('$amd64')
sha256sums_aarch64=('$arm64')

package() {
	local dir=tuimessenger_${tag}_linux_amd64
	[ "\$CARCH" = aarch64 ] && dir=tuimessenger_${tag}_linux_arm64
	cd "\$dir"
	install -Dm755 tuimessenger "\$pkgdir/usr/bin/tuimessenger"
	install -Dm644 LICENSE "\$pkgdir/usr/share/licenses/\$pkgname/LICENSE"
	install -Dm644 THIRD_PARTY_NOTICES.md "\$pkgdir/usr/share/licenses/\$pkgname/THIRD_PARTY_NOTICES.md"
	install -Dm644 README.md "\$pkgdir/usr/share/doc/\$pkgname/README.md"
}
EOF

# .SRCINFO is what makepkg --printsrcinfo would print; written here so no Arch machine is needed.
cat >"$out/.SRCINFO" <<EOF
pkgbase = tuimessenger-bin
	pkgdesc = $desc
	pkgver = $version
	pkgrel = 1
	url = https://github.com/whyyor/tuimessenger
	arch = x86_64
	arch = aarch64
	license = AGPL-3.0-or-later
	depends = glibc
	optdepends = ffmpeg: images, stickers, GIFs and voice notes
	optdepends = mpv: play videos and voice notes in the terminal
	optdepends = yt-dlp: play video links with mpv
	optdepends = libnotify: desktop notifications (notify-send)
	optdepends = zenity: graphical file picker for attachments
	optdepends = yazi: terminal file picker for attachments
	optdepends = xdg-utils: open files and links
	provides = tuimessenger
	conflicts = tuimessenger
	options = !strip
	options = !debug
	source_x86_64 = $base/tuimessenger_${tag}_linux_amd64.tar.gz
	sha256sums_x86_64 = $amd64
	source_aarch64 = $base/tuimessenger_${tag}_linux_arm64.tar.gz
	sha256sums_aarch64 = $arm64

pkgname = tuimessenger-bin
EOF
