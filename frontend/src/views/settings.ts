import { App } from '../api';
import { hooks } from '../bus';
import { esc } from '../format';
import { state } from '../state';
import { copyText, toast, withBusy } from '../ui';
import { openTemplateManager, openUpdateDialog } from './dialogs';
import { refreshArchives, refreshProjects } from './data';

/* ---------------- 编辑草稿 ----------------
 * 设置页的编辑态只属于这一个页面，所以留在模块内，不进全局 state。
 * 进入页面时从 state.cfg 重建，离开时丢弃；只有编辑中才可能「脏」。
 */

type DraftKey = 'projectRoot' | 'archiveRoot' | 'editorCmd' | 'terminalCmd' | 'editorFont' | 'author';
type Draft = Record<DraftKey, string>;

const DRAFT_KEYS: DraftKey[] = ['projectRoot', 'archiveRoot', 'editorCmd', 'terminalCmd', 'editorFont', 'author'];

let draft: Draft = { projectRoot: '', archiveRoot: '', editorCmd: '', terminalCmd: '', editorFont: '', author: '' };
// 字号是数字，不进字符串草稿机制，单独存；越界值在输入时就 clamp，保证预览与保存所见即所得。
// 界面字号不做实时预览：它直接改全局 --ui-scale，「放弃离开」路径没有还原因，保存后生效最稳。
let draftFontSize = 14;
let draftUiFontSize = 14;
let editing = false;

function resetDraft(): void {
    draft = {
        projectRoot: state.cfg.projectRoot ?? '',
        archiveRoot: state.cfg.archiveRoot ?? '',
        editorCmd: state.cfg.editorCmd ?? '',
        terminalCmd: state.cfg.terminalCmd ?? '',
        editorFont: state.cfg.editorFont ?? '',
        author: state.cfg.author ?? '',
    };
    draftFontSize = state.cfg.editorFontSize || 14;
    draftUiFontSize = state.cfg.uiFontSize || 14;
}

/** 有未保存改动吗？没在编辑设置页时恒为 false，否则离开拦截会误报。 */
export function settingsDirty(): boolean {
    if (!editing) return false;
    return DRAFT_KEYS.some(k => draft[k].trim() !== (state.cfg[k] ?? '').trim())
        || draftFontSize !== (state.cfg.editorFontSize || 14)
        || draftUiFontSize !== (state.cfg.uiFontSize || 14);
}

/* ---------------- 快捷键表 ---------------- */

const KEYS: { group: string; items: [string, string][] }[] = [
    {
        group: '编辑器',
        items: [
            ['Ctrl+S', '保存当前文件'],
            ['Ctrl+F', '编辑器内查找替换'],
            ['Ctrl+空格', '唤出补全'],
        ],
    },
    {
        group: '文件管理',
        items: [
            ['F2', '重命名选中项'],
            ['Delete', '删除到回收站'],
            ['Ctrl+X / C / V', '剪切 / 复制 / 粘贴'],
            ['Ctrl+A', '全选当前目录'],
            ['Alt+← / →', '文件管理器前进 / 后退'],
            ['Backspace', '返回上一级目录'],
            ['↑ ↓ ← →', '在列表 / 网格中移动选中项'],
        ],
    },
    {
        group: '通用',
        items: [
            ['Enter', '打开文件（或项目文件夹）'],
            ['Esc', '关闭弹窗 / 取消选择'],
        ],
    },
];

/* ---------------- 模板片段 ---------------- */

/** 一条设置 = 「标签 168px ｜ 控件 1fr」。没有对应控件时标签不用 label（避免无主 for）。 */
function row(label: string, id: string | null, control: string, hint = ''): string {
    const head = id
        ? `<label class="set-label" for="${id}">${esc(label)}</label>`
        : `<div class="set-label">${esc(label)}</div>`;
    return `<div class="set-row">${head}<div class="set-ctrl">${control}${
        hint ? `<div class="hint">${esc(hint)}</div>` : ''
    }</div></div>`;
}

function section(key: string, title: string, desc: string, body: string): string {
    return `<section class="set-sec" aria-labelledby="set-h-${key}">
        <div class="set-sec-head">
            <h3 id="set-h-${key}">${esc(title)}</h3>
            <p>${esc(desc)}</p>
        </div>
        ${body}
    </section>`;
}

function textRow(key: DraftKey, label: string, placeholder: string, hint = ''): string {
    const id = `set-${key}`;
    return row(
        label,
        id,
        `<input id="${id}" class="input" type="text" data-cfg="${key}" value="${esc(draft[key])}" placeholder="${esc(placeholder)}" />`,
        hint,
    );
}

function pathRow(key: 'projectRoot' | 'archiveRoot', label: string, hint: string): string {
    const id = `set-${key}`;
    return row(
        label,
        id,
        `<div class="row">
            <input id="${id}" class="input" type="text" data-cfg="${key}" value="${esc(draft[key])}" />
            <button class="btn ghost" type="button" data-pick="${key}">选择</button>
        </div>`,
        hint,
    );
}

function fontRow(): string {
    return row(
        '编辑器字体',
        'set-editorFont',
        `<input id="set-editorFont" class="input" type="text" list="set-font-suggest" data-cfg="editorFont"
                value="${esc(draft.editorFont)}" placeholder="Consolas" />
        <div class="font-preview" id="set-font-preview"></div>
        <datalist id="set-font-suggest">
            <option value="Consolas"></option>
            <option value="Cascadia Mono"></option>
            <option value="Cascadia Code"></option>
            <option value="JetBrains Mono"></option>
            <option value="Fira Code"></option>
            <option value="Source Code Pro"></option>
            <option value="IBM Plex Mono"></option>
            <option value="DejaVu Sans Mono"></option>
            <option value="Courier New"></option>
        </datalist>`,
        '留空 = 默认等宽字体；改一下就能在样张里看到实际效果',
    );
}

function fontSizeRow(id: 'editorFontSize' | 'uiFontSize', label: string, value: number, min: number, max: number, hint: string): string {
    return row(
        label,
        `set-${id}`,
        `<input id="set-${id}" class="input sort" type="number" min="${min}" max="${max}" step="1" value="${value}" />`,
        hint,
    );
}

function tplRow(): string {
    return row(
        'AGENTS.md 模板',
        null,
        `<div class="row">
            <button class="btn ghost" type="button" id="set-tpl">管理模板…</button>
            <span class="hint" id="set-tpl-count"></span>
        </div>`,
        '新建项目时可以从这些模板里挑一份',
    );
}

function pathInfo(p: string, kind: 'cfg' | 'log'): string {
    if (!p) return '<span class="hint">未获取到路径</span>';
    return `<div class="set-info">
        <code class="set-mono" title="${esc(p)}">${esc(p)}</code>
        <button class="btn tiny ghost" type="button" data-open="${kind}">${
            kind === 'cfg' ? '打开所在目录' : '打开'
        }</button>
        <button class="btn tiny ghost" type="button" data-copy="${esc(p)}">复制</button>
    </div>`;
}

function keysBody(): string {
    return `<div class="keys">${KEYS.map(g => `
        <div class="keys-group">
            <div class="keys-title">${esc(g.group)}</div>
            ${g.items.map(([k, d]) => `<div class="key-row"><kbd>${esc(k)}</kbd><span>${esc(d)}</span></div>`).join('')}
        </div>`).join('')}</div>`;
}

/* ---------------- 局部刷新（不整块重绘，避免输入焦点被吃掉） ---------------- */

function syncInputs(): void {
    for (const k of DRAFT_KEYS) {
        const inp = document.getElementById(`set-${k}`) as HTMLInputElement | null;
        if (inp) inp.value = draft[k];
    }
    const size = document.getElementById('set-editorFontSize') as HTMLInputElement | null;
    if (size) size.value = String(draftFontSize);
    const uiSize = document.getElementById('set-uiFontSize') as HTMLInputElement | null;
    if (uiSize) uiSize.value = String(draftUiFontSize);
}

function syncFontPreview(): void {
    const box = document.getElementById('set-font-preview');
    if (!box) return;
    const f = draft.editorFont.trim();
    box.style.fontFamily = f
        ? `"${f}", Consolas, "Courier New", monospace`
        : 'Consolas, "Cascadia Mono", "Courier New", monospace';
    box.style.fontSize = `${draftFontSize}px`;
    box.textContent = 'Abc 0123 (){}[] => 中文字体样张';
}

function syncBar(): void {
    const dirty = settingsDirty();
    const save = document.getElementById('set-save') as HTMLButtonElement | null;
    const revert = document.getElementById('set-revert') as HTMLButtonElement | null;
    if (save) save.disabled = !dirty;
    if (revert) revert.disabled = !dirty;
    document.getElementById('set-dirty')?.classList.toggle('hidden', !dirty);
}

function syncVersion(): void {
    const el = document.getElementById('set-version');
    if (el && state.version) el.textContent = `v${state.version}`;
}

/** 把「有没有新版本」同步到「关于」区那一行；announce 为真时顺便说出来。 */
function syncUpdateState(announce = false): void {
    const label = document.getElementById('set-update-state');
    if (!label) return;
    const info = state.update;
    const doBtn = document.getElementById('set-do-update');

    if (info?.available) {
        label.textContent = `有新版本 v${info.latest}`;
        doBtn?.classList.remove('hidden');
        if (announce) toast(`发现新版本 v${info.latest}`);
        return;
    }
    doBtn?.classList.add('hidden');
    label.textContent = info ? '已是最新版本' : '';
    if (announce && info) toast(`已是最新版本（v${info.current}）`);
}

function syncTplCount(): void {
    const el = document.getElementById('set-tpl-count');
    if (el) el.textContent = tplCount >= 0 ? `共 ${tplCount} 份可选` : '';
}

/* ---------------- 异步补齐 ---------------- */

let tplCount = -1;

async function loadTplCount(): Promise<void> {
    try {
        tplCount = (await App.GetAgentTemplates()).length;
    } catch {
        tplCount = 0;
    }
    syncTplCount();
}

async function loadVersion(): Promise<void> {
    if (!state.version) {
        try {
            state.version = await App.AppVersion();
        } catch {
            return;
        }
    }
    syncVersion();
}

function dirOf(p: string): string {
    const i = p.lastIndexOf('\\');
    return i > 0 ? p.slice(0, i) : p;
}

/* ---------------- 保存 / 放弃 ---------------- */

async function save(): Promise<void> {
    const next = {
        ...state.cfg,
        projectRoot: draft.projectRoot.trim(),
        archiveRoot: draft.archiveRoot.trim(),
        editorCmd: draft.editorCmd.trim(),
        terminalCmd: draft.terminalCmd.trim(),
        editorFont: draft.editorFont.trim(),
        editorFontSize: draftFontSize,
        uiFontSize: draftUiFontSize,
        author: draft.author.trim(),
    };
    try {
        state.cfg = await App.SaveConfig(next);
        resetDraft();
        syncInputs();
        syncFontPreview();
        hooks.theme();
        toast('设置已保存');
        await Promise.all([refreshProjects(), refreshArchives()]);
    } catch (e) {
        toast(String(e), 'err');
    }
}

function revert(): void {
    resetDraft();
    syncInputs();
    syncFontPreview();
    syncBar();
    toast('已放弃改动');
}

/* ---------------- 接线 ---------------- */

function wire(el: HTMLElement): void {
    el.querySelectorAll<HTMLInputElement>('[data-cfg]').forEach(inp => {
        inp.addEventListener('input', () => {
            const key = inp.dataset.cfg as DraftKey;
            draft[key] = inp.value;
            if (key === 'editorFont') syncFontPreview();
            syncBar();
        });
    });

    const clampSize = (v: number, min: number, max: number) => Math.min(max, Math.max(min, v));
    el.querySelector('#set-editorFontSize')?.addEventListener('input', ev => {
        const v = parseInt((ev.target as HTMLInputElement).value, 10);
        draftFontSize = clampSize(Number.isFinite(v) ? v : 14, 10, 24);
        syncFontPreview();
        syncBar();
    });
    el.querySelector('#set-uiFontSize')?.addEventListener('input', ev => {
        const v = parseInt((ev.target as HTMLInputElement).value, 10);
        draftUiFontSize = clampSize(Number.isFinite(v) ? v : 14, 12, 18);
        syncBar();
    });

    el.querySelectorAll<HTMLElement>('[data-pick]').forEach(btn => {
        btn.addEventListener('click', () => {
            const key = btn.dataset.pick as 'projectRoot' | 'archiveRoot';
            void withBusy(btn, async () => {
                try {
                    const val = await App.ChooseFolder('选择目录');
                    if (!val) return;
                    draft[key] = val;
                    syncInputs();
                    syncBar();
                } catch (e) {
                    toast(String(e), 'err');
                }
            });
        });
    });

    el.querySelector('#set-tpl')?.addEventListener('click', () => {
        void openTemplateManager(() => {
            renderSettings();
            void loadTplCount();
        });
    });

    el.querySelectorAll<HTMLElement>('[data-copy]').forEach(btn => {
        btn.addEventListener('click', () => void copyText(btn.dataset.copy!, '路径'));
    });

    el.querySelectorAll<HTMLElement>('[data-open]').forEach(btn => {
        btn.addEventListener('click', async () => {
            try {
                if (btn.dataset.open === 'cfg') await App.OpenFolder(dirOf(state.configPath));
                else await App.OpenWithSystem(state.logPath);
            } catch (e) {
                toast(String(e), 'err');
            }
        });
    });

    el.querySelector('#set-check-update')?.addEventListener('click', ev => {
        const btn = ev.currentTarget as HTMLElement;
        void withBusy(btn, async () => {
            try {
                state.update = await App.CheckUpdate();
            } catch (e) {
                toast(String(e), 'err');
                return;
            }
            hooks.updateBadge();
            syncUpdateState(true);
        });
    });

    el.querySelector('#set-do-update')?.addEventListener('click', () => {
        if (state.update?.available) openUpdateDialog(state.update);
    });

    el.querySelector('#set-revert')?.addEventListener('click', revert);

    const saveBtn = el.querySelector('#set-save') as HTMLButtonElement;
    saveBtn.addEventListener('click', () => {
        // withBusy 会在结束时把 disabled 还原成点击前的值，所以脏值同步必须排在它后面
        void withBusy(saveBtn, save).then(syncBar);
    });
}

/* ---------------- 进出页面 ---------------- */

export function renderSettings(): void {
    const el = document.getElementById('settings-view');
    if (!el) return;

    el.innerHTML = `
        <div class="set-scroll" tabindex="-1" role="region" aria-label="设置">
            <div class="set-page">
                ${section('paths', '项目路径', '工具只扫描这些目录，文件操作也都被限制在项目根之内', `<div class="set-rows">
                    ${pathRow('projectRoot', '项目根目录', '只列这个目录下的第一级子目录')}
                    ${pathRow('archiveRoot', '归档目录', '归档的 zip 与索引存放处')}
                </div>`)}
                ${section('tools', '外部工具', '打开文件、终端与编辑器时用到的命令', `<div class="set-rows">
                    ${textRow('editorCmd', '编辑器命令', 'code', '双击文件时用它打开，例如 code、subl')}
                    ${textRow('terminalCmd', '终端命令', 'wt', '在项目目录开终端，例如 wt、powershell')}
                    ${fontRow()}
                    ${fontSizeRow('editorFontSize', '编辑器字号', draftFontSize, 10, 24, '10–24，样张马上跟着变')}
                    ${fontSizeRow('uiFontSize', '界面字号', draftUiFontSize, 12, 18, '12–18，保存后生效')}
                </div>`)}
                ${section('newproj', '新建项目', '新建向导的默认值', `<div class="set-rows">
                    ${textRow('author', '协议署名', '留空则用项目名', '写进 LICENSE 的版权行')}
                    ${tplRow()}
                </div>`)}
                ${section('keys', '快捷键', '只在对应场景里生效', keysBody())}
                ${section('about', '关于', '版本与数据位置；这两处都不在项目目录里', `<div class="set-rows">
                    ${row('版本', null, `<div class="set-info">
                        <code class="set-mono" id="set-version">${state.version ? `v${esc(state.version)}` : '—'}</code>
                        <span class="hint" id="set-update-state"></span>
                        <button class="btn tiny ghost hidden" type="button" id="set-do-update">更新…</button>
                        <button class="btn tiny ghost" type="button" id="set-check-update">检查更新</button>
                    </div>`)}
                    ${row('配置文件', null, pathInfo(state.configPath, 'cfg'))}
                    ${row('日志文件', null, pathInfo(state.logPath, 'log'))}
                </div>`)}
            </div>
        </div>
        <div class="set-bar">
            <span class="set-dirty hidden" id="set-dirty">有未保存的改动</span>
            <span class="grow"></span>
            <button class="btn ghost" type="button" id="set-revert" disabled>放弃更改</button>
            <button class="btn primary" type="button" id="set-save" disabled>保存</button>
        </div>`;

    wire(el);
    syncBar();
    syncFontPreview();
    syncVersion();
    syncUpdateState();
    syncTplCount();
}

/** 进入设置页：重建草稿再画，保证每次都从磁盘上的配置开始。 */
export function enterSettings(): void {
    editing = true;
    resetDraft();
    renderSettings();
    void loadTplCount();
    void loadVersion();
    // 键盘用户落到页面容器上，接着按 Tab 就是第一个控件
    document.querySelector<HTMLElement>('#settings-view .set-scroll')?.focus({ preventScroll: true });
}

/** 离开设置页：丢掉草稿，免得下次进来看到上次没保存的残留。 */
export function leaveSettings(): void {
    editing = false;
}
