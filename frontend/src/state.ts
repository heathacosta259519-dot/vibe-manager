import { agents, model } from '../wailsjs/go/models';

export type Project = model.Project;
export type Commit = model.Commit;
export type DiffSummary = model.DiffSummary;
export type DiffFileStat = model.DiffFileStat;
export type DiffText = model.DiffText;
export type Backup = model.Backup;
export type FileChange = model.FileChange;
export type ArchiveEntry = model.ArchiveEntry;
export type Config = model.Config;
export type FileEntry = model.FileEntry;
export type DirListing = model.DirListing;
export type FilePreview = model.FilePreview;
export type SearchHit = model.SearchHit;
export type LicenseOption = model.LicenseOption;
export type CreateOptions = model.CreateOptions;
export type AgentTemplate = agents.Template;
export type Remote = model.Remote;
export type GitOpResult = model.OpResult;
export type Branch = model.Branch;
export type Tag = model.Tag;

export type FilterKey = 'all' | 'dirty' | 'noupstream' | 'stale' | 'nogit';
export type SortKey = 'activity' | 'name' | 'ahead';
export type ArchiveSortKey = 'time' | 'name' | 'size';
export type ViewMode = 'icons' | 'details';
export type FileSortKey = 'name' | 'time' | 'size' | 'type';
export type DetailMode = 'version' | 'files' | 'compare';

/** 顶栏能切到的视图。设置是独立页面，不再是弹窗。 */
export type ViewKey = 'projects' | 'archives' | 'search' | 'settings';

/** 对比时目标侧选「工作区」用的哨兵值：git 侧收到空字符串就表示比工作区。 */
export const WORKTREE = '__worktree__';

export const STALE_DAYS = 30;
export const NAME_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/;

export const state = {
    cfg: null as unknown as Config,
    configPath: '',
    logPath: '',
    version: '',

    projects: [] as Project[],
    archives: [] as ArchiveEntry[],
    selected: null as Project | null,

    view: 'projects' as ViewKey,
    mode: 'version' as DetailMode,

    search: '',
    filter: 'all' as FilterKey,
    sortBy: 'activity' as SortKey,
    archiveSort: 'time' as ArchiveSortKey,

    commits: [] as Commit[],
    backups: [] as Backup[],
    remotes: [] as Remote[],
    worktree: [] as FileChange[],
    branches: [] as Branch[],
    tags: [] as Tag[],
    commitPicked: [] as string[],
    commitMessage: '',
    commitAmend: false,

    gs: {
        query: '',
        hits: [] as SearchHit[],
        running: false,
        truncated: false,
        token: 0,
    },

    // 右侧编辑器栏（0.3.0 阶段三，取代原来的只读预览）
    editor: {
        kind: 'none' as 'none' | 'text' | 'image' | 'binary' | 'other',
        path: '',
        name: '',
        ext: '',
        value: '',
        original: '',
        dirty: false,
        truncated: false,
        message: '',
        dataUrl: '',
        width: 580,
        open: true,
        sel: null as { anchor: number; head: number } | null,
        scrollTop: 0,
    },

    // 版本对比：base → target 之间的文件清单，以及右栏正在看的那个文件的 diff
    compare: {
        project: '',        // 这份对比属于哪个项目；换项目才重置
        base: 'HEAD',
        target: WORKTREE,
        files: [] as DiffFileStat[],
        adds: 0,
        dels: 0,
        loading: false,
        error: '',
        full: false,        // 右栏是「只看改动」还是「整份文件」
        picked: '',
        text: '',
        truncated: false,
        binary: false,
        diffLoading: false,
        diffError: '',
    },

    file: {
        rel: '',
        listing: null as DirListing | null,
        history: [] as string[],
        historyIndex: -1,
        selected: [] as string[],
        anchor: null as string | null,
        clipboard: null as { mode: 'cut' | 'copy'; paths: string[] } | null,
        showHidden: false,
        filter: '',
        sortKey: 'name' as FileSortKey,
        sortDesc: false,
        loading: false,
    },
};

export function resetExplorer(): void {
    state.file.rel = '';
    state.file.listing = null;
    state.file.history = [];
    state.file.historyIndex = -1;
    state.file.selected = [];
    state.file.anchor = null;
    state.file.filter = '';
    state.editor.kind = 'none';
    state.editor.path = '';
    state.editor.name = '';
    state.editor.dirty = false;
}
