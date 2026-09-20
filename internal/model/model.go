package model

type Config struct {
	ProjectRoot string `json:"projectRoot"`
	ArchiveRoot string `json:"archiveRoot"`
	EditorCmd   string `json:"editorCmd"`
	TerminalCmd string `json:"terminalCmd"`
	Theme       string `json:"theme"`

	WindowWidth  int `json:"windowWidth"`
	WindowHeight int `json:"windowHeight"`

	FileViewMode    string `json:"fileViewMode"`
	FileSortKey     string `json:"fileSortKey"`
	FileShowHidden  bool   `json:"fileShowHidden"`
	FilePreviewOpen bool   `json:"filePreviewOpen"`
	EditorWidth     int    `json:"editorWidth"`
	RailWidth       int    `json:"railWidth"`
	EditorFont      string `json:"editorFont"`
	EditorFontSize  int    `json:"editorFontSize"`
	UiFontSize      int    `json:"uiFontSize"`
	Author          string `json:"author"`
}

// LicenseOption 是新建项目时可选的协议。
type LicenseOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateOptions 是新建项目时由界面给出的骨架勾选状态。
type CreateOptions struct {
	Name           string `json:"name"`
	Readme         bool   `json:"readme"`
	Gitignore      bool   `json:"gitignore"`
	License        string `json:"license"` // 空 = 不加协议文件
	Src            bool   `json:"src"`
	Agents         bool   `json:"agents"`
	AgentsTemplate string `json:"agentsTemplate"` // 模板 id，空 = 用默认模板
	GitInit        bool   `json:"gitInit"`
	Author         string `json:"author"` // 协议里的署名
}

type Project struct {
	Name           string `json:"name"`
	Alias          string `json:"alias"`
	Path           string `json:"path"`  // 实际根目录：所有操作都作用于它
	Entry          string `json:"entry"` // 条目位置（项目根下的目录，或导入的目录）
	Imported       bool   `json:"imported"`
	RootOverridden bool   `json:"rootOverridden"`
	ModTime        int64  `json:"modTime"`
	IsGit          bool   `json:"isGit"`
	Branch         string `json:"branch"`
	Dirty          bool   `json:"dirty"`
	HasUpstream    bool   `json:"hasUpstream"`
	Ahead          int    `json:"ahead"`
	Behind         int    `json:"behind"`
	LastCommit     string `json:"lastCommit"`
	LastCommitAt   int64  `json:"lastCommitAt"`
}

// UpdateInfo 是「检查更新」的结果。没有新版本时 Available=false，其余字段仍会带上当前版本。
type UpdateInfo struct {
	Available bool   `json:"available"`
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Notes     string `json:"notes"`   // release 正文（Markdown）
	Size      int64  `json:"size"`    // 附件字节数
	PageURL   string `json:"pageURL"` // release 页面，供「查看详情」
}

// TextFile 是编辑器的读取结果。
type TextFile struct {
	Content   string `json:"content"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary"`
}

// SearchHit 是全局搜索的一条命中。
// Kind 取 project / commit / file / content。
type SearchHit struct {
	Project string `json:"project"`
	Path    string `json:"path"`
	Line    string `json:"line"`
	LineNo  int    `json:"lineNo"`
	Kind    string `json:"kind"`
	Detail  string `json:"detail"`
}

// TaskSection 是任务书里的一个「任务 N」小节。
type TaskSection struct {
	No     int      `json:"no"`
	Title  string   `json:"title"`
	Goal   string   `json:"goal"`
	How    string   `json:"how"`
	Rules  []string `json:"rules"`
	Checks []string `json:"checks"`
	Raw    string   `json:"raw"`
}

// TaskBoard 是任务面板要展示的全部信息。
type TaskBoard struct {
	Current      string        `json:"current"`
	HasTaskDoc   bool          `json:"hasTaskDoc"`
	AllDone      bool          `json:"allDone"`
	Docs         []string      `json:"docs"`
	Tasks        []TaskSection `json:"tasks"`
	Blocked      bool          `json:"blocked"`
	BlockedText  string        `json:"blockedText"`
	ProgressText string        `json:"progressText"`
	RawText      string        `json:"rawText"`
}

type GitStatus struct {
	Branch      string
	HasUpstream bool
	Ahead       int
	Behind      int
	Dirty       bool
	MTime       int64
}

type Commit struct {
	Hash    string `json:"hash"`
	Short   string `json:"short"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	When    int64  `json:"when"`
	Files   int    `json:"files"`
}

type FileChange struct {
	Status  string `json:"status"`
	Path    string `json:"path"`    // 重命名时是新路径，可直接拿去取 diff
	OldPath string `json:"oldPath"` // 仅重命名 / 复制时有值，只用于显示
	Staged  bool   `json:"staged"`
}

// DiffFileStat 是两个版本之间某个文件的变更概要。
// Adds / Dels 为 -1 表示统计不到（二进制文件，或重命名时 git 不给行数），界面显示成「—」。
type DiffFileStat struct {
	Status    string `json:"status"`  // A 新增 / M 修改 / D 删除 / R 重命名 / U 冲突
	Path      string `json:"path"`    // 目标侧路径；重命名时是新路径，直接拿它取 diff
	OldPath   string `json:"oldPath"` // 仅重命名 / 复制时有值
	Adds      int    `json:"adds"`
	Dels      int    `json:"dels"`
	Binary    bool   `json:"binary"`
	Untracked bool   `json:"untracked"` // 目标侧是工作区时，新加入但还没跟踪的文件
}

// DiffSummary 是两个版本之间的整体对比结果。Target 为空表示拿工作区比。
type DiffSummary struct {
	Base   string         `json:"base"`
	Target string         `json:"target"`
	Files  []DiffFileStat `json:"files"`
	Adds   int            `json:"adds"`
	Dels   int            `json:"dels"`
}

// DiffText 是单个文件在两个版本之间的 diff 内容。
type DiffText struct {
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary"`
}

// Remote 是一个 git 远程。
type Remote struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Proxy string `json:"proxy"` // 仓库/全局配置里的 http.proxy，空表示没配
}

// Branch 是一个本地分支。
type Branch struct {
	Name     string `json:"name"`
	Upstream string `json:"upstream"`
	Subject  string `json:"subject"`
	Current  bool   `json:"current"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
	When     int64  `json:"when"`
}

// Tag 是一个标签。
type Tag struct {
	Name      string `json:"name"`
	Subject   string `json:"subject"`
	Annotated bool   `json:"annotated"`
	When      int64  `json:"when"`
}

// OpResult 是远程等操作的结果：Message 给界面看，Output 是 git 原始输出（便于看懂进度与报错）。
type OpResult struct {
	Message string `json:"message"`
	Output  string `json:"output"`
}

type Backup struct {
	Ref     string `json:"ref"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
	When    int64  `json:"when"`
}

type ArchiveEntry struct {
	Name         string `json:"name"`
	ZipPath      string `json:"zipPath"`
	OriginalPath string `json:"originalPath"`
	ArchivedAt   int64  `json:"archivedAt"`
	Size         int64  `json:"size"`
}

// FileEntry 是模拟文件管理器里的一个条目。
// Path 为相对项目根的斜杠分隔路径；GitState 取 "", "M", "A", "D", "R", "??"。
type FileEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Ext      string `json:"ext"`
	IsDir    bool   `json:"isDir"`
	Size     int64  `json:"size"`
	ModTime  int64  `json:"modTime"`
	GitState string `json:"gitState"`
	Hidden   bool   `json:"hidden"`
	Special  bool   `json:"special"`
}

type DirListing struct {
	RelPath      string      `json:"relPath"`
	Entries      []FileEntry `json:"entries"`
	Total        int         `json:"total"`
	Truncated    bool        `json:"truncated"`
	GitAvailable bool        `json:"gitAvailable"`
}

// FilePreview 描述只读预览内容。Kind 取 "text" / "image" / "other"。
type FilePreview struct {
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	DataURL   string `json:"dataUrl"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary"`
	Message   string `json:"message"`
}
