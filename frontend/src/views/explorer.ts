import { App, fullPath, persistConfig } from '../api';
import { hooks } from '../bus';
import { esc, fmtAgo, fmtSize, typeLabel } from '../format';
import { fileIcon } from '../icons';
import { state, type DirListing, type FileEntry, type FileSortKey, type ViewMode } from '../state';
import { closeModal, confirmDialog, copyText, openModal, promptDialog, renderEmpty, renderLoading, showContextMenu, toast, withBusy } from '../ui';
import {
    clearPane, confirmDiscardChanges, isDirty, openFileInPane, renderEditorPane, scrollEditorToLine,
} from './editor';

const GIT_LABEL: Record<string, string> = {
    M: '已修改',
    A: '新增',
    D: '已删除',
    R: '重命名',
    '??': '未跟踪',
};

function currentRoot(): string | null {
    return state.selected?.path ?? null;
}

function isCut(entry: FileEntry): boolean {
    const cb = state.file.clipboard;
    return !!cb && cb.mode === 'cut' && cb.paths.includes(entry.path);
}

/* ---------------- 导航 ---------------- */

// 从搜索点进来时，记下要定位的文件与行号，由下一次 openExplorer() 消费。
let pendingReveal: { path: string; line: number } | null = null;

export function requestReveal(path: string, line: number): void {
    pendingReveal = { path, line };
}

export function openExplorer(): void {
    if (!state.selected) return;
    state.file.rel = '';
    state.file.selected = [];
    state.file.anchor = null;
    state.file.filter = '';
    state.file.history = [];
    state.file.historyIndex = -1;
    state.file.showHidden = state.cfg.fileShowHidden;
    state.editor.open = state.cfg.filePreviewOpen;
    state.editor.width = state.cfg.editorWidth || 580;
    state.file.sortKey = (state.cfg.fileSortKey as typeof state.file.sortKey) ?? 'name';

        if (pendingReveal) {
        const { path, line } = pendingReveal;
        pendingReveal = null;
        void reveal(path, line);
        return;
    }
    void load('');
}

/** 打开某个文件所在的目录并选中它；lineNo > 0 时把编辑器滚到那一行。 */
async function reveal(relPath: string, lineNo: number): Promise<void> {
    const dir = relPath.includes('/') ? relPath.slice(0, relPath.lastIndexOf('/')) : '';
    await load(dir);
    await selectEntry(relPath);
    if (lineNo > 0) scrollEditorToLine(lineNo);
}

async function load(rel: string, pushHistory = true): Promise<void> {
    const root = currentRoot();
    if (!root) return;
    state.file.loading = true;
    render();
    try {
        const listing = await App.ListDir(root, rel);
        state.file.listing = listing;
        state.file.rel = listing.relPath;
        state.file.selected = [];
        state.file.anchor = null;
        clearPane();
        if (pushHistory) {
            state.file.history = state.file.history.slice(0, state.file.historyIndex + 1);
            state.file.history.push(listing.relPath);
            state.file.historyIndex = state.file.history.length - 1;
        }
    } catch (e) {
        toast(String(e), 'err');
    } finally {
        state.file.loading = false;
        render();
    }
}

function goBack(): void {
    if (state.file.historyIndex <= 0) return;
    state.file.historyIndex--;
    void load(state.file.history[state.file.historyIndex], false);
}

function goForward(): void {
    if (state.file.historyIndex >= state.file.history.length - 1) return;
    state.file.historyIndex++;
    void load(state.file.history[state.file.historyIndex], false);
}

function goUp(): void {
    const parts = state.file.rel.split('/').filter(Boolean);
    if (!parts.length) return;
    parts.pop();
    void load(parts.join('/'));
}

export async function refresh(): Promise<void> {
    await load(state.file.rel, false);
}

/* ---------------- 选择 ---------------- */

function visibleEntries(): FileEntry[] {
    const listing = state.file.listing;
    if (!listing) return [];
    const kw = state.file.filter.trim().toLowerCase();
    const items = listing.entries.filter(e => {
        if (!state.file.showHidden && e.hidden) return false;
        if (kw && !e.name.toLowerCase().includes(kw)) return false;
        return true;
    });
    const key = state.file.sortKey;
    const dir = state.file.sortDesc ? -1 : 1;
    return items.sort((a, b) => {
        if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
        let cmp: number;
        switch (key) {
            case 'time':
                cmp = a.modTime - b.modTime;
                break;
            case 'size':
                cmp = a.size - b.size;
                break;
            case 'type':
                cmp = (a.ext || a.name).localeCompare(b.ext || b.name) || a.name.localeCompare(b.name);
                break;
            default:
                cmp = a.name.localeCompare(b.name, 'zh-Hans-CN');
        }
        return cmp * dir;
    });
}

/** 点表头/切换排序：同一列再点一次反向。 */
function setSort(key: FileSortKey): void {
    if (state.file.sortKey === key) {
        state.file.sortDesc = !state.file.sortDesc;
    } else {
        state.file.sortKey = key;
        state.file.sortDesc = key === 'time' || key === 'size';
    }
    void persistConfig({ fileSortKey: key }).then(render);
}

function sortArrow(key: FileSortKey): string {
    if (state.file.sortKey !== key) return '';
    return state.file.sortDesc ? ' ▼' : ' ▲';
}

function syncSelection(): void {
    document.querySelectorAll<HTMLElement>('.fx-tile, .fx-row').forEach(el => {
        el.classList.toggle('selected', state.file.selected.includes(el.dataset.path!));
    });
}

async function selectEntry(path: string, e?: MouseEvent): Promise<void> {
    const ctrl = e ? (e.ctrlKey || e.metaKey) : false;
    const shift = e ? e.shiftKey : false;

    if (ctrl) {
        state.file.selected = state.file.selected.includes(path)
            ? state.file.selected.filter(p => p !== path)
            : [...state.file.selected, path];
        state.file.anchor = path;
    } else if (shift && state.file.anchor) {
        const order = visibleEntries().map(x => x.path);
        const a = order.indexOf(state.file.anchor);
        const b = order.indexOf(path);
        if (a >= 0 && b >= 0) {
            const [from, to] = a < b ? [a, b] : [b, a];
            state.file.selected = order.slice(from, to + 1);
        } else {
            state.file.selected = [path];
        }
    } else {
        state.file.selected = [path];
        state.file.anchor = path;
    }

    syncSelection();
    await openSelectedInPane();
}

function selectAll(): void {
    state.file.selected = visibleEntries().map(e => e.path);
    state.file.anchor = state.file.selected[0] ?? null;
    syncSelection();
    void openSelectedInPane();
}

function clearSelection(): void {
    state.file.selected = [];
    state.file.anchor = null;
    syncSelection();
    clearPane();
}

function selectedEntries(): FileEntry[] {
    const listing = state.file.listing;
    if (!listing) return [];
    return state.file.selected
        .map(p => listing.entries.find(e => e.path === p))
        .filter((e): e is FileEntry => e !== undefined);
}

/* ---------------- 拖拽移动 ---------------- */

let dragPaths: string[] = [];

function clearDropHighlight(): void {
    document.querySelectorAll('.drop-target').forEach(el => el.classList.remove('drop-target'));
}

function wireEntry(el: HTMLElement): void {
    el.addEventListener('click', e => void selectEntry(el.dataset.path!, e));
    el.addEventListener('dblclick', () => void openEntry(el.dataset.path!, el.dataset.dir === '1'));

    el.setAttribute('draggable', 'true');
    el.addEventListener('dragstart', e => {
        const path = el.dataset.path!;
        if (!state.file.selected.includes(path)) void selectEntry(path);
        dragPaths = [...state.file.selected];
        e.dataTransfer?.setData('text/plain', dragPaths.join('\n'));
        if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
        el.classList.add('dragging');
    });
    el.addEventListener('dragend', () => {
        el.classList.remove('dragging');
        clearDropHighlight();
        dragPaths = [];
    });
}

/** 把一个位置登记为拖拽落点（文件夹图块、面包屑、上一级按钮）。 */
function wireDropTarget(el: HTMLElement, rel: string): void {
    el.addEventListener('dragover', e => {
        if (!dragPaths.length) return;
        e.preventDefault();
        e.stopPropagation();
        if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
        el.classList.add('drop-target');
    });
    el.addEventListener('dragleave', () => el.classList.remove('drop-target'));
    el.addEventListener('drop', e => {
        e.preventDefault();
        e.stopPropagation();
        el.classList.remove('drop-target');
        void moveInto(rel);
    });
}

async function moveInto(dstRel: string): Promise<void> {
    const root = currentRoot();
    const paths = [...dragPaths];
    dragPaths = [];
    if (!root || !paths.length) return;
    try {
        const conflicts = (await App.CheckConflicts(root, paths, dstRel)) ?? [];
        const run = async (policy: string): Promise<void> => {
            toast(await App.MoveEntries(root, paths, dstRel, policy));
            await load(state.file.rel, false);
        };
        if (conflicts.length) conflictDialog(conflicts, policy => void run(policy));
        else await run('fail');
    } catch (e) {
        toast(String(e), 'err');
    }
}

/** 方向键在网格/表格里移动选中项。 */
async function moveSelection(dx: number, dy: number): Promise<void> {
    const order = visibleEntries().map(e => e.path);
    if (!order.length) return;
    const cur = state.file.selected[0];
    const idx = cur ? order.indexOf(cur) : -1;
    if (idx < 0) {
        await selectEntry(order[0]);
        scrollSelectedIntoView();
        return;
    }
    let cols = 1;
    if (state.cfg.fileViewMode !== 'details') {
        const grid = document.querySelector<HTMLElement>('.fx-grid');
        if (grid) {
            const tpl = getComputedStyle(grid).gridTemplateColumns;
            cols = Math.max(1, tpl.split(' ').filter(Boolean).length);
        }
    }
    let next = idx + dy * cols + dx;
    if (next < 0) next = 0;
    if (next >= order.length) next = order.length - 1;
    await selectEntry(order[next]);
    scrollSelectedIntoView();
}

function scrollSelectedIntoView(): void {
    document.querySelector('.fx-tile.selected, .fx-row.selected')
        ?.scrollIntoView({ block: 'nearest' });
}

/* ---------------- 文件操作 ---------------- */

/** 把当前选中项载入右侧编辑器；有未保存改动时先问一句。 */
async function openSelectedInPane(): Promise<void> {
    const picked = selectedEntries();
    if (picked.length !== 1 || picked[0].isDir) {
        clearPane();
        return;
    }
    const entry = picked[0];
    if (isDirty() && state.editor.path !== entry.path) {
        const ok = await confirmDiscardChanges();
        if (!ok) {
            state.file.selected = state.editor.path ? [state.editor.path] : [];
            syncSelection();
            return;
        }
    }
    await openFileInPane(entry);
}

function conflictDialog(names: string[], onPick: (policy: string) => void): void {
    const root = openModal(`
        <h3>目标已存在</h3>
        <div class="modal-body">
            <div class="hint">以下条目在目标目录里已经存在：</div>
            <div class="commit-line">${names.map(esc).join('、')}</div>
            <div class="hint">覆盖会把旧条目先移入回收站（可恢复），再放入新条目。</div>
        </div>
        <div class="modal-actions">
            <button class="btn ghost" data-act="cancel">取消</button>
            <button class="btn ghost" data-act="rename">保留两者</button>
            <button class="btn danger" data-act="overwrite">覆盖</button>
        </div>
    `);
    root.querySelector('[data-act="cancel"]')!.addEventListener('click', closeModal);
    root.querySelector('[data-act="rename"]')!.addEventListener('click', () => {
        closeModal();
        onPick('rename');
    });
    root.querySelector('[data-act="overwrite"]')!.addEventListener('click', () => {
        closeModal();
        onPick('overwrite');
    });
}

function newFolder(): void {
    const root = currentRoot();
    if (!root) return;
    promptDialog({
        title: '新建文件夹',
        label: '文件夹名称',
        confirmText: '创建',
        onSubmit: async (name) => {
            try {
                const rel = await App.NewFolder(root, state.file.rel, name);
                toast(`已创建 ${rel}`);
                await load(state.file.rel, false);
                const created = state.file.listing?.entries.find(e => e.path === rel);
                if (created) await selectEntry(created.path);
            } catch (e) {
                toast(String(e), 'err');
            }
        },
    });
}

function renameEntry(entry: FileEntry): void {
    const root = currentRoot();
    if (!root) return;
    promptDialog({
        title: '重命名',
        label: '新名称',
        initial: entry.name,
        confirmText: '重命名',
        onSubmit: async (name) => {
            if (name === entry.name) return;
            const clash = (state.file.listing?.entries ?? []).some(
                e => e.path !== entry.path && e.name.toLowerCase() === name.toLowerCase(),
            );
            const run = async (policy: string): Promise<void> => {
                try {
                    const rel = await App.RenameEntry(root, entry.path, name, policy);
                    toast(`已重命名为 ${rel}`);
                    await load(state.file.rel, false);
                } catch (e) {
                    toast(String(e), 'err');
                }
            };
            if (clash) conflictDialog([name], policy => void run(policy));
            else await run('fail');
        },
    });
}

function deleteSelection(): void {
    const root = currentRoot();
    const picked = selectedEntries();
    if (!root || !picked.length) return;
    const names = picked.map(e => e.name);
    const paths = picked.map(e => e.path);
    const project = state.selected;
    const canCommit = !!project?.isGit && !!project.dirty;
    const commitPick = canCommit
        ? `<label class="radio"><input type="checkbox" id="del-commit" checked />
           <span>先打点：把当前未提交改动提交一次，留个 Git 恢复点</span></label>`
        : '';
    confirmDialog({
        title: '删除到回收站',
        body: `<div class="hint">以下 ${names.length} 个条目会被移入系统回收站：</div>
               <div class="commit-line">${names.map(esc).join('、')}</div>
               <div class="hint">回收站里可以找回，因此不是永久删除。</div>
               ${commitPick}`,
        confirmText: '删除',
        danger: true,
        onConfirm: async (rootEl) => {
            const box = rootEl.querySelector<HTMLInputElement>('#del-commit');
            try {
                if (project && box?.checked) {
                    toast(await App.GitCommitAll(project.path, `chore: 删除前打点 ${new Date().toLocaleString('zh-CN')}`));
                }
                toast(await App.DeleteEntries(root, paths));
                await load(state.file.rel, false);
            } catch (e) {
                offerForceDelete(root, paths, String(e));
            }
        },
    });
}

/** 回收站接口搞不定时（典型：嵌套上千层的「路径炸弹」）给用户一个明确说明的出口。 */
function offerForceDelete(root: string, paths: string[], reason: string): void {
    confirmDialog({
        title: '无法移入回收站',
        body: `<div class="commit-line">${esc(reason)}</div>
               <div class="hint">回收站接口不认识超长路径，这类目录通常只能改为<b>永久删除</b>。</div>
               <div class="hint">永久删除<b>不可恢复</b>，也不会留在回收站里。请确认这些内容确实不再需要。</div>`,
        confirmText: '永久删除',
        danger: true,
        onConfirm: async () => {
            try {
                toast(await App.DeleteEntriesForce(root, paths));
                await load(state.file.rel, false);
            } catch (e) {
                toast(String(e), 'err');
            }
        },
    });
}

function cutSelection(): void {
    if (!state.file.selected.length) return;
    state.file.clipboard = { mode: 'cut', paths: [...state.file.selected] };
    toast(`已剪切 ${state.file.selected.length} 个条目`);
    render();
}

function copySelection(): void {
    if (!state.file.selected.length) return;
    state.file.clipboard = { mode: 'copy', paths: [...state.file.selected] };
    toast(`已复制 ${state.file.selected.length} 个条目`);
    render();
}

async function pasteInto(dstDir?: string): Promise<void> {
    const root = currentRoot();
    const cb = state.file.clipboard;
    if (!root || !cb) return;
    const dst = dstDir ?? state.file.rel;

    const run = async (policy: string): Promise<void> => {
        try {
            const res = cb.mode === 'cut'
                ? await App.MoveEntries(root, cb.paths, dst, policy)
                : await App.CopyEntries(root, cb.paths, dst, policy);
            toast(res);
            if (cb.mode === 'cut') state.file.clipboard = null;
            await load(state.file.rel, false);
        } catch (e) {
            toast(String(e), 'err');
        }
    };

    let conflicts: string[] = [];
    if (cb.mode === 'cut') {
        try {
            conflicts = (await App.CheckConflicts(root, cb.paths, dst)) ?? [];
        } catch (e) {
            toast(String(e), 'err');
            return;
        }
    }
    if (conflicts.length) conflictDialog(conflicts, policy => void run(policy));
    else await run('fail');
}

export async function openEntry(path: string, isDir: boolean): Promise<void> {
    if (isDir) {
        await load(path);
        return;
    }
    const root = currentRoot();
    if (!root) return;
    try {
        await App.OpenEditor(fullPath(root, path));
    } catch (e) {
        toast(String(e), 'err');
    }
}

async function openWithSystem(path: string): Promise<void> {
    const root = currentRoot();
    if (!root) return;
    try {
        await App.OpenWithSystem(fullPath(root, path));
    } catch (e) {
        toast(String(e), 'err');
    }
}

function revealHere(): void {
    const root = currentRoot();
    if (!root) return;
    void App.OpenFolder(fullPath(root, state.file.rel)).catch(e => toast(String(e), 'err'));
}

export function copySelectedPath(): void {
    const root = currentRoot();
    const picked = selectedEntries();
    if (!root || !picked.length) return;
    const text = picked.length === 1
        ? fullPath(root, picked[0].path)
        : picked.map(e => fullPath(root, e.path)).join('\n');
    void copyText(text, picked.length === 1 ? '已复制路径' : `已复制 ${picked.length} 条路径`);
}

export function selectedEntry(): FileEntry | null {
    return selectedEntries()[0] ?? null;
}

/* ---------------- 右键菜单 ---------------- */

function entryMenu(entry: FileEntry, x: number, y: number): void {
    if (!state.file.selected.includes(entry.path)) void selectEntry(entry.path);
    const many = state.file.selected.length > 1;
    const label = many ? `${state.file.selected.length} 个条目` : entry.name;
    const items = [
        { label: '打开', onClick: () => void openEntry(entry.path, entry.isDir) },
        { label: '用默认程序打开', onClick: () => void openWithSystem(entry.path) },
        { label: '用系统资源管理器打开此处', onClick: () => revealHere() },
        { sep: true } as const,
        { label: `剪切${many ? ` ${label}` : ''}`, onClick: () => cutSelection() },
        { label: `复制${many ? ` ${label}` : ''}`, onClick: () => copySelection() },
        { label: '复制路径', onClick: () => copySelectedPath() },
        { sep: true } as const,
        ...(many ? [] : [{ label: '重命名', onClick: () => renameEntry(entry) }]),
        { label: many ? `删除这 ${state.file.selected.length} 项（回收站）` : '删除（回收站）', danger: true, onClick: () => deleteSelection() },
    ];
    showContextMenu(x, y, items);
}

function backgroundMenu(x: number, y: number): void {
    const hasClipboard = !!state.file.clipboard;
    showContextMenu(x, y, [
        { label: '新建文件夹', onClick: () => newFolder() },
        {
            label: hasClipboard
                ? `粘贴${state.file.clipboard!.mode === 'cut' ? '（移动）' : ''}`
                : '粘贴',
            onClick: () => void pasteInto(),
        },
        { sep: true },
        { label: '全选', onClick: () => selectAll() },
        { label: '刷新', onClick: () => refresh() },
        { sep: true },
        { label: '用系统资源管理器打开此处', onClick: () => revealHere() },
    ]);
}

/* ---------------- 渲染 ---------------- */

function crumbs(): string {
    const parts = state.file.rel.split('/').filter(Boolean);
    const rootName = state.selected?.name ?? '项目';
    const segs = [`<button class="fx-crumb" data-crumb="">${esc(rootName)}</button>`];
    let acc = '';
    for (const part of parts) {
        acc = acc ? `${acc}/${part}` : part;
        segs.push('<span class="fx-crumb-sep">/</span>');
        segs.push(`<button class="fx-crumb" data-crumb="${esc(acc)}">${esc(part)}</button>`);
    }
    return segs.join('');
}

function gitBadge(e: FileEntry): string {
    const label = GIT_LABEL[e.gitState];
    if (!label) return '';
    const cls = e.gitState === '??' ? 'untracked' : e.gitState === 'D' ? 'del' : 'mod';
    return `<span class="fx-state ${cls}" title="${esc(label)}">${esc(e.gitState === '??' ? '?' : e.gitState)}</span>`;
}

function renderGrid(entries: FileEntry[]): string {
    if (!entries.length) return renderEmpty({ title: '这个目录是空的', hint: '没有可显示的内容' });
    return `<div class="fx-grid">${entries.map(e => `
        <div class="fx-tile ${state.file.selected.includes(e.path) ? 'selected' : ''} ${isCut(e) ? 'cut' : ''}"
             data-path="${esc(e.path)}" data-dir="${e.isDir ? '1' : '0'}"
             title="${esc(e.name)}${e.gitState ? ' · ' + (GIT_LABEL[e.gitState] ?? '') : ''}">
            <div class="fx-icon">${fileIcon(e)}</div>
            <div class="fx-name">${esc(e.name)}</div>
            <div class="fx-tile-meta">${e.isDir ? '' : esc(fmtSize(e.size))}</div>
            ${gitBadge(e)}
        </div>
    `).join('')}</div>`;
}

function renderTable(entries: FileEntry[]): string {
    if (!entries.length) return renderEmpty({ title: '这个目录是空的', hint: '没有可显示的内容' });
    return `
    <table class="fx-table">
        <thead><tr>
            <th data-sort="name">名称${sortArrow('name')}</th>
            <th data-sort="type">类型${sortArrow('type')}</th>
            <th data-sort="size" class="num">大小${sortArrow('size')}</th>
            <th data-sort="time">修改时间${sortArrow('time')}</th>
            <th>状态</th>
        </tr></thead>
        <tbody>
        ${entries.map(e => `
            <tr class="fx-row ${state.file.selected.includes(e.path) ? 'selected' : ''} ${isCut(e) ? 'cut' : ''}"
                data-path="${esc(e.path)}" data-dir="${e.isDir ? '1' : '0'}">
                <td class="fx-cell-name"><span class="fx-icon-sm">${fileIcon(e)}</span>${esc(e.name)}</td>
                <td class="muted">${esc(typeLabel(e.isDir, e.ext))}</td>
                <td class="num muted">${e.isDir ? '—' : esc(fmtSize(e.size))}</td>
                <td class="muted">${esc(fmtAgo(e.modTime))}</td>
                <td>${gitBadge(e)}</td>
            </tr>
        `).join('')}
        </tbody>
    </table>`;
}

function render(): void {
    const host = document.querySelector<HTMLElement>('#mode-body');
    if (!host || state.mode !== 'files') return;

    const listing: DirListing | null = state.file.listing;
    const entries = visibleEntries();
    const view: ViewMode = state.cfg.fileViewMode === 'details' ? 'details' : 'icons';
    const canBack = state.file.historyIndex > 0;
    const canForward = state.file.historyIndex < state.file.history.length - 1;
    const canUp = state.file.rel !== '';
    const clipboard = state.file.clipboard;

    // 编辑器栏是 #app 那一层的通高右栏，不在这里建；这里只管文件区
    if (!host.querySelector('.fx')) {
        host.innerHTML = `
        <div class="fx">
            <div class="fx-body">
                <div class="fx-left" id="fx-left"></div>
            </div>
        </div>`;
    }

    const left = host.querySelector<HTMLElement>('#fx-left');
    if (!left) return;
    left.innerHTML = `
                <div class="fx-toolbar">
                    <div class="fx-nav">
                        <button class="btn tiny ghost" data-nav="back" ${canBack ? '' : 'disabled'} title="后退（Alt+←）">←</button>
                        <button class="btn tiny ghost" data-nav="forward" ${canForward ? '' : 'disabled'} title="前进（Alt+→）">→</button>
                        <button class="btn tiny ghost" data-nav="up" ${canUp ? '' : 'disabled'} title="上一级（Backspace）">↑</button>
                        <button class="btn tiny ghost" data-nav="refresh" title="刷新">刷新</button>
                    </div>
                    <div class="fx-crumbs">${crumbs()}</div>
                    <div class="fx-view">
                        <div class="seg">
                            <button class="seg-btn ${view === 'icons' ? 'active' : ''}" data-view="icons">大图标</button>
                            <button class="seg-btn ${view === 'details' ? 'active' : ''}" data-view="details">详细信息</button>
                        </div>
                        <button class="btn tiny ghost" data-act="toggle-preview" ${state.editor.open ? 'active' : ''}
                                title="显示 / 隐藏右侧编辑器">编辑器</button>
                    </div>
                </div>
                <div class="fx-actions">
                    <button class="btn tiny ghost" data-act="new-folder">新建文件夹</button>
                    <div class="fx-filter-wrap">
                        <input id="fx-filter" class="input fx-filter" type="text" placeholder="筛选当前目录…" value="${esc(state.file.filter)}"/>
                        <button id="fx-filter-clear" class="input-clear${state.file.filter ? '' : ' hidden'}" title="清除筛选">✕</button>
                    </div>
                    <select id="fx-sort" class="input sort" title="排序方式">
                        <option value="name" ${state.file.sortKey === 'name' ? 'selected' : ''}>名称</option>
                        <option value="time" ${state.file.sortKey === 'time' ? 'selected' : ''}>修改时间</option>
                        <option value="size" ${state.file.sortKey === 'size' ? 'selected' : ''}>大小</option>
                        <option value="type" ${state.file.sortKey === 'type' ? 'selected' : ''}>类型</option>
                    </select>
                    <label class="check"><input type="checkbox" id="fx-hidden" ${state.file.showHidden ? 'checked' : ''}/> 显示隐藏项</label>
                    <div class="fx-grow"></div>
                    <button class="btn tiny ghost" data-act="reveal" title="用系统资源管理器打开当前目录">资源管理器</button>
                </div>
                <div class="fx-status">
                    ${state.file.selected.length ? `已选中 ${state.file.selected.length} 项` : `${entries.length} 项`}
                    ${clipboard ? `<span class="fx-clip">剪贴板：${clipboard.mode === 'cut' ? '剪切' : '复制'} ${clipboard.paths.length} 项（Ctrl+V 粘贴）</span>` : ''}
                </div>
                <div class="fx-content">
                    ${state.file.loading ? renderLoading() : view === 'icons' ? renderGrid(entries) : renderTable(entries)}
                    ${listing?.truncated ? `<div class="hint fx-note">条目过多，仅显示前 ${listing.entries.length} 项</div>` : ''}
                    ${listing && !listing.gitAvailable ? `<div class="hint fx-note">该项目不是 git 仓库，不显示变更状态</div>` : ''}
                </div>`;

    wire(left);
    renderEditorPane();
}

function wire(host: HTMLElement): void {
    host.querySelectorAll<HTMLElement>('[data-nav]').forEach(btn => {
        btn.addEventListener('click', () => {
            switch (btn.dataset.nav) {
                case 'back': goBack(); break;
                case 'forward': goForward(); break;
                case 'up': goUp(); break;
                default: void withBusy(btn, refresh);
            }
        });
    });

    host.querySelectorAll<HTMLElement>('[data-crumb]').forEach(btn => {
        btn.addEventListener('click', () => void load(btn.dataset.crumb ?? ''));
        wireDropTarget(btn, btn.dataset.crumb ?? '');
    });

    // 上一级按钮也是落点
    const upBtn = host.querySelector<HTMLElement>('[data-nav="up"]');
    if (upBtn && state.file.rel !== '') {
        const parts = state.file.rel.split('/').filter(Boolean);
        parts.pop();
        wireDropTarget(upBtn, parts.join('/'));
    }

    host.querySelectorAll<HTMLElement>('[data-view]').forEach(btn => {
        btn.addEventListener('click', () => {
            void persistConfig({ fileViewMode: btn.dataset.view as ViewMode }).then(render);
        });
    });

    host.querySelectorAll<HTMLElement>('[data-sort]').forEach(th => {
        th.addEventListener('click', () => setSort(th.dataset.sort as FileSortKey));
    });

    host.querySelector('#fx-sort')?.addEventListener('change', e => {
        const key = (e.target as HTMLSelectElement).value as FileSortKey;
        if (state.file.sortKey === key) return;
        state.file.sortKey = key;
        state.file.sortDesc = key === 'time' || key === 'size';
        void persistConfig({ fileSortKey: key }).then(render);
    });

    host.querySelector('#fx-filter')?.addEventListener('input', e => {
        state.file.filter = (e.target as HTMLInputElement).value;
        host.querySelector('#fx-filter-clear')?.classList.toggle('hidden', !state.file.filter);
        renderListOnly();
    });

    host.querySelector('#fx-filter-clear')?.addEventListener('click', () => {
        state.file.filter = '';
        render();
    });

    host.querySelector('#fx-hidden')?.addEventListener('change', e => {
        state.file.showHidden = (e.target as HTMLInputElement).checked;
        void persistConfig({ fileShowHidden: state.file.showHidden }).then(render);
    });

    host.querySelector('[data-act="new-folder"]')?.addEventListener('click', () => newFolder());
    host.querySelector('[data-act="reveal"]')?.addEventListener('click', () => revealHere());

    host.querySelector('[data-act="toggle-preview"]')?.addEventListener('click', () => {
        state.editor.open = !state.editor.open;
        void persistConfig({ filePreviewOpen: state.editor.open });
        render();
    });

    host.querySelectorAll<HTMLElement>('.fx-tile, .fx-row').forEach(el => {
        wireEntry(el);
        if (el.dataset.dir === '1') wireDropTarget(el, el.dataset.path!);
    });

    host.querySelector<HTMLElement>('.fx')?.addEventListener('contextmenu', e => {
        e.preventDefault();
        const target = e.target as HTMLElement;
        const el = target.closest('.fx-tile, .fx-row') as HTMLElement | null;
        if (el) {
            const entry = state.file.listing?.entries.find(x => x.path === el.dataset.path);
            if (entry) entryMenu(entry, e.clientX, e.clientY);
            return;
        }
        if (target.closest('.fx-content, .fx-crumbs')) backgroundMenu(e.clientX, e.clientY);
    });

    host.querySelector<HTMLElement>('.fx-content')?.addEventListener('click', e => {
        const el = (e.target as HTMLElement).closest('.fx-tile, .fx-row');
        if (!el) clearSelection();
    });

    host.querySelector<HTMLElement>('.fx-crumbs')?.addEventListener('contextmenu', e => e.preventDefault());
}

/** 只重画列表区域，避免筛选输入时整块重建导致输入框失焦。 */
function renderListOnly(): void {
    const content = document.querySelector<HTMLElement>('.fx-content');
    const status = document.querySelector<HTMLElement>('.fx-status');
    if (!content) return;
    const view: ViewMode = state.cfg.fileViewMode === 'details' ? 'details' : 'icons';
    const entries = visibleEntries();
    content.innerHTML = view === 'icons' ? renderGrid(entries) : renderTable(entries);
    if (status) {
        status.innerHTML = state.file.selected.length
            ? `已选中 ${state.file.selected.length} 项`
            : `${entries.length} 项`;
    }
    content.querySelectorAll<HTMLElement>('.fx-tile, .fx-row').forEach(el => {
        wireEntry(el);
        if (el.dataset.dir === '1') wireDropTarget(el, el.dataset.path!);
    });
}

/** 供详情外壳重建后重新填充文件区（projects.ts 切换模式时调用）。 */
export function renderExplorerBody(): void {
    render();
}

/* ---------------- 键盘快捷键 ---------------- */

function openModalVisible(): boolean {
    return (document.getElementById('modal-root')?.childElementCount ?? 0) > 0;
}

document.addEventListener('keydown', e => {
    if (state.mode !== 'files' || state.view !== 'projects') return;
    if (openModalVisible()) return;
    const t = e.target as HTMLElement | null;
    if (t && (t.tagName === 'INPUT' || t.tagName === 'SELECT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;

    const ctrl = e.ctrlKey || e.metaKey;
    if (ctrl && e.key.toLowerCase() === 'a') {
        e.preventDefault();
        selectAll();
        return;
    }
    if (ctrl && e.key.toLowerCase() === 'c') {
        e.preventDefault();
        copySelection();
        return;
    }
    if (ctrl && e.key.toLowerCase() === 'x') {
        e.preventDefault();
        cutSelection();
        return;
    }
    if (ctrl && e.key.toLowerCase() === 'v') {
        e.preventDefault();
        void pasteInto();
        return;
    }

    switch (e.key) {
        case 'Backspace':
            e.preventDefault();
            goUp();
            break;
        case 'ArrowUp':
            e.preventDefault();
            void moveSelection(0, -1);
            break;
        case 'ArrowDown':
            e.preventDefault();
            void moveSelection(0, 1);
            break;
        case 'ArrowLeft':
            e.preventDefault();
            if (e.altKey) goBack();
            else void moveSelection(-1, 0);
            break;
        case 'ArrowRight':
            e.preventDefault();
            if (e.altKey) goForward();
            else void moveSelection(1, 0);
            break;
        case 'F2': {
            const picked = selectedEntries();
            if (picked.length === 1) {
                e.preventDefault();
                renameEntry(picked[0]);
            }
            break;
        }
        case 'Delete': {
            if (state.file.selected.length) {
                e.preventDefault();
                deleteSelection();
            }
            break;
        }
        case 'Enter': {
            const picked = selectedEntries();
            if (picked.length === 1) {
                e.preventDefault();
                void openEntry(picked[0].path, picked[0].isDir);
            }
            break;
        }
        case 'Escape':
            e.preventDefault();
            hooks.setMode('version');
            break;
        default:
            break;
    }
});
