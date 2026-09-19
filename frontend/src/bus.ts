// bus.ts：只放渲染/主题/模式钩子，避免 view 模块与 main 互相 import 造成循环依赖。
import type { DetailMode, ViewKey } from './state';

export type Hooks = {
    list(): void;
    detail(): void;
    archives(): void;
    theme(): void;
    setMode(mode: DetailMode): void;
    /**
     * 切视图。返回 false 表示被「未保存的设置」拦下了（确认框已经弹出）。
     * 传了 after 的话，它会在视图**真的切过去之后**才执行——也包括
     * 「用户点了确认才切」的情况；用户取消则整个不会执行。
     */
    setView(view: ViewKey, after?: () => void): boolean;
    status(): void;
};

export const hooks: Hooks = {
    list() {},
    detail() {},
    archives() {},
    theme() {},
    setMode() {},
    setView() {
        return false;
    },
    status() {},
};
