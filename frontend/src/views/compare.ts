import { App } from '../api';
import { esc } from '../format';
import { state, WORKTREE, type DiffFileStat } from '../state';
import { renderEmpty, renderLoading, withBusy } from '../ui';
import { renderEditorPane } from './editor';

const DIFF_KINDS: Record<string, { label: string; cls: string }> = {
    A: { label: '新增', cls: 'add' },
    M: { label: '修改', cls: 'mod' },
    D: { label: '删除', cls: 'del' },
    R: { label: '重命名', cls: 'ren' },
    U: { label: '冲突', cls: 'del' },
};

/* ---------------- 版本下拉框 ---------------- */

type RefOption = { value: string; label: string; group: string };

/** 可比较的版本只来自分支、标签、最近提交，外加目标侧的「工作区」。 */
function refOptions(includeWorktree: boolean): RefOption[] {
    const opts: RefOption[] = [];
    if (includeWorktree) opts.push({ value: WORKTREE, label: '工作区（未提交改动）', group: '' });
    opts.push({ value: 'HEAD', label: '最新提交（HEAD）', group: '' });
    for (const b of state.branches) {
        opts.push({ value: `refs/heads/${b.name}`, label: b.name, group: '分支' });
    }
    for (const t of state.tags) {
        opts.push({ value: `refs/tags/${t.name}`, label: t.name, group: '标签' });
    }
    for (const c of state.commits) {
        opts.push({ value: c.hash, label: `${c.short} ${c.subject}`, group: '最近提交' });
    }
    return opts;
}

function optionsHtml(includeWorktree: boolean, selected: string): string {
    const groups = new Map<string, RefOption[]>();
    for (const o of refOptions(includeWorktree)) {
        const arr = groups.get(o.group) ?? [];
        arr.push(o);
        groups.set(o.group, arr);
    }
    return [...groups].map(([group, items]) => {
        const body = items.map(o =>
            `<option value="${esc(o.value)}"${o.value === selected ? ' selected' : ''}>${esc(o.label)}</option>`,
        ).join('');
        return group ? `<optgroup label="${esc(group)}">${body}</optgroup>` : body;
    }).join('');
}

function canSwap(): boolean {
    return state.compare.base !== WORKTREE && state.compare.target !== WORKTREE;
}

/**
 * 默认范围：最新提交 相对 上一次提交。
 * 不默认拿工作区来比——工作区常常是干净的，那样一进来就是「没有差异」，等于什么都没看到。
 * 仓库只有一个提交时没有「上一次」，退回「工作区 相对 最新提交」。
 */
function setDefaultSpan(): void {
    const cmp = state.compare;
    const prev = state.commits[1];
    if (prev) {
        cmp.target = 'HEAD';
        cmp.base = prev.hash;
    } else {
        cmp.target = WORKTREE;
        cmp.base = 'HEAD';
    }
}

/* ---------------- 渲染 ---------------- */

function rowHtml(f: DiffFileStat): string {
    const meta = DIFF_KINDS[f.status] ?? { label: f.status, cls: 'mod' };
    let num = '';
    if (f.binary) num = '<span class="cmp-num muted">二进制</span>';
    else if (f.adds < 0 && f.dels < 0) num = '<span class="cmp-num muted">—</span>';
    else num = `<span class="cmp-num"><span class="cmp-add">+${f.adds}</span><span class="cmp-del">−${f.dels}</span></span>`;

    const path = f.oldPath
        ? `${esc(f.oldPath)} <span class="cmp-arrow">→</span> ${esc(f.path)}`
        : esc(f.path);
    return `
    <div class="cmp-file${state.compare.picked === f.path ? ' active' : ''}" data-path="${esc(f.path)}" title="${esc(f.path)}">
        <span class="badge ${meta.cls}">${esc(meta.label)}</span>
        <span class="cmp-path">${path}</span>
        ${num}
    </div>`;
}

function summaryHtml(): string {
    const cmp = state.compare;
    if (cmp.loading) return '<span class="cmp-sum muted">读取中…</span>';
    if (cmp.error) return `<span class="cmp-sum bad">${esc(cmp.error)}</span>`;
    return `<span class="cmp-sum">${cmp.files.length} 个文件变更 · <span class="cmp-add">+${cmp.adds}</span> <span class="cmp-del">−${cmp.dels}</span></span>`;
}

function listHtml(): string {
    const cmp = state.compare;
    if (cmp.loading) return renderLoading();
    if (cmp.error) return '';
    if (!cmp.files.length) {
        return renderEmpty({ title: '这两个版本之间没有差异', hint: '换一组版本试试，或者直接开始改代码' });
    }
    return cmp.files.map(rowHtml).join('');
}

/** 把当前对比状态画进 #mode-body。数据加载走 loadCompare / openCompareFile。 */
export function renderCompareBody(): void {
    const host = document.getElementById('mode-body');
    if (!host || state.mode !== 'compare') return;
    const p = state.selected;
    if (!p) return;
    if (!p.isGit) {
        host.innerHTML = `<div class="hint">该项目不是 git 仓库，对比功能不可用。</div>`;
        return;
    }
    if (!state.commits.length) {
        host.innerHTML = renderEmpty({ title: '还没有任何提交', hint: '先打一次点，才有可以拿来比较的版本' });
        return;
    }

    const cmp = state.compare;
    // 换项目时 renderDetail 会先跑一步，这时手上的清单还属于上一个项目，先别显示
    const ready = cmp.project === p.entry;
    host.innerHTML = `
    <div class="cmp">
        <div class="cmp-bar">
            <select id="cmp-target" class="input sort" title="要看的那一边（比较的终点）">${optionsHtml(true, cmp.target)}</select>
            <button class="btn tiny ghost" data-cmp="swap" ${canSwap() ? '' : 'disabled'}
                    title="${canSwap() ? '交换两边' : '工作区不能当基准，无法交换'}">⇄</button>
            <span class="cmp-label">相对</span>
            <select id="cmp-base" class="input sort" title="拿来当作基准的那一边（比较的起点）">${optionsHtml(false, cmp.base)}</select>
            <div class="cmp-grow"></div>
            <div class="seg">
                <button class="seg-btn ${cmp.full ? '' : 'active'}" data-cmp-full="0">只看改动</button>
                <button class="seg-btn ${cmp.full ? 'active' : ''}" data-cmp-full="1">全部代码</button>
            </div>
            <button class="btn tiny ghost" data-cmp="refresh">刷新</button>
        </div>
        <div class="cmp-sum-row">${ready ? summaryHtml() : ''}</div>
        <div class="cmp-list">${ready ? listHtml() : renderLoading()}</div>
    </div>`;

    wire(host);
}

function wire(host: HTMLElement): void {
    host.querySelector('#cmp-base')?.addEventListener('change', e => {
        state.compare.base = (e.target as HTMLSelectElement).value;
        void changedSpan();
    });
    host.querySelector('#cmp-target')?.addEventListener('change', e => {
        state.compare.target = (e.target as HTMLSelectElement).value;
        void changedSpan();
    });

    host.querySelector('[data-cmp="swap"]')?.addEventListener('click', () => {
        if (!canSwap()) return;
        const { base, target } = state.compare;
        state.compare.base = target;
        state.compare.target = base;
        void changedSpan();
    });

    host.querySelectorAll<HTMLElement>('[data-cmp-full]').forEach(btn => {
        btn.addEventListener('click', () => void toggleFull(btn.dataset.cmpFull === '1'));
    });

    host.querySelector('[data-cmp="refresh"]')?.addEventListener('click', e => {
        const btn = e.currentTarget as HTMLElement;
        void withBusy(btn, async () => {
            await loadCompare();
            if (state.compare.picked) await openCompareFile(state.compare.picked);
        }, '刷新中…');
    });

    host.querySelectorAll<HTMLElement>('.cmp-file').forEach(el => {
        el.addEventListener('click', () => {
            const path = el.dataset.path!;
            // 再点一次同一个文件就收起右栏的 diff
            if (state.compare.picked === path) void clearPicked();
            else void openCompareFile(path);
        });
    });
}

/* ---------------- 数据 ---------------- */

async function changedSpan(): Promise<void> {
    clearPickedState();
    renderCompareBody();
    renderEditorPane();
    await loadCompare();
}

/** 切换「只看改动 / 全部代码」：范围没变，只是把当前文件的 diff 重新取一份。 */
async function toggleFull(full: boolean): Promise<void> {
    const cmp = state.compare;
    if (cmp.full === full) return;
    cmp.full = full;
    cmp.text = '';
    cmp.diffLoading = !!cmp.picked;
    renderCompareBody();
    renderEditorPane();
    if (cmp.picked) await openCompareFile(cmp.picked);
}

function clearPickedState(): void {
    const cmp = state.compare;
    cmp.picked = '';
    cmp.text = '';
    cmp.truncated = false;
    cmp.binary = false;
    cmp.diffLoading = false;
    cmp.diffError = '';
}

async function clearPicked(): Promise<void> {
    clearPickedState();
    renderCompareBody();
    renderEditorPane();
}

/** 拉取 base → target 的变更清单。 */
export async function loadCompare(): Promise<void> {
    const p = state.selected;
    const cmp = state.compare;
    if (!p || !p.isGit) return;
    cmp.loading = true;
    cmp.error = '';
    renderCompareBody();
    renderEditorPane();
    try {
        const summary = await App.GitDiffRefs(p.path, cmp.base, cmp.target);
        cmp.files = summary?.files ?? [];
        cmp.adds = summary?.adds ?? 0;
        cmp.dels = summary?.dels ?? 0;
        // 刷新后原来选中的文件可能已经不在清单里了
        if (cmp.picked && !cmp.files.some(f => f.path === cmp.picked)) clearPickedState();
    } catch (e) {
        cmp.files = [];
        cmp.adds = 0;
        cmp.dels = 0;
        cmp.error = String(e);
        clearPickedState();
    } finally {
        cmp.loading = false;
        renderCompareBody();
        renderEditorPane();
    }
}

/** 进入「对比」模式（或换项目）时的入口。同一项目内来回切模式不会丢掉当前选的版本。 */
export async function openCompare(): Promise<void> {
    const p = state.selected;
    if (!p) return;
    const cmp = state.compare;
    if (cmp.project !== p.entry) {
        cmp.project = p.entry;
        setDefaultSpan();
        cmp.files = [];
        cmp.adds = 0;
        cmp.dels = 0;
        cmp.error = '';
        clearPickedState();
    }
    renderCompareBody();
    renderEditorPane();
    if (!p.isGit || !state.commits.length) return;
    await loadCompare();
}

/** 把某个文件的 diff 取回来，填进右栏。 */
export async function openCompareFile(path: string): Promise<void> {
    const p = state.selected;
    const cmp = state.compare;
    if (!p) return;
    cmp.picked = path;
    cmp.text = '';
    cmp.truncated = false;
    cmp.binary = false;
    cmp.diffError = '';
    cmp.diffLoading = true;
    markPicked();
    renderEditorPane();
    try {
        const res = await App.GitDiffRefFile(p.path, cmp.base, cmp.target, path, cmp.full);
        cmp.text = res?.text ?? '';
        cmp.truncated = res?.truncated ?? false;
        cmp.binary = res?.binary ?? false;
    } catch (e) {
        cmp.diffError = String(e);
    } finally {
        cmp.diffLoading = false;
        renderEditorPane();
    }
}

/** 只改左栏的选中高亮，不整块重绘（避免下拉框失焦）。 */
function markPicked(): void {
    document.querySelectorAll<HTMLElement>('.cmp-file').forEach(el => {
        el.classList.toggle('active', el.dataset.path === state.compare.picked);
    });
}
