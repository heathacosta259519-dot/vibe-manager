export type IconEntry = { name: string; isDir: boolean; ext: string; special?: boolean };

const CODE = new Set([
    '.go', '.ts', '.tsx', '.js', '.jsx', '.py', '.rs', '.java', '.c', '.h', '.cpp', '.hpp',
    '.cs', '.rb', '.php', '.sh', '.ps1', '.lua', '.kt', '.swift', '.sql', '.bat', '.cmd',
]);
const WEB = new Set(['.html', '.htm', '.css', '.scss', '.vue', '.svelte']);
const DOC = new Set(['.md', '.txt', '.rst', '.pdf', '.doc', '.docx']);
const IMAGE = new Set(['.png', '.jpg', '.jpeg', '.gif', '.webp', '.bmp', '.ico', '.svg', '.tiff']);
const CONFIG = new Set(['.json', '.toml', '.yaml', '.yml', '.ini', '.cfg', '.conf', '.xml', '.env']);
const ARCHIVE = new Set(['.zip', '.7z', '.rar', '.tar', '.gz', '.xz', '.tgz']);

const SHELL = (inner: string, cls: string) =>
    `<svg class="ic ${cls}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">${inner}</svg>`;

const PAGE = 'M6.5 3h6.5l5 5v12a1 1 0 0 1-1 1H6.5a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z';
const FOLD = 'M13 3v5h5';

const ICON = {
    folder: SHELL('<path d="M3 7a1.5 1.5 0 0 1 1.5-1.5h3.6l2 2.2h8.4A1.5 1.5 0 0 1 20 9.2v8.3A1.5 1.5 0 0 1 18.5 19h-14A1.5 1.5 0 0 1 3 17.5z"/>', 'ic-folder'),
    code: SHELL(`<path d="${PAGE}"/><path d="${FOLD}"/><path d="M9.4 12.4l-1.7 1.9 1.7 1.9M13.6 12.4l1.7 1.9-1.7 1.9"/>`, 'ic-code'),
    web: SHELL(`<path d="${PAGE}"/><path d="${FOLD}"/><path d="M8 12.5h7M8 15.5h4.5"/>`, 'ic-web'),
    doc: SHELL(`<path d="${PAGE}"/><path d="${FOLD}"/><path d="M8 12h7M8 15h7M8 18h4"/>`, 'ic-doc'),
    special: SHELL(`<path d="${PAGE}"/><path d="${FOLD}"/><path d="M12 11.6l1 2.1 2.2.3-1.6 1.6.4 2.2-2-1.1-2 1.1.4-2.2-1.6-1.6 2.2-.3z"/>`, 'ic-special'),
    image: SHELL(`<path d="${PAGE}"/><path d="${FOLD}"/><circle cx="10.5" cy="13" r="1.3"/><path d="M7.5 19l3.4-3.6 2.2 2.1 1.8-1.7 2 2"/>`, 'ic-image'),
    config: SHELL(`<path d="${PAGE}"/><path d="${FOLD}"/><path d="M9 13h6M9 16.2h3.5"/>`, 'ic-config'),
    archive: SHELL('<path d="M3.5 6.5h17v3.5h-17z"/><path d="M5 10v8.5h14V10"/><path d="M10 13.5h4"/>', 'ic-archive'),
    binary: SHELL(`<path d="${PAGE}"/><path d="${FOLD}"/><circle cx="9.5" cy="15" r="1"/><circle cx="14.5" cy="15" r="1"/>`, 'ic-binary'),
    file: SHELL(`<path d="${PAGE}"/><path d="${FOLD}"/>`, 'ic-file'),
    unknown: SHELL('<circle cx="12" cy="12" r="8"/><path d="M12 9.5v.01M12 12.5v3"/>', 'ic-file'),
};

export function fileIcon(e: IconEntry): string {
    if (e.isDir) return ICON.folder;
    if (e.special) return ICON.special;
    const ext = e.ext;
    if (CODE.has(ext)) return ICON.code;
    if (WEB.has(ext)) return ICON.web;
    if (IMAGE.has(ext)) return ICON.image;
    if (ARCHIVE.has(ext)) return ICON.archive;
    if (CONFIG.has(ext)) return ICON.config;
    if (DOC.has(ext)) return ICON.doc;
    if (['.exe', '.dll', '.so', '.dylib', '.o', '.obj', '.class', '.pyc'].includes(ext)) return ICON.binary;
    if (!ext) return ICON.file;
    return ICON.unknown;
}
