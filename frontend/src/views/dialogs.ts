import { App, openInEditor } from '../api';
import { esc, fmtSize, renderDiffText } from '../format';
import { renderMarkdown } from '../markdown';
import { type AgentTemplate, type FileChange, type Project, type UpdateInfo } from '../state';
import { closeModal, confirmDialog, openModal, renderEmpty, renderLoading, toast, withBusy } from '../ui';

const FILE_KINDS: Record<string, { label: string; cls: string }> = {
    A: { label: '新增', cls: 'add' },
    M: { label: '修改', cls: 'mod' },
    D: { label: '删除', cls: 'del' },
    R: { label: '重命名', cls: 'ren' },
    C: { label: '复制', cls: 'ren' },
    T: { label: '类型', cls: 'mod' },
};

/** 提交详情：左边列出这次改动的文件，右边看选中文件具体改了什么代码。 */
export async function showCommitFiles(p: Project, sha: string): Promise<void> {
    // 先弹窗再取数据：反过来会出现"一帧没有弹窗"的空档，看起来是闪一下
    const root = openModal(`
        <h3>提交 ${esc(sha.slice(0, 7))} 的变更</h3>
        <div class="modal-body cf">
            <div class="cf-list" id="cf-list">${renderLoading()}</div>
            <div class="cf-diff" id="cf-diff"><div class="cf-diff-body"><div class="empty small">左边选一个文件，这里显示它改了什么</div></div></div>
        </div>
        <div class="modal-actions"><button class="btn primary" data-act="ok">关闭</button></div>
    `);
    root.classList.add('modal-wide');
    root.querySelector('[data-act="ok"]')!.addEventListener('click', closeModal);

    const listEl = root.querySelector('#cf-list') as HTMLElement;
    const diffEl = root.querySelector('#cf-diff') as HTMLElement;

    let files: FileChange[] = [];
    try {
        files = (await App.GitCommitFiles(p.path, sha)) ?? [];
    } catch (e) {
        listEl.innerHTML = `<div class="empty small">读取失败：${esc(String(e))}</div>`;
        return;
    }
    if (!files.length) {
        listEl.innerHTML = '';
        diffEl.innerHTML = renderEmpty({ title: '这次提交没有文件变更' });
        return;
    }

    listEl.innerHTML = files.map(f => {
        const k = FILE_KINDS[f.status] ?? { label: f.status, cls: 'mod' };
        // 重命名显示成「旧 → 新」，但取 diff 用的是新路径
        const label = f.oldPath ? `${f.oldPath} → ${f.path}` : f.path;
        return `<div class="cf-item" data-path="${esc(f.path)}" title="${esc(f.path)}">
            <span class="badge ${k.cls}">${esc(k.label)}</span>
            <span class="file-path">${esc(label)}</span>
        </div>`;
    }).join('');

    // 右栏随时能用「外部」打开：外部编辑器只能开磁盘上的文件，所以开的是工作区那一份
    let pickedPath = '';
    diffEl.addEventListener('click', e => {
        if (!(e.target as HTMLElement).closest('[data-cf-ext]')) return;
        void openInEditor(pickedPath).catch(err => toast(String(err), 'err'));
    });

    const paneHtml = (path: string, inner: string, notes: string[] = []): string => `
        <div class="ed-head">
            <span class="ed-name" title="${esc(path)}">${esc(path)}</span>
            <button class="btn tiny ghost" data-cf-ext="1" title="用外部编辑器打开工作区里的这个文件">外部</button>
        </div>
        ${notes.length ? `<div class="ed-note">${esc(notes.join('；'))}</div>` : ''}
        <div class="cf-diff-body">${inner}</div>`;

    const pick = async (el: HTMLElement): Promise<void> => {
        const path = el.dataset.path!;
        pickedPath = path;
        listEl.querySelectorAll<HTMLElement>('.cf-item').forEach(x => x.classList.toggle('active', x === el));
        diffEl.innerHTML = paneHtml(path, renderLoading());

        const notes: string[] = [];
        let inner: string;
        try {
            const res = await App.GitDiffCommitFile(p.path, sha, path);
            if (res.binary) notes.push('二进制文件，只能看出内容变了');
            if (res.truncated) notes.push('diff 过长，只显示了前面一段');
            inner = renderDiffText(res.text);
        } catch (e) {
            inner = `<div class="empty small">${esc(String(e))}</div>`;
        }
        diffEl.innerHTML = paneHtml(path, inner, notes);
    };

    listEl.querySelectorAll<HTMLElement>('.cf-item').forEach(el => {
        el.addEventListener('click', () => void pick(el));
    });

    // 直接把第一个文件展开，省得还要多点一下
    const first = listEl.querySelector<HTMLElement>('.cf-item');
    if (first) void pick(first);
}

/* ---------------- 更新 ---------------- */

/**
 * 发现新版本时的弹窗：说清新旧版本与体积，确认后下载、替换并自动重启。
 * 下载与校验都在后端完成，界面这里只管确认和反馈。
 */
export function openUpdateDialog(info: UpdateInfo): void {
    const root = openModal(`
        <h3>发现新版本 v${esc(info.latest)}</h3>
        <div class="modal-body">
            <div class="hint">当前 v${esc(info.current)}${
                info.size > 0 ? ` · 安装包 ${esc(fmtSize(info.size))}` : ''
            }</div>
            ${
                info.notes
                    ? `<div class="upd-notes">${renderMarkdown(info.notes)}</div>`
                    : '<div class="hint">这次发布没有写说明。</div>'
            }
            <div class="hint">下载并校验完成后，程序会关闭、替换文件，然后自动重新打开。</div>
        </div>
        <div class="modal-actions">
            <button class="btn ghost" data-act="cancel">稍后</button>
            <button class="btn primary" data-act="ok">下载并重启</button>
        </div>
    `);

    root.querySelector('[data-act="cancel"]')!.addEventListener('click', () => closeModal());

    const ok = root.querySelector('[data-act="ok"]') as HTMLButtonElement;
    ok.addEventListener('click', () => {
        void withBusy(ok, async () => {
            try {
                // 成功时后端会在一秒内退出程序，这条提示是它关掉前最后看到的东西
                toast(await App.ApplyUpdate());
            } catch (e) {
                toast(String(e), 'err');
            }
        });
    });
}

/* ---------------- AGENTS.md 模板管理 ---------------- */

/**
 * 打开模板管理弹窗。
 * onClose 会在弹窗被关掉时回调（点关闭 / 按 Esc / 点背景都算），
 * 调用方靠它判断"管理器用完了"，再决定后续动作。
 */
export async function openTemplateManager(onClose?: () => void): Promise<void> {
    // 关键：先同步把窗口弹出来，再异步去读模板列表。
    // 反过来写（先 await 再 openModal）会出现"一帧没有任何弹窗"的空档，看起来就是闪一下。
    const root = openModal(`
        <h3>AGENTS.md 模板</h3>
        <div class="modal-body tm">
            <aside class="tm-side">
                <div class="tm-side-head">模板列表</div>
                <div class="tm-list" id="tm-list"></div>
                <div class="tm-btns">
                    <button class="btn tiny ghost" data-act="new">新建</button>
                    <button class="btn tiny ghost" data-act="copy">另存为</button>
                    <button class="btn tiny ghost danger-text" data-act="del">删除</button>
                </div>
            </aside>
            <section class="tm-main">
                <label class="field">
                    <span>模板名</span>
                    <input id="tm-name" class="input" type="text" autocomplete="off" />
                </label>
                <label class="field tm-content">
                    <span>内容（<code>{{NAME}}</code> 会被替换成项目名）</span>
                    <textarea id="tm-body" class="input tm-body" spellcheck="false"></textarea>
                </label>
                <div class="hint" id="tm-hint">读取中…</div>
            </section>
        </div>
        <div class="modal-actions">
            <button class="btn ghost" data-act="close">关闭</button>
            <button class="btn primary" data-act="save" disabled>保存</button>
        </div>
    `, onClose);
    root.classList.add('modal-wide');

    const listEl = root.querySelector('#tm-list') as HTMLElement;
    const nameEl = root.querySelector('#tm-name') as HTMLInputElement;
    const bodyEl = root.querySelector('#tm-body') as HTMLTextAreaElement;
    const hintEl = root.querySelector('#tm-hint') as HTMLElement;
    const saveBtn = root.querySelector('[data-act="save"]') as HTMLButtonElement;
    const delBtn = root.querySelector('[data-act="del"]') as HTMLButtonElement;

    let list: AgentTemplate[] = [];
    let selectedId = '';
    let draft = false; // 正在编辑一份还没保存过的新模板

    const renderList = (): void => {
        listEl.innerHTML = list.map(t => `
            <div class="tm-item${!draft && t.id === selectedId ? ' active' : ''}" data-id="${esc(t.id)}">
                <span class="tm-item-name">${esc(t.name)}</span>
                <span class="tm-badge">${t.builtin ? '内置' : '我的'}</span>
            </div>`).join('');
        listEl.querySelectorAll<HTMLElement>('.tm-item').forEach(el => {
            el.addEventListener('click', () => {
                selectedId = el.dataset.id ?? '';
                draft = false;
                renderList();
                loadCurrent();
            });
        });
    };

    const loadCurrent = (): void => {
        const t = list.find(x => x.id === selectedId);
        if (!t) return;
        nameEl.value = t.name;
        bodyEl.value = t.body;
        nameEl.readOnly = t.builtin;
        bodyEl.readOnly = t.builtin;
        saveBtn.disabled = t.builtin;
        delBtn.disabled = t.builtin;
        hintEl.textContent = t.builtin
            ? '内置模板只读。想改就点「另存为」，存成自己的模板。'
            : '这是你自己的模板，可以直接改并保存。';
    };

    const reload = async (wantId?: string): Promise<void> => {
        try {
            list = (await App.GetAgentTemplates()) ?? [];
        } catch (e) {
            list = [];
            toast(String(e), 'err');
        }
        draft = false;
        if (wantId && list.some(t => t.id === wantId)) {
            selectedId = wantId;
        } else if (!list.some(t => t.id === selectedId)) {
            selectedId = list[0]?.id ?? '';
        }
        renderList();
        loadCurrent();
        if (!list.length) {
            hintEl.textContent = '读取模板失败，请重试。';
            saveBtn.disabled = true;
            delBtn.disabled = true;
        }
    };

    const startDraft = (name: string, body: string, hint: string): void => {
        draft = true;
        selectedId = '';
        renderList();
        nameEl.value = name;
        bodyEl.value = body;
        nameEl.readOnly = false;
        bodyEl.readOnly = false;
        saveBtn.disabled = false;
        delBtn.disabled = true;
        hintEl.textContent = hint;
        nameEl.focus();
        nameEl.select();
    };

    void reload();

    root.querySelector('[data-act="new"]')!.addEventListener('click', () => {
        startDraft('', '', '新模板：填好名称与内容后点「保存」。');
    });

    root.querySelector('[data-act="copy"]')!.addEventListener('click', () => {
        const t = list.find(x => x.id === selectedId);
        startDraft(t ? `${t.name} 副本` : '新模板', t?.body ?? '', '另存为新模板（不会改动原模板）。');
    });

    saveBtn.addEventListener('click', () => {
        void withBusy(saveBtn, async () => {
            try {
                const saved = await App.SaveAgentTemplate(draft ? '' : selectedId, nameEl.value, bodyEl.value);
                await reload(saved.id);
                toast(`模板「${saved.name}」已保存`);
            } catch (e) {
                toast(String(e), 'err');
            }
        }, '保存中…');
    });

    delBtn.addEventListener('click', () => {
        const t = list.find(x => x.id === selectedId);
        if (!t) return;
        confirmDialog({
            title: '删除模板',
            body: `<div class="hint">确定删除模板「${esc(t.name)}」吗？此操作不可恢复。</div>`,
            confirmText: '删除',
            danger: true,
            onConfirm: async () => {
                try {
                    await App.DeleteAgentTemplate(t.id);
                    selectedId = '';
                    await reload();
                    toast('模板已删除');
                } catch (e) {
                    toast(String(e), 'err');
                }
            },
        });
    });

    root.querySelector('[data-act="close"]')!.addEventListener('click', () => closeModal());
}

