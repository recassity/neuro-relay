#!/bin/bash

# NeuroRelay Build Script

set -e

DIST_DIR="dist"

echo "======================================"
echo "  Building NeuroRelay v1.0.0"
echo "======================================"
echo ""

# Ensure dist directory exists
echo "Preparing dist directory..."
mkdir -p "$DIST_DIR"
echo "✅ dist/ directory ready"
echo ""

# Check Go version
echo "Checking Go installation..."
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed"
    echo "Please install Go 1.21 or higher from https://go.dev/dl/"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "✅ Found Go $GO_VERSION"
echo ""

# Install dependencies
echo "Installing dependencies..."
go get github.com/gorilla/websocket
go get github.com/cassitly/neuro-integration-sdk
echo "✅ Dependencies installed"
echo ""

# Build NeuroRelay
echo "Building NeuroRelay..."
cd src
go build -o "../$DIST_DIR/neurorelay" -ldflags="-s -w" entrypoint.go
cd ..
echo "✅ NeuroRelay built successfully"
echo ""

# Build example game
echo "Building example game..."
cd examples
go build -o "../$DIST_DIR/example_game" -ldflags="-s -w" example_game.go
echo "✅ Example game built successfully"
echo ""

# Switch to dist directory
cd "$DIST_DIR"

echo "======================================"
echo "  Build Complete!"
echo "======================================"
echo ""
echo "Executables created in ./dist:"
echo "  - neurorelay         (Main relay server)"
echo "  - example_game       (Basic example integration)"
echo ""
echo "You are now in the dist/ directory."
echo ""
echo "To start NeuroRelay:"
echo "  ./neurorelay -name \"Game Hub\" -neuro-url \"ws://localhost:8000\" -emulated-addr \"127.0.0.1:8001\""
echo ""
echo "For more information, see:"
echo "  - README.md (Overview)"
echo "  - QUICKSTART.md (Getting started)"
echo "  - docs/NRC-Endpoints.md (NRC endpoints documentation)"
echo "  - docs/Migration-Guide.md (Upgrading guide)"
echo ""
