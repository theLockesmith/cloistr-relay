#!/bin/bash
# Copy icons from assets submodule to static directory
# Run this after cloning or when icons are updated

set -e

cd "$(dirname "$0")/.."

# Ensure submodule is initialized
if [ ! -d "assets/generated" ]; then
    echo "Initializing submodules..."
    git submodule update --init --recursive
fi

# Create target directory if needed
mkdir -p web/static

# Copy generated icons
cp assets/generated/relay/favicon.ico web/static/
cp assets/generated/relay/favicon.svg web/static/
cp assets/generated/relay/apple-touch-icon.png web/static/
cp assets/generated/relay/icon-192.png web/static/
cp assets/generated/relay/icon-512.png web/static/

echo "Icons copied to web/static/"
