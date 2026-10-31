# Maintainer: liking008 <wang392719101@163.com>
pkgname=gi-launcher-tui
pkgver=0.1.7
pkgrel=1
pkgdesc="原神 / Genshin Impact 终端启动器（TUI：多服务器、sophon 下载、校验、账号切换、dwproton 启动）"
arch=('x86_64')
url="https://github.com/liking008/gi-launcher-tui"
license=('MIT')
depends=()
makedepends=('go')
optdepends=('dwproton-bin: 启动游戏所需（其他发行版可从 https://dawn.wine/dawn-winery/dwproton/ 下载）')
provides=('gi-launcher-tui')
conflicts=()
source=("$pkgname-$pkgver.tar.gz::$url/archive/refs/tags/v$pkgver.tar.gz")
sha256sums=('SKIP')

build() {
  cd "$pkgname-$pkgver"
  export CGO_ENABLED=0
  go build -trimpath -ldflags="-s -w" -o "$pkgname" .
}

package() {
  cd "$pkgname-$pkgver"
  install -Dm755 "$pkgname" "$pkgdir/usr/bin/$pkgname"
  install -Dm644 "$pkgname.desktop" "$pkgdir/usr/share/applications/$pkgname.desktop"
  install -Dm644 "$pkgname.svg" "$pkgdir/usr/share/icons/hicolor/scalable/apps/$pkgname.svg"
  install -Dm644 LICENSE "$pkgdir/usr/share/licenses/$pkgname/LICENSE"
}
