// Package web 将前端构建产物嵌入二进制，由后端同源提供（免跨域、单端口部署）。
package web

import (
	"embed"
	"io/fs"
)

// dist 由前端构建写入（`npm run build` 输出到 internal/web/dist）。
// 未构建时保留一份占位 index.html，保证后端可独立编译。
//
//go:embed all:dist
var dist embed.FS

// FS 返回以 dist 为根的文件系统。
func FS() (fs.FS, error) { return fs.Sub(dist, "dist") }
