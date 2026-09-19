package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"vibe-manager/internal/model"
)

const (
	ThemeLight = "light"
	ThemeDark  = "dark"
)

// Default 是首次运行的配置。项目根与归档根刻意留空：
// 每个人的目录布局都不一样，猜一个默认值只会让人先踩一次坑，
// 所以交给用户在设置里指定（列表空着时会给出引导）。
func Default() model.Config {
	return model.Config{
		ProjectRoot:     "",
		ArchiveRoot:     "",
		EditorCmd:       "code",
		TerminalCmd:     "wt",
		Theme:           ThemeLight,
		FileViewMode:    ViewIcons,
		FileSortKey:     SortName,
		FileShowHidden:  false,
		FilePreviewOpen: true,
		EditorWidth:     DefaultEditorWidth,
		RailWidth:       DefaultRailWidth,
	}
}

const (
	DefaultEditorWidth = 580
	MinEditorWidth     = 260
	MaxEditorWidth     = 1000
)

const (
	DefaultRailWidth = 240
	MinRailWidth     = 200
	MaxRailWidth     = 420
)

// ClampRailWidth 把左侧栏宽度收进合理区间，0 表示用默认值。
func ClampRailWidth(w int) int {
	if w <= 0 {
		return DefaultRailWidth
	}
	if w < MinRailWidth {
		return MinRailWidth
	}
	if w > MaxRailWidth {
		return MaxRailWidth
	}
	return w
}

// ClampEditorWidth 把编辑器宽度收进合理区间，0 表示用默认值。
func ClampEditorWidth(w int) int {
	if w <= 0 {
		return DefaultEditorWidth
	}
	if w < MinEditorWidth {
		return MinEditorWidth
	}
	if w > MaxEditorWidth {
		return MaxEditorWidth
	}
	return w
}

// SanitizeFont 清掉可能破坏 CSS 的字符，字体名会被直接塞进 style。
func SanitizeFont(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ';', '{', '}', '(', ')', '"', '\'', '<', '>', '\\', '!', '@', '#', '$', '%', '&', '*', '/', '|', '`', '~', '^', '=':
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if len(out) > 120 {
		out = out[:120]
	}
	return out
}

const (
	ViewIcons   = "icons"
	ViewDetails = "details"

	SortName = "name"
	SortTime = "time"
	SortSize = "size"
	SortType = "type"
)

const (
	DefaultWindowWidth  = 1180
	DefaultWindowHeight = 760
)

// FilePath 是配置文件的位置。
// 目录名仍用 vibe-pm：项目改叫 Vibe-Manager 之后若跟着改目录，
// 用户已有的配置 / 备注名 / 导入记录就全留在旧目录里读不到了。
func FilePath() string {
	base := os.Getenv("APPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	return filepath.Join(base, "vibe-pm", "config.json")
}

// LogPath 是运行日志文件的位置，GUI 程序没有控制台，出错时靠它留痕。
func LogPath() string {
	return filepath.Join(filepath.Dir(FilePath()), "vibe-pm.log")
}

func Load() model.Config {
	cfg := Default()
	data, err := os.ReadFile(FilePath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	def := Default()
	if cfg.ProjectRoot == "" {
		cfg.ProjectRoot = def.ProjectRoot
	}
	if cfg.ArchiveRoot == "" {
		cfg.ArchiveRoot = def.ArchiveRoot
	}
	if cfg.EditorCmd == "" {
		cfg.EditorCmd = def.EditorCmd
	}
	if cfg.TerminalCmd == "" {
		cfg.TerminalCmd = def.TerminalCmd
	}
	if cfg.Theme != ThemeDark {
		cfg.Theme = ThemeLight
	}
	if cfg.FileViewMode != ViewDetails {
		cfg.FileViewMode = ViewIcons
	}
	switch cfg.FileSortKey {
	case SortTime, SortSize, SortType:
	default:
		cfg.FileSortKey = SortName
	}
	cfg.EditorWidth = ClampEditorWidth(cfg.EditorWidth)
	cfg.RailWidth = ClampRailWidth(cfg.RailWidth)
	cfg.EditorFont = SanitizeFont(cfg.EditorFont)
	cfg.Author = strings.TrimSpace(cfg.Author)
	return cfg
}

func Save(cfg model.Config) error {
	p := FilePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
