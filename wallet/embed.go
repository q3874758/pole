package wallet

import "embed"

// WebFS 钱包前端静态文件（嵌入到二进制，任意目录启动都能打开钱包）
//go:embed web/*
var WebFS embed.FS
