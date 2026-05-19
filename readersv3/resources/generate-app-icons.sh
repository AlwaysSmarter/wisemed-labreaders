#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
SOURCE_SVG="$ROOT_DIR/WiseMED LION.svg"
OUT_DIR="$ROOT_DIR/app-icons"
PNG_DIR="$OUT_DIR/png"
ICO_DIR="$OUT_DIR/ico"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT
export XDG_CACHE_HOME="$TMP_DIR/cache"
mkdir -p "$XDG_CACHE_HOME/fontconfig"

if ! command -v magick >/dev/null 2>&1; then
  echo "magick is required to generate app icons" >&2
  exit 1
fi

mkdir -p "$PNG_DIR" "$ICO_DIR"

BASE_PNG="$TMP_DIR/wisemed-lion-base.png"
magick "$SOURCE_SVG" \
  -background none \
  -density 384 \
  -resize 820x820 \
  -trim +repage \
  "PNG32:$BASE_PNG"

generate_icon() {
  local app="$1"
  local primary="$2"
  local secondary="$3"
  local accent="$4"
  local label="$5"
  local subtitle="$6"
  local symbol="$7"

  local canvas="$TMP_DIR/${app}-canvas.png"
  local badge="$TMP_DIR/${app}-badge.png"
  local preview="$PNG_DIR/${app}.png"
  local icon="$ICO_DIR/${app}.ico"

  magick -size 512x512 xc:none \
    -fill "#FFFFFF" \
    -draw "roundrectangle 16,16 496,496 116,116" \
    "PNG32:$canvas"

  magick "$BASE_PNG" \
    -resize 332x332 \
    "PNG32:$TMP_DIR/${app}-lion.png"

  magick "$canvas" "$TMP_DIR/${app}-lion.png" -gravity center -geometry +0-56 -compose over -composite "$canvas"

  magick -size 280x92 xc:none \
    -fill "${primary}" \
    -draw "roundrectangle 4,4 276,88 30,30" \
    -fill "${secondary}" \
    -draw "roundrectangle 14,14 90,78 22,22" \
    -fill white \
    -font Helvetica-Bold \
    -pointsize 36 \
    -gravity northwest \
    -annotate +30+50 "$symbol" \
    -fill white \
    -font Helvetica-Bold \
    -pointsize 28 \
    -gravity northwest \
    -annotate +108+36 "$label" \
    -fill "#F3F7FA" \
    -font Helvetica \
    -pointsize 16 \
    -gravity northwest \
    -annotate +108+64 "$subtitle" \
    "PNG32:$badge"

  magick "$canvas" "$badge" -gravity south -geometry +0+36 -compose over -composite \
    "PNG32:$preview"

  magick "$preview" \
    \( +clone -resize 256x256 \) \
    \( +clone -resize 128x128 \) \
    \( +clone -resize 64x64 \) \
    \( +clone -resize 48x48 \) \
    \( +clone -resize 32x32 \) \
    \( +clone -resize 16x16 \) \
    "$icon"
}

generate_icon "barcodeprinter"      "#0D3B66" "#F59E0B" "#184E77" "BARCODE" "Label print utility" "BC"
generate_icon "beosl-reader"        "#0F4C5C" "#14B8A6" "#1D7874" "BEOSL"   "CSV file reader"      "BE"
generate_icon "cary60-uvvis-reader" "#352F6B" "#8B5CF6" "#4C1D95" "CARY60"  "UV-VIS workflow"      "UV"
generate_icon "gemini-reader"       "#113A5D" "#06B6D4" "#0B7285" "GEMINI"  "ASTM TCP/IP reader"   "GM"
generate_icon "generic-test-reader" "#374151" "#22C55E" "#1F2937" "GENERIC" "Flexible test reader"  "GT"
generate_icon "seegene-reader"      "#5B1F28" "#EF4444" "#7F1D1D" "SEEGENE" "Excel import reader"   "SG"
generate_icon "update-server"       "#0E4F45" "#10B981" "#065F46" "UPDATES" "Release distribution"  "UP"

echo "Generated PNG previews in: $PNG_DIR"
echo "Generated ICO files in: $ICO_DIR"
