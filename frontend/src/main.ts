import './style.css';
import { App, persistConfig } from './api';
import { hooks } from './bus';
import { esc } from './format';
import { state, type ViewKey } from './state';
import { confirmDialog, toast, withBusy } from './ui';
import { renderArchives } from './views/archives';
import { refreshArchives, refreshProjects } from './views/data';
import { openCompare } from './views/compare';
import { openUpdateDialog } from './views/dialogs';
import { enterSettings, leaveSettings, settingsDirty } from './views/settings';
import { openExplorer } from './views/explorer';
import { reconfigureEditorTheme, renderEditorPane, wireResizer } from './views/editor';
import { importProject, openNewProject, renderDetail, renderList } from './views/projects';
import { renderSearch } from './views/search';

const MOON = '<svg class="ic-nav" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M20 14.2A7.8 7.8 0 0 1 9.8 4 7.6 7.6 0 1 0 20 14.2Z"/></svg>';
const SUN = '<svg class="ic-nav" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4.2"/><path d="M12 2.5v2.2M12 19.3v2.2M2.5 12h2.2M19.3 12h2.2M5.3 5.3l1.6 1.6M17.1 17.1l1.6 1.6M5.3 18.7l1.6-1.6M17.1 6.9l1.6-1.6"/></svg>';

function applyTheme(): void {
    const theme = state.cfg.theme === 'dark' ? 'dark' : 'light';
    document.documentElement.dataset.theme = theme;
    const btn = document.getElementById('btn-theme');
    if (btn) {
        btn.innerHTML = theme === 'dark' ? SUN : MOON;
        btn.title = theme === 'dark' ? '切换到日间模式' : '切换到夜间模式';
    }

    // 编辑器字体：CodeMirror 的主题由 JS 侧维护，这里只需更新变量并通知它重算
    const font = (state.cfg.editorFont ?? '').trim();
    document.documentElement.style.setProperty(
        '--ed-font',
        font ? `"${font}", Consolas, "Courier New", monospace` : 'Consolas, "Cascadia Mono", "Courier New", monospace',
    );
    document.documentElement.style.setProperty('--ed-font-size', `${state.cfg.editorFontSize || 14}px`);
    document.documentElement.style.setProperty('--ui-scale', String((state.cfg.uiFontSize || 14) / 14));
    reconfigureEditorTheme();
}

async function setTheme(theme: 'light' | 'dark'): Promise<void> {
    try {
        state.cfg = await App.SetTheme(theme);
        applyTheme();
    } catch (e) {
        toast(String(e), 'err');
    }
}

const VIEW_TITLE: Record<string, string> = { projects: '项目', search: '搜索', archives: '归档', settings: '设置' };
const VIEWS: ViewKey[] = ['projects', 'search', 'archives', 'settings'];

/**
 * 切换视图。离开设置页且有未保存改动时先拦一道，
 * 确认后才真的走——force 用来避免确认之后又被自己拦一次。
 * 返回 false 表示这次没切过去，调用方不要再往下做（比如改选中项）。
 * after 是「切过去之后再做的那件事」，用户取消离开时它不会执行。
 */
function setView(next: ViewKey, force = false, after = null as (() => void) | null): boolean {
    if (!force && next !== 'settings' && state.view === 'settings' && settingsDirty()) {
        confirmDialog({
            title: '放弃未保存的设置？',
            body: '<div class="hint">设置页还有改动没保存，离开就会丢掉。</div>',
            confirmText: '放弃并离开',
            danger: true,
            onConfirm: () => {
                setView(next, true, after);
            },
        });
        return false;
    }
    if (state.view === 'settings' && next !== 'settings') leaveSettings();

    state.view = next;
    for (const v of VIEWS) {
        document.getElementById(`tab-${v}`)?.classList.toggle('active', next === v);
        document.getElementById(`${v}-view`)?.classList.toggle('hidden', next !== v);
    }
    document.getElementById('btn-settings')?.classList.toggle('active', next === 'settings');

    // 设置页里「当前项目」胶囊与刷新按钮没有意义，隐掉少点噪音
    const inSettings = next === 'settings';
    document.getElementById('status-pill')?.classList.toggle('hidden', inSettings);
    document.getElementById('btn-refresh')?.classList.toggle('hidden', inSettings);

    const title = document.getElementById('content-title');
    if (title) title.textContent = VIEW_TITLE[next] ?? '';
    if (next === 'projects') renderDetail();
    if (next === 'search') renderSearch();
    if (next === 'archives') renderArchives();
    if (next === 'settings') enterSettings();
    renderEditorPane();
    renderStatus();
    after?.();
    return true;
}

/* ---------------- 左侧栏宽度 ---------------- */

const RAIL_MIN = 200;
const RAIL_MAX = 420;
const RAIL_FALLBACK = 240;

/** 把配置里的宽度应用到左栏；后端钳制过，这里直接用。 */
function applyRailWidth(): void {
    const rail = document.getElementById('rail');
    if (!rail) return;
    rail.style.flexBasis = `${state.cfg.railWidth || RAIL_FALLBACK}px`;
}

function wireRailResizer(): void {
    const grip = document.getElementById('rail-resizer');
    const rail = document.getElementById('rail');
    if (!grip || !rail) return;
    grip.addEventListener('mousedown', e => {
        e.preventDefault();
        const startX = e.clientX;
        const startW = state.cfg.railWidth || RAIL_FALLBACK;
        grip.classList.add('dragging');
        const onMove = (ev: MouseEvent): void => {
            const w = Math.min(RAIL_MAX, Math.max(RAIL_MIN, startW + (ev.clientX - startX)));
            state.cfg.railWidth = w;
            rail.style.flexBasis = `${w}px`;
        };
        const onUp = (): void => {
            document.removeEventListener('mousemove', onMove);
            document.removeEventListener('mouseup', onUp);
            grip.classList.remove('dragging');
            void persistConfig({ railWidth: state.cfg.railWidth }).then(applyRailWidth);
        };
        document.addEventListener('mousemove', onMove);
        document.addEventListener('mouseup', onUp);
    });
}

/** 内容区右上角的状态小胶囊：跟着当前选中的项目走。 */
function renderStatusPill(): void {
    const dot = document.getElementById('status-pill-dot');
    const text = document.getElementById('status-pill-text');
    if (!dot || !text) return;
    const p = state.selected;
    if (!p) {
        dot.className = 'dot nogit';
        text.textContent = '未选择项目';
        return;
    }
    if (!p.isGit) {
        dot.className = 'dot nogit';
        text.textContent = '非 git 仓库';
        return;
    }
    const bits: string[] = [p.branch || '尚无提交'];
    if (p.dirty) bits.push('有改动');
    if (p.ahead > 0) bits.push(`未推送 ${p.ahead}`);
    if (p.behind > 0) bits.push(`落后 ${p.behind}`);
    dot.className = `dot ${p.dirty || p.ahead > 0 ? 'dirty' : 'clean'}`;
    text.textContent = bits.join(' · ');
}

/** 底部状态条：一眼看清管着哪些项目、有多少没推。 */
function renderStatus(): void {
    const el = document.getElementById('statusbar');
    if (!el) return;
    renderStatusPill();
    const n = state.projects.length;
    const unpushed = state.projects.filter(p => (p.ahead ?? 0) > 0).length;
    const dirty = state.projects.filter(p => p.dirty).length;
    const total = state.projects.reduce((sum, p) => sum + (p.ahead ?? 0), 0);
    const bits = [`共 ${n} 个项目`];
    if (dirty) bits.push(`${dirty} 个有改动`);
    if (unpushed) bits.push(`${unpushed} 个待推送（${total} 条提交）`);
    el.innerHTML = `
        <span class="dot ${dirty ? 'dirty' : 'clean'}"></span>
        <span>${esc(state.cfg.projectRoot || '未设置项目根')}</span>
        <span class="sep">·</span>
        <span>${esc(bits.join(' · '))}</span>`;
}

async function refreshAll(): Promise<void> {
    await Promise.all([refreshProjects(), refreshArchives()]);
}

/** 左上角 logo 下方的版本号；顺手存进 state，设置页的「关于」区也用它。 */
async function applyVersion(): Promise<void> {
    try {
        const v = await App.AppVersion();
        if (!v) return;
        state.version = v;
        const el = document.getElementById('brand-version');
        if (el) el.textContent = `v${v}`;
    } catch {
        /* 版本号无关功能，失败不打扰用户 */
    }
}

/* ---------------- 更新提示 ---------------- */

/** 按最近一次检查结果刷新顶栏的更新胶囊。 */
function applyUpdateBadge(): void {
    const btn = document.getElementById('btn-update');
    if (!btn) return;
    const info = state.update;
    if (info?.available) {
        const text = document.getElementById('btn-update-text');
        if (text) text.textContent = `新版本 v${info.latest}`;
        btn.title = `有新版本 v${info.latest}，点击查看`;
        btn.classList.remove('hidden');
    } else {
        btn.classList.add('hidden');
    }
}

/**
 * 启动时静默查一次更新。断网、被墙、GitHub 限流都算正常情况，
 * 一律不打扰用户，只是没有提示而已（设置页里可以手动再查）。
 */
async function checkUpdate(): Promise<void> {
    try {
        state.update = await App.CheckUpdate();
    } catch {
        state.update = null;
    }
    applyUpdateBadge();
}

function wireStatic(): void {
    document.getElementById('btn-new')!.addEventListener('click', openNewProject);
    document.getElementById('btn-import')!.addEventListener('click', e => void importProject(e.currentTarget as HTMLElement));
    document.getElementById('btn-settings')!.addEventListener('click', () => setView('settings'));
    document.getElementById('btn-refresh')!.addEventListener('click', e => {
        void withBusy(e.currentTarget as HTMLElement, refreshAll);
    });
    document.getElementById('btn-theme')!.addEventListener('click', () => {
        void setTheme(state.cfg.theme === 'dark' ? 'light' : 'dark');
    });
    document.getElementById('btn-update')!.addEventListener('click', () => {
        if (state.update?.available) openUpdateDialog(state.update);
    });

    document.getElementById('tab-projects')!.addEventListener('click', () => setView('projects'));
    document.getElementById('tab-search')!.addEventListener('click', () => setView('search'));
    document.getElementById('tab-archives')!.addEventListener('click', () => setView('archives'));

    const search = document.getElementById('search') as HTMLInputElement;
    const clearBtn = document.getElementById('search-clear') as HTMLButtonElement;
    const syncClear = (): void => {
        clearBtn.classList.toggle('hidden', !search.value);
    };
    search.addEventListener('input', () => {
        state.search = search.value;
        syncClear();
        renderList();
    });
    clearBtn.addEventListener('click', () => {
        search.value = '';
        state.search = '';
        syncClear();
        renderList();
        search.focus();
    });

    const chips = [...document.querySelectorAll<HTMLElement>('#filters .chip')];
    chips.forEach((chip, i) => {
        chip.addEventListener('click', () => {
            state.filter = chip.dataset.filter as typeof state.filter;
            chips.forEach(c => c.classList.toggle('active', c.dataset.filter === state.filter));
            renderList();
        });
        // ←→ 在筛选项之间移动焦点（Enter / 空格 照常激活）
        chip.addEventListener('keydown', (e: KeyboardEvent) => {
            let next = -1;
            if (e.key === 'ArrowRight') next = (i + 1) % chips.length;
            else if (e.key === 'ArrowLeft') next = (i - 1 + chips.length) % chips.length;
            if (next < 0) return;
            e.preventDefault();
            chips[next].focus();
        });
    });

    const sort = document.getElementById('sort') as HTMLSelectElement;
    sort.addEventListener('change', () => {
        state.sortBy = sort.value as typeof state.sortBy;
        renderList();
    });
}

async function init(): Promise<void> {
    try {
        state.cfg = await App.GetConfig();
        state.configPath = await App.ConfigPath();
        state.logPath = await App.LogPath();
    } catch (e) {
        toast(String(e), 'err');
        return;
    }

    hooks.list = renderList;
    hooks.detail = renderDetail;
    hooks.archives = renderArchives;
    hooks.theme = applyTheme;
    hooks.status = renderStatus;
    hooks.setMode = mode => {
        state.mode = mode;
        renderDetail();
        if (mode === 'files') openExplorer();
        if (mode === 'compare') void openCompare();
    };
    // force 是内部用来避免确认后被自己再拦一次的参数，不外泄给调用方
    hooks.setView = (view, after) => setView(view, false, after);
    hooks.updateBadge = applyUpdateBadge;

    wireStatic();
    applyTheme();
    applyRailWidth();
    wireRailResizer();
    wireResizer();
    void applyVersion();
    void checkUpdate();
    setView('projects');
    renderDetail();
    await Promise.all([refreshProjects(), refreshArchives()]);
}

void init();
