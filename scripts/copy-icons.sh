#!/bin/bash
# Copy icons from icons directory to static directory
# Run this after updating icons

set -e

cd "$(dirname "$0")/.."

# Ensure icons directory exists
if [ ! -d "icons" ]; then
    echo "Error: icons directory not found"
    exit 1
fi

# Create target directory if needed
mkdir -p web/static

# Copy icons (SVG-based)
cp icons/cloistr-relay.svg web/static/favicon.svg
cp icons/favicon/cloistr-relay-16.svg web/static/favicon-16.svg
cp icons/cloistr-base.svg web/static/icon.svg

echo "Icons copied to web/static/"
