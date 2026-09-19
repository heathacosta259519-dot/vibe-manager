import { App, openInEditor } from '../api';
import { hooks } from '../bus';
import { esc, fmtAgo, fmtTime, renderDiffText } from '../format';
import { NAME_RE, STALE_DAYS, state, type Branch, type Commit, type CreateOptions, type Project } from '../state';
import { closeModal, confirmDialog, copyText, openModal, promptDialog, renderEmpty, renderHero, renderLoading, toast, wireHero, withBusy } from '../ui';
import { refreshArchives, refreshProjects } from './data';
import { openTemplateManager, showCommitFiles } from './dialogs';
import { openCompare, renderCompareBody } from './compare';
import { renderEditorPane } from './editor';
import { openExplorer, renderExplorerBody } from './explorer';

/* ---------------- 数据加载 ---------------- */

export async function loadProjectGit(p: Project): Promise<void> {
    state.commits = [];
    state.backups = [];
    state.remotes = [];
    state.worktree = [];
    state.branches = [];
    state.tags = [];
    if (!p.isGit) return;
    try {
        const [log, bks, remotes, files, branches, tags] = await Promise.all([
            App.GitLog(p.path),
            App.GitBackups(p.path),
            App.GitRemotes(p.path),
            App.GitWorktreeFiles(p.path),
            App.GitBranches(p.path),
            App.GitTags(p.path),
        ]);
        state.commits = log ?? [];
        state.backups = bks ?? [];
        state.remotes = remotes ?? [];
        state.worktree = files ?? [];
        state.branches = branches ?? [];
        state.tags = tags ?? [];
        syncCommitPicked();
    } catch (e) {
        toast(String(e), 'err');
    }
}

/** 保持勾选集合与当前改动一致：消失的去掉、新出现的默认勾上。 */
function syncCommitPicked(): void {
    const alive = new Set(state.worktree.map(f => f.path));
    const kept = state.commitPicked.filter(p => alive.has(p));
    for (const f of state.worktree) {
        if (!kept.includes(f.path)) kept.push(f.path);
    }
    state.commitPicked = kept;
}

async function reloadDetailGit(p: Project): Promise<void> {
    await loadProjectGit(p);
    await refreshProjects();
    renderDetail();
}

export async function selectProject(p: Project): Promise<void> {
    // 人可能正停在设置页等别的视图上（左栏项目列表一直都是可点的）。
    // 这时把「选中它」交给切视图的续作：干净就直接切完再选；有未保存的设置
    // 会先弹确认，确认后才切+选；用户取消则什么都不做，不会偷偷改掉选中项。
    if (state.view !== 'projects') {
        hooks.setView('projects', () => void selectProject(p));
        return;
    }

    // 快速连点/连按方向键时，只让最后一次选择决定渲染结果
    const token = ++selectToken;
    state.selected = p;
    renderList();
    await loadProjectGit(p);
    if (token !== selectToken) return;
    renderDetail();
    if (state.mode === 'files') openExplorer();
    if (state.mode === 'compare') void openCompare();
}

let selectToken = 0;

/** 键盘上下切换后，把焦点还给当前选中项（列表每次都会重绘）。 */
function focusSelectedProject(): void {
    document.querySelector<HTMLElement>('#project-list .proj.active')?.focus();
}

/* ---------------- 项目列表 ---------------- */

function passFilter(p: Project): boolean {
    switch (state.filter) {
        case 'dirty':
            return p.isGit && p.dirty;
        case 'noupstream':
            return p.isGit && !p.hasUpstream;
        case 'stale':
            return daysIdleSince(p.modTime) >= STALE_DAYS;
        case 'nogit':
            return !p.isGit;
        default:
            return true;
    }
}

function daysIdleSince(unix: number): number {
    return unix ? Math.floor((Date.now() / 1000 - unix) / 86400) : 0;
}

function sortProjects(items: Project[]): Project[] {
    return items.slice().sort((a, b) => {
        switch (state.sortBy) {
            case 'name':
                return a.name.localeCompare(b.name);
            case 'ahead':
                return (b.ahead - a.ahead) || (b.modTime - a.modTime);
            default:
                return b.modTime - a.modTime;
        }
    });
}

const DAY_MS = 86400_000;

/** 把时间戳归到「今天 / 昨天 / 一周内 / 两周内 / 更早」。 */
function activityBucket(ts: number): string {
    if (!ts) return '更早';
    const now = new Date();
    const startToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
    const t = ts * 1000;
    if (t >= startToday) return '今天';
    if (t >= startToday - DAY_MS) return '昨天';
    if (t >= startToday - 7 * DAY_MS) return '一周内';
    if (t >= startToday - 14 * DAY_MS) return '两周内';
    return '更早';
}

function groupByActivity(items: Project[]): [string, Project[]][] {
    const order = ['今天', '昨天', '一周内', '两周内', '更早'];
    const map = new Map<string, Project[]>();
    for (const p of items) {
        const key = activityBucket(p.modTime);
        const arr = map.get(key) ?? [];
        arr.push(p);
        map.set(key, arr);
    }
    return order.filter(k => map.has(k)).map(k => [k, map.get(k)!] as [string, Project[]]);
}

export function renderList(): void {
    const list = document.getElementById('project-list');
    if (!list) return;

    const countEl = document.getElementById('count-projects');
    if (countEl) countEl.textContent = state.projects.length ? String(state.projects.length) : '';
    hooks.status();

    const kw = state.search.trim().toLowerCase();
    const items = sortProjects(state.projects.filter(p => {
        if (!passFilter(p)) return false;
        if (!kw) return true;
        return p.name.toLowerCase().includes(kw)
            || (p.alias ?? '').toLowerCase().includes(kw)
            || (p.lastCommit ?? '').toLowerCase().includes(kw);
    }));
    if (!items.length) {
        if (state.projects.length) {
            list.innerHTML = renderHero({
                title: '没有找到',
                hint: '换个关键词，或把筛选条件放宽一点',
                actions: [{ label: '清空筛选', act: 'clear', primary: true }],
            });
            wireHero(list, {
                clear: () => {
                    state.search = '';
                    state.filter = 'all';
                    const box = document.getElementById('search') as HTMLInputElement | null;
                    if (box) box.value = '';
                    document.querySelectorAll<HTMLElement>('#filters .chip').forEach(c => {
                        c.classList.toggle('active', c.dataset.filter === 'all');
                    });
                    renderList();
                },
            });
        } else {
            list.innerHTML = renderHero({
                title: '想做什么？',
                hint: '先把项目根设好，工具会扫描它下面的目录；外面的仓库也可以直接导进来',
                actions: [
                    { label: '新建项目', act: 'new', primary: true },
                    { label: '导入已有目录', act: 'import' },
                    { label: '打开设置', act: 'settings' },
                ],
            });
            wireHero(list, {
                new: openNewProject,
                import: () => void importProject(),
                settings: () => hooks.setView('settings'),
            });
        }
        return;
    }

    const row = (p: Project): string => {
        const idle = daysIdleSince(p.modTime);
        const badges: string[] = [];
        if (p.isGit && p.dirty) badges.push('<span class="badge dirty">有改动</span>');
        if (p.isGit && p.ahead > 0) badges.push(`<span class="badge ahead">未推送 ${p.ahead}</span>`);
        if (p.imported) badges.push('<span class="badge muted">导入</span>');
        if (p.rootOverridden) badges.push('<span class="badge task">自定义根</span>');
        if (p.isGit && !p.hasUpstream) badges.push('<span class="badge muted">未设上游</span>');
        if (!p.isGit) badges.push('<span class="badge muted">非 git</span>');
        if (idle >= STALE_DAYS) badges.push(`<span class="badge stale">${idle} 天未动</span>`);
        return `
        <div class="proj ${state.selected?.entry === p.entry ? 'active' : ''}" data-entry="${esc(p.entry)}" title="${esc(p.path)}"
             role="option" aria-selected="${state.selected?.entry === p.entry}" tabindex="${state.selected?.entry === p.entry ? 0 : -1}">
            <div class="proj-top">
                <span class="proj-name">${esc(p.alias || p.name)}</span>
                <span class="proj-ago">${fmtAgo(p.modTime)}</span>
            </div>
            ${p.alias ? `<div class="proj-real">${esc(p.name)}</div>` : ''}
            <div class="proj-meta">
                <span class="dot ${p.isGit ? (p.dirty ? 'dirty' : 'clean') : 'nogit'}"></span>
                <span>${p.isGit ? esc(p.branch || '尚无提交') : '非 git'}</span>
            </div>
            ${p.isGit && p.lastCommit ? `<div class="proj-last">${esc(p.lastCommit)}</div>` : ''}
            ${badges.length ? `<div class="proj-badges">${badges.join('')}</div>` : ''}
        </div>`;
    };

    // 按最后活动排序时切成「今天 / 昨天 / …」分组；按名称等其它方式排就不分组
    list.innerHTML = state.sortBy === 'activity'
        ? groupByActivity(items).map(([label, group]) =>
            `<div class="group-title">${esc(label)}</div>${group.map(row).join('')}`).join('')
        : items.map(row).join('');
    list.querySelectorAll<HTMLElement>('.proj').forEach(el => {
        el.addEventListener('click', () => {
            const p = state.projects.find(x => x.entry === el.dataset.entry);
            if (p) void selectProject(p);
        });
        el.addEventListener('dblclick', async () => {
            const p = state.projects.find(x => x.entry === el.dataset.entry);
            if (!p) return;
            try {
                await App.OpenFolder(p.path);
            } catch (e) {
                toast(String(e), 'err');
            }
        });
    });

    // 键盘：↑↓ 切换项目，Home/End 跳首尾，Enter 打开文件夹
    list.onkeydown = e => {
        const arr = [...list.querySelectorAll<HTMLElement>('.proj')];
        if (!arr.length) return;
        const at = arr.findIndex(el => el.dataset.entry === state.selected?.entry);
        const pick = (i: number): void => {
            const p = state.projects.find(x => x.entry === arr[i].dataset.entry);
            if (p) void selectProject(p).then(focusSelectedProject);
        };
        switch (e.key) {
            case 'ArrowDown': e.preventDefault(); pick(at < 0 ? 0 : Math.min(arr.length - 1, at + 1)); break;
            case 'ArrowUp': e.preventDefault(); pick(at < 0 ? 0 : Math.max(0, at - 1)); break;
            case 'Home': e.preventDefault(); pick(0); break;
            case 'End': e.preventDefault(); pick(arr.length - 1); break;
            case 'Enter': {
                const p = state.projects.find(x => x.entry === state.selected?.entry);
                if (p) void App.OpenFolder(p.path).catch(err => toast(String(err), 'err'));
                break;
            }
            default:
        }
    };
}

function renderCommits(): string {
    if (!state.commits.length) return renderEmpty({ title: '还没有任何提交', hint: '用「一键打点」把当前状态记下来' });
    return state.commits.map(c => `
        <div class="commit">
            <div class="commit-main">
                <code>${esc(c.short)}</code>
                <span class="subject">${esc(c.subject)}</span>
            </div>
            <div class="commit-meta">${fmtAgo(c.when)} · ${esc(c.author)} · ${c.files} 文件</div>
            <div class="commit-actions">
                <button class="btn tiny ghost" data-files="${esc(c.hash)}">详情</button>
                <button class="btn tiny ghost" data-revert="${esc(c.hash)}" title="新建一条反向提交抵消这次改动，不改写历史">撤销</button>
                <button class="btn tiny" data-sha="${esc(c.hash)}">回滚到此…</button>
            </div>
        </div>
    `).join('');
}

function renderBackups(): string {
    if (!state.backups.length) {
        return renderEmpty({
            title: '暂无自动备份',
            hint: '回滚或撤销改动时，工具会自动把现场存进 stash 并打保底标签',
        });
    }
    return state.backups.map(b => `
        <div class="commit">
            <div class="commit-main">
                <span class="badge ${b.kind === 'tag' ? 'tag' : 'stash'}">${b.kind === 'tag' ? '标签' : 'stash'}</span>
                <span class="subject">${esc(b.message)}</span>
            </div>
            <div class="commit-meta">${fmtTime(b.when)}</div>
            <div class="commit-actions">
                ${b.kind === 'tag'
                    ? `<button class="btn tiny" data-restore-backup="${esc(b.ref)}">回到此处</button>`
                    : `<button class="btn tiny" data-apply-backup="${esc(b.ref)}">应用</button>`}
                <button class="btn tiny ghost danger-text" data-drop-backup="${esc(b.ref)}" data-kind="${b.kind}">删除</button>
            </div>
        </div>
    `).join('');
}

/* ---------------- 远程同步条 ---------------- */

function renderSyncBar(p: Project): string {
    const branch = p.branch || '（尚无提交）';
    const remote = state.remotes[0];
    const ab: string[] = [];
    if (p.ahead > 0) ab.push(`↑${p.ahead}`);
    if (p.behind > 0) ab.push(`↓${p.behind}`);
    const abText = ab.length ? ab.join(' ') : '与远端一致';

    if (!remote) {
        return `
        <div class="sync-bar">
            <div class="sync-main">
                <span class="sync-branch" title="当前分支">${esc(branch)}</span>
                <span class="sync-hint">这个项目还没有远程仓库，纯本地工作</span>
            </div>
            <div class="sync-actions">
                <button class="btn tiny ghost" data-branch-panel="1">分支…</button>
                <button class="btn tiny ghost" data-sync="remote-help">怎么加远程？</button>
            </div>
        </div>`;
    }

    const proxy = remote.proxy ? `<span class="sync-proxy">走代理 ${esc(remote.proxy)}</span>` : '';
    const upstreamHint = p.hasUpstream ? '' : '<span class="sync-warn">未设上游</span>';
    return `
    <div class="sync-bar">
        <div class="sync-main">
            <button class="sync-branch" data-branch-panel="1" title="点开分支管理">${esc(branch)}</button>
            <span class="sync-ab">${esc(abText)}</span>
            ${upstreamHint}
            <span class="sync-remote" title="${esc(remote.url)}">${esc(remote.name)} → ${esc(remote.url)}</span>
            ${proxy}
        </div>
        <div class="sync-actions">
            <button class="btn tiny ghost" data-sync="fetch">抓取</button>
            <button class="btn tiny ghost" data-sync="pull">拉取</button>
            <button class="btn tiny ${p.hasUpstream ? 'primary' : ''}" data-sync="push">
                ${p.hasUpstream ? '推送' : '推送并设为上游'}
            </button>
        </div>
    </div>`;
}

/** 工作区脏时先问一句，给出「先打点 / 直接继续 / 取消」三条路。 */
function withDirtyGuard(p: Project, title: string, what: string, run: () => Promise<void>): void {
    if (!p.dirty) {
        void run();
        return;
    }
    const root = openModal(`
        <h3>${esc(title)}</h3>
        <div class="modal-body">
            <div class="hint">当前工作区有<b>未提交改动</b>，${esc(what)}可能失败或产生冲突。</div>
            <div class="hint">最稳的做法是先打点（提交一次），这样随时能回退。</div>
        </div>
        <div class="modal-actions">
            <button class="btn ghost" data-act="cancel">取消</button>
            <button class="btn ghost danger-text" data-act="force">直接继续</button>
            <button class="btn primary" data-act="stamp">先打点</button>
        </div>
    `);
    const finish = (): void => {
        closeModal();
        void run();
    };
    root.querySelector('[data-act="cancel"]')!.addEventListener('click', closeModal);
    root.querySelector('[data-act="force"]')!.addEventListener('click', finish);
    root.querySelector('[data-act="stamp"]')!.addEventListener('click', () => {
        const btn = root.querySelector('[data-act="stamp"]') as HTMLElement;
        void withBusy(btn, async () => {
            try {
                await App.GitCommitAll(p.path, `chore: ${what}前打点 ${fmtTime(Math.floor(Date.now() / 1000))}`);
            } catch (e) {
                toast(String(e), 'err');
                return;
            }
            closeModal();
            await run();
        }, '打点中…');
    });
}

async function doFetch(p: Project, btn: HTMLElement): Promise<void> {
    await withBusy(btn, async () => {
        try {
            const res = await App.GitFetch(p.path, state.remotes[0]?.name ?? '', true);
            toast(res.message);
            await reloadDetailGit(p);
        } catch (e) {
            toast(String(e), 'err');
        }
    }, '抓取中…');
}

async function doPull(p: Project, btn: HTMLElement): Promise<void> {
    await withDirtyGuard(p, '拉取远端更新', '拉取', async () => {
        await withBusy(btn, async () => {
            try {
                const res = await App.GitPull(p.path, state.remotes[0]?.name ?? '', p.branch, false);
                toast(res.message);
                await reloadDetailGit(p);
            } catch (e) {
                toast(String(e), 'err');
            }
        }, '拉取中…');
    });
}

async function doPush(p: Project, btn: HTMLElement): Promise<void> {
    const setUpstream = !p.hasUpstream;
    await withBusy(btn, async () => {
        try {
            const res = await App.GitPush(p.path, state.remotes[0]?.name ?? '', p.branch, setUpstream);
            toast(res.message);
            await reloadDetailGit(p);
        } catch (e) {
            toast(String(e), 'err');
        }
    }, '推送中…');
}

function showRemoteHelp(p: Project): void {
    const cmds = [`git remote add origin <仓库地址>`, `git push -u origin ${p.branch || 'main'}`].join('\n');
    const root = openModal(`
        <h3>怎么加远程仓库</h3>
        <div class="modal-body">
            <div class="hint">在项目目录里执行下面两条命令即可（本工具不代填地址，避免填错）：</div>
            <div class="commit-line"><code>${esc(cmds)}</code></div>
            <div class="hint">如果 GitHub 需要代理，记得在仓库里配 <code>http.proxy</code>；本工具会沿用仓库自身的 git 配置。</div>
        </div>
        <div class="modal-actions">
            <button class="btn ghost" data-act="cancel">关闭</button>
            <button class="btn primary" data-act="copy">复制命令</button>
        </div>
    `);
    root.querySelector('[data-act="cancel"]')!.addEventListener('click', closeModal);
    root.querySelector('[data-act="copy"]')!.addEventListener('click', () => {
        void copyText(cmds, '已复制命令');
        closeModal();
    });
}

/* ---------------- 提交卡片 ---------------- */

const FILE_STATUS: Record<string, { label: string; cls: string }> = {
    A: { label: '新增', cls: 'add' },
    M: { label: '修改', cls: 'mod' },
    D: { label: '删除', cls: 'del' },
    R: { label: '重命名', cls: 'ren' },
    U: { label: '冲突', cls: 'del' },
};

const COMMIT_PREFIXES = ['feat', 'fix', 'docs', 'refactor', 'chore', 'test'];

function renderCommitBox(): string {
    const files = state.worktree;
    const n = state.commitPicked.length;

    const rows = files.length
        ? files.map(f => {
            const meta = FILE_STATUS[f.status] ?? { label: f.status, cls: 'mod' };
            const picked = state.commitPicked.includes(f.path);
            return `
        <label class="commit-file">
            <input type="checkbox" data-pick="${esc(f.path)}" ${picked ? 'checked' : ''} />
            <span class="badge ${meta.cls}">${esc(meta.label)}</span>
            <span class="commit-path" title="${esc(f.path)}">${esc(f.path)}</span>
            <button class="btn tiny ghost" type="button" data-diff="${esc(f.path)}" data-staged="${f.staged ? '1' : '0'}">diff</button>
            <button class="btn tiny ghost" type="button" data-open-ext="${esc(f.path)}" title="用外部编辑器打开">外部</button>
        </label>`;
        }).join('')
        : '<div class="hint commit-none">工作区没有改动</div>';

    const hint = files.length
        ? `${files.length} 个文件有改动，已勾选 ${n} 个`
        : '工作区干净；勾上 amend 可以只改上一次提交的信息';
    const label = state.commitAmend ? '修正上一次提交' : `提交选中（${n}）`;

    return `
    <div class="section-head">
        <h3>提交</h3>
        <div class="hint">${hint}</div>
        ${files.length ? `<div class="actions">
            <button class="btn tiny ghost" data-pick-all="1">全选</button>
            <button class="btn tiny ghost" data-pick-all="0">清空</button>
        </div>` : ''}
    </div>
    <div class="commit-box">
        <div class="commit-files">${rows}</div>
        <div class="commit-form">
            <textarea id="commit-msg" class="input commit-msg" placeholder="写提交信息（例如 feat: 新增 XXX）" spellcheck="false">${esc(state.commitMessage)}</textarea>
            <div class="commit-prefixes">
                ${COMMIT_PREFIXES.map(x => `<button class="btn tiny ghost" type="button" data-prefix="${x}">${x}</button>`).join('')}
            </div>
            <div class="commit-row">
                <label class="check">
                    <input type="checkbox" id="commit-amend" ${state.commitAmend ? 'checked' : ''} />
                    修正上一次提交（amend，会改写历史）
                </label>
                <button class="btn primary" data-commit="1" ${state.commitAmend || n ? '' : 'disabled'}>${label}</button>
            </div>
        </div>
    </div>`;
}

async function showDiff(p: Project, file: string, staged: boolean): Promise<void> {
    const root = openModal(`
        <h3>${esc(file)}</h3>
        <div class="modal-body" id="diff-body">${renderLoading()}</div>
        <div class="modal-actions"><button class="btn primary" data-act="ok">关闭</button></div>
    `);
    root.classList.add('modal-wide');
    root.querySelector('[data-act="ok"]')!.addEventListener('click', closeModal);
    const body = root.querySelector('#diff-body') as HTMLElement;
    try {
        const text = await App.GitDiffFile(p.path, file, staged);
        body.innerHTML = renderDiffText(text);
    } catch (e) {
        body.innerHTML = `<div class="hint bad">${esc(String(e))}</div>`;
    }
}

async function doCommitSelected(p: Project, btn: HTMLElement): Promise<void> {
    const msg = state.commitMessage.trim();
    if (!msg) {
        toast('请先写提交信息', 'err');
        return;
    }
    if (state.commitAmend) {
        await withBusy(btn, () => runCommit(p, msg), '提交中…');
        return;
    }
    if (!state.commitPicked.length) {
        toast('请先勾选要提交的文件', 'err');
        return;
    }
    await withBusy(btn, () => runCommit(p, msg), '提交中…');
}

async function runCommit(p: Project, msg: string): Promise<void> {
    try {
        toast(await App.GitCommitPaths(p.path, state.commitPicked, msg, state.commitAmend));
        state.commitMessage = '';
        state.commitAmend = false;
        await reloadDetailGit(p);
    } catch (e) {
        toast(String(e), 'err');
    }
}

function wireCommitBox(p: Project): void {
    const msgEl = document.getElementById('commit-msg') as HTMLTextAreaElement | null;
    msgEl?.addEventListener('input', () => {
        state.commitMessage = msgEl.value;
    });

    document.querySelectorAll<HTMLInputElement>('#mode-body [data-pick]').forEach(box => {
        box.addEventListener('change', () => {
            const path = box.dataset.pick!;
            if (box.checked) {
                if (!state.commitPicked.includes(path)) state.commitPicked.push(path);
            } else {
                state.commitPicked = state.commitPicked.filter(x => x !== path);
            }
            renderDetail();
        });
    });

    document.querySelectorAll<HTMLElement>('#mode-body [data-pick-all]').forEach(btn => {
        btn.addEventListener('click', () => {
            state.commitPicked = btn.dataset.pickAll === '1' ? state.worktree.map(f => f.path) : [];
            renderDetail();
        });
    });

    document.querySelectorAll<HTMLElement>('#mode-body [data-diff]').forEach(btn => {
        btn.addEventListener('click', e => {
            e.preventDefault();
            e.stopPropagation();
            void showDiff(p, btn.dataset.diff!, btn.dataset.staged === '1');
        });
    });

    document.querySelectorAll<HTMLElement>('#mode-body [data-open-ext]').forEach(btn => {
        btn.addEventListener('click', e => {
            // 这一行本身是个 label，不拦住会顺带把勾选也切了
            e.preventDefault();
            e.stopPropagation();
            void openInEditor(btn.dataset.openExt!).catch(err => toast(String(err), 'err'));
        });
    });

    document.querySelectorAll<HTMLElement>('#mode-body [data-prefix]').forEach(btn => {
        btn.addEventListener('click', () => {
            const prefix = btn.dataset.prefix!;
            // 已有前缀就替换掉，避免叠成 feat: fix: xxx
            const body = state.commitMessage.replace(/^[a-z\u4e00-\u9fa5]+(\([^)]*\))?:\s*/i, '');
            state.commitMessage = `${prefix}: ${body}`;
            renderDetail();
            const el = document.getElementById('commit-msg') as HTMLTextAreaElement | null;
            el?.focus();
            el?.setSelectionRange(el.value.length, el.value.length);
        });
    });

    const amend = document.getElementById('commit-amend') as HTMLInputElement | null;
    amend?.addEventListener('change', () => {
        state.commitAmend = amend.checked;
        renderDetail();
    });

    document.querySelector<HTMLElement>('#mode-body [data-commit]')?.addEventListener('click', e => {
        void doCommitSelected(p, e.currentTarget as HTMLElement);
    });
}

/* ---------------- 分支面板 ---------------- */

function abMark(b: Branch): string {
    if (b.ahead && b.behind) return ' ↑↓';
    if (b.ahead) return ' ↑';
    if (b.behind) return ' ↓';
    return '';
}

async function openBranchPanel(p: Project): Promise<void> {
    const root = openModal(`
        <h3>分支</h3>
        <div class="modal-body">
            <div id="br-list">${renderLoading()}</div>
        </div>
        <div class="modal-actions">
            <button class="btn ghost" data-act="close">关闭</button>
            <button class="btn primary" data-act="new">新建分支…</button>
        </div>
    `);
    root.classList.add('modal-wide');

    const listEl = root.querySelector('#br-list') as HTMLElement;
    let branches: Branch[] = [];

    const fill = async (): Promise<void> => {
        try {
            branches = (await App.GitBranches(p.path)) ?? [];
        } catch (e) {
            listEl.innerHTML = `<div class="hint bad">${esc(String(e))}</div>`;
            return;
        }
        if (!branches.length) {
            listEl.innerHTML = renderEmpty({ title: '还没有分支（可能还没有提交）' });
            return;
        }
        listEl.innerHTML = branches.map(b => `
            <div class="branch-row${b.current ? ' current' : ''}">
                <span class="branch-name">${esc(b.name)}</span>
                ${b.current ? '<span class="badge task">当前</span>' : ''}
                <span class="branch-up">${b.upstream ? esc(b.upstream) + abMark(b) : '未设上游'}</span>
                <span class="branch-subject" title="${esc(b.subject)}">${esc(b.subject)}</span>
                <span class="branch-when">${fmtAgo(b.when)}</span>
                <div class="branch-actions">
                    ${b.current ? '' : `
                        <button class="btn tiny" data-br-switch="${esc(b.name)}">切换</button>
                        <button class="btn tiny ghost" data-br-merge="${esc(b.name)}">合并到当前</button>
                        <button class="btn tiny ghost danger-text" data-br-del="${esc(b.name)}">删除</button>`}
                </div>
            </div>`).join('');
        wireRows();
    };

    const afterOp = async (msg: string): Promise<void> => {
        toast(msg);
        await reloadDetailGit(p);
        await fill();
    };

    const wireRows = (): void => {
        listEl.querySelectorAll<HTMLElement>('[data-br-switch]').forEach(btn => {
            btn.addEventListener('click', () => {
                const name = btn.dataset.brSwitch!;
                withDirtyGuard(p, '切换分支', '切换', async () => {
                    await withBusy(btn, async () => {
                        try {
                            const res = await App.GitCheckout(p.path, name);
                            await afterOp(res.message);
                        } catch (e) {
                            toast(String(e), 'err');
                        }
                    }, '切换中…');
                });
            });
        });

        listEl.querySelectorAll<HTMLElement>('[data-br-merge]').forEach(btn => {
            btn.addEventListener('click', () => {
                const name = btn.dataset.brMerge!;
                confirmDialog({
                    title: '合并分支',
                    body: `<div class="hint">把 <b>${esc(name)}</b> 合并进当前分支 <b>${esc(p.branch)}</b>。</div>
                           <div class="hint">如果有冲突，工具会列出冲突文件，需要你去命令行解决。</div>`,
                    confirmText: '合并',
                    onConfirm: async () => {
                        const res = await App.GitMerge(p.path, name);
                        await afterOp(res.message);
                    },
                });
            });
        });

        listEl.querySelectorAll<HTMLElement>('[data-br-del]').forEach(btn => {
            btn.addEventListener('click', () => {
                const name = btn.dataset.brDel!;
                const del = (force: boolean): void => {
                    confirmDialog({
                        title: force ? '强制删除分支' : '删除分支',
                        body: force
                            ? `<div class="hint"><b>${esc(name)}</b> 还有未合并的提交，强制删除会<b>丢掉</b>这些内容。</div>`
                            : `<div class="hint">删除本地分支 <b>${esc(name)}</b>。</div>`,
                        confirmText: force ? '强制删除' : '删除',
                        danger: true,
                        onConfirm: async () => {
                            try {
                                const res = await App.GitDeleteBranch(p.path, name, force);
                                await afterOp(res.message);
                            } catch (e) {
                                const msg = String(e);
                                if (!force && msg.includes('未合并')) {
                                    closeModal();
                                    del(true);
                                    return;
                                }
                                throw e;
                            }
                        },
                    });
                };
                del(false);
            });
        });
    };

    root.querySelector('[data-act="close"]')!.addEventListener('click', closeModal);
    root.querySelector('[data-act="new"]')!.addEventListener('click', () => newBranchDialog(p, fill));

    await fill();
}

function newBranchDialog(p: Project, onDone: () => Promise<void>): void {
    const root = openModal(`
        <h3>新建分支</h3>
        <div class="modal-body">
            <label class="field">
                <span>分支名</span>
                <input id="nb-name" class="input" type="text" autocomplete="off" placeholder="feature-xxx" />
            </label>
            <label class="check">
                <input type="checkbox" id="nb-switch" checked />
                创建后立即切换过去
            </label>
        </div>
        <div class="modal-actions">
            <button class="btn ghost" data-act="cancel">取消</button>
            <button class="btn primary" data-act="ok">创建</button>
        </div>
    `);
    const nameEl = root.querySelector('#nb-name') as HTMLInputElement;
    const switchEl = root.querySelector('#nb-switch') as HTMLInputElement;
    const okBtn = root.querySelector('[data-act="ok"]') as HTMLButtonElement;

    root.querySelector('[data-act="cancel"]')!.addEventListener('click', closeModal);
    const submit = (): void => {
        const name = nameEl.value.trim();
        if (!name) return;
        void withBusy(okBtn, async () => {
            try {
                const res = await App.GitCreateBranch(p.path, name, switchEl.checked);
                closeModal();
                toast(res.message);
                await reloadDetailGit(p);
                await onDone();
            } catch (e) {
                toast(String(e), 'err');
            }
        }, '创建中…');
    };
    okBtn.addEventListener('click', submit);
    nameEl.addEventListener('keydown', e => {
        if (e.key === 'Enter') submit();
    });
    nameEl.focus();
}

/* ---------------- 标签 ---------------- */

function renderTags(): string {
    if (!state.tags.length) {
        return renderEmpty({ title: '还没有标签', hint: '标签是回滚时的坐标，可以给重要节点打一个' });
    }
    return state.tags.map(tg => `
        <div class="commit">
            <div class="commit-main">
                <span class="badge ${tg.annotated ? 'tag' : 'muted'}">${tg.annotated ? '附注' : '轻量'}</span>
                <span class="subject">${esc(tg.name)}</span>
                ${tg.subject ? `<span class="commit-meta" title="${esc(tg.subject)}">${esc(tg.subject)}</span>` : ''}
            </div>
            <div class="commit-meta">${fmtAgo(tg.when)}</div>
            <div class="commit-actions">
                <button class="btn tiny ghost" data-tag-push="${esc(tg.name)}">推送</button>
                <button class="btn tiny ghost danger-text" data-tag-del="${esc(tg.name)}">删除</button>
            </div>
        </div>`).join('');
}

function newTagDialog(p: Project): void {
    const root = openModal(`
        <h3>新建标签</h3>
        <div class="modal-body">
            <label class="field">
                <span>标签名</span>
                <input id="nt-name" class="input" type="text" autocomplete="off" placeholder="v0.1.0" />
            </label>
            <label class="field">
                <span>说明（留空则为轻量标签）</span>
                <input id="nt-msg" class="input" type="text" autocomplete="off" placeholder="首个可用版本" />
            </label>
            <div class="hint">标签默认打在 <b>当前提交</b> 上。</div>
        </div>
        <div class="modal-actions">
            <button class="btn ghost" data-act="cancel">取消</button>
            <button class="btn primary" data-act="ok">创建</button>
        </div>
    `);
    const nameEl = root.querySelector('#nt-name') as HTMLInputElement;
    const msgEl = root.querySelector('#nt-msg') as HTMLInputElement;
    const okBtn = root.querySelector('[data-act="ok"]') as HTMLButtonElement;

    root.querySelector('[data-act="cancel"]')!.addEventListener('click', closeModal);
    const submit = (): void => {
        const name = nameEl.value.trim();
        if (!name) return;
        void withBusy(okBtn, async () => {
            try {
                const res = await App.GitCreateTag(p.path, name, msgEl.value.trim());
                closeModal();
                toast(res.message);
                await reloadDetailGit(p);
            } catch (e) {
                toast(String(e), 'err');
            }
        }, '创建中…');
    };
    okBtn.addEventListener('click', submit);
    nameEl.addEventListener('keydown', e => {
        if (e.key === 'Enter') submit();
    });
    nameEl.focus();
}

function wireTags(p: Project): void {
    document.querySelector<HTMLElement>('#mode-body [data-tag-new]')?.addEventListener('click', () => newTagDialog(p));

    document.querySelectorAll<HTMLElement>('#mode-body [data-tag-push]').forEach(btn => {
        btn.addEventListener('click', () => {
            const name = btn.dataset.tagPush!;
            void withBusy(btn, async () => {
                try {
                    const res = await App.GitPushTag(p.path, name, state.remotes[0]?.name ?? '');
                    toast(res.message);
                } catch (e) {
                    toast(String(e), 'err');
                }
            }, '推送中…');
        });
    });

    document.querySelectorAll<HTMLElement>('#mode-body [data-tag-del]').forEach(btn => {
        btn.addEventListener('click', () => {
            const name = btn.dataset.tagDel!;
            confirmDialog({
                title: '删除标签',
                body: `<div class="hint">删除本地标签 <b>${esc(name)}</b>。</div>
                       <div class="hint">只会删本地标签，不会动远端已有的同名标签。</div>`,
                confirmText: '删除',
                danger: true,
                onConfirm: async () => {
                    toast(await App.GitDropBackup(p.path, name, 'tag'));
                    await reloadDetailGit(p);
                },
            });
        });
    });
}

function renderVersionBody(): string {
    const p = state.selected;
    if (!p?.isGit) {
        return `<div class="hint">该项目不是 git 仓库，版本功能不可用。</div>`;
    }
    return `
        ${renderSyncBar(p)}
        ${renderCommitBox()}
        <div class="section-head">
            <h3>历史</h3>
            <div class="actions">
                <button class="btn primary" data-git="commit">一键打点</button>
                <button class="btn ghost" data-git="discard">撤销未提交改动</button>
                <button class="btn ghost" data-git="refresh">刷新</button>
            </div>
        </div>
        <div class="commit-list">${renderCommits()}</div>
        <div class="section-head">
            <h3>标签</h3>
            <div class="hint">回滚时的坐标</div>
            <div class="actions">
                <button class="btn tiny ghost" data-tag-new="1">新建标签…</button>
            </div>
        </div>
        <div class="commit-list">${renderTags()}</div>
        <div class="section-head">
            <h3>备份</h3>
            <div class="hint">回滚 / 撤销前自动留下的现场，可随时找回</div>
        </div>
        <div class="commit-list">${renderBackups()}</div>`;
}

export function renderDetail(): void {
    const d = document.getElementById('detail');
    if (!d) return;
    hooks.status();
    // 编辑器栏是通高右栏，跟着当前项目 / 模式收起或展开
    renderEditorPane();
    if (!state.selected) {
        d.innerHTML = renderHero({
            title: '选一个项目',
            hint: '左侧点一个项目，就能看到它的版本、文件与任务；也可以新建一个',
            actions: [{ label: '新建项目', act: 'new', primary: true }],
        });
        wireHero(d, { new: openNewProject });
        return;
    }
    const p = state.selected;
    const tags: string[] = [];
    if (p.isGit) {
        tags.push(`<span class="tag ${p.dirty ? 'dirty' : 'clean'}">${p.dirty ? '有未提交改动' : '工作区干净'}</span>`);
        tags.push(`<span class="tag">分支 ${esc(p.branch || '—')}</span>`);
        tags.push(p.hasUpstream
            ? `<span class="tag">未推送 ${p.ahead} · 落后 ${p.behind}</span>`
            : `<span class="tag">未设上游</span>`);
        tags.push(`<span class="tag">最后活动 ${fmtTime(p.modTime)}</span>`);
    } else {
        tags.push(`<span class="tag nogit">非 git 仓库，版本功能不可用</span>`);
    }
    const filesActive = state.mode === 'files';
    const compareActive = state.mode === 'compare';
    d.innerHTML = `
        <div class="detail-head">
            <div>
                <h2>${esc(p.alias || p.name)}${p.alias ? `<span class="h2-alias">（${esc(p.name)}）</span>` : ''}</h2>
                <div class="path">${esc(p.path)}</div>
                ${p.rootOverridden ? `<div class="hint">条目位置：${esc(p.entry)}</div>` : ''}
            </div>
            <div class="actions">
                <button class="btn ghost" data-open="editor">编辑器</button>
                <button class="btn ghost" data-open="terminal">终端</button>
                <details class="menu">
                    <summary class="btn ghost">更多…</summary>
                    <div class="menu-body">
                        <button class="menu-item" data-open="alias">设置备注名…</button>
                        <button class="menu-item" data-open="root">设置实际根目录…</button>
                        ${p.rootOverridden ? '<button class="menu-item" data-open="reset-root">恢复默认目录</button>' : ''}
                        <button class="menu-item" data-open="reveal">用系统资源管理器打开</button>
                        <button class="menu-item" data-open="copy">复制项目路径</button>
                        <div class="menu-sep"></div>
                        <button class="menu-item danger-text" data-open="archive">归档项目…</button>
                        ${p.imported ? '<button class="menu-item danger-text" data-open="unimport">移出列表（不删文件）</button>' : ''}
                    </div>
                </details>
            </div>
        </div>
        <div class="status-bar">${tags.join('')}</div>
        <div class="mode-tabs">
            <button class="mode-tab ${state.mode === 'version' ? 'active' : ''}" data-mode="version">版本管理</button>
            <button class="mode-tab ${filesActive ? 'active' : ''}" data-mode="files">文件</button>
            <button class="mode-tab ${compareActive ? 'active' : ''}" data-mode="compare">对比</button>
        </div>
        <div id="mode-body"></div>
    `;
    wireDetail(p);

    // 文件与对比模式都要占满高度（各自内部滚动），版本模式整块滚动
    document.getElementById('mode-body')!.classList.toggle('fx-mode', filesActive || compareActive);

    if (state.mode === 'files') {
        renderExplorerBody();
    } else if (state.mode === 'compare') {
        renderCompareBody();
    } else {
        const body = document.getElementById('mode-body')!;
        body.innerHTML = renderVersionBody();
        wireVersion(p);
    }
}

function wireDetail(p: Project): void {
    document.querySelectorAll<HTMLElement>('[data-mode]').forEach(btn => {
        btn.addEventListener('click', () => hooks.setMode(btn.dataset.mode as typeof state.mode));
    });

    document.querySelectorAll<HTMLElement>('[data-open]').forEach(btn => {
        btn.addEventListener('click', async () => {
            const kind = btn.dataset.open!;
            const menu = btn.closest('details');
            if (menu) menu.removeAttribute('open');
            try {
                if (kind === 'editor') await App.OpenEditor(p.path);
                else if (kind === 'terminal') await App.OpenTerminal(p.path);
                else if (kind === 'reveal') await App.OpenFolder(p.path);
                else if (kind === 'copy') await copyText(p.path, '已复制路径');
                else if (kind === 'alias') openAliasDialog(p);
                else if (kind === 'root') await chooseProjectRoot(p);
                else if (kind === 'reset-root') await resetProjectRoot(p);
                else if (kind === 'unimport') confirmRemoveImport(p);
                else if (kind === 'archive') confirmArchive(p);
            } catch (e) {
                toast(String(e), 'err');
            }
        });
    });

    const menu = document.querySelector<HTMLDetailsElement>('#detail details.menu');
    if (menu) {
        document.addEventListener('mousedown', e => {
            if (menu.hasAttribute('open') && !menu.contains(e.target as Node)) menu.removeAttribute('open');
        });
    }
}

function wireVersion(p: Project): void {
    wireCommitBox(p);
    wireTags(p);

    document.querySelectorAll<HTMLElement>('#mode-body [data-branch-panel]').forEach(btn => {
        btn.addEventListener('click', () => void openBranchPanel(p));
    });

    document.querySelectorAll<HTMLElement>('#mode-body [data-sync]').forEach(btn => {
        btn.addEventListener('click', () => {
            switch (btn.dataset.sync) {
                case 'fetch': void doFetch(p, btn); break;
                case 'pull': void doPull(p, btn); break;
                case 'push': void doPush(p, btn); break;
                case 'remote-help': showRemoteHelp(p); break;
            }
        });
    });

    document.querySelectorAll<HTMLElement>('#mode-body [data-git]').forEach(btn => {
        btn.addEventListener('click', () => {
            const kind = btn.dataset.git;
            if (kind === 'commit') void withBusy(btn, () => doCommit(p), '打点中…');
            else if (kind === 'discard') confirmDiscard(p);
            else if (kind === 'refresh') void (async () => {
                await loadProjectGit(p);
                renderDetail();
            })();
        });
    });

    document.querySelectorAll<HTMLElement>('#mode-body [data-files]').forEach(btn => {
        btn.addEventListener('click', () => void showCommitFiles(p, btn.dataset.files!));
    });

    document.querySelectorAll<HTMLElement>('#mode-body [data-sha]').forEach(btn => {
        btn.addEventListener('click', () => {
            const c = state.commits.find(x => x.hash === btn.dataset.sha);
            if (c) openRollback(p, c);
        });
    });

    document.querySelectorAll<HTMLElement>('#mode-body [data-revert]').forEach(btn => {
        btn.addEventListener('click', () => {
            const sha = btn.dataset.revert!;
            confirmDialog({
                title: '撤销这次提交（revert）',
                body: `<div class="commit-line"><code>${esc(sha.slice(0, 7))}</code></div>
                       <div class="hint">会新建一条「反向提交」把这次改动抵消掉，<b>不改写历史</b>，
                       所以原有提交都还在，随时可以再撤销回来。</div>
                       <div class="hint">若有冲突，工具会列出冲突文件，需要去命令行解决。</div>`,
                confirmText: '撤销',
                onConfirm: async () => {
                    toast(await App.GitRevert(p.path, sha));
                    await reloadDetailGit(p);
                },
            });
        });
    });

    document.querySelectorAll<HTMLElement>('#mode-body [data-apply-backup]').forEach(btn => {
        btn.addEventListener('click', () => void withBusy(btn, () => applyBackup(p, btn.dataset.applyBackup!), '应用中…'));
    });

    document.querySelectorAll<HTMLElement>('#mode-body [data-restore-backup]').forEach(btn => {
        btn.addEventListener('click', () => confirmRestoreBackup(p, btn.dataset.restoreBackup!));
    });

    document.querySelectorAll<HTMLElement>('#mode-body [data-drop-backup]').forEach(btn => {
        btn.addEventListener('click', () => confirmDropBackup(p, btn.dataset.dropBackup!, btn.dataset.kind!));
    });
}

/* ---------------- 版本相关动作 ---------------- */

async function doCommit(p: Project): Promise<void> {
    try {
        const msg = `chore: 打点 ${fmtTime(Math.floor(Date.now() / 1000))}`;
        toast(await App.GitCommitAll(p.path, msg));
        await reloadDetailGit(p);
    } catch (e) {
        toast(String(e), 'err');
    }
}

function confirmDiscard(p: Project): void {
    confirmDialog({
        title: '撤销未提交改动',
        body: `<div class="hint">会把工作区所有未提交改动（含新增文件）收进 <code>git stash</code> 后清空工作区。</div>
               <div class="hint">原改动会出现在下方「备份」区，可随时一键找回，因此这一步是可逆的。</div>`,
        confirmText: '撤销',
        danger: true,
        onConfirm: async () => {
            try {
                toast(await App.GitDiscardChanges(p.path));
                await reloadDetailGit(p);
            } catch (e) {
                toast(String(e), 'err');
            }
        },
    });
}

async function applyBackup(p: Project, ref: string): Promise<void> {
    try {
        toast(await App.GitApplyStash(p.path, ref));
        await reloadDetailGit(p);
    } catch (e) {
        toast(String(e), 'err');
    }
}

function confirmRestoreBackup(p: Project, ref: string): void {
    confirmDialog({
        title: '回到该备份',
        body: `<div class="commit-line"><code>${esc(ref)}</code></div>
               <div class="hint">会把项目硬回滚到这个备份标签指向的状态（丢弃其后的提交）。</div>
               <div class="hint">执行前会再自动留一份现场，所以这一步同样可逆。</div>`,
        confirmText: '回到此处',
        danger: true,
        onConfirm: async () => {
            try {
                toast(await App.GitRollback(p.path, ref, true));
                await reloadDetailGit(p);
            } catch (e) {
                toast(String(e), 'err');
            }
        },
    });
}

function confirmDropBackup(p: Project, ref: string, kind: string): void {
    confirmDialog({
        title: kind === 'tag' ? '删除备份标签' : '删除 stash',
        body: `<div class="commit-line"><code>${esc(ref)}</code></div>
               <div class="hint">删除后这份备份就不再可用，此操作不可恢复。</div>`,
        confirmText: '删除',
        danger: true,
        onConfirm: async () => {
            try {
                toast(await App.GitDropBackup(p.path, ref, kind));
                await reloadDetailGit(p);
            } catch (e) {
                toast(String(e), 'err');
            }
        },
    });
}

function openRollback(p: Project, c: Commit): void {
    const root = openModal(`
        <h3>回滚到提交</h3>
        <div class="modal-body">
            <div class="commit-line"><code>${esc(c.short)}</code> ${esc(c.subject)}</div>
            <div class="hint">执行前会自动把当前未提交改动存入 <code>git stash</code>，并给当前提交打一个保底标签——两者都会出现在「备份」区，保证可恢复。</div>
            <label class="radio">
                <input type="radio" name="rb" value="hard" checked />
                <span><b>硬回滚</b>：丢弃该提交之后的所有改动，工作区完全回到该提交状态。</span>
            </label>
            <label class="radio">
                <input type="radio" name="rb" value="soft" />
                <span><b>软回滚</b>：回到该提交，但之后的改动保留在工作区（暂存），由你自行处理。</span>
            </label>
        </div>
        <div class="modal-actions">
            <button class="btn ghost" data-act="cancel">取消</button>
            <button class="btn danger" data-act="ok">执行回滚</button>
        </div>
    `);
    root.querySelector('[data-act="cancel"]')!.addEventListener('click', closeModal);
    root.querySelector('[data-act="ok"]')!.addEventListener('click', async () => {
        const picked = root.querySelector<HTMLInputElement>('input[name="rb"]:checked');
        const hard = picked?.value !== 'soft';
        closeModal();
        try {
            toast(await App.GitRollback(p.path, c.hash, hard));
            await reloadDetailGit(p);
        } catch (e) {
            toast(String(e), 'err');
        }
    });
}

function confirmArchive(p: Project): void {
    const dirtyPick = p.isGit && p.dirty
        ? `<label class="radio"><input type="checkbox" id="af-commit" checked /><span>归档前先打点（把当前未提交改动提交一次）</span></label>`
        : '';
    confirmDialog({
        title: '归档项目',
        body: `<div class="hint">将把 <b>${esc(p.name)}</b> 打包成 zip 存入归档目录，然后删除原目录。</div>
               <div class="commit-line"><code>${esc(p.path)}</code></div>
               ${dirtyPick}
               <div class="hint">之后可在「归档」标签页还原回原路径。</div>`,
        confirmText: '归档',
        danger: true,
        onConfirm: async (root) => {
            const box = root.querySelector<HTMLInputElement>('#af-commit');
            try {
                toast(await App.ArchiveProject(p.path, box ? box.checked : false));
                state.selected = null;
                await Promise.all([refreshProjects(), refreshArchives()]);
                renderDetail();
            } catch (e) {
                toast(String(e), 'err');
            }
        },
    });
}

/* ---------------- 导入 / 实际根目录 ---------------- */

/** 把一个项目根之外的目录加入列表。 */
export async function importProject(target?: HTMLElement): Promise<void> {
	const run = async (): Promise<void> => {
		try {
			const path = await App.ChooseFolder('选择要导入的项目目录');
			if (!path) return;
			toast(await App.ImportProject(path));
			await refreshProjects();
			const added = state.projects.find(p => p.imported && p.entry === path);
			if (added) await selectProject(added);
		} catch (e) {
			toast(String(e), 'err');
		}
	};
	if (target) await withBusy(target, run, '导入中…');
	else await run();
}

/** 指定某个项目真正的仓库目录，用于「真仓库在子目录里」的情况。 */
async function chooseProjectRoot(p: Project): Promise<void> {
	try {
		const chosen = await App.ChooseFolder('选择这个项目真正的仓库目录');
		if (!chosen) return;
		toast(await App.SetProjectRoot(p.entry, chosen));
		await refreshProjects();
		const next = state.projects.find(x => x.entry === p.entry);
		if (next) await selectProject(next);
	} catch (e) {
		toast(String(e), 'err');
	}
}

async function resetProjectRoot(p: Project): Promise<void> {
	try {
		toast(await App.SetProjectRoot(p.entry, ''));
		await refreshProjects();
		const next = state.projects.find(x => x.entry === p.entry);
		if (next) await selectProject(next);
	} catch (e) {
		toast(String(e), 'err');
	}
}

function confirmRemoveImport(p: Project): void {
	confirmDialog({
		title: '移出列表',
		body: `<div class="hint">把 <b>${esc(p.entry)}</b> 移出项目列表。</div>
		       <div class="hint">只删列表记录，<b>磁盘上的文件一个都不动</b>。</div>`,
		confirmText: '移出',
		danger: true,
		onConfirm: async () => {
			try {
				toast(await App.RemoveImportedProject(p.entry));
				state.selected = null;
				await refreshProjects();
				renderDetail();
			} catch (e) {
				toast(String(e), 'err');
			}
		},
	});
}

/* ---------------- 备注名 ---------------- */

function openAliasDialog(p: Project): void {
    promptDialog({
        title: '项目备注名',
        label: '显示名（留空即恢复用目录名）',
        initial: p.alias || '',
        confirmText: '保存',
        allowEmpty: true,
        onSubmit: async (value) => {
            try {
                toast(await App.SetAlias(p.entry, value));
                await refreshProjects();
            } catch (e) {
                toast(String(e), 'err');
            }
        },
    });
}

/* ---------------- 新建项目 ---------------- */

export function openNewProject(): void {
    void (async () => {
        // 先弹窗、后取数据：反过来会出现"一帧没有弹窗"的空档，看起来是闪一下
        const root = openModal(`
        <h3>新建项目</h3>
        <div class="modal-body">
            <label class="field">
                <span>项目名（kebab-case）</span>
                <input id="np-name" class="input" type="text" autocomplete="off" placeholder="my-new-project" />
            </label>
            <div id="np-hint" class="hint">只允许小写字母、数字与连字符</div>
            <div class="hint">位置：<code>${esc(state.cfg.projectRoot)}</code></div>

            <div class="field">
                <span>初始化内容（可逐项勾选）</span>
                <div class="np-checks">
                    <label class="radio"><input type="checkbox" id="np-readme" checked /><span>README.md</span></label>
                    <label class="radio"><input type="checkbox" id="np-gitignore" checked /><span>.gitignore</span></label>
                    <label class="radio"><input type="checkbox" id="np-src" checked /><span>src/ 目录</span></label>
                    <label class="radio"><input type="checkbox" id="np-agents" checked /><span>AGENTS.md（AI 协作与工程化约定）</span></label>
                    <label class="radio"><input type="checkbox" id="np-git" checked /><span>git init 并首次提交</span></label>
                </div>
            </div>

            <div class="field" id="np-tpl-field">
                <span>AGENTS.md 模板</span>
                <div class="np-tplrow">
                    <select id="np-tpl" class="input sort"></select>
                    <button class="btn tiny ghost" type="button" data-act="tpl">管理模板…</button>
                </div>
            </div>

            <label class="field">
                <span>开源协议</span>
                <select id="np-license" class="input sort"></select>
            </label>

            <label class="field" id="np-author-field">
                <span>协议署名（写进 LICENSE）</span>
                <input id="np-author" class="input" type="text" value="${esc(state.cfg.author ?? '')}" placeholder="留空则用项目名" />
            </label>
        </div>
        <div class="modal-actions">
            <button class="btn ghost" data-act="cancel">取消</button>
            <button class="btn primary" data-act="ok" disabled>创建</button>
        </div>
        `);

        const input = root.querySelector('#np-name') as HTMLInputElement;
        const ok = root.querySelector('[data-act="ok"]') as HTMLButtonElement;
        const hint = root.querySelector('#np-hint') as HTMLElement;
        const licSel = root.querySelector('#np-license') as HTMLSelectElement;
        const authorField = root.querySelector('#np-author-field') as HTMLElement;
        const authorInput = root.querySelector('#np-author') as HTMLInputElement;
        const box = (id: string): boolean => (root.querySelector(`#${id}`) as HTMLInputElement).checked;
        const tplSel = root.querySelector('#np-tpl') as HTMLSelectElement;
        const tplField = root.querySelector('#np-tpl-field') as HTMLElement;
        const agentsBox = root.querySelector('#np-agents') as HTMLInputElement;

        const fillTemplates = async (keep?: string): Promise<void> => {
            try {
                const list = (await App.GetAgentTemplates()) ?? [];
                const want = keep ?? tplSel.value;
                tplSel.innerHTML = list
                    .map(t => `<option value="${esc(t.id)}">${esc(t.name)}${t.builtin ? '' : '（我的）'}</option>`)
                    .join('');
                if (want) tplSel.value = want;
            } catch (e) {
                toast(String(e), 'err');
            }
        };
        const syncTplField = (): void => {
            tplField.classList.toggle('hidden', !agentsBox.checked);
        };
        agentsBox.addEventListener('change', syncTplField);
        syncTplField();
        void fillTemplates();

        root.querySelector('[data-act="tpl"]')!.addEventListener('click', () => {
            // 直接叠在新建项目对话框上（弹窗支持层叠），不关掉再重开 —— 那样会闪一下。
            // 管理器关掉后再刷新模板下拉，保证拿到最新列表。
            void openTemplateManager(() => {
                void fillTemplates(tplSel.value);
            });
        });

        const syncAuthor = (): void => {
            authorField.classList.toggle('hidden', licSel.value === '');
        };
        licSel.addEventListener('change', syncAuthor);
        syncAuthor();

        // 协议列表也是异步取回来的，拿到之后再填进下拉
        const fillLicenses = async (): Promise<void> => {
            try {
                const list = (await App.GetLicenseOptions()) ?? [];
                if (list.length) {
                    licSel.innerHTML = list
                        .map(l => `<option value="${esc(l.id)}">${esc(l.name)}</option>`)
                        .join('');
                }
            } catch (e) {
                toast(String(e), 'err');
            }
            syncAuthor();
        };
        void fillLicenses();

        input.addEventListener('input', () => {
            const v = input.value.trim();
            const good = NAME_RE.test(v);
            ok.disabled = !good;
            hint.className = `hint${v && !good ? ' bad' : ''}`;
            hint.textContent = !v
                ? '只允许小写字母、数字与连字符'
                : good ? '✓ 名称合法' : '✗ 只允许小写字母、数字与连字符';
        });

        root.querySelector('[data-act="cancel"]')!.addEventListener('click', closeModal);
        ok.addEventListener('click', () => {
            const opts: CreateOptions = {
                name: input.value.trim(),
                readme: box('np-readme'),
                gitignore: box('np-gitignore'),
                license: licSel.value,
                src: box('np-src'),
                agents: box('np-agents'),
                agentsTemplate: tplSel?.value ?? '',
                gitInit: box('np-git'),
                author: authorInput.value.trim(),
            };
            void withBusy(ok, async () => {
                try {
                    toast(await App.CreateProject(opts));
                    closeModal();
                    await refreshProjects();
                    const p = state.projects.find(x => x.name === opts.name);
                    if (p) await selectProject(p);
                } catch (e) {
                    toast(String(e), 'err');
                }
            }, '创建中…');
        });
        input.focus();
    })();
}
