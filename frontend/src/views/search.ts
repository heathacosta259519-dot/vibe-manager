import { App } from '../api';
import { hooks } from '../bus';
import { esc } from '../format';
import { state, type SearchHit } from '../state';
import { renderHero, renderLoading, toast } from '../ui';
import { requestReveal } from './explorer';
import { selectProject } from './projects';

const GLOBAL_LIMIT = 300;

export function renderSearch(): void {
    const host = document.getElementById('search-view');
    if (!host) return;
    const s = state.gs;

    host.innerHTML = `
    <div class="gs">
        <div class="gs-bar">
            <input id="gs-input" class="input gs-input" type="text" autocomplete="off"
                   placeholder="跨项目搜索：项目名 / 提交信息 / 文件名 / 文件内容…"
                   value="${esc(s.query)}"/>
            <button class="btn primary" id="gs-run" ${s.running ? 'disabled' : ''}>搜索</button>
            ${s.running ? '<button class="btn ghost" id="gs-stop">停止</button>' : ''}
        </div>
        <div class="hint gs-meta">${metaText()}</div>
        <div class="gs-results">${renderResults()}</div>
    </div>`;

    const input = host.querySelector('#gs-input') as HTMLInputElement;
    input.addEventListener('input', () => {
        state.gs.query = input.value;
    });
    input.addEventListener('keydown', e => {
        if (e.key === 'Enter') void runSearch();
    });
    host.querySelector('#gs-run')?.addEventListener('click', () => void runSearch());
    host.querySelector('#gs-stop')?.addEventListener('click', () => {
        state.gs.token++;
        state.gs.running = false;
        renderSearch();
    });

    host.querySelectorAll<HTMLElement>('.gs-hit').forEach(el => {
        el.addEventListener('click', () => void jump(el));
    });

    // 从搜索视图切回来时把光标放回输入框
    if (state.gs.running === false && !s.hits.length) input.focus();
}

function metaText(): string {
    const s = state.gs;
    if (s.running) return '搜索中…（内容搜索会遍历项目文件，稍等）';
    if (!s.query.trim()) return '输入关键词后回车开始搜索';
    if (!s.hits.length) return '没有命中';
    const counts = { project: 0, commit: 0, file: 0, content: 0 } as Record<string, number>;
    for (const h of s.hits) counts[h.kind] = (counts[h.kind] ?? 0) + 1;
    const parts = [
        counts.project ? `项目名 ${counts.project}` : '',
        counts.commit ? `提交 ${counts.commit}` : '',
        counts.file ? `文件名 ${counts.file}` : '',
        counts.content ? `内容 ${counts.content}` : '',
    ].filter(Boolean);
    return `共 ${s.hits.length} 条：${parts.join(' · ')}${s.truncated ? '（已达上限，结果被截断）' : ''}`;
}

function renderResults(): string {
    const s = state.gs;
    if (s.running && !s.hits.length) return renderLoading('搜索中…');
    if (!s.hits.length) {
        return s.query.trim()
            ? renderHero({ title: '没有找到', hint: '换个关键词试试；内容搜索会遍历项目文件，范围可以放宽一点' })
            : renderHero({
                title: '想找什么？',
                hint: '一次搜项目名、提交信息、文件名与文件内容；回车开始',
            });
    }

    const groups = new Map<string, SearchHit[]>();
    for (const h of s.hits) {
        const list = groups.get(h.project) ?? [];
        list.push(h);
        groups.set(h.project, list);
    }
    return [...groups.entries()].map(([project, hits]) => `
        <div class="gs-project">
            <div class="gs-project-name">${esc(project)}<span class="gs-count">${hits.length}</span></div>
            <div class="gs-hits">${hits.map(hitRow).join('')}</div>
        </div>`).join('');
}

function hitRow(h: SearchHit): string {
    const attrs = `data-kind="${h.kind}" data-project="${esc(h.project)}" data-path="${esc(h.path)}" data-line="${h.lineNo}"`;
    switch (h.kind) {
        case 'project':
            return `<div class="gs-hit" ${attrs}>
                <span class="badge task">项目</span>
                <span class="gs-main">${esc(h.project)}</span>
            </div>`;
        case 'commit':
            return `<div class="gs-hit" ${attrs}>
                <span class="badge ahead">提交</span>
                <code>${esc(h.detail)}</code>
                <span class="gs-main">${highlightQuery(h.line)}</span>
            </div>`;
        case 'file':
            return `<div class="gs-hit" ${attrs}>
                <span class="badge">文件</span>
                <span class="gs-main">${highlightQuery(h.path)}</span>
            </div>`;
        default:
            return `<div class="gs-hit" ${attrs}>
                <span class="badge">内容</span>
                <span class="gs-main">
                    <span class="gs-path">${highlightQuery(h.path)}:${h.lineNo}</span>
                    <span class="gs-line">${highlightQuery(h.line)}</span>
                </span>
            </div>`;
    }
}

export function highlightQuery(text: string): string {
    const q = state.gs.query.trim();
    if (!q) return esc(text);
    const haystack = text.toLowerCase();
    const needle = q.toLowerCase();
    let out = '';
    let i = 0;
    for (;;) {
        const at = haystack.indexOf(needle, i);
        if (at < 0) {
            out += esc(text.slice(i));
            break;
        }
        out += esc(text.slice(i, at)) + '<mark>' + esc(text.slice(at, at + needle.length)) + '</mark>';
        i = at + needle.length;
    }
    return out;
}

export async function runSearch(): Promise<void> {
    const s = state.gs;
    const q = s.query.trim();
    if (!q) {
        s.hits = [];
        renderSearch();
        return;
    }
    s.token++;
    const token = s.token;
    s.running = true;
    renderSearch();
    try {
        const hits = (await App.SearchAll(q)) ?? [];
        if (token !== s.token) return; // 已被新的搜索取代
        s.hits = hits;
        s.truncated = hits.length >= GLOBAL_LIMIT;
    } catch (e) {
        if (token === s.token) {
            s.hits = [];
            toast(String(e), 'err');
        }
    } finally {
        if (token === s.token) {
            s.running = false;
            renderSearch();
        }
    }
}

async function jump(el: HTMLElement): Promise<void> {
    const kind = el.dataset.kind ?? '';
    const projectName = el.dataset.project ?? '';
    const path = el.dataset.path ?? '';
    const line = Number(el.dataset.line ?? '0');

    const project = state.projects.find(p => p.name === projectName);
    if (!project) {
        toast('该项目不在当前列表里，请先刷新');
        return;
    }

    hooks.setView('projects');

    if (kind === 'content' || kind === 'file') {
        requestReveal(path, line);
        await selectProject(project);
        if (state.mode !== 'files') hooks.setMode('files');
        return;
    }
    await selectProject(project);
    if (kind === 'commit' && state.mode !== 'version') hooks.setMode('version');
}
