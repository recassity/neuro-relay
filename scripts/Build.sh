#!/bin/bash

# NeuroRelay Build Script

set -e

echo "======================================"
echo "  Building NeuroRelay v1.0.0"
echo "======================================"
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
go build -o ../neurorelay -ldflags="-s -w" entrypoint.go
cd ..
echo "✅ NeuroRelay built successfully"
echo ""

# Build example game
echo "Building example game..."
cd examples
go build -o ../example_game -ldflags="-s -w" example_game.go
cd ..
echo "✅ Example game built successfully"
echo ""

echo "======================================"
echo "  Build Complete!"
echo "======================================"
echo ""
echo "Executables created:"
echo "  - ./neurorelay       (Main relay server)"
echo "  - ./example_game     (Example integration)"
echo ""
echo "To start NeuroRelay:"
echo "  ./neurorelay -name \"Game Hub\" -neuro-url \"ws://localhost:8000\" -emulated-addr \"127.0.0.1:8001\""
echo ""
echo "To run the example game:"
echo "  ./example_game"
echo ""
echo "For more information, see README.md and QUICKSTART.md"
echo ""
