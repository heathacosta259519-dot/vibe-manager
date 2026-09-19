import { esc } from './format';

/* ---------------- Toast ---------------- */

// 同一句话 3 秒内只弹一次，避免重复点击时刷屏
const recentToasts = new Map<string, number>();

export function toast(msg: string, kind: 'ok' | 'err' = 'ok'): void {
    const key = `${kind}:${msg}`;
    const now = Date.now();
    if ((recentToasts.get(key) ?? 0) > now - 3000) return;
    recentToasts.set(key, now);

    const el = document.createElement('div');
    el.className = `toast ${kind}`;
    el.textContent = msg;
    el.title = '点击关闭';
    el.addEventListener('click', () => el.remove());
    document.getElementById('toast-root')!.appendChild(el);

    // 失败信息停留更久，让人来得及看清
    const life = kind === 'err' ? 7000 : 3600;
    setTimeout(() => {
        el.classList.add('out');
        setTimeout(() => el.remove(), 300);
    }, life);
}

/* ---------------- 忙碌态 ---------------- */

/**
 * 给按钮加忙碌态：转圈 + 禁用 + 保持宽度（避免布局跳动），
 * 任务结束（成功或失败）自动还原。**同时起到防重复点击的作用**。
 */
export async function withBusy(btn: HTMLElement, task: () => Promise<void>, busyLabel?: string): Promise<void> {
    if (btn.dataset.busy === '1') return;
    btn.dataset.busy = '1';

    const button = btn as HTMLButtonElement;
    const prevDisabled = button.disabled;
    const prevHtml = btn.innerHTML;
    const width = Math.round(btn.getBoundingClientRect().width);
    if (width > 0) btn.style.minWidth = `${width}px`;
    button.disabled = true;
    btn.classList.add('busy');
    btn.innerHTML = `<span class="spinner"></span>${busyLabel ? `<span>${esc(busyLabel)}</span>` : ''}`;

    try {
        await task();
    } finally {
        delete btn.dataset.busy;
        btn.classList.remove('busy');
        btn.innerHTML = prevHtml;
        btn.style.minWidth = '';
        button.disabled = prevDisabled;
    }
}

/* ---------------- 统一状态占位 ---------------- */

/** 统一空状态：可选图标 + 主文案 + 提示 + 动作按钮。 */
export function renderEmpty(opts: { icon?: string; title: string; hint?: string; action?: string }): string {
    return `<div class="empty">
        ${opts.icon ? `<div class="empty-icon">${opts.icon}</div>` : ''}
        <div class="empty-title">${esc(opts.title)}</div>
        ${opts.hint ? `<div class="empty-hint">${esc(opts.hint)}</div>` : ''}
        ${opts.action ? `<button class="btn ghost" data-empty-action>${esc(opts.action)}</button>` : ''}
    </div>`;
}

/** 统一加载态。 */
export function renderLoading(text = '读取中…'): string {
    return `<div class="empty"><span class="spinner big"></span><div class="empty-hint">${esc(text)}</div></div>`;
}

/**
 * 整块区域的空状态：居中衬线大标题 + 说明 + 建议胶囊（照参考图的 hero）。
 * 局部小空档仍用 renderEmpty。
 */
export function renderHero(opts: {
    title: string;
    hint?: string;
    actions?: { label: string; act: string; primary?: boolean }[];
}): string {
    const actions = opts.actions?.length
        ? `<div class="hero-actions">${opts.actions.map(a =>
            `<button class="pill-btn${a.primary ? ' primary' : ''}" data-hero-act="${esc(a.act)}">${esc(a.label)}</button>`,
        ).join('')}</div>`
        : '';
    return `<div class="hero">
        <h1 class="hero-title">${esc(opts.title)}</h1>
        ${opts.hint ? `<div class="hero-hint">${esc(opts.hint)}</div>` : ''}
        ${actions}
    </div>`;
}

/** 给 hero 里的建议胶囊接上动作。 */
export function wireHero(root: HTMLElement, handlers: Record<string, () => void>): void {
    root.querySelectorAll<HTMLElement>('[data-hero-act]').forEach(btn => {
        btn.addEventListener('click', () => handlers[btn.dataset.heroAct ?? '']?.());
    });
}

function onEsc(e: KeyboardEvent): void {
    if (e.key === 'Escape') closeModal();
}

// 弹窗可以叠：每次 openModal 压一层，closeModal 只关最上面那一层。
// 这样「在弹窗里再弹确认框」不会把下面那个弹窗一起顶掉。
type ModalLayer = { el: HTMLElement; onClose?: () => void; prevFocus: HTMLElement | null };
const modalStack: ModalLayer[] = [];

/** 弹窗里可获得焦点的元素（跳过隐藏与禁用项）。 */
function focusables(root: HTMLElement): HTMLElement[] {
    const sel = 'input:not([disabled]), select:not([disabled]), textarea:not([disabled]), button:not([disabled]), [href], [tabindex]:not([tabindex="-1"])';
    return [...root.querySelectorAll<HTMLElement>(sel)].filter(el => el.offsetParent !== null);
}

export function closeModal(): void {
    const top = modalStack.pop();
    if (top) {
        top.el.remove();
        top.onClose?.();
        // 焦点还给打开它的那个元素（元素可能已被重绘掉，所以先判断还在不在文档里）
        if (top.prevFocus && document.contains(top.prevFocus)) top.prevFocus.focus();
    }
    if (!modalStack.length) document.removeEventListener('keydown', onEsc);
}

export function openModal(html: string, onClose?: () => void): HTMLElement {
    const root = document.getElementById('modal-root')!;
    const wrap = document.createElement('div');
    wrap.className = 'modal-backdrop';
    wrap.innerHTML = `<div class="modal">${html}</div>`;
    root.appendChild(wrap);
    modalStack.push({ el: wrap, onClose, prevFocus: document.activeElement as HTMLElement | null });
    if (modalStack.length === 1) document.addEventListener('keydown', onEsc);

    wrap.addEventListener('mousedown', e => {
        // 只有最上层能被点背景关掉
        if (e.target === wrap && modalStack[modalStack.length - 1]?.el === wrap) closeModal();
    });

    const modal = wrap.querySelector('.modal') as HTMLElement;

    // Tab 不跑出弹窗，首尾循环
    wrap.addEventListener('keydown', e => {
        if (e.key !== 'Tab') return;
        const list = focusables(modal);
        if (!list.length) return;
        const first = list[0];
        const last = list[list.length - 1];
        const active = document.activeElement as HTMLElement | null;
        if (e.shiftKey && (active === first || !modal.contains(active))) {
            e.preventDefault();
            last.focus();
        } else if (!e.shiftKey && (active === last || !modal.contains(active))) {
            e.preventDefault();
            first.focus();
        }
    });

    // 自动聚焦：优先可编辑的输入框，其次主按钮（延后一帧，让调用方自己的 focus 先生效）
    requestAnimationFrame(() => {
        if (modalStack[modalStack.length - 1]?.el !== wrap) return;
        const target =
            modal.querySelector<HTMLElement>('input:not([readonly]):not([disabled]), textarea:not([readonly]):not([disabled]), select:not([disabled])')
            ?? modal.querySelector<HTMLElement>('[data-act="ok"]')
            ?? focusables(modal)[0];
        target?.focus();
    });

    return modal;
}

export type ConfirmOpts = {
    title: string;
    body: string;
    confirmText: string;
    cancelText?: string;
    danger?: boolean;
    onConfirm: (root: HTMLElement) => void | Promise<void>;
};

export function confirmDialog(opts: ConfirmOpts): void {
    const root = openModal(`
        <h3>${esc(opts.title)}</h3>
        <div class="modal-body">${opts.body}</div>
        <div class="modal-actions">
            <button class="btn ghost" data-act="cancel">${esc(opts.cancelText ?? '取消')}</button>
            <button class="btn ${opts.danger ? 'danger' : 'primary'}" data-act="ok">${esc(opts.confirmText)}</button>
        </div>
    `);
    const okBtn = root.querySelector('[data-act="ok"]') as HTMLButtonElement;
    const cancelBtn = root.querySelector('[data-act="cancel"]') as HTMLButtonElement;
    cancelBtn.addEventListener('click', closeModal);

    okBtn.addEventListener('click', () => {
        // 先同步取值（弹窗还开着），执行期间按钮转圈并禁用，从机制上防重复触发
        void withBusy(okBtn, async () => {
            cancelBtn.disabled = true;
            let failed = false;
            try {
                await opts.onConfirm(root);
            } catch (e) {
                failed = true;
                toast(String(e), 'err');
            }
            cancelBtn.disabled = false;
            // 失败就不关窗，让人看清原因后重试
            if (!failed) closeModal();
        });
    });
}

export async function copyText(text: string, label: string): Promise<void> {
    try {
        await navigator.clipboard.writeText(text);
    } catch {
        const ta = document.createElement('textarea');
        ta.value = text;
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        ta.remove();
    }
    toast(label);
}

export function promptDialog(opts: {
    title: string;
    label: string;
    initial?: string;
    confirmText?: string;
    allowEmpty?: boolean;
    onSubmit: (value: string) => void | Promise<void>;
}): void {
    const root = openModal(`
        <h3>${esc(opts.title)}</h3>
        <div class="modal-body">
            <label class="field">
                <span>${esc(opts.label)}</span>
                <input id="pm-value" class="input" type="text" autocomplete="off" value="${esc(opts.initial ?? '')}" />
            </label>
        </div>
        <div class="modal-actions">
            <button class="btn ghost" data-act="cancel">取消</button>
            <button class="btn primary" data-act="ok">${esc(opts.confirmText ?? '确定')}</button>
        </div>
    `);
    const input = root.querySelector('#pm-value') as HTMLInputElement;
    const okBtn = root.querySelector('[data-act="ok"]') as HTMLButtonElement;
    const cancelBtn = root.querySelector('[data-act="cancel"]') as HTMLButtonElement;
    const submit = (): void => {
        const value = input.value.trim();
        if (!value && !opts.allowEmpty) return;
        void withBusy(okBtn, async () => {
            cancelBtn.disabled = true;
            let failed = false;
            try {
                await opts.onSubmit(value);
            } catch (e) {
                failed = true;
                toast(String(e), 'err');
            }
            cancelBtn.disabled = false;
            if (!failed) closeModal();
        });
    };
    cancelBtn.addEventListener('click', closeModal);
    okBtn.addEventListener('click', submit);
    input.addEventListener('keydown', e => {
        if (e.key === 'Enter') void submit();
    });
    input.focus();
    input.select();
}

/* ---------------- 右键菜单 ---------------- */

export type MenuEntry =
    | { sep: true }
    | { label: string; danger?: boolean; onClick: () => void };

function onOutsideDown(e: MouseEvent): void {
    const menu = document.getElementById('ctx-menu');
    if (menu && !menu.contains(e.target as Node)) hideContextMenu();
}

function onMenuEsc(e: KeyboardEvent): void {
    if (e.key === 'Escape') hideContextMenu();
}

export function hideContextMenu(): void {
    document.getElementById('ctx-menu')?.remove();
    document.removeEventListener('mousedown', onOutsideDown, true);
    document.removeEventListener('keydown', onMenuEsc, true);
    window.removeEventListener('blur', hideContextMenu);
}

export function showContextMenu(x: number, y: number, entries: MenuEntry[]): void {
    hideContextMenu();
    const menu = document.createElement('div');
    menu.className = 'ctx-menu';
    menu.id = 'ctx-menu';
    menu.innerHTML = entries.map((it, i) => ('sep' in it
        ? '<div class="menu-sep"></div>'
        : `<button class="menu-item${it.danger ? ' danger-text' : ''}" data-i="${i}">${esc(it.label)}</button>`
    )).join('');
    document.body.appendChild(menu);

    const rect = menu.getBoundingClientRect();
    menu.style.left = `${Math.max(4, Math.min(x, window.innerWidth - rect.width - 8))}px`;
    menu.style.top = `${Math.max(4, Math.min(y, window.innerHeight - rect.height - 8))}px`;

    menu.querySelectorAll<HTMLElement>('[data-i]').forEach(btn => {
        btn.addEventListener('click', () => {
            const item = entries[Number(btn.dataset.i!)];
            hideContextMenu();
            if (!('sep' in item)) item.onClick();
        });
    });

    setTimeout(() => {
        document.addEventListener('mousedown', onOutsideDown, true);
        document.addEventListener('keydown', onMenuEsc, true);
        window.addEventListener('blur', hideContextMenu);
    }, 0);
}
