package main

import "embed"

// frontendDist は React + kumo のビルド成果物 (portal/web/dist) を埋め込む。
// ビルド手順: cd portal/web && npm run build
//
//go:embed dist
var frontendDist embed.FS
