// Перевод интерфейса. Ключ — русский текст: разметка остаётся на русском,
// а при английском языке текстовые узлы и подсказки подменяются по словарю.
import {en} from "./en";

export type Lang = "ru" | "en";

let lang: Lang = "ru";
const listeners: ((l: Lang) => void)[] = [];

export const getLang = () => lang;
export const locale = () => (lang === "en" ? "en-US" : "ru-RU");

/** Переводит строку; {name} заменяется значением из vars. */
export function t(ru: string, vars: Record<string, string | number> = {}): string {
    const s = lang === "en" ? (en[ru] ?? ru) : ru;
    return s.replace(/\{(\w+)\}/g, (_, k) => String(vars[k] ?? ""));
}

const norm = (s: string) => s.replace(/\s+/g, " ").trim();
const ATTRS = ["title", "aria-label", "placeholder"];

// Что было в разметке и что мы туда записали: узлы, которые потом поменял
// код, не трогаем — их перерисует сам код через t().
const texts = new WeakMap<Text, {orig: string; shown: string}>();
const attrs = new WeakMap<Element, Record<string, {orig: string; shown: string}>>();
let scanned = false;

function swap(orig: string): string {
    const key = norm(orig);
    if (!key || lang === "ru") return orig;
    const tr = en[key];
    if (!tr) return orig;
    const lead = orig.match(/^\s*/)![0];
    const tail = orig.match(/\s*$/)![0];
    return lead + tr + tail;
}

export function translateDOM(root: ParentNode = document.body) {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    for (let n = walker.nextNode() as Text | null; n; n = walker.nextNode() as Text | null) {
        const p = n.parentElement;
        if (!p || p.closest("script, style, [data-no-i18n]")) continue;
        let rec = texts.get(n);
        if (!rec && scanned) continue;
        if (!rec) rec = {orig: n.data, shown: n.data};
        else if (rec.shown !== n.data) continue;
        rec.shown = swap(rec.orig);
        if (n.data !== rec.shown) n.data = rec.shown;
        texts.set(n, rec);
    }
    const els = root instanceof Element ? [root, ...root.querySelectorAll("*")] : [...root.querySelectorAll("*")];
    for (const el of els) {
        for (const a of ATTRS) {
            const v = el.getAttribute(a);
            if (v === null) continue;
            const map = attrs.get(el) ?? {};
            let rec = map[a];
            if (!rec && scanned) continue;
            if (!rec) rec = {orig: v, shown: v};
            else if (rec.shown !== v) continue;
            rec.shown = swap(rec.orig);
            if (v !== rec.shown) el.setAttribute(a, rec.shown);
            map[a] = rec;
            attrs.set(el, map);
        }
    }
    scanned = true;
}

export function setLang(l: Lang) {
    lang = l;
    document.documentElement.lang = l;
    translateDOM();
    listeners.forEach((f) => f(l));
}

export function onLang(f: (l: Lang) => void) {
    listeners.push(f);
}
