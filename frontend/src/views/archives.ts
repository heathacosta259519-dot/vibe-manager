import { App } from '../api';
import { hooks } from '../bus';
import { esc, fmtSize, fmtTime } from '../format';
import { state, type ArchiveEntry } from '../state';
import { confirmDialog, renderHero, toast } from '../ui';
import { refreshArchives, refreshProjects } from './data';

function sortArchives(items: ArchiveEntry[]): ArchiveEntry[] {
    return items.slice().sort((a, b) => {
        switch (state.archiveSort) {
            case 'name':
                return a.name.localeCompare(b.name);
            case 'size':
                return b.size - a.size;
            default:
                return b.archivedAt - a.archivedAt;
        }
    });
}

export function renderArchives(): void {
    const el = document.getElementById('archives-view');
    if (!el) return;

    const countEl = document.getElementById('count-archives');
    if (countEl) countEl.textContent = state.archives.length ? String(state.archives.length) : '';
    hooks.status();

    if (!state.archives.length) {
        el.innerHTML = renderHero({
            title: '归档区还是空的',
            hint: '在项目详情里点「更多… → 归档项目」，会把整个目录打包成 zip 放进这里，随时能还原',
        });
        return;
    }
    el.innerHTML = `
        <div class="archive-head">
            <h3>归档区（${state.archives.length}）</h3>
            <div class="hint">归档以 zip 存放于归档目录，可还原回原路径</div>
            <div class="sort-row">
                <span class="hint">排序</span>
                <select id="archive-sort" class="input sort">
                    <option value="time">归档时间</option>
                    <option value="name">名称</option>
                    <option value="size">体积</option>
                </select>
            </div>
        </div>
        <div class="archive-list">
            ${sortArchives(state.archives).map(a => `
                <div class="archive-item">
                    <div class="archive-main">
                        <div class="proj-name">${esc(a.name)}</div>
                        <div class="commit-meta">归档于 ${fmtTime(a.archivedAt)} · ${fmtSize(a.size)}</div>
                        <div class="path small">${esc(a.originalPath)}</div>
                    </div>
                    <button class="btn ghost" data-restore="${esc(a.zipPath)}">还原</button>
                    <button class="btn ghost danger-text" data-delete="${esc(a.zipPath)}">删除</button>
                </div>
            `).join('')}
        </div>`;

    const sel = el.querySelector('#archive-sort') as HTMLSelectElement;
    sel.value = state.archiveSort;
    sel.addEventListener('change', () => {
        state.archiveSort = sel.value as typeof state.archiveSort;
        renderArchives();
    });

    el.querySelectorAll<HTMLElement>('[data-restore]').forEach(btn => {
        btn.addEventListener('click', () => {
            const zip = btn.dataset.restore!;
            const entry = state.archives.find(a => a.zipPath === zip);
            if (!entry) return;
            confirmDialog({
                title: '还原归档',
                body: `<div class="hint">将把 <b>${esc(entry.name)}</b> 解压回原路径：</div>
                       <div class="commit-line"><code>${esc(entry.originalPath)}</code></div>
                       <div class="hint">若该路径已被占用，还原会失败。</div>`,
                confirmText: '还原',
                onConfirm: async () => {
                    try {
                        toast(await App.RestoreArchive(entry.zipPath));
                        await Promise.all([refreshProjects(), refreshArchives()]);
                    } catch (e) {
                        toast(String(e), 'err');
                    }
                },
            });
        });
    });

    el.querySelectorAll<HTMLElement>('[data-delete]').forEach(btn => {
        btn.addEventListener('click', () => {
            const zip = btn.dataset.delete!;
            const entry = state.archives.find(a => a.zipPath === zip);
            if (!entry) return;
            confirmDialog({
                title: '删除归档',
                body: `<div class="hint">将删除 <b>${esc(entry.name)}</b> 的归档 zip 与索引记录。</div>
                       <div class="commit-line"><code>${esc(entry.zipPath)}</code></div>
                       <div class="hint">此操作不可恢复——如果还想留着，请改用「还原」把它解回项目目录。</div>`,
                confirmText: '删除',
                danger: true,
                onConfirm: async () => {
                    try {
                        toast(await App.DeleteArchive(entry.zipPath));
                        await refreshArchives();
                    } catch (e) {
                        toast(String(e), 'err');
                    }
                },
            });
        });
    });
}
