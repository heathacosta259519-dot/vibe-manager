export function esc(s: string): string {
    return String(s ?? '').replace(/[&<>"']/g, c => ({
        '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
    }[c] as string));
}

/**
 * 把 git diff 文本渲染成 <pre>。
 * 每行是一个块级 span，这样增删行才能铺出整行的背景色块（行内元素只能盖住字符本身）；
 * 也正因为是块级，行之间不能再留换行符，否则每行会多出一个空行。
 */
export function renderDiffText(text: string): string {
    const lines = text.replace(/\n$/, '').split('\n').map(line => {
        let cls = '';
        if (line.startsWith('+++') || line.startsWith('---')) cls = 'diff-file';
        else if (line.startsWith('+')) cls = 'diff-add';
        else if (line.startsWith('-')) cls = 'diff-del';
        else if (line.startsWith('@@')) cls = 'diff-hunk';
        else if (line.startsWith('diff ') || line.startsWith('index ') || line.startsWith('new file')) cls = 'diff-meta';
        return `<span class="diff-line ${cls}">${esc(line)}</span>`;
    });
    return `<pre class="diff">${lines.join('')}</pre>`;
}

/**
 * 渲染带行号的 diff。逐个 @@ 头推出目标侧行号，把改动行标成 changed，
 * 这样界面既知道每行是文件的第几行，也能把第一处改动滚到眼前。
 */
export function renderNumberedDiff(text: string): string {
    const out: string[] = [];
    let no = 0;
    let inHunk = false;

    for (const raw of text.replace(/\n$/, '').split('\n')) {
        if (raw.startsWith('@@')) {
            const m = /^@@ -\d+(?:,\d+)? \+(\d+)/.exec(raw);
            no = m ? Number(m[1]) : 0;
            inHunk = true;
            out.push(`<span class="diff-line diff-hunk">${esc(raw)}</span>`);
            continue;
        }
        if (!inHunk) {
            const cls = raw.startsWith('+++') || raw.startsWith('---') ? 'diff-file' : 'diff-meta';
            out.push(`<span class="diff-line ${cls}">${esc(raw)}</span>`);
            continue;
        }
        const mark = raw[0] ?? ' ';
        if (mark === '\\') {
            // "\ No newline at end of file"
            out.push(`<span class="diff-line diff-meta">${esc(raw)}</span>`);
            continue;
        }
        const body = raw.slice(1);
        if (mark === '+') {
            out.push(diffLineHtml('diff-add changed', String(no), body));
            no++;
        } else if (mark === '-') {
            // 删除的行在目标侧不存在，行号列留空
            out.push(diffLineHtml('diff-del changed', '', body));
        } else {
            out.push(diffLineHtml('', String(no), body));
            no++;
        }
    }
    return `<pre class="diff">${out.join('')}</pre>`;
}

function diffLineHtml(cls: string, no: string, body: string): string {
    return `<span class="diff-line${cls ? ' ' + cls : ''}"><span class="diff-no">${no}</span><span class="diff-code">${esc(body)}</span></span>`;
}

export function fmtTime(unix: number): string {
    if (!unix) return '—';
    const d = new Date(unix * 1000);
    const p = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

export function fmtAgo(unix: number): string {
    if (!unix) return '—';
    const secs = Math.floor(Date.now() / 1000 - unix);
    if (secs < 60) return '刚刚';
    const mins = Math.floor(secs / 60);
    if (mins < 60) return `${mins} 分钟前`;
    const hours = Math.floor(mins / 60);
    if (hours < 24) return `${hours} 小时前`;
    const days = Math.floor(hours / 24);
    if (days < 30) return `${days} 天前`;
    const months = Math.floor(days / 30);
    if (months < 12) return `${months} 个月前`;
    return `${Math.floor(months / 12)} 年前`;
}

export function daysIdle(unix: number): number {
    if (!unix) return 0;
    return Math.floor((Date.now() / 1000 - unix) / 86400);
}

export function fmtSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
    return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

const TYPE_NAMES: Record<string, string> = {
    '.go': 'Go 源文件',
    '.ts': 'TypeScript',
    '.tsx': 'TypeScript React',
    '.js': 'JavaScript',
    '.jsx': 'JavaScript React',
    '.py': 'Python 脚本',
    '.rs': 'Rust 源文件',
    '.java': 'Java 源文件',
    '.c': 'C 源文件',
    '.h': 'C 头文件',
    '.cpp': 'C++ 源文件',
    '.hpp': 'C++ 头文件',
    '.cs': 'C# 源文件',
    '.rb': 'Ruby 脚本',
    '.php': 'PHP 脚本',
    '.sh': 'Shell 脚本',
    '.ps1': 'PowerShell 脚本',
    '.html': 'HTML 文档',
    '.css': '样式表',
    '.vue': 'Vue 组件',
    '.md': 'Markdown 文档',
    '.txt': '文本文件',
    '.json': 'JSON 配置',
    '.toml': 'TOML 配置',
    '.yml': 'YAML 配置',
    '.yaml': 'YAML 配置',
    '.ini': 'INI 配置',
    '.xml': 'XML 文档',
    '.png': 'PNG 图片',
    '.jpg': 'JPEG 图片',
    '.jpeg': 'JPEG 图片',
    '.gif': 'GIF 图片',
    '.webp': 'WebP 图片',
    '.bmp': 'BMP 图片',
    '.ico': '图标文件',
    '.svg': 'SVG 图片',
    '.zip': 'ZIP 压缩包',
    '.7z': '7z 压缩包',
    '.rar': 'RAR 压缩包',
    '.tar': 'TAR 归档',
    '.gz': 'GZip 压缩',
    '.exe': '可执行文件',
    '.dll': '动态库',
    '.so': '共享库',
    '.pdf': 'PDF 文档',
    '.db': 'SQLite 数据库',
    '.sqlite': 'SQLite 数据库',
    '.log': '日志文件',
};

export function typeLabel(isDir: boolean, ext: string): string {
    if (isDir) return '文件夹';
    if (!ext) return '文件';
    return TYPE_NAMES[ext] ?? `${ext.slice(1).toUpperCase()} 文件`;
}
