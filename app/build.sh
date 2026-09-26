#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

rm -rf dist
mkdir -p dist

apps=(status spec plan state drift policy git-sync init)

for app in "${apps[@]}"; do
  echo "Building $app..."
  APP_ENTRY="$app" npx vite build --logLevel warn
done

# Flatten: vite outputs to dist/src/apps/*.html, move to dist/*.html
if [ -d dist/src/apps ]; then
  mv dist/src/apps/*.html dist/
  rm -rf dist/src
fi

# Copy to Go embed location
embed_dir="../internal/infrastructure/mcp/dist"
rm -rf "$embed_dir"
mkdir -p "$embed_dir"
cp dist/*.html "$embed_dir/"

echo "Built ${#apps[@]} apps to dist/ and copied to $embed_dir"
