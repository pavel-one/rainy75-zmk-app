// Привязка элементов к настройкам приложения (settings.json).
// data-cfg="секция.поле" — флажок, цвет, ползунок или число;
// data-seg="секция.поле" — сегменты-кнопки с data-v; data-out — подпись значения.
import {KeyboardService as KB} from "../bindings/rainy75";
import type {Settings} from "../bindings/rainy75/internal/settings/models";
import {getLang, locale, onLang} from "./i18n";

type Ctx = {toast(message: string, error?: boolean): void; errText(e: unknown): string};

let cfg: Settings | null = null;
let ctx: Ctx;
const listeners: ((s: Settings) => void)[] = [];

const $$ = <T extends HTMLElement = HTMLElement>(sel: string) => Array.from(document.querySelectorAll(sel)) as T[];

function get(path: string): unknown {
    let v: any = cfg;
    for (const p of path.split(".")) v = v?.[p];
    return v;
}

function set(path: string, value: unknown) {
    const parts = path.split(".");
    let o: any = cfg;
    for (const p of parts.slice(0, -1)) o = o[p];
    o[parts.at(-1)!] = value;
}

const fmt: Record<string, (v: number) => string> = {
    ms: (v) => `${(v / 1000).toLocaleString(locale(), {maximumFractionDigits: 2})} ${getLang() === "en" ? "s" : "с"}`,
    x: (v) => `${v.toLocaleString(locale())}×`,
};

function paintRange(input: HTMLInputElement) {
    const p = ((+input.value - +input.min) / (+input.max - +input.min)) * 100;
    input.style.setProperty("--p", `${p}%`);
}

export function render() {
    if (!cfg) return;
    for (const el of $$<HTMLInputElement>("[data-cfg]")) {
        const v = get(el.dataset.cfg!);
        if (el.type === "checkbox") el.checked = Boolean(v);
        else if (document.activeElement !== el) el.value = String(v ?? "");
        if (el.type === "range") paintRange(el);
    }
    for (const box of $$("[data-seg]")) {
        const v = String(get(box.dataset.seg!));
        box.querySelectorAll<HTMLElement>("button").forEach((b) => b.classList.toggle("active", b.dataset.v === v));
    }
    for (const out of $$("[data-out]")) {
        const v = Number(get(out.dataset.out!));
        out.textContent = (fmt[out.dataset.fmt ?? ""] ?? String)(v);
    }
    // Карточки с выключенным флажком «enabled» приглушаем.
    for (const card of $$(".card")) {
        const toggle = card.querySelector<HTMLInputElement>('.card-title [data-cfg$=".enabled"]');
        if (toggle) card.classList.toggle("off", !toggle.checked);
    }
    listeners.forEach((f) => f(cfg!));
}

let saveTimer: ReturnType<typeof setTimeout> | undefined;

function save() {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(async () => {
        if (!cfg) return;
        try {
            cfg = await KB.SetSettings(cfg);
        } catch (e) {
            ctx.toast(ctx.errText(e), true);
            cfg = await KB.Settings();
        }
        render();
    }, 250);
}

/** Меняет настройку и сохраняет её. */
export function update(path: string, value: unknown) {
    if (!cfg) return;
    set(path, value);
    render();
    save();
}

export const settings = () => cfg;

/** Вызывается после каждого изменения настроек. */
export function onSettings(f: (s: Settings) => void) {
    listeners.push(f);
}

onLang(() => render());

export async function initSettings(c: Ctx) {
    ctx = c;
    cfg = await KB.Settings();
    for (const el of $$<HTMLInputElement>("[data-cfg]")) {
        const path = el.dataset.cfg!;
        const read = () => (el.type === "checkbox" ? el.checked : el.dataset.num !== undefined ? Number(el.value) : el.value);
        el.addEventListener(el.type === "checkbox" || el.type === "number" ? "change" : "input", () => update(path, read()));
    }
    for (const box of $$("[data-seg]")) {
        const num = box.dataset.num !== undefined;
        box.querySelectorAll<HTMLElement>("button").forEach((b) =>
            b.addEventListener("click", () => update(box.dataset.seg!, num ? Number(b.dataset.v) : b.dataset.v)),
        );
    }
    render();
}
