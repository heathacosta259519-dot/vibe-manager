export namespace agents {
	
	export class Template {
	    id: string;
	    name: string;
	    builtin: boolean;
	    body: string;
	
	    static createFrom(source: any = {}) {
	        return new Template(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.builtin = source["builtin"];
	        this.body = source["body"];
	    }
	}

}

export namespace model {
	
	export class ArchiveEntry {
	    name: string;
	    zipPath: string;
	    originalPath: string;
	    archivedAt: number;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new ArchiveEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.zipPath = source["zipPath"];
	        this.originalPath = source["originalPath"];
	        this.archivedAt = source["archivedAt"];
	        this.size = source["size"];
	    }
	}
	export class Backup {
	    ref: string;
	    kind: string;
	    message: string;
	    when: number;
	
	    static createFrom(source: any = {}) {
	        return new Backup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.kind = source["kind"];
	        this.message = source["message"];
	        this.when = source["when"];
	    }
	}
	export class Branch {
	    name: string;
	    upstream: string;
	    subject: string;
	    current: boolean;
	    ahead: number;
	    behind: number;
	    when: number;
	
	    static createFrom(source: any = {}) {
	        return new Branch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.upstream = source["upstream"];
	        this.subject = source["subject"];
	        this.current = source["current"];
	        this.ahead = source["ahead"];
	        this.behind = source["behind"];
	        this.when = source["when"];
	    }
	}
	export class Commit {
	    hash: string;
	    short: string;
	    subject: string;
	    author: string;
	    when: number;
	    files: number;
	
	    static createFrom(source: any = {}) {
	        return new Commit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.short = source["short"];
	        this.subject = source["subject"];
	        this.author = source["author"];
	        this.when = source["when"];
	        this.files = source["files"];
	    }
	}
	export class Config {
	    projectRoot: string;
	    archiveRoot: string;
	    editorCmd: string;
	    terminalCmd: string;
	    theme: string;
	    windowWidth: number;
	    windowHeight: number;
	    fileViewMode: string;
	    fileSortKey: string;
	    fileShowHidden: boolean;
	    filePreviewOpen: boolean;
	    editorWidth: number;
	    railWidth: number;
	    editorFont: string;
	    author: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectRoot = source["projectRoot"];
	        this.archiveRoot = source["archiveRoot"];
	        this.editorCmd = source["editorCmd"];
	        this.terminalCmd = source["terminalCmd"];
	        this.theme = source["theme"];
	        this.windowWidth = source["windowWidth"];
	        this.windowHeight = source["windowHeight"];
	        this.fileViewMode = source["fileViewMode"];
	        this.fileSortKey = source["fileSortKey"];
	        this.fileShowHidden = source["fileShowHidden"];
	        this.filePreviewOpen = source["filePreviewOpen"];
	        this.editorWidth = source["editorWidth"];
	        this.railWidth = source["railWidth"];
	        this.editorFont = source["editorFont"];
	        this.author = source["author"];
	    }
	}
	export class CreateOptions {
	    name: string;
	    readme: boolean;
	    gitignore: boolean;
	    license: string;
	    src: boolean;
	    agents: boolean;
	    agentsTemplate: string;
	    gitInit: boolean;
	    author: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.readme = source["readme"];
	        this.gitignore = source["gitignore"];
	        this.license = source["license"];
	        this.src = source["src"];
	        this.agents = source["agents"];
	        this.agentsTemplate = source["agentsTemplate"];
	        this.gitInit = source["gitInit"];
	        this.author = source["author"];
	    }
	}
	export class DiffFileStat {
	    status: string;
	    path: string;
	    oldPath: string;
	    adds: number;
	    dels: number;
	    binary: boolean;
	    untracked: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DiffFileStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.path = source["path"];
	        this.oldPath = source["oldPath"];
	        this.adds = source["adds"];
	        this.dels = source["dels"];
	        this.binary = source["binary"];
	        this.untracked = source["untracked"];
	    }
	}
	export class DiffSummary {
	    base: string;
	    target: string;
	    files: DiffFileStat[];
	    adds: number;
	    dels: number;
	
	    static createFrom(source: any = {}) {
	        return new DiffSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.base = source["base"];
	        this.target = source["target"];
	        this.files = this.convertValues(source["files"], DiffFileStat);
	        this.adds = source["adds"];
	        this.dels = source["dels"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DiffText {
	    text: string;
	    truncated: boolean;
	    binary: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DiffText(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.truncated = source["truncated"];
	        this.binary = source["binary"];
	    }
	}
	export class FileEntry {
	    name: string;
	    path: string;
	    ext: string;
	    isDir: boolean;
	    size: number;
	    modTime: number;
	    gitState: string;
	    hidden: boolean;
	    special: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.ext = source["ext"];
	        this.isDir = source["isDir"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	        this.gitState = source["gitState"];
	        this.hidden = source["hidden"];
	        this.special = source["special"];
	    }
	}
	export class DirListing {
	    relPath: string;
	    entries: FileEntry[];
	    total: number;
	    truncated: boolean;
	    gitAvailable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DirListing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.relPath = source["relPath"];
	        this.entries = this.convertValues(source["entries"], FileEntry);
	        this.total = source["total"];
	        this.truncated = source["truncated"];
	        this.gitAvailable = source["gitAvailable"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FileChange {
	    status: string;
	    path: string;
	    oldPath: string;
	    staged: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.path = source["path"];
	        this.oldPath = source["oldPath"];
	        this.staged = source["staged"];
	    }
	}
	
	export class FilePreview {
	    kind: string;
	    text: string;
	    dataUrl: string;
	    size: number;
	    truncated: boolean;
	    binary: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new FilePreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.text = source["text"];
	        this.dataUrl = source["dataUrl"];
	        this.size = source["size"];
	        this.truncated = source["truncated"];
	        this.binary = source["binary"];
	        this.message = source["message"];
	    }
	}
	export class LicenseOption {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new LicenseOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class OpResult {
	    message: string;
	    output: string;
	
	    static createFrom(source: any = {}) {
	        return new OpResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = source["message"];
	        this.output = source["output"];
	    }
	}
	export class Project {
	    name: string;
	    alias: string;
	    path: string;
	    entry: string;
	    imported: boolean;
	    rootOverridden: boolean;
	    modTime: number;
	    isGit: boolean;
	    branch: string;
	    dirty: boolean;
	    hasUpstream: boolean;
	    ahead: number;
	    behind: number;
	    lastCommit: string;
	    lastCommitAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.alias = source["alias"];
	        this.path = source["path"];
	        this.entry = source["entry"];
	        this.imported = source["imported"];
	        this.rootOverridden = source["rootOverridden"];
	        this.modTime = source["modTime"];
	        this.isGit = source["isGit"];
	        this.branch = source["branch"];
	        this.dirty = source["dirty"];
	        this.hasUpstream = source["hasUpstream"];
	        this.ahead = source["ahead"];
	        this.behind = source["behind"];
	        this.lastCommit = source["lastCommit"];
	        this.lastCommitAt = source["lastCommitAt"];
	    }
	}
	export class Remote {
	    name: string;
	    url: string;
	    proxy: string;
	
	    static createFrom(source: any = {}) {
	        return new Remote(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.url = source["url"];
	        this.proxy = source["proxy"];
	    }
	}
	export class SearchHit {
	    project: string;
	    path: string;
	    line: string;
	    lineNo: number;
	    kind: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchHit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project = source["project"];
	        this.path = source["path"];
	        this.line = source["line"];
	        this.lineNo = source["lineNo"];
	        this.kind = source["kind"];
	        this.detail = source["detail"];
	    }
	}
	export class Tag {
	    name: string;
	    subject: string;
	    annotated: boolean;
	    when: number;
	
	    static createFrom(source: any = {}) {
	        return new Tag(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.subject = source["subject"];
	        this.annotated = source["annotated"];
	        this.when = source["when"];
	    }
	}
	export class TaskSection {
	    no: number;
	    title: string;
	    goal: string;
	    how: string;
	    rules: string[];
	    checks: string[];
	    raw: string;
	
	    static createFrom(source: any = {}) {
	        return new TaskSection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.no = source["no"];
	        this.title = source["title"];
	        this.goal = source["goal"];
	        this.how = source["how"];
	        this.rules = source["rules"];
	        this.checks = source["checks"];
	        this.raw = source["raw"];
	    }
	}
	export class TaskBoard {
	    current: string;
	    hasTaskDoc: boolean;
	    allDone: boolean;
	    docs: string[];
	    tasks: TaskSection[];
	    blocked: boolean;
	    blockedText: string;
	    progressText: string;
	    rawText: string;
	
	    static createFrom(source: any = {}) {
	        return new TaskBoard(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.hasTaskDoc = source["hasTaskDoc"];
	        this.allDone = source["allDone"];
	        this.docs = source["docs"];
	        this.tasks = this.convertValues(source["tasks"], TaskSection);
	        this.blocked = source["blocked"];
	        this.blockedText = source["blockedText"];
	        this.progressText = source["progressText"];
	        this.rawText = source["rawText"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class TextFile {
	    content: string;
	    size: number;
	    truncated: boolean;
	    binary: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TextFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.content = source["content"];
	        this.size = source["size"];
	        this.truncated = source["truncated"];
	        this.binary = source["binary"];
	    }
	}

}

