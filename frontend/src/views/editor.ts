import { autocompletion, closeBrackets, closeBracketsKeymap, completionKeymap } from '@codemirror/autocomplete';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { cpp } from '@codemirror/lang-cpp';
import { css } from '@codemirror/lang-css';
import { go } from '@codemirror/lang-go';
import { html } from '@codemirror/lang-html';
import { java } from '@codemirror/lang-java';
import { javascript } from '@codemirror/lang-javascript';
import { json } from '@codemirror/lang-json';
import { markdown } from '@codemirror/lang-markdown';
import { php } from '@codemirror/lang-php';
import { python } from '@codemirror/lang-python';
import { rust } from '@codemirror/lang-rust';
import { sql } from '@codemirror/lang-sql';
import { xml } from '@codemirror/lang-xml';
import { yaml } from '@codemirror/lang-yaml';
import {
    bracketMatching, foldGutter, foldKeymap, HighlightStyle, indentOnInput,
    StreamLanguage, syntaxHighlighting,
} from '@codemirror/language';
import { properties } from '@codemirror/legacy-modes/mode/properties';
import { shell } from '@codemirror/legacy-modes/mode/shell';
import { toml } from '@codemirror/legacy-modes/mode/toml';
import { highlightSelectionMatches, searchKeymap } from '@codemirror/search';
import { Compartment, EditorState, type Extension } from '@codemirror/state';
import {
    crosshairCursor, drawSelection, dropCursor, EditorView, highlightActiveLine, highlightActiveLineGutter,
    highlightSpecialChars, keymap, lineNumbers, rectangularSelection,
} from '@codemirror/view';
import { tags as t } from '@lezer/highlight';

import { App, openInEditor, persistConfig } from '../api';
import { esc, renderNumberedDiff } from '../format';
import { state, type FileEntry } from '../state';
import { closeModal, openModal, renderLoading, toast, withBusy } from '../ui';

const IMAGE_EXTS = new Set(['.png', '.jpg', '.jpeg', '.gif', '.webp', '.bmp', '.ico']);

let view: EditorView | null = null;
let viewPath = ''; // 当前编辑器实例里装的是哪个文件；换文件必须重建
const themeCompartment = new Compartment();

/* ---------------- 语言 ---------------- */

/** 按扩展名选语言；CodeMirror 各语言包自带 Lezer 语法，比手写正则可靠得多。 */
function languageFor(ext: string): Extension {
    switch (ext) {
        case '.ts': return javascript({ typescript: true });
        case '.tsx': return javascript({ typescript: true, jsx: true });
        case '.jsx': return javascript({ jsx: true });
        case '.js': case '.mjs': case '.cjs': return javascript();
        case '.json': return json();
        case '.go': return go();
        case '.py': return python();
        case '.html': case '.htm': case '.vue': case '.svelte': return html();
        case '.xml': case '.svg': return xml();
        case '.css': case '.scss': case '.less': return css();
        case '.md': case '.markdown': return markdown();
        case '.yml': case '.yaml': return yaml();
        case '.rs': return rust();
        case '.c': case '.h': case '.hpp': case '.cc': case '.cpp': return cpp();
        case '.java': return java();
        case '.php': return php();
        case '.sql': return sql();
        case '.sh': case '.bash': case '.zsh': return StreamLanguage.define(shell);
        case '.toml': return StreamLanguage.define(toml);
        case '.ini': case '.conf': case '.env': return StreamLanguage.define(properties);
        default: return [];
    }
}

const LANG_LABEL: Record<string, string> = {
    '.ts': 'TypeScript', '.tsx': 'TypeScript JSX', '.js': 'JavaScript', '.jsx': 'JavaScript JSX',
    '.json': 'JSON', '.go': 'Go', '.py': 'Python', '.html': 'HTML', '.htm': 'HTML',
    '.css': 'CSS', '.scss': 'SCSS', '.md': 'Markdown', '.yml': 'YAML', '.yaml': 'YAML',
    '.rs': 'Rust', '.c': 'C', '.h': 'C/C++ 头文件', '.cpp': 'C++', '.hpp': 'C++ 头文件',
    '.java': 'Java', '.php': 'PHP', '.sql': 'SQL', '.sh': 'Shell', '.toml': 'TOML',
    '.ini': 'INI', '.conf': '配置文件', '.env': '环境变量', '.xml': 'XML', '.svg': 'SVG',
};

function langLabel(ext: string): string {
    return LANG_LABEL[ext] ?? (ext ? ext.slice(1).toUpperCase() : '纯文本');
}

/* ---------------- 主题 ---------------- */

/** 颜色全部走项目既有的 CSS 变量，日间/夜间自动跟随。 */
function themeFor(dark: boolean): Extension {
    return EditorView.theme({
        '&': { height: '100%', color: 'var(--text)', backgroundColor: 'var(--panel)', fontSize: 'var(--ed-font-size, 13px)' },
        '&.cm-focused': { outline: 'none' },
        '.cm-scroller': { fontFamily: 'var(--ed-font, Consolas, "Cascadia Mono", "Courier New", monospace)', lineHeight: '1.55' },
        '.cm-content': { caretColor: 'var(--text)', padding: '6px 0' },
        '.cm-line': { padding: '0 10px' },
        '.cm-gutters': {
            backgroundColor: 'var(--panel-2)', color: 'var(--muted)',
            border: 'none', borderRight: '1px solid var(--line)',
        },
        '.cm-lineNumbers .cm-gutterElement': { padding: '0 8px 0 14px' },
        '.cm-activeLine': { backgroundColor: 'var(--active-line)' },
        '.cm-activeLineGutter': { backgroundColor: 'var(--panel)', color: 'var(--text)' },
        '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--text)' },
        '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
            backgroundColor: 'var(--sel)',
        },
        '.cm-selectionMatch': { backgroundColor: 'var(--sel)' },
        '.cm-matchingBracket, &.cm-focused .cm-matchingBracket': {
            backgroundColor: 'var(--sel)', outline: '1px solid var(--accent)',
        },
        '.cm-tooltip': {
            backgroundColor: 'var(--panel-2)', border: '1px solid var(--line)',
            borderRadius: '8px', color: 'var(--text)',
        },
        '.cm-tooltip-autocomplete ul li[aria-selected]': {
            backgroundColor: 'var(--accent)', color: 'var(--on-accent)',
        },
        '.cm-panels': { backgroundColor: 'var(--panel-2)', color: 'var(--text)', border: 'none' },
        '.cm-foldGutter .cm-gutterElement': { color: 'var(--muted)' },
    }, { dark });
}

const highlightStyle = HighlightStyle.define([
    { tag: [t.comment, t.lineComment, t.blockComment, t.docComment], color: 'var(--muted)', fontStyle: 'italic' },
    { tag: [t.keyword, t.modifier, t.controlKeyword, t.moduleKeyword, t.operatorKeyword, t.definitionKeyword], color: 'var(--tk-keyword)' },
    { tag: [t.string, t.special(t.string), t.regexp, t.character], color: 'var(--tk-string)' },
    { tag: [t.number, t.bool, t.null, t.atom], color: 'var(--tk-number)' },
    { tag: [t.typeName, t.className, t.namespace, t.definition(t.typeName)], color: 'var(--tk-type)' },
    { tag: [t.function(t.variableName), t.function(t.propertyName), t.labelName], color: 'var(--tk-type)' },
    { tag: [t.propertyName, t.attributeName], color: 'var(--tk-keyword)' },
    { tag: [t.tagName, t.angleBracket], color: 'var(--tk-keyword)' },
    { tag: [t.operator, t.punctuation, t.bracket, t.separator], color: 'var(--muted)' },
    { tag: [t.heading, t.heading1, t.heading2, t.heading3], color: 'var(--tk-keyword)', fontWeight: '600' },
    { tag: [t.link, t.url], color: 'var(--tk-string)', textDecoration: 'underline' },
    { tag: t.emphasis, fontStyle: 'italic' },
    { tag: t.strong, fontWeight: '600' },
    { tag: [t.meta, t.processingInstruction], color: 'var(--muted)' },
    { tag: t.invalid, color: 'var(--red)' },
]);

export function reconfigureEditorTheme(): void {
    view?.dispatch({ effects: themeCompartment.reconfigure(themeFor(state.cfg.theme === 'dark')) });
}

/* ---------------- 打开 / 渲染 ---------------- */

export function isDirty(): boolean {
    return state.editor.kind === 'text' && state.editor.dirty;
}

export async function openFileInPane(entry: FileEntry): Promise<void> {
    const root = state.selected?.path;
    if (!root) return;
    const ed = state.editor;
    // 先扔掉旧实例：即使点的是同一个文件，也要以磁盘内容为准重新载入
    destroyView();
    ed.path = entry.path;
    ed.name = entry.name;
    ed.ext = entry.ext;
    ed.dirty = false;
    ed.value = '';
    ed.original = '';
    ed.sel = null;
    ed.scrollTop = 0;
    ed.truncated = false;
    ed.message = '';
    ed.dataUrl = '';

    try {
        if (IMAGE_EXTS.has(entry.ext)) {
            const p = await App.PreviewFile(root, entry.path);
            ed.kind = 'image';
            ed.dataUrl = p.dataUrl;
            ed.message = p.message;
        } else {
            const tf = await App.ReadTextFile(root, entry.path, 0);
            if (tf.binary) {
                ed.kind = 'binary';
                ed.message = '二进制文件，无法编辑';
            } else {
                ed.kind = 'text';
                ed.value = tf.content;
                ed.original = tf.content;
                ed.truncated = tf.truncated;
            }
        }
    } catch (e) {
        ed.kind = 'other';
        ed.message = String(e);
    }
    renderEditorPane();
}

export function clearPane(): void {
    const ed = state.editor;
    ed.kind = 'none';
    ed.path = '';
    ed.name = '';
    ed.value = '';
    ed.original = '';
    ed.dirty = false;
    ed.message = '';
    ed.dataUrl = '';
    ed.sel = null;
    renderEditorPane();
}

/**
 * 重画编辑器栏。
 * 文件没变且编辑器实例还在原位时就什么都不做 —— 这样切换隐藏项、刷新列表等
 * 不会把撤销历史与光标重置掉。
 */
export function renderEditorPane(): void {
    const host = document.getElementById('fx-editor');
    if (!host) return;
    const ed = state.editor;
    const resizer = document.getElementById('fx-resizer');

    // 「对比」模式下这条右栏改成只读的 diff 面板，复用同一根分隔条与同一份宽度
    if (state.view === 'projects' && state.mode === 'compare' && state.selected) {
        destroyView();
        if (!state.selected.isGit || !state.commits.length) {
            host.classList.add('hidden');
            resizer?.classList.add('hidden');
            return;
        }
        host.classList.remove('hidden');
        resizer?.classList.remove('hidden');
        host.style.flexBasis = `${ed.width}px`;
        renderDiffPane(host);
        return;
    }

    // 编辑器栏是通高右栏，只在「选中项目的文件模式」里出现。
    // 因为切模式只是隐藏而非销毁，撤销历史与光标能在切回来时保住。
    const inFiles = state.view === 'projects' && state.mode === 'files' && !!state.selected;
    if (!ed.open || !inFiles) {
        host.classList.add('hidden');
        resizer?.classList.add('hidden');
        if (!ed.open) {
            destroyView();
            host.innerHTML = '';
        }
        return;
    }
    host.classList.remove('hidden');
    resizer?.classList.remove('hidden');
    host.style.flexBasis = `${ed.width}px`;

    if (view && viewPath === ed.path && ed.kind === 'text' && host.contains(view.dom)) {
        // 从隐藏切回可见时容器尺寸刚变过，让 CodeMirror 重新量一次
        view.requestMeasure();
        return;
    }

    destroyView();
    host.innerHTML = shellHtml();
    wireHead(host);
    if (ed.kind === 'text') createView(host);
}

/** 对比模式右栏：只读展示当前选中文件在两个版本之间的 diff。 */
function renderDiffPane(host: HTMLElement): void {
    const cmp = state.compare;
    if (!cmp.picked) {
        host.innerHTML = `<div class="ed-empty">在左边选一个文件<br/><span class="hint">这里会显示它在两个版本之间的差异</span></div>`;
        return;
    }

    const notes: string[] = [];
    if (cmp.binary) notes.push('二进制文件，只能看出内容变了');
    if (cmp.truncated) notes.push('diff 过长，只显示了前面一段');

    let body: string;
    if (cmp.diffLoading) body = renderLoading();
    else if (cmp.diffError) body = `<div class="ed-empty">${esc(cmp.diffError)}</div>`;
    else body = `<div class="ed-diff">${renderNumberedDiff(cmp.text)}</div>`;

    host.innerHTML = `
    <div class="ed-head">
        <span class="ed-name" title="${esc(cmp.picked)}">${esc(cmp.picked)}</span>
        <button class="btn tiny ghost" data-diff-ext="1" title="用外部编辑器打开工作区里的这个文件">外部</button>
    </div>
    ${notes.length ? `<div class="ed-note">${esc(notes.join('；'))}</div>` : ''}
    ${body}`;

    host.querySelector('[data-diff-ext]')?.addEventListener('click', () => {
        void openInEditor(cmp.picked).catch(e => toast(String(e), 'err'));
    });
    if (!cmp.diffLoading && !cmp.diffError) scrollToFirstChange(host.querySelector<HTMLElement>('.ed-diff'));
}

/**
 * 把第一处改动滚到可视区顶部。
 * "全部代码"模式下文件可能很长，不定位就得自己翻半天。
 */
function scrollToFirstChange(box: HTMLElement | null): void {
    if (!box) return;
    const first = box.querySelector<HTMLElement>('.diff-line.changed');
    if (!first) return;
    // 往上留三行左右的上下文，别让第一处改动正好贴着顶边
    const context = 72;
    // 用 rect 差值算：offsetTop 依赖祖先有没有定位上下文，这里不一定有
    box.scrollTop += first.getBoundingClientRect().top - box.getBoundingClientRect().top - context;
}

function shellHtml(): string {
    const ed = state.editor;
    if (ed.kind === 'none') {
        return `<div class="ed-empty">选中一个文件即可编辑<br/><span class="hint">Ctrl+S 保存 · Ctrl+空格 补全 · Ctrl+F 查找</span></div>`;
    }

    const head = `
    <div class="ed-head">
        <span class="ed-name" title="${esc(ed.path)}">${esc(ed.name)}<span class="ed-dot${ed.dirty ? '' : ' hidden'}">●</span></span>
        <button class="btn tiny primary" data-ed="save" ${ed.dirty && ed.kind === 'text' && !ed.truncated ? '' : 'disabled'}>保存</button>
        <button class="btn tiny ghost" data-ed="external">外部</button>
        <button class="btn tiny ghost" data-ed="close">关闭</button>
    </div>`;

    if (ed.kind !== 'text') {
        const body = ed.kind === 'image'
            ? `<div class="ed-img"><img src="${ed.dataUrl}" alt="${esc(ed.name)}"/></div>`
            : `<div class="ed-empty">${esc(ed.message || '无法编辑此文件')}</div>`;
        return head + body;
    }

    return head + `
    ${ed.truncated ? '<div class="ed-note">文件超过 2 MB，只载入了前一段。为避免写坏后面的内容，已停用编辑。</div>' : ''}
    <div id="ed-host" class="ed-host"></div>
    <div class="ed-foot"><span id="ed-pos">行 1，列 1</span><span>${esc(langLabel(ed.ext))}</span></div>`;
}

function createView(host: HTMLElement): void {
    const holder = host.querySelector('#ed-host') as HTMLElement;
    const ed = state.editor;
    if (!holder) return;

    view = new EditorView({
        state: EditorState.create({
            doc: ed.value,
            selection: ed.sel ?? undefined,
            extensions: [
                lineNumbers(),
                highlightActiveLineGutter(),
                highlightSpecialChars(),
                history(),
                foldGutter(),
                drawSelection(),
                dropCursor(),
                rectangularSelection(),
                crosshairCursor(),
                highlightActiveLine(),
                highlightSelectionMatches(),
                indentOnInput(),
                bracketMatching(),
                closeBrackets(),
                autocompletion(),
                keymap.of([
                    { key: 'Mod-s', run: () => { void save(); return true; } },
                    ...closeBracketsKeymap,
                    ...defaultKeymap,
                    ...searchKeymap,
                    ...historyKeymap,
                    ...foldKeymap,
                    ...completionKeymap,
                    indentWithTab,
                ]),
                themeCompartment.of(themeFor(state.cfg.theme === 'dark')),
                syntaxHighlighting(highlightStyle),
                languageFor(ed.ext),
                EditorState.readOnly.of(ed.truncated),
                EditorView.updateListener.of(onUpdate),
            ],
        }),
        parent: holder,
    });

    view.scrollDOM.scrollTop = ed.scrollTop;
    view.scrollDOM.addEventListener('scroll', () => {
        state.editor.scrollTop = view?.scrollDOM.scrollTop ?? 0;
    });
    viewPath = ed.path;
    updateDirty();
    updateStatus();
}

function destroyView(): void {
    view?.destroy();
    view = null;
    viewPath = '';
}

function onUpdate(u: { docChanged: boolean; selectionSet: boolean; state: EditorState }): void {
    const ed = state.editor;
    if (u.docChanged) {
        ed.value = u.state.doc.toString();
        updateDirty();
    }
    if (u.docChanged || u.selectionSet) {
        const r = u.state.selection.main;
        ed.sel = { anchor: r.anchor, head: r.head };
        updateStatus();
    }
}

function wireHead(host: HTMLElement): void {
    host.querySelector('[data-ed="save"]')?.addEventListener('click', () => void save());
    host.querySelector('[data-ed="external"]')?.addEventListener('click', () => {
        void openInEditor(state.editor.path).catch(e => toast(String(e), 'err'));
    });
    host.querySelector('[data-ed="close"]')?.addEventListener('click', () => {
        state.editor.open = false;
        void persistConfig({ filePreviewOpen: false });
        renderEditorPane();
    });
}

export function togglePane(): void {
    state.editor.open = !state.editor.open;
    void persistConfig({ filePreviewOpen: state.editor.open });
    renderEditorPane();
}

/* ---------------- 状态指示 ---------------- */

function updateStatus(): void {
    const pos = document.getElementById('ed-pos');
    if (!pos || !view) return;
    const head = view.state.selection.main.head;
    const line = view.state.doc.lineAt(head);
    pos.textContent = `行 ${line.number}，列 ${head - line.from + 1}`;
}

function updateDirty(): void {
    if (view) state.editor.dirty = view.state.doc.toString() !== state.editor.original;
    const dot = document.querySelector('.ed-dot');
    if (dot) dot.classList.toggle('hidden', !state.editor.dirty);
    const saveBtn = document.querySelector<HTMLButtonElement>('[data-ed="save"]');
    if (saveBtn) saveBtn.disabled = !state.editor.dirty || state.editor.truncated;
}

export async function save(): Promise<void> {
    const root = state.selected?.path;
    const ed = state.editor;
    if (!root || ed.kind !== 'text' || ed.truncated || !view) return;
    if (!ed.dirty) {
        toast('没有改动');
        return;
    }
    const text = view.state.doc.toString();
    const btn = document.querySelector<HTMLElement>('[data-ed="save"]');
    const run = async (): Promise<void> => {
        try {
            toast(await App.WriteTextFile(root, ed.path, text));
            ed.original = text;
            ed.value = text;
            updateDirty();
        } catch (e) {
            toast(String(e), 'err');
        }
    };
    if (btn) await withBusy(btn, run, '保存中…');
    else await run();
}

/** 有未保存改动时先问一句。返回 true 表示可以继续切换。 */
export async function confirmDiscardChanges(): Promise<boolean> {
    const ed = state.editor;
    if (ed.kind !== 'text' || !ed.dirty) return true;
    return new Promise<boolean>(resolve => {
        const root = openModal(`
            <h3>有未保存的改动</h3>
            <div class="modal-body">
                <div class="hint"><b>${esc(ed.name)}</b> 的改动还没有保存。</div>
            </div>
            <div class="modal-actions">
                <button class="btn ghost" data-act="cancel">取消</button>
                <button class="btn ghost danger-text" data-act="discard">不保存</button>
                <button class="btn primary" data-act="save">保存</button>
            </div>`);
        const done = (v: boolean): void => {
            closeModal();
            resolve(v);
        };
        root.querySelector('[data-act="cancel"]')!.addEventListener('click', () => done(false));
        root.querySelector('[data-act="discard"]')!.addEventListener('click', () => {
            if (view) {
                view.dispatch({
                    changes: { from: 0, to: view.state.doc.length, insert: ed.original },
                });
            }
            done(true);
        });
        root.querySelector('[data-act="save"]')!.addEventListener('click', () => {
            void save().then(() => done(!ed.dirty));
        });
    });
}

/** 跳到指定行并把光标放过去。 */
export function scrollEditorToLine(lineNo: number): void {
    if (!view || lineNo <= 0) return;
    const total = view.state.doc.lines;
    const target = view.state.doc.line(Math.min(Math.max(lineNo, 1), total));
    view.dispatch({ selection: { anchor: target.from }, scrollIntoView: true });
    view.focus();
}

/* ---------------- 分隔条 ---------------- */

export function wireResizer(): void {
    const grip = document.getElementById('fx-resizer');
    if (!grip) return;
    grip.addEventListener('mousedown', e => {
        e.preventDefault();
        const startX = e.clientX;
        const startW = state.editor.width;
        const pane = document.getElementById('fx-editor');
        grip.classList.add('dragging');
        const onMove = (ev: MouseEvent): void => {
            const w = Math.min(1000, Math.max(260, startW - (ev.clientX - startX)));
            state.editor.width = w;
            if (pane) pane.style.flexBasis = `${w}px`;
        };
        const onUp = (): void => {
            document.removeEventListener('mousemove', onMove);
            document.removeEventListener('mouseup', onUp);
            grip.classList.remove('dragging');
            void persistConfig({ editorWidth: state.editor.width });
        };
        document.addEventListener('mousemove', onMove);
        document.addEventListener('mouseup', onUp);
    });
}
