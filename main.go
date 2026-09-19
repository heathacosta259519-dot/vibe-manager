package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"vibe-manager/internal/config"
)

//go:embed all:frontend/dist
var assets embed.FS

// Version 是运行时版本号，界面把它显示在左上角 logo 下方，也是版本的唯一真源。
// 发布构建可用 -ldflags "-X main.Version=X.Y.Z" 覆盖；日常改版本就改这里。
var Version = "0.5.0"

func main() {
	app := NewApp()
	cfg := app.GetConfig()

	width, height := cfg.WindowWidth, cfg.WindowHeight
	if width <= 0 {
		width = config.DefaultWindowWidth
	}
	if height <= 0 {
		height = config.DefaultWindowHeight
	}

	err := wails.Run(&options.App{
		Title:     "Vibe-Manager",
		Width:     width,
		Height:    height,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 22, G: 24, B: 30, A: 1},
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		Bind: []interface{}{
			app,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "vibe-manager-single-instance",
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
