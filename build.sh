#!/bin/bash

# Build script for KnowThis application

set -e

echo "Building KnowThis application..."

# Clean up previous builds
rm -f knowthis

# Download dependencies
go mod tidy

# Build the application
go build -o knowthis main.go

echo "Build completed successfully!"
echo "Run with: ./knowthis"