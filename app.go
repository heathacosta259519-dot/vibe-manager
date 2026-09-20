package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"vibe-manager/internal/agents"
	"vibe-manager/internal/archive"
	"vibe-manager/internal/config"
	"vibe-manager/internal/execx"
	"vibe-manager/internal/fsx"
	"vibe-manager/internal/gitx"
	"vibe-manager/internal/model"
	"vibe-manager/internal/project"
	"vibe-manager/internal/taskdoc"
	"vibe-manager/internal/update"
)

type App struct {
	ctx context.Context
	cfg model.Config
}

func NewApp() *App {
	return &App{cfg: config.Load()}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.applyTheme()
	// 上次更新换下来的旧程序与残留下载，这时候已经没人占用，顺手清掉
	if exe, err := os.Executable(); err == nil {
		update.CleanupStale(exe)
	}
	if strings.TrimSpace(a.cfg.Author) == "" {
		if name := gitx.GlobalUserName(); name != "" {
			a.cfg.Author = name
			_ = config.Save(a.cfg)
		}
	}
}

// beforeClose 在窗口销毁前记住尺寸，下次启动时恢复。
func (a *App) beforeClose(ctx context.Context) bool {
	if w, h := runtime.WindowGetSize(ctx); w > 0 && h > 0 {
		a.cfg.WindowWidth, a.cfg.WindowHeight = w, h
		_ = config.Save(a.cfg)
	}
	return false
}

func (a *App) applyTheme() {
	if a.ctx == nil {
		return
	}
	if a.cfg.Theme == config.ThemeDark {
		runtime.WindowSetDarkTheme(a.ctx)
		runtime.WindowSetBackgroundColour(a.ctx, 22, 24, 30, 255)
		return
	}
	runtime.WindowSetLightTheme(a.ctx)
	runtime.WindowSetBackgroundColour(a.ctx, 244, 245, 247, 255)
}

func (a *App) GetConfig() model.Config {
	return a.cfg
}

func (a *App) SetTheme(theme string) (model.Config, error) {
	if theme != config.ThemeDark {
		theme = config.ThemeLight
	}
	a.cfg.Theme = theme
	if err := config.Save(a.cfg); err != nil {
		return a.cfg, err
	}
	a.applyTheme()
	return a.cfg, nil
}

func (a *App) SaveConfig(cfg model.Config) (model.Config, error) {
	if cfg.ProjectRoot == "" {
		return a.cfg, fmt.Errorf("项目根不能为空")
	}
	if cfg.ArchiveRoot == "" {
		return a.cfg, fmt.Errorf("归档根不能为空")
	}
	if cfg.Theme != config.ThemeDark {
		cfg.Theme = config.ThemeLight
	}
	if cfg.FileViewMode != config.ViewDetails {
		cfg.FileViewMode = config.ViewIcons
	}
	switch cfg.FileSortKey {
	case config.SortTime, config.SortSize, config.SortType:
	default:
		cfg.FileSortKey = config.SortName
	}
	cfg.EditorWidth = config.ClampEditorWidth(cfg.EditorWidth)
	cfg.RailWidth = config.ClampRailWidth(cfg.RailWidth)
	cfg.EditorFontSize = config.ClampEditorFontSize(cfg.EditorFontSize)
	cfg.UiFontSize = config.ClampUiFontSize(cfg.UiFontSize)
	cfg.EditorFont = config.SanitizeFont(cfg.EditorFont)
	cfg.Author = strings.TrimSpace(cfg.Author)
	if err := config.Save(cfg); err != nil {
		return a.cfg, err
	}
	a.cfg = cfg
	a.applyTheme()
	return a.cfg, nil
}

func (a *App) ConfigPath() string {
	return config.FilePath()
}

// AppVersion 返回运行时版本号，供界面显示在左上角 logo 下方。
func (a *App) AppVersion() string {
	return Version
}

/* ---------------- 更新 ---------------- */

// CheckUpdate 查询公开仓库有没有更高版本。没有新版本时 Available 为 false。
func (a *App) CheckUpdate() (model.UpdateInfo, error) {
	info := model.UpdateInfo{Current: Version}
	rel, err := update.Latest(Version)
	if err != nil {
		logError("CheckUpdate", err)
		return info, err
	}
	if rel == nil {
		return info, nil
	}
	info.Available = true
	info.Latest = rel.Version
	info.Notes = rel.Notes
	info.Size = rel.Size
	info.PageURL = rel.PageURL
	return info, nil
}

// ApplyUpdate 下载新版本、替换当前程序，然后退出并自动重启。
// 下载与校验都在替换之前做完：任何一步失败都不会动到正在用的程序。
func (a *App) ApplyUpdate() (string, error) {
	rel, err := update.Latest(Version)
	if err != nil {
		logError("ApplyUpdate/check", err)
		return "", err
	}
	if rel == nil {
		return "已经是最新版本", nil
	}

	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("无法定位当前程序：%v", err)
	}

	downloaded := exe + ".new"
	if err := update.Download(rel, downloaded); err != nil {
		logError("ApplyUpdate/download", err)
		return "", err
	}
	if err := update.Swap(exe, downloaded, exe+".old"); err != nil {
		_ = os.Remove(downloaded)
		logError("ApplyUpdate/swap", err)
		return "", err
	}

	// 程序已经换好了。先排好「本进程退出后再启动」，再退出——
	// 顺序反过来的话新进程会被单实例锁顶掉。
	note := ""
	if err := update.RelaunchDetached(exe, os.Getpid(), 120*time.Second); err != nil {
		logError("ApplyUpdate/relaunch", err)
		note = "；自动重启没排上，请手动打开程序"
	}
	go func() {
		time.Sleep(900 * time.Millisecond) // 留点时间让界面把结果提示出来
		runtime.Quit(a.ctx)
	}()
	return fmt.Sprintf("已更新到 v%s，正在重启…%s", rel.Version, note), nil
}

func (a *App) ChooseFolder(title string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: title})
}

// projectEntries 汇总项目列表：项目根下的直接子目录 + 用户导入的目录，
// 并挂上备注名与实际根目录。
func (a *App) projectEntries() ([]project.Entry, error) {
	// 项目根允许没设（首次运行就是空的）：那就只列导入的项目，不报错。
	// 但填了却读不到（路径写错、盘没挂）仍要报出来，好让人知道是哪儿的问题。
	var scanned []project.Entry
	if strings.TrimSpace(a.cfg.ProjectRoot) != "" {
		s, err := project.Scan(a.cfg.ProjectRoot)
		if err != nil {
			return nil, err
		}
		scanned = s
	}
	store := config.LoadProjectStore()
	out := make([]project.Entry, 0, len(scanned)+len(store.Imports))
	seen := map[string]bool{}

	add := func(entryPath string, imported bool) {
		key := config.NormalizePath(entryPath)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, project.Entry{
			Path:     entryPath,
			Root:     store.Roots[key],
			Alias:    store.Aliases[key],
			Imported: imported,
		})
	}
	for _, e := range scanned {
		add(e.Path, false)
	}
	for _, p := range store.Imports {
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			continue // 导入过但现在已经不在了，先跳过
		}
		add(p, true)
	}
	return out, nil
}

// ListProjects 扫描项目根 + 导入的目录，并合并备注名与自定义根目录。
func (a *App) ListProjects() ([]model.Project, error) {
	entries, err := a.projectEntries()
	if err != nil {
		return nil, err
	}
	return project.DescribeAll(entries), nil
}

// ImportProject 把一个项目根之外的目录加入项目列表。
func (a *App) ImportProject(path string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." {
		return "", fmt.Errorf("请选择一个目录")
	}
	fi, err := os.Stat(clean)
	if err != nil || !fi.IsDir() {
		return "", fmt.Errorf("目录不存在：%s", path)
	}
	if err := config.AddImport(clean); err != nil {
		logError(fmt.Sprintf("ImportProject(%s)", clean), err)
		return "", err
	}
	return fmt.Sprintf("已导入 %s", clean), nil
}

// RemoveImportedProject 把导入的项目移出列表（不动磁盘上的任何文件）。
func (a *App) RemoveImportedProject(entry string) (string, error) {
	if err := config.RemoveImport(entry); err != nil {
		logError(fmt.Sprintf("RemoveImportedProject(%s)", entry), err)
		return "", err
	}
	return "已移出列表（磁盘文件未改动）", nil
}

// SetProjectRoot 设置某个项目的实际根目录。
// 用于「真仓库在子目录里」的情况，例如 winglass\WinGlass。传空字符串即恢复默认。
func (a *App) SetProjectRoot(entry, root string) (string, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		if err := config.SetProjectMeta(entry, a.aliasOf(entry), ""); err != nil {
			logError(fmt.Sprintf("SetProjectRoot(%s)", entry), err)
			return "", err
		}
		return "已恢复默认目录", nil
	}
	clean := filepath.Clean(trimmed)
	fi, err := os.Stat(clean)
	if err != nil || !fi.IsDir() {
		return "", fmt.Errorf("目录不存在：%s", root)
	}
	if config.NormalizePath(clean) == config.NormalizePath(entry) {
		return "", fmt.Errorf("与当前目录相同")
	}
	if err := config.SetProjectMeta(entry, a.aliasOf(entry), clean); err != nil {
		logError(fmt.Sprintf("SetProjectRoot(%s)", entry), err)
		return "", err
	}
	return fmt.Sprintf("实际根目录已设为 %s", clean), nil
}

func (a *App) aliasOf(entry string) string {
	return config.LoadProjectStore().Aliases[config.NormalizePath(entry)]
}

// ensureProject 校验目标目录是本工具管理的项目：
// 要么是项目根的直接子目录，要么是用户显式导入 / 指定过根目录的项目。
func (a *App) ensureProject(path string) error {
	if strings.TrimSpace(path) == "" {
		// 必须先挡掉空路径：filepath.Abs("") 会变成当前目录，
		// 而当前目录有可能正好是某个已注册的项目。
		return fmt.Errorf("项目路径为空")
	}
	rootAbs, err := filepath.Abs(a.cfg.ProjectRoot)
	if err != nil {
		return err
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if isDirectChild(rootAbs, pathAbs) {
		return nil
	}
	if config.IsRegisteredPath(pathAbs) {
		if fi, err := os.Stat(pathAbs); err != nil || !fi.IsDir() {
			return fmt.Errorf("目录不存在：%s", path)
		}
		return nil
	}
	return fmt.Errorf("不在项目根内，也不是已导入的项目：%s", path)
}

func isDirectChild(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." {
		return false
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !strings.ContainsRune(rel, filepath.Separator)
}

// SetAlias 设置或清除某个项目的备注名（空字符串即清除）。
func (a *App) SetAlias(path, alias string) (string, error) {
	if err := a.ensureProject(path); err != nil {
		return "", err
	}
	if err := config.SetAlias(path, alias); err != nil {
		logError(fmt.Sprintf("SetAlias(%s)", path), err)
		return "", err
	}
	if strings.TrimSpace(alias) == "" {
		return "已清除备注名", nil
	}
	return "备注名已设为「" + strings.TrimSpace(alias) + "」", nil
}

// GetLicenseOptions 返回新建项目时可选的协议列表。
func (a *App) GetLicenseOptions() []model.LicenseOption {
	return project.LicenseOptions
}

// GetAgentTemplates 返回可用的 AGENTS.md 模板（内置 + 用户自建）。
func (a *App) GetAgentTemplates() []agents.Template {
	return agents.List()
}

// SaveAgentTemplate 新建或覆盖一份用户模板（id 为空表示新建）。
func (a *App) SaveAgentTemplate(id, name, body string) (agents.Template, error) {
	t, err := agents.SaveUser(id, name, body)
	if err != nil {
		logError(fmt.Sprintf("SaveAgentTemplate(%s)", name), err)
	}
	return t, err
}

// DeleteAgentTemplate 删除一份用户模板。
func (a *App) DeleteAgentTemplate(id string) error {
	err := agents.DeleteUser(id)
	if err != nil {
		logError(fmt.Sprintf("DeleteAgentTemplate(%s)", id), err)
	}
	return err
}

// CreateProject 按勾选项生成项目骨架。
func (a *App) CreateProject(opts model.CreateOptions) (string, error) {
	if strings.TrimSpace(opts.Author) == "" {
		opts.Author = a.cfg.Author
	}
	path, err := project.Create(a.cfg.ProjectRoot, opts)
	if err != nil {
		logError(fmt.Sprintf("CreateProject(%s)", opts.Name), err)
		return "", err
	}
	return fmt.Sprintf("已创建 %s", path), nil
}

// launchHidden 经 cmd 执行目标程序，全程不创建可见的控制台窗口。
// 编辑器命令多为 .cmd 批处理（如 code.cmd），必须由 cmd 执行；
// 若改用 start，start 会为批处理另开一个新控制台窗口，就会出现「点编辑器弹黑窗」。
func launchHidden(exe string, args ...string) error {
	if exe == "" {
		return fmt.Errorf("未配置命令")
	}
	cmd := exec.Command("cmd", append([]string{"/c", exe}, args...)...)
	execx.Hide(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 %s 失败：%v", exe, err)
	}
	return nil
}

// launchWindow 通过 start 开一个新窗口，用于终端这类需要可见控制台的目标。
func launchWindow(exe string, args ...string) error {
	if exe == "" {
		return fmt.Errorf("未配置命令")
	}
	cmd := exec.Command("cmd", append([]string{"/c", "start", "", exe}, args...)...)
	execx.Hide(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 %s 失败：%v", exe, err)
	}
	return nil
}

func (a *App) OpenFolder(path string) error {
	return launchHidden("explorer", path)
}

func (a *App) OpenTerminal(path string) error {
	parts := strings.Fields(a.cfg.TerminalCmd)
	if len(parts) == 0 {
		return fmt.Errorf("未配置终端命令")
	}
	exe, args := parts[0], parts[1:]
	if strings.EqualFold(exe, "wt") {
		return launchHidden(exe, append(args, "-d", path)...)
	}
	return launchWindow(exe, append(args, path)...)
}

func (a *App) OpenEditor(path string) error {
	parts := strings.Fields(a.cfg.EditorCmd)
	if len(parts) == 0 {
		return fmt.Errorf("未配置编辑器命令")
	}
	return launchHidden(parts[0], append(parts[1:], path)...)
}

func (a *App) GitLog(path string) ([]model.Commit, error) {
	if !gitx.IsRepo(path) {
		return []model.Commit{}, fmt.Errorf("这不是一个 git 仓库")
	}
	return gitx.Log(path, 200)
}

func (a *App) GitCommitAll(path, message string) (string, error) {
	sha, changed, err := gitx.CommitAll(path, message)
	if err != nil {
		return "", err
	}
	if !changed {
		return "工作区无改动，无需打点", nil
	}
	return "已打点 " + shortSHA(sha), nil
}

func (a *App) GitRollback(path, sha string, hard bool) (string, error) {
	if !gitx.IsRepo(path) {
		return "", fmt.Errorf("这不是一个 git 仓库")
	}
	return gitx.Rollback(path, sha, hard, gitx.Stamp())
}

func (a *App) GitDiscardChanges(path string) (string, error) {
	if !gitx.IsRepo(path) {
		return "", fmt.Errorf("这不是一个 git 仓库")
	}
	return gitx.DiscardChanges(path, gitx.Stamp())
}

/* ---------------- 分支与标签 ---------------- */

// GitBranches 列出本地分支。
func (a *App) GitBranches(path string) ([]model.Branch, error) {
	if err := a.ensureProject(path); err != nil {
		return nil, err
	}
	list, err := gitx.Branches(path)
	if err != nil {
		logError(fmt.Sprintf("GitBranches(%s)", path), err)
	}
	return list, err
}

func (a *App) GitCheckout(path, branch string) (model.OpResult, error) {
	if err := a.ensureProject(path); err != nil {
		return model.OpResult{}, err
	}
	res, err := gitx.Checkout(path, branch)
	if err != nil {
		logError(fmt.Sprintf("GitCheckout(%s)", branch), err)
	}
	return res, err
}

func (a *App) GitCreateBranch(path, branch string, checkout bool) (model.OpResult, error) {
	if err := a.ensureProject(path); err != nil {
		return model.OpResult{}, err
	}
	res, err := gitx.CreateBranch(path, branch, checkout)
	if err != nil {
		logError(fmt.Sprintf("GitCreateBranch(%s)", branch), err)
	}
	return res, err
}

func (a *App) GitDeleteBranch(path, branch string, force bool) (model.OpResult, error) {
	if err := a.ensureProject(path); err != nil {
		return model.OpResult{}, err
	}
	res, err := gitx.DeleteBranch(path, branch, force)
	if err != nil {
		logError(fmt.Sprintf("GitDeleteBranch(%s)", branch), err)
	}
	return res, err
}

func (a *App) GitMerge(path, branch string) (model.OpResult, error) {
	if err := a.ensureProject(path); err != nil {
		return model.OpResult{}, err
	}
	res, err := gitx.Merge(path, branch)
	if err != nil {
		logError(fmt.Sprintf("GitMerge(%s)", branch), err)
	}
	return res, err
}

func (a *App) GitTags(path string) ([]model.Tag, error) {
	if err := a.ensureProject(path); err != nil {
		return nil, err
	}
	list, err := gitx.Tags(path)
	if err != nil {
		logError(fmt.Sprintf("GitTags(%s)", path), err)
	}
	return list, err
}

func (a *App) GitCreateTag(path, name, message string) (model.OpResult, error) {
	if err := a.ensureProject(path); err != nil {
		return model.OpResult{}, err
	}
	res, err := gitx.CreateTag(path, name, message)
	if err != nil {
		logError(fmt.Sprintf("GitCreateTag(%s)", name), err)
	}
	return res, err
}

func (a *App) GitPushTag(path, name, remote string) (model.OpResult, error) {
	if err := a.ensureProject(path); err != nil {
		return model.OpResult{}, err
	}
	res, err := gitx.PushTag(path, name, remote)
	if err != nil {
		logError(fmt.Sprintf("GitPushTag(%s)", name), err)
	}
	return res, err
}

// GitRevert 生成一条反向提交来撤销指定提交（不改写历史）。
func (a *App) GitRevert(path, sha string) (string, error) {
	if err := a.ensureProject(path); err != nil {
		return "", err
	}
	msg, err := gitx.Revert(path, sha)
	if err != nil {
		logError(fmt.Sprintf("GitRevert(%s)", sha), err)
	}
	return msg, err
}

/* ---------------- 版本对比 ---------------- */

// GitDiffRefs 列出两个版本之间变更的文件。target 传空表示拿工作区来比。
func (a *App) GitDiffRefs(path, base, target string) (model.DiffSummary, error) {
	if err := a.ensureProject(path); err != nil {
		return model.DiffSummary{}, err
	}
	summary, err := gitx.DiffRefs(path, base, target)
	if err != nil {
		logError(fmt.Sprintf("GitDiffRefs(%s..%s)", base, target), err)
	}
	return summary, err
}

// GitDiffRefFile 返回单个文件在两个版本之间的 diff。
// fullContext 为真时把整份文件都带回来（界面上的「显示全部代码」）。
func (a *App) GitDiffRefFile(path, base, target, file string, fullContext bool) (model.DiffText, error) {
	if err := a.ensureProject(path); err != nil {
		return model.DiffText{}, err
	}
	text, err := gitx.DiffRefFile(path, base, target, file, fullContext)
	if err != nil {
		logError(fmt.Sprintf("GitDiffRefFile(%s..%s %s)", base, target, file), err)
	}
	return text, err
}

// GitDiffCommitFile 返回某次提交里单个文件的 diff（提交详情里逐文件展开用）。
func (a *App) GitDiffCommitFile(path, sha, file string) (model.DiffText, error) {
	if err := a.ensureProject(path); err != nil {
		return model.DiffText{}, err
	}
	text, err := gitx.DiffCommitFile(path, sha, file)
	if err != nil {
		logError(fmt.Sprintf("GitDiffCommitFile(%s %s)", sha, file), err)
	}
	return text, err
}

/* ---------------- 提交工作流 ---------------- */

// GitWorktreeFiles 列出工作区里有改动的文件（含未跟踪），供勾选提交。
func (a *App) GitWorktreeFiles(path string) ([]model.FileChange, error) {
	if err := a.ensureProject(path); err != nil {
		return nil, err
	}
	files, err := gitx.WorktreeFiles(path)
	if err != nil {
		logError(fmt.Sprintf("GitWorktreeFiles(%s)", path), err)
	}
	return files, err
}

// GitDiffFile 返回单个文件的 diff。
func (a *App) GitDiffFile(path, file string, staged bool) (string, error) {
	if err := a.ensureProject(path); err != nil {
		return "", err
	}
	return gitx.DiffFile(path, file, staged)
}

// GitCommitPaths 只提交勾选的路径。
func (a *App) GitCommitPaths(path string, files []string, message string, amend bool) (string, error) {
	if err := a.ensureProject(path); err != nil {
		return "", err
	}
	sha, changed, err := gitx.CommitPaths(path, files, message, amend)
	if err != nil {
		logError(fmt.Sprintf("GitCommitPaths(%s)", path), err)
		return "", err
	}
	if !changed {
		return "没有需要提交的改动", nil
	}
	return "已提交 " + shortSHA(sha), nil
}

/* ---------------- 远程同步 ---------------- */

// GitRemotes 列出远程仓库（含仓库配置里的代理，界面上要提示）。
func (a *App) GitRemotes(path string) ([]model.Remote, error) {
	if err := a.ensureProject(path); err != nil {
		return nil, err
	}
	remotes, err := gitx.Remotes(path)
	if err != nil {
		logError(fmt.Sprintf("GitRemotes(%s)", path), err)
	}
	return remotes, err
}

func (a *App) GitFetch(path, remote string, prune bool) (model.OpResult, error) {
	if err := a.ensureProject(path); err != nil {
		return model.OpResult{}, err
	}
	res, err := gitx.Fetch(path, remote, prune)
	if err != nil {
		logError(fmt.Sprintf("GitFetch(%s)", path), err)
	}
	return res, err
}

func (a *App) GitPull(path, remote, branch string, rebase bool) (model.OpResult, error) {
	if err := a.ensureProject(path); err != nil {
		return model.OpResult{}, err
	}
	res, err := gitx.Pull(path, remote, branch, rebase)
	if err != nil {
		logError(fmt.Sprintf("GitPull(%s)", path), err)
	}
	return res, err
}

func (a *App) GitPush(path, remote, branch string, setUpstream bool) (model.OpResult, error) {
	if err := a.ensureProject(path); err != nil {
		return model.OpResult{}, err
	}
	res, err := gitx.Push(path, remote, branch, setUpstream)
	if err != nil {
		logError(fmt.Sprintf("GitPush(%s)", path), err)
	}
	return res, err
}

func (a *App) GitSetUpstream(path, remote, branch string) (model.OpResult, error) {
	if err := a.ensureProject(path); err != nil {
		return model.OpResult{}, err
	}
	res, err := gitx.SetUpstream(path, remote, branch)
	if err != nil {
		logError(fmt.Sprintf("GitSetUpstream(%s)", path), err)
	}
	return res, err
}

func (a *App) GitCommitFiles(path, sha string) ([]model.FileChange, error) {
	if !gitx.IsRepo(path) {
		return nil, fmt.Errorf("这不是一个 git 仓库")
	}
	return gitx.CommitFiles(path, sha)
}

func (a *App) GitBackups(path string) ([]model.Backup, error) {
	if !gitx.IsRepo(path) {
		return nil, fmt.Errorf("这不是一个 git 仓库")
	}
	stashes, err := gitx.Stashes(path)
	if err != nil {
		return nil, err
	}
	tags, err := gitx.BackupTags(path, gitx.BackupTagPrefix)
	if err != nil {
		return nil, err
	}
	all := append(stashes, tags...)
	sort.Slice(all, func(i, j int) bool { return all[i].When > all[j].When })
	return all, nil
}

func (a *App) GitApplyStash(path, ref string) (string, error) {
	if !gitx.IsRepo(path) {
		return "", fmt.Errorf("这不是一个 git 仓库")
	}
	return gitx.ApplyStash(path, ref)
}

func (a *App) GitDropBackup(path, ref, kind string) (string, error) {
	if !gitx.IsRepo(path) {
		return "", fmt.Errorf("这不是一个 git 仓库")
	}
	if kind == "tag" {
		return gitx.DeleteTag(path, ref)
	}
	return gitx.DropStash(path, ref)
}

func (a *App) ArchiveProject(path string, commitFirst bool) (string, error) {
	if commitFirst && gitx.IsRepo(path) {
		if _, _, err := gitx.CommitAll(path, "chore: 归档前打点 "+gitx.Stamp()); err != nil {
			return "", fmt.Errorf("归档前打点失败：%v", err)
		}
	}
	name := filepath.Base(path)
	entry, err := archive.Archive(a.cfg, path, name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("已归档到 %s", entry.ZipPath), nil
}

func (a *App) ListArchives() ([]model.ArchiveEntry, error) {
	return archive.List(a.cfg)
}

// ListDir 列出项目内某个目录。传入的 root 是项目实际根目录，先过 ensureProject。
func (a *App) ListDir(root, rel string) (model.DirListing, error) {
	if err := a.ensureProject(root); err != nil {
		return model.DirListing{}, err
	}
	return fsx.List(root, rel)
}

func (a *App) PreviewFile(root, rel string) (model.FilePreview, error) {
	if err := a.ensureProject(root); err != nil {
		return model.FilePreview{}, err
	}
	return fsx.Preview(root, rel)
}

// GetTaskBoard 读取项目里的任务文档（TASK*.md / PROGRESS.md / BLOCKED.md）。
//
// 注意：界面上「任务」面板已暂时下线（觉得鸡肋），这个绑定与 internal/taskdoc 一并保留，
// 以后想恢复只需要把界面接回来。
func (a *App) GetTaskBoard(project string) (model.TaskBoard, error) {
	if err := a.ensureProject(project); err != nil {
		return model.TaskBoard{}, err
	}
	board, err := taskdoc.Scan(project)
	if err != nil {
		logError(fmt.Sprintf("GetTaskBoard(%s)", project), err)
	}
	return board, err
}

// ReadTextFile 读取文本内容供编辑器使用（目录 / 二进制 / 超限都会如实标注）。
func (a *App) ReadTextFile(root, rel string, maxBytes int64) (model.TextFile, error) {
	if err := a.ensureProject(root); err != nil {
		return model.TextFile{}, err
	}
	return fsx.ReadText(root, rel, maxBytes)
}

// WriteTextFile 原子写回文本文件。这是应用唯一会改文件内容的入口。
func (a *App) WriteTextFile(root, rel, content string) (string, error) {
	if err := a.ensureProject(root); err != nil {
		logError("WriteTextFile/ensureProject", err)
		return "", err
	}
	if err := fsx.WriteText(root, rel, content); err != nil {
		logError(fmt.Sprintf("WriteTextFile(%s)", rel), err)
		return "", err
	}
	return "已保存 " + rel, nil
}

// SearchAll 跨项目搜索：项目名、提交信息、文件名、文件内容。
// 全局命中上限 300 条；项目之间并发跑——实测耗时几乎全在 git 提交检索上
// （每次 git 调用约 350ms），串行 6 个项目就要 2 秒，并发后降到几百毫秒。
func (a *App) SearchAll(query string) ([]model.SearchHit, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return []model.SearchHit{}, nil
	}
	entries, err := a.projectEntries()
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(q)

	// 搜索作用于每个项目的实际根目录
	type target struct{ name, root string }
	targets := make([]target, 0, len(entries))
	for _, e := range entries {
		root := e.Root
		if root == "" {
			root = e.Path
		}
		name := filepath.Base(filepath.Clean(e.Path))
		targets = append(targets, target{name: name, root: root})
	}

	perProject := make([][]model.SearchHit, len(targets))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i, tg := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, tg target) {
			defer wg.Done()
			defer func() { <-sem }()
			perProject[i] = searchOneProject(tg.root, tg.name, q, lower)
		}(i, tg)
	}
	wg.Wait()

	const globalLimit = 300
	out := []model.SearchHit{}
	// 项目名命中排最前，最直接
	for _, tg := range targets {
		if strings.Contains(strings.ToLower(tg.name), lower) {
			out = append(out, model.SearchHit{Project: tg.name, Kind: "project"})
		}
	}
	for _, hits := range perProject {
		for _, h := range hits {
			if len(out) >= globalLimit {
				return out, nil
			}
			out = append(out, h)
		}
	}
	return out, nil
}

func searchOneProject(full, name, query, lower string) []model.SearchHit {
	const perHitLimit = 100
	out := []model.SearchHit{}

	if gitx.IsRepo(full) {
		commits, err := gitx.Subjects(full, 200)
		if err != nil {
			logError(fmt.Sprintf("SearchAll/commits(%s)", name), err)
		}
		for _, c := range commits {
			if strings.Contains(strings.ToLower(c.Subject), lower) {
				out = append(out, model.SearchHit{
					Project: name,
					Kind:    "commit",
					Line:    c.Subject,
					Detail:  c.Short,
				})
				if len(out) >= perHitLimit {
					return out
				}
			}
		}
	}

	hits, err := fsx.SearchFiles(full, query)
	if err != nil {
		logError(fmt.Sprintf("SearchAll/files(%s)", name), err)
		return out
	}
	for _, h := range hits {
		h.Project = name
		out = append(out, h)
		if len(out) >= perHitLimit {
			break
		}
	}
	return out
}

func (a *App) NewFolder(root, rel, name string) (string, error) {
	if err := a.ensureProject(root); err != nil {
		logError("NewFolder/ensureProject", err)
		return "", err
	}
	created, err := fsx.Mkdir(root, rel, name)
	if err != nil {
		logError(fmt.Sprintf("NewFolder(%s/%s)", rel, name), err)
	}
	return created, err
}

func (a *App) RenameEntry(root, rel, newName, policy string) (string, error) {
	if err := a.ensureProject(root); err != nil {
		logError("RenameEntry/ensureProject", err)
		return "", err
	}
	renamed, err := fsx.Rename(root, rel, newName, policy)
	if err != nil {
		logError(fmt.Sprintf("RenameEntry(%s -> %s, %s)", rel, newName, policy), err)
	}
	return renamed, err
}

func (a *App) DeleteEntries(root string, rels []string) (string, error) {
	if err := a.ensureProject(root); err != nil {
		logError("DeleteEntries/ensureProject", err)
		return "", err
	}
	removed, err := fsx.Remove(root, rels)
	if err != nil {
		logError(fmt.Sprintf("DeleteEntries(%v)", rels), err)
		return "", err
	}
	return fmt.Sprintf("已将 %d 个条目移入回收站", removed), nil
}

// DeleteEntriesForce 永久删除（不进回收站）。
// 只在回收站接口无能为力时使用（例如嵌套上千层、路径超长的目录），界面上必须先明确警告。
func (a *App) DeleteEntriesForce(root string, rels []string) (string, error) {
	if err := a.ensureProject(root); err != nil {
		logError("DeleteEntriesForce/ensureProject", err)
		return "", err
	}
	removed, err := fsx.RemoveForce(root, rels)
	if err != nil {
		logError(fmt.Sprintf("DeleteEntriesForce(%v)", rels), err)
		return "", err
	}
	return fmt.Sprintf("已永久删除 %d 个条目", removed), nil
}

func (a *App) CheckConflicts(root string, srcs []string, dstDir string) ([]string, error) {
	if err := a.ensureProject(root); err != nil {
		return nil, err
	}
	return fsx.CheckConflicts(root, srcs, dstDir)
}

func (a *App) MoveEntries(root string, srcs []string, dstDir, policy string) (string, error) {
	if err := a.ensureProject(root); err != nil {
		logError("MoveEntries/ensureProject", err)
		return "", err
	}
	if err := fsx.Move(root, srcs, dstDir, policy); err != nil {
		logError(fmt.Sprintf("MoveEntries(%v -> %s, %s)", srcs, dstDir, policy), err)
		return "", err
	}
	return fmt.Sprintf("已移动 %d 个条目", len(srcs)), nil
}

func (a *App) CopyEntries(root string, srcs []string, dstDir, policy string) (string, error) {
	if err := a.ensureProject(root); err != nil {
		logError("CopyEntries/ensureProject", err)
		return "", err
	}
	if err := fsx.Copy(root, srcs, dstDir, policy); err != nil {
		logError(fmt.Sprintf("CopyEntries(%v -> %s, %s)", srcs, dstDir, policy), err)
		return "", err
	}
	return fmt.Sprintf("已复制 %d 个条目", len(srcs)), nil
}

// OpenWithSystem 用系统默认关联程序打开文件或目录。
func (a *App) OpenWithSystem(path string) error {
	return launchWindow(path)
}

func (a *App) RestoreArchive(zipPath string) (string, error) {
	entry, err := archive.Restore(a.cfg, zipPath)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("已还原到 %s", entry.OriginalPath), nil
}

func (a *App) DeleteArchive(zipPath string) (string, error) {
	if err := archive.Delete(a.cfg, zipPath); err != nil {
		return "", err
	}
	return "已删除该归档", nil
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// logError 把失败原因追加到 %APPDATA%\vibe-pm\vibe-pm.log。
// 这是 GUI 程序，没有控制台，出错只能靠日志留痕，否则只能看到一闪而过的提示。
// 搜索是并发跑的，这里加锁避免多行交错。
var logMu sync.Mutex

func logError(op string, err error) {
	if err == nil {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	f, e := os.OpenFile(config.LogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if e != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\t%s\t%v\n", time.Now().Format("2006-01-02 15:04:05"), op, err)
}

func (a *App) LogPath() string {
	return config.LogPath()
}
