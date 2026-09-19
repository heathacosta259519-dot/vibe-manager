import * as App from '../wailsjs/go/main/App';
import { state, type Config } from './state';

export { App };

/** 把配置改动写回磁盘（同时更新内存里的 cfg）。 */
export async function persistConfig(patch: Partial<Config>): Promise<void> {
    const next = { ...state.cfg, ...patch } as Config;
    state.cfg = await App.SaveConfig(next);
}

/** 把「项目根 + 相对路径」拼成 Windows 绝对路径。 */
export function fullPath(root: string, rel: string): string {
    if (!rel) return root;
    return `${root}\\${rel.replace(/\//g, '\\')}`;
}

/**
 * 用设置里的外部编辑器打开项目内的某个文件（rel 为相对项目根路径）。
 * 外部编辑器只能开磁盘上的文件，所以打开的一律是工作区里的那一份，失败交给调用方处理。
 */
export async function openInEditor(rel: string): Promise<void> {
    const root = state.selected?.path;
    if (!root || !rel) return;
    await App.OpenEditor(fullPath(root, rel));
}
