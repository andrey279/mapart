#!/bin/bash
# Build script for cross-platform binaries

set -e

VERSION=${1:-"dev"}
BUILD_DIR="build"
LDFLAGS="-s -w"

echo "🔨 Building mapartscript v${VERSION}..."

# Create build directory
mkdir -p ${BUILD_DIR}

# Build for different platforms
build_binary() {
    local GOOS=$1
    local GOARCH=$2
    local OUTPUT=$3

    echo "→ Building ${GOOS}/${GOARCH}..."

    GOOS=${GOOS} GOARCH=${GOARCH} go build -ldflags="${LDFLAGS}" -o ${BUILD_DIR}/${OUTPUT} .

    if [ $? -eq 0 ]; then
        echo "  ✅ ${OUTPUT}"
    else
        echo "  ❌ Failed to build ${OUTPUT}"
        exit 1
    fi
}

# Linux
build_binary linux amd64 mapart-linux-amd64
build_binary linux arm64 mapart-linux-arm64

# Windows
build_binary windows amd64 mapart-windows-amd64.exe

# macOS
build_binary darwin amd64 mapart-darwin-amd64
build_binary darwin arm64 mapart-darwin-arm64

echo ""
echo "✅ All binaries built successfully!"
echo ""
echo "Output directory: ${BUILD_DIR}/"
ls -lh ${BUILD_DIR}/
echo ""

# Create archives
echo "📦 Creating archives..."

cd ${BUILD_DIR}

# Linux AMD64
tar czf mapart-linux-amd64-${VERSION}.tar.gz mapart-linux-amd64
echo "  ✅ mapart-linux-amd64-${VERSION}.tar.gz"

# Linux ARM64
tar czf mapart-linux-arm64-${VERSION}.tar.gz mapart-linux-arm64
echo "  ✅ mapart-linux-arm64-${VERSION}.tar.gz"

# Windows AMD64
if command -v zip &> /dev/null; then
    zip -q mapart-windows-amd64-${VERSION}.zip mapart-windows-amd64.exe
    echo "  ✅ mapart-windows-amd64-${VERSION}.zip"
else
    echo "  ⚠️  zip not found, skipping Windows archive"
fi

# macOS AMD64
tar czf mapart-darwin-amd64-${VERSION}.tar.gz mapart-darwin-amd64
echo "  ✅ mapart-darwin-amd64-${VERSION}.tar.gz"

# macOS ARM64
tar czf mapart-darwin-arm64-${VERSION}.tar.gz mapart-darwin-arm64
echo "  ✅ mapart-darwin-arm64-${VERSION}.tar.gz"

cd ..

echo ""
echo "✅ Build complete!"
echo ""
