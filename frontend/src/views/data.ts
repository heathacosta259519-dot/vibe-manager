import { App } from '../api';
import { hooks } from '../bus';
import { state } from '../state';
import { toast } from '../ui';

export async function refreshProjects(): Promise<void> {
    try {
        state.projects = (await App.ListProjects()) ?? [];
    } catch (e) {
        state.projects = [];
        toast(String(e), 'err');
    }
    if (state.selected) {
        state.selected = state.projects.find(p => p.path === state.selected!.path) ?? null;
    }
    hooks.list();
    if (state.view === 'projects') hooks.detail();
}

export async function refreshArchives(): Promise<void> {
    try {
        state.archives = (await App.ListArchives()) ?? [];
    } catch (e) {
        state.archives = [];
        toast(String(e), 'err');
    }
    hooks.archives();
}
