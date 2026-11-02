#!/bin/bash
set -e

echo "🖼️  Building Mapartscript GUI for Linux"
echo "======================================"
echo ""

# Check Go installation
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed!"
    echo "   Install Go 1.26+ from: https://go.dev/dl/"
    exit 1
fi

echo "✅ Go found: $(go version)"
echo ""

# Install Fyne dependencies for Linux
echo "📦 Installing Fyne dependencies for Linux..."
if command -v apt-get &> /dev/null; then
    echo "   Detected: Debian/Ubuntu"
    echo "   Required packages: libgl1-mesa-dev xorg-dev"
    echo ""
    echo "   Please run:"
    echo "   sudo apt-get install -y libgl1-mesa-dev xorg-dev"
elif command -v dnf &> /dev/null; then
    echo "   Detected: Fedora/RHEL"
    echo "   Required packages: libX11-devel libXcursor-devel libXrandr-devel libXinerama-devel mesa-libGL-devel libXi-devel libXxf86vm-devel"
    echo ""
    echo "   Please run:"
    echo "   sudo dnf install -y libX11-devel libXcursor-devel libXrandr-devel libXinerama-devel mesa-libGL-devel libXi-devel libXxf86vm-devel"
elif command -v pacman &> /dev/null; then
    echo "   Detected: Arch Linux"
    echo "   Required packages: xorg-server-devel mesa"
    echo ""
    echo "   Please run:"
    echo "   sudo pacman -S xorg-server-devel mesa"
fi

echo ""
echo "📥 Downloading pinned Go dependencies..."
go mod download

echo ""
echo "🔨 Building GUI binary..."
go build -tags gui -o mapart-gui .

echo ""
echo "✅ Build complete!"
echo ""
echo "🚀 To run the GUI:"
echo "   ./mapart-gui"
echo ""
echo "   Or double-click mapart-gui in your file manager"
echo ""
