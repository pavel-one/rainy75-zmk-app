// Схема клавиатуры: 83 позиции keymap с геометрией из прошивки.
import type {Key, Layout} from "../bindings/rainy75/internal/layout/models";

export type Board = {el: HTMLElement; keys: HTMLElement[]};

export function renderBoard(el: HTMLElement, layout: Layout, interactive = false): Board {
    el.innerHTML = "";
    el.style.setProperty("--kh", String(layout.height));
    const keys: HTMLElement[] = [];
    for (const k of layout.keys ?? []) {
        const b = document.createElement(interactive ? "button" : "div");
        b.className = "bkey";
        b.dataset.pos = String(k.pos);
        b.style.setProperty("--x", String(k.x));
        b.style.setProperty("--y", String(k.y));
        b.style.setProperty("--w", String(k.w));
        b.style.setProperty("--h", String(k.h));
        el.append(b);
        keys[k.pos] = b;
    }
    const fit = () => el.style.setProperty("--u", `${el.clientWidth / layout.width}px`);
    new ResizeObserver(fit).observe(el);
    fit();
    return {el, keys};
}

const esc = (s: string) => s.replace(/[&<>"]/g, (c) => ({"&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;"})[c]!);

// Подписи зависят от режима: в macOS вместо Win и Alt — ⌥ и ⌘, F-ряд — функции Mac.
export function paintLabels(board: Board, layout: Layout, mac: boolean) {
    for (const k of layout.keys ?? []) {
        const text = mac ? k.macLabel : k.label;
        const b = board.keys[k.pos];
        b.innerHTML = `<span>${esc(text)}</span>`;
        b.classList.toggle("long", text.length > 4);
    }
}

export const keyCode = (k: Key, mac: boolean) => (mac ? k.macCode : k.code);
