#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
SRC_DIR="$ROOT_DIR/src"
BUILD_DIR="$ROOT_DIR/build"
CLASSES_DIR="$BUILD_DIR/classes"
JAR_PATH="$ROOT_DIR/signing-pad-helper.jar"

rm -rf "$BUILD_DIR"
mkdir -p "$CLASSES_DIR"

javac -d "$CLASSES_DIR" $(find "$SRC_DIR" -name '*.java')
jar --create --file "$JAR_PATH" -C "$CLASSES_DIR" .

echo "Built $JAR_PATH"
