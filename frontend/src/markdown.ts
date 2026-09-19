import { esc } from './format';

/**
 * 极简 Markdown 渲染：只覆盖任务书用得到的语法——标题、列表、代码块、行内代码、加粗。
 * 刻意不引第三方库：任务书的排版就这么几种。
 */
export function renderMarkdown(src: string): string {
    const lines = src.replace(/\r\n/g, '\n').split('\n');
    const out: string[] = [];
    let inFence = false;
    let list: 'ul' | 'ol' | null = null;

    const closeList = (): void => {
        if (list) {
            out.push(`</${list}>`);
            list = null;
        }
    };

    for (const line of lines) {
        if (/^\s*```/.test(line)) {
            closeList();
            out.push(inFence ? '</pre>' : '<pre class="md-pre">');
            inFence = !inFence;
            continue;
        }
        if (inFence) {
            out.push(esc(line));
            continue;
        }
        if (/^\s*$/.test(line)) {
            closeList();
            continue;
        }

        const heading = /^(#{1,6})\s+(.*)$/.exec(line);
        if (heading) {
            closeList();
            const level = heading[1].length;
            out.push(`<h${level} class="md-h">${inline(heading[2])}</h${level}>`);
            continue;
        }

        const ul = /^\s*[-*+]\s+(.*)$/.exec(line);
        if (ul) {
            if (list !== 'ul') {
                closeList();
                out.push('<ul class="md-ul">');
                list = 'ul';
            }
            out.push(`<li>${inline(ul[1])}</li>`);
            continue;
        }

        const ol = /^\s*\d+[.、]\s+(.*)$/.exec(line);
        if (ol) {
            if (list !== 'ol') {
                closeList();
                out.push('<ol class="md-ol">');
                list = 'ol';
            }
            out.push(`<li>${inline(ol[1])}</li>`);
            continue;
        }

        closeList();
        out.push(`<p class="md-p">${inline(line)}</p>`);
    }

    closeList();
    if (inFence) out.push('</pre>');
    return out.join('\n');
}

function inline(s: string): string {
    return esc(s)
        .replace(/`([^`]+)`/g, '<code>$1</code>')
        .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
}
