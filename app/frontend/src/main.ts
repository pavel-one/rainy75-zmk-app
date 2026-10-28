import "./style.css";
import {Browser, Events} from "@wailsio/runtime";
import {KeyboardService as KB} from "../bindings/rainy75";
import type {AnimationParams, DeviceStatus, FirmwareFile, KeyColor, LightingState, Progress} from "../bindings/rainy75";
import type {Effect} from "../bindings/rainy75/internal/effects/models";
import type {Layout} from "../bindings/rainy75/internal/layout/models";
import type {Image} from "../bindings/rainy75/internal/rainy/models";
import type {PomodoroState} from "../bindings/rainy75/internal/lights/models";
import type {Status as BatteryStatus} from "../bindings/rainy75/internal/battery/models";
import {type Board, keyCode, paintLabels, renderBoard} from "./board";
import {initSettings, onSettings, settings, update} from "./settings";
import {getLang, type Lang, onLang, setLang, t, translateDOM} from "./i18n";
import type {UpdateInfo} from "../bindings/rainy75";

type UpdateProgress = {done: number; total: number};

// Запоминаем исходную разметку до того, как код начнёт её менять.
translateDOM();

const $ = <T extends HTMLElement = HTMLElement>(sel: string) => document.querySelector(sel) as T;
const $$ = <T extends HTMLElement = HTMLElement>(sel: string) => Array.from(document.querySelectorAll(sel)) as T[];
const root = document.documentElement;

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e ?? "")) || t("Неизвестная ошибка");
const esc = (s: string) => s.replace(/[&<>"]/g, (c) => ({"&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;"})[c]!);

function toast(message: string, error = false) {
    const el = document.createElement("div");
    el.className = "toast" + (error ? " error" : "");
    el.textContent = message;
    $("#toasts").append(el);
    setTimeout(() => {
        el.classList.add("out");
        setTimeout(() => el.remove(), 300);
    }, error ? 5000 : 2600);
}

function debounce<A extends unknown[]>(fn: (...a: A) => void, ms: number) {
    let t: ReturnType<typeof setTimeout> | undefined;
    return (...a: A) => {
        clearTimeout(t);
        t = setTimeout(() => fn(...a), ms);
    };
}

// localStorage может быть недоступен — это только удобство.
const store = {
    get(key: string): string | null {
        try {
            return localStorage.getItem(key);
        } catch {
            return null;
        }
    },
    set(key: string, value: string) {
        try {
            localStorage.setItem(key, value);
        } catch {
            /* не сохраняем */
        }
    },
};

function paintRange(input: HTMLInputElement) {
    const p = ((+input.value - +input.min) / (+input.max - +input.min)) * 100;
    input.style.setProperty("--p", `${p}%`);
}

let page = "dashboard";
$$(".nav-item").forEach((btn) =>
    btn.addEventListener("click", () => {
        page = btn.dataset.page!;
        $$(".nav-item").forEach((b) => b.classList.toggle("active", b === btn));
        $$(".page").forEach((p) => p.classList.toggle("active", p.id === `page-${page}`));
        if (page === "firmware") loadFirmware();
        if (page === "animations") loadPermissions();
    }),
);

let device: DeviceStatus = {connected: false, connection: "", port: "", firmware: "", windows: false, host: false, message: ""};
let layout: Layout | null = null;

const plugIcon = `<svg class="big" viewBox="0 0 24 24"><path d="M9 2v6M15 2v6M6 8h12v3a6 6 0 0 1-12 0zM12 17v5"/></svg>`;

function renderDevice(st: DeviceStatus) {
    const was = device;
    device = st;
    $("#dev-dot").classList.toggle("on", st.connected);
    $("#dev-title").textContent = st.connected ? t("Rainy75 подключена") : t("Не подключена");
    const channel = st.connection === "bluetooth" ? "Bluetooth · SMP" : "USB-C · SMP";
    $("#dev-rows").innerHTML = st.connected
        ? `<div><span>${t("Канал")}</span><b><span class="badge">${channel}</span></b></div>
           <div><span>${t("Прошивка")}</span><b>${esc(st.firmware)}</b></div>
           <div><span>${t("Режим")}</span><b>${st.windows ? "Windows" : "macOS"}</b></div>
           <div><span>${t("Подсветка")}</span><b>${st.host ? t("с компьютера") : t("эффекты прошивки")}</b></div>
           ${st.connection === "usb" ? `<div><span>${t("Порт")}</span><b class="port">${esc(st.port.replace("/dev/", ""))}</b></div>` : ""}`
        : `<div><span>${esc(st.message)}</span></div>`;

    for (const block of $$("[data-needs]")) {
        block.classList.toggle("hide", !st.connected);
        const empty = block.parentElement!.querySelector(`[data-empty]`) as HTMLElement;
        empty.classList.toggle("show", !st.connected);
        if (!st.connected) empty.innerHTML = `${plugIcon}<b>${t("Клавиатура не найдена")}</b><span>${esc(st.message)}</span>`;
    }
    osSwitch.disabled = !st.connected;
    renderOSMode(st.windows ? "windows" : "mac");
    $("#host-badge").textContent = st.connected ? (st.host ? t("цвета с компьютера") : t("работают эффекты прошивки")) : "";
    $("#host-badge").classList.toggle("on", st.host);
    if (was.windows !== st.windows || was.connected !== st.connected) refreshTesterLabels();
    renderFlashButton();
}

Events.On("device:status", (ev) => renderDevice(ev.data as DeviceStatus));

const osSwitch = $<HTMLButtonElement>("#os-switch");

function renderOSMode(mode: string) {
    osSwitch.setAttribute("aria-checked", String(mode === "mac"));
    $$(".os-label").forEach((l) => l.classList.toggle("active", l.dataset.mode === mode));
}

async function setOSMode(target: string) {
    if (osSwitch.disabled || osSwitch.classList.contains("busy")) return;
    const prev = device.windows ? "windows" : "mac";
    if (prev === target) return;
    renderOSMode(target);
    osSwitch.classList.add("busy");
    try {
        const mode = await KB.SetOSMode(target);
        renderOSMode(mode);
        toast(mode === "mac" ? t("macOS: ⌥ Option и ⌘ Command, F-ряд — функции Mac") : t("Windows: Win и Alt, обычный F-ряд"));
    } catch (e) {
        renderOSMode(prev);
        toast(errText(e), true);
    } finally {
        osSwitch.classList.remove("busy");
    }
}

osSwitch.addEventListener("click", () => setOSMode(osSwitch.getAttribute("aria-checked") === "true" ? "windows" : "mac"));
$$(".os-label").forEach((l) => l.addEventListener("click", () => setOSMode(l.dataset.mode!)));

const CANVAS_KEY = "rainy75.canvas";
const BLACK = "#000000";
let canvas: string[] = Array(83).fill(BLACK);
let paintBoard: Board;
let animBoard: Board; // мини-схема на странице «Анимации»
let tool = "paint";
let color = store.get("rainy75.color") ?? "#7c8cff";
let liveFrame: string[] | null = null; // кадр, который сейчас горит на клавиатуре

try {
    const saved = JSON.parse(store.get(CANVAS_KEY) ?? "null");
    if (Array.isArray(saved) && saved.length === 83) canvas = saved.map((c) => (/^#[0-9a-f]{6}$/i.test(c) ? c : BLACK));
} catch {
    /* холст по умолчанию */
}

const rgb = (hex: string) => ({r: parseInt(hex.slice(1, 3), 16), g: parseInt(hex.slice(3, 5), 16), b: parseInt(hex.slice(5, 7), 16)});
const keyColor = (pos: number): KeyColor => ({pos, ...rgb(canvas[pos])});

function lightKey(b: HTMLElement, c: string) {
    const {r, g, b: bl} = rgb(c);
    const lit = c !== BLACK;
    b.style.setProperty("--key", lit ? c : "");
    b.classList.toggle("lit", lit);
    b.classList.toggle("dark-text", lit && 0.299 * r + 0.587 * g + 0.114 * bl > 150);
}

// Схема показывает то, что горит на клавиатуре, а без кадров — холст.
function paintBoardKeys() {
    const src = liveFrame ?? canvas;
    const live = liveFrame !== null && lighting.base !== "canvas";
    for (const board of [paintBoard, animBoard]) {
        if (!board) continue;
        board.keys.forEach((b, i) => lightKey(b, src[i] ?? BLACK));
        board.el.classList.toggle("live", live);
    }
}

function setColor(hex: string) {
    color = hex.toLowerCase();
    $<HTMLInputElement>("#color").value = color;
    $("#color-hex").textContent = color.toUpperCase();
    root.style.setProperty("--pick", color);
    store.set("rainy75.color", color);
}

const saveCanvas = debounce(() => store.set(CANVAS_KEY, JSON.stringify(canvas)), 300);

// Холст уходит целиком: приложение само отправит в клавиатуру только изменения.
const sendCanvas = debounce(async () => {
    try {
        await KB.SetFrame(canvas.map((_, i) => keyColor(i)));
    } catch (e) {
        toast(errText(e), true);
    }
}, 30);

function applyTool(pos: number) {
    if (tool === "pick") {
        const src = liveFrame ?? canvas;
        if (src[pos] !== BLACK) setColor(src[pos]);
        selectTool("paint");
        return;
    }
    canvas[pos] = tool === "erase" ? BLACK : color;
    liveFrame = null;
    paintBoardKeys();
    saveCanvas();
    sendCanvas();
}

function selectTool(t: string) {
    tool = t;
    $$("#tools button").forEach((b) => b.classList.toggle("active", b.dataset.tool === t));
    $("#paint-board").dataset.tool = t;
}

function initPaint() {
    paintBoard = renderBoard($("#paint-board"), layout!, true);
    paintLabels(paintBoard, layout!, !device.windows);
    paintBoardKeys();
    let dragging = false;
    for (const b of paintBoard.keys) {
        const pos = +b.dataset.pos!;
        b.addEventListener("pointerdown", (e) => {
            e.preventDefault();
            dragging = tool !== "pick";
            applyTool(pos);
        });
        b.addEventListener("pointerenter", () => dragging && applyTool(pos));
    }
    window.addEventListener("pointerup", () => (dragging = false));
}

Events.On("lighting:frame", (ev) => {
    const frame = ev.data as string[] | null;
    liveFrame = frame?.length ? frame : null; // пустой кадр — горят эффекты прошивки
    if (paintBoard) paintBoardKeys();
});

$$("#tools button").forEach((b) => b.addEventListener("click", () => selectTool(b.dataset.tool!)));
$<HTMLInputElement>("#color").addEventListener("input", (e) => setColor((e.target as HTMLInputElement).value));

const presets: [string, string][] = [
    ["Красный", "#ff2a3c"], ["Оранжевый", "#ff7a1a"], ["Жёлтый", "#ffd21a"], ["Зелёный", "#2bd96b"],
    ["Бирюзовый", "#1ad6d6"], ["Синий", "#2a6bff"], ["Фиолетовый", "#9a3cff"], ["Розовый", "#ff3cb4"], ["Белый", "#ffffff"],
];
for (const [name, hex] of presets) {
    const b = document.createElement("button");
    b.className = "preset";
    b.dataset.name = name;
    b.title = name;
    b.style.background = hex;
    b.addEventListener("click", () => setColor(hex));
    $("#presets").append(b);
}

async function fillAll(hex: string, message: string) {
    canvas = canvas.map(() => hex);
    liveFrame = null;
    paintBoardKeys();
    saveCanvas();
    try {
        const {r, g, b} = rgb(hex);
        await KB.FillColor(r, g, b);
        toast(message);
    } catch (e) {
        toast(errText(e), true);
    }
}

$("#fill-all").addEventListener("click", () => fillAll(color, t("Клавиатура залита цветом")));
$("#clear-canvas").addEventListener("click", () => fillAll(BLACK, t("Подсветка погашена")));
$("#restore-effects").addEventListener("click", async () => {
    try {
        await KB.RestoreEffects();
        toast(t("Работают эффекты прошивки"));
    } catch (e) {
        toast(errText(e), true);
    }
});

let effects: Effect[] = [];
let lighting: LightingState = {base: "firmware", animation: {} as AnimationParams};

// Параметры анимации помним между запусками приложения.
const ANIM_KEY = "rainy75.animation";
let anim: AnimationParams = {kind: "", color: "#7c8cff", color2: "#ff3cb4", speed: 1, brightness: 0.8, hueShift: 0, saturation: 1};
try {
    Object.assign(anim, JSON.parse(store.get(ANIM_KEY) ?? "{}"));
} catch {
    /* по умолчанию */
}

function renderLighting(st: LightingState) {
    lighting = st;
    const running = st.base === "animation" ? st.animation.kind : "";
    $$(".effect").forEach((el) => el.classList.toggle("active", el.dataset.key === running));
    $("#anim-stop").hidden = !running;
    const name = effects.find((e) => e.key === running)?.name;
    $("#anim-state").textContent = running ? t("идёт: {name}", {name: name ?? running}) : "";
    const viz = st.base === "visualizer";
    const btn = $("#viz-toggle");
    btn.classList.toggle("ghost", viz);
    btn.lastChild!.textContent = " " + (viz ? t("Выключить") : t("Включить"));
    if (paintBoard) paintBoardKeys();
    renderAnimControls();
}

Events.On("lighting:state", (ev) => renderLighting(ev.data as LightingState));

// Видны только настройки, которые использует выбранная анимация.
function renderAnimControls() {
    const e = effects.find((x) => x.key === anim.kind);
    const colors = e?.colors ?? 1;
    $$(".anim-col").forEach((el) => {
        const f = el.dataset.for;
        el.hidden = !e ? f === "palette" : f === "palette" ? colors !== 0 : f === "color2" ? colors < 2 : colors === 0;
    });
    $<HTMLInputElement>("#anim-color").value = anim.color;
    $<HTMLInputElement>("#anim-color2").value = anim.color2;
    $("#anim-color-hex").textContent = anim.color.toUpperCase();
    $("#anim-color2-hex").textContent = anim.color2.toUpperCase();
    for (const [id, v] of [["#anim-hue", anim.hueShift], ["#anim-sat", anim.saturation], ["#anim-speed", anim.speed], ["#anim-bright", anim.brightness]] as const) {
        const input = $<HTMLInputElement>(id);
        if (document.activeElement !== input) input.value = String(v);
        paintRange(input);
    }
    $("#anim-hue-out").textContent = `${anim.hueShift > 0 ? "+" : ""}${anim.hueShift}°`;
    $("#anim-sat-out").textContent = `${Math.round(anim.saturation * 100)}%`;
    $("#anim-speed-out").textContent = `${anim.speed}×`;
    $("#anim-bright-out").textContent = `${Math.round(anim.brightness * 100)}%`;
}

async function startAnimation(kind = anim.kind) {
    if (!kind) return;
    anim.kind = kind;
    store.set(ANIM_KEY, JSON.stringify(anim));
    renderAnimControls();
    try {
        await KB.StartAnimation(anim);
    } catch (e) {
        toast(errText(e), true);
    }
}

// Пока анимация идёт, изменения применяются сразу.
const retune = debounce(() => {
    store.set(ANIM_KEY, JSON.stringify(anim));
    if (lighting.base === "animation") startAnimation();
}, 80);

function buildEffects() {
    const box = $("#effects");
    box.innerHTML = "";
    for (const [title, test] of [[t("Свои цвета"), (e: Effect) => e.colors === 0], [t("Выбранными цветами"), (e: Effect) => e.colors > 0]] as const) {
        const group = effects.filter(test);
        if (!group.length) continue;
        const h = document.createElement("div");
        h.className = "effects-group";
        h.textContent = title;
        box.append(h);
        for (const e of group) {
            const b = document.createElement("button");
            b.className = `effect t-${e.key}`;
            b.dataset.key = e.key;
            b.innerHTML = `<div class="effect-thumb"></div><span>${esc(e.name)}</span>${e.colors === 2 ? '<i class="two"></i>' : ""}`;
            b.addEventListener("click", () => startAnimation(e.key));
            box.append(b);
        }
    }
}

for (const [id, field] of [["#anim-color", "color"], ["#anim-color2", "color2"]] as const) {
    $<HTMLInputElement>(id).addEventListener("input", (e) => {
        anim[field] = (e.target as HTMLInputElement).value;
        renderAnimControls();
        retune();
    });
}
for (const [id, field] of [["#anim-hue", "hueShift"], ["#anim-sat", "saturation"], ["#anim-speed", "speed"], ["#anim-bright", "brightness"]] as const) {
    $<HTMLInputElement>(id).addEventListener("input", (e) => {
        anim[field] = +(e.target as HTMLInputElement).value;
        renderAnimControls();
        retune();
    });
}

$("#anim-stop").addEventListener("click", async () => {
    try {
        await KB.StopAnimation();
    } catch (e) {
        toast(errText(e), true);
    }
});

$("#viz-toggle").addEventListener("click", async () => {
    try {
        if (lighting.base === "visualizer") await KB.RestoreEffects();
        else await KB.StartVisualizer();
    } catch (e) {
        toast(errText(e), true);
    }
});

async function loadPermissions() {
    const p = await KB.Permissions();
    const reactive = settings()?.reactive.enabled ?? false;
    $("#perm-input").hidden = !p.supported || p.inputMonitoring || !reactive;
}

onSettings(() => {
    if (page === "animations") loadPermissions();
    renderFlashCmd();
});

$("#perm-input-btn").addEventListener("click", async () => {
    await KB.RequestInputMonitoring();
    toast(t("Включите Rainy75 в «Мониторинге ввода» и перезапустите приложение"));
});

for (const [id, mode] of [["#ind-test-mac", "mac"], ["#ind-test-win", "windows"]] as const) {
    $(id).addEventListener("click", async () => {
        const cfg = settings();
        if (!cfg) return;
        try {
            await KB.PreviewIndicator(mode, cfg.indicator);
        } catch (e) {
            toast(errText(e), true);
        }
    });
}

function renderFlashCmd() {
    const c = $<HTMLInputElement>("#flash-color").value;
    $("#flash-cmd").textContent = `open -a Rainy75 --args --flash '${c}' --times 3`;
}
$("#flash-color").addEventListener("input", renderFlashCmd);
$("#flash-test").addEventListener("click", async () => {
    try {
        await KB.Flash($<HTMLInputElement>("#flash-color").value, 3);
    } catch (e) {
        toast(errText(e), true);
    }
});

$("#bluetooth").addEventListener("change", (e) => update("bluetooth", (e.target as HTMLInputElement).checked));
onSettings((s) => ($<HTMLInputElement>("#bluetooth").checked = s.bluetooth));

let pomo: PomodoroState = {phase: "idle", running: false, left: 0, total: 0};
let pomoAt = Date.now(); // когда получено состояние — отсчёт ведём локально

function renderPomodoro() {
    let left = pomo.left;
    if (pomo.running) left = Math.max(0, pomo.left - Math.floor((Date.now() - pomoAt) / 1000));
    if (pomo.phase === "idle") left = (settings()?.pomodoro.work ?? 25) * 60;
    $("#pomo-time").textContent = `${String(Math.floor(left / 60)).padStart(2, "0")}:${String(left % 60).padStart(2, "0")}`;
    $("#pomo-phase").textContent =
        pomo.phase === "idle" ? t("не запущен") : `${pomo.phase === "work" ? t("работа") : t("отдых")}${pomo.running ? "" : t(" · пауза")}`;
    $(".pomo-card").dataset.phase = pomo.phase;
    $("#pomo-start").textContent = pomo.running ? t("Пауза") : pomo.phase === "idle" ? t("Старт") : t("Продолжить");
}

function setPomodoro(st: PomodoroState) {
    pomo = st;
    pomoAt = Date.now();
    renderPomodoro();
}

Events.On("pomodoro:state", (ev) => {
    const st = ev.data as PomodoroState;
    setPomodoro(st);
    if (st.phase !== "idle" && st.running && st.left === st.total) toast(st.phase === "work" ? t("Помодоро: за работу") : t("Помодоро: перерыв"));
});
setInterval(() => pomo.running && renderPomodoro(), 1000);
onSettings(renderPomodoro);

$("#pomo-start").addEventListener("click", async () => setPomodoro(pomo.running ? await KB.PomodoroPause() : await KB.PomodoroStart()));
$("#pomo-skip").addEventListener("click", async () => setPomodoro(await KB.PomodoroSkip()));
$("#pomo-stop").addEventListener("click", async () => setPomodoro(await KB.PomodoroStop()));

const autostartBox = $<HTMLInputElement>("#autostart");
autostartBox.addEventListener("change", async () => {
    try {
        const on = await KB.SetAutostart(autostartBox.checked);
        autostartBox.checked = on;
        toast(on ? t("Rainy75 будет запускаться при входе — сразу в трей") : t("Автозапуск выключен"));
    } catch (e) {
        autostartBox.checked = !autostartBox.checked;
        toast(errText(e), true);
    }
});
Events.On("autostart:changed", (ev) => (autostartBox.checked = ev.data as boolean));

let tsBoard: Board;
const tsTested = new Set<number>();
let tsLog: string[] = [];

const testerMac = () => !(device.connected && device.windows);

function refreshTesterLabels() {
    if (!layout || !tsBoard) return;
    paintLabels(tsBoard, layout, testerMac());
    paintLabels(paintBoard, layout, testerMac());
    $("#ts-note").textContent =
        (testerMac()
            ? t("Схема в режиме macOS: F-ряд шлёт функции Mac, их браузер не видит — проверяйте его с Fn. ")
            : t("Схема в режиме Windows. ")) +
        t("Fn и системные сочетания (⌘Tab и т. п.) перехватывает ОС, в тестере их не видно.");
    renderTesterStats();
}

function positionsOf(code: string) {
    return (layout?.keys ?? []).filter((k) => keyCode(k, testerMac()) === code).map((k) => k.pos);
}

function renderTesterStats() {
    const total = (layout?.keys ?? []).filter((k) => keyCode(k, testerMac())).length;
    $("#ts-count").textContent = String(tsTested.size);
    $("#ts-total").textContent = t("из {n} клавиш", {n: total});
    $("#ts-bar").style.width = `${total ? (tsTested.size / total) * 100 : 0}%`;
}

function onKey(e: KeyboardEvent, down: boolean) {
    if (page !== "tester" || !layout) return;
    e.preventDefault();
    const pos = positionsOf(e.code);
    for (const i of pos) {
        tsBoard.keys[i].classList.toggle("pressed", down);
        if (down) {
            tsBoard.keys[i].classList.add("tested");
            tsTested.add(i);
        }
    }
    if (!down || e.repeat) return;
    const where = pos.length ? t("позиция {n}", {n: pos[0]}) : t("нет на схеме");
    tsLog.unshift(`<div><kbd>${esc(e.code || e.key)}</kbd><span>${esc(e.key)}</span><span class="muted">${where}</span></div>`);
    tsLog = tsLog.slice(0, 12);
    $("#ts-log").innerHTML = tsLog.join("");
    renderTesterStats();
}

function resetTester() {
    tsTested.clear();
    tsLog = [];
    tsBoard.keys.forEach((b) => b.classList.remove("tested", "pressed"));
    $("#ts-log").innerHTML = `<div class="muted small">${t("Нажмите любую клавишу…")}</div>`;
    renderTesterStats();
}

$("#ts-reset").addEventListener("click", resetTester);
window.addEventListener("keydown", (e) => onKey(e, true));
window.addEventListener("keyup", (e) => onKey(e, false));
window.addEventListener("blur", () => tsBoard?.keys.forEach((b) => b.classList.remove("pressed")));

const CIRC = 2 * Math.PI * 84;

function renderBattery(st: BatteryStatus) {
    const pct = st.available ? st.percent : 0;
    $("#gauge-value").style.strokeDashoffset = String(CIRC * (1 - pct / 100));
    const [g1, g2] = pct > 50 ? ["#3ddc97", "#44c7ff"] : pct > 20 ? ["#ffb547", "#ffd66b"] : ["#ff5d73", "#ff8a5d"];
    root.style.setProperty("--g1", g1);
    root.style.setProperty("--g2", g2);
    $("#battery-pct").textContent = st.available ? `${pct}%` : "—";
    $("#battery-label").textContent = st.available ? (st.charging ? t("заряжается") : t("заряд")) : t("нет данных");
    $("#battery-state").textContent = !st.available ? t("Неизвестно") : st.charging ? t("⚡ Заряжается") : pct <= 20 ? t("Низкий заряд") : t("От батареи");
    $("#battery-source").textContent = st.source || "—";
    $("#battery-note").hidden = st.available;
    $("#battery-message").textContent = st.message;
}

Events.On("battery:status", (ev) => renderBattery(ev.data as BatteryStatus));

$("#battery-refresh").addEventListener("click", async () => {
    const btn = $<HTMLButtonElement>("#battery-refresh");
    btn.classList.add("spin");
    try {
        renderBattery(await KB.Battery());
    } finally {
        setTimeout(() => btn.classList.remove("spin"), 400);
    }
});

let fwFile: FirmwareFile | null = null;
let flashing = false;

function slotCard(img: Image) {
    const flags = [
        img.active && [t("работает"), "ok"],
        img.confirmed && [t("подтверждена"), ""],
        img.pending && [t("ждёт перезагрузки"), "warn"],
        !img.bootable && [t("не загрузочная"), "bad"],
    ].filter(Boolean) as [string, string][];
    return `<div class="slot ${img.active ? "active" : ""}">
        <div class="slot-num">${img.slot}</div>
        <div><div class="muted small">${t("Слот {n}", {n: img.slot})}${img.slot === 0 ? t(" · основной") : t(" · для обновления")}</div>
        <div class="fw-version">${esc(img.version || "—")}</div></div>
        <div class="slot-flags">${flags.map(([t, c]) => `<span class="flag ${c}">${t}</span>`).join("")}</div>
    </div>`;
}

async function loadFirmware() {
    const btn = $<HTMLButtonElement>("#fw-refresh");
    btn.classList.add("spin");
    try {
        const info = await KB.FirmwareInfo();
        const imgs = info.images ?? [];
        $("#slots").innerHTML = imgs.length ? imgs.map(slotCard).join("") : `<div class="muted small">${t("Прошивка не сообщила слоты.")}</div>`;
    } catch (e) {
        $("#slots").innerHTML = `<div class="empty show">${plugIcon}<b>${t("Нет связи с клавиатурой")}</b><span>${esc(errText(e))}</span></div>`;
    } finally {
        btn.classList.remove("spin");
        renderFlashButton();
    }
}

function renderFlashButton() {
    $<HTMLButtonElement>("#fw-flash").disabled = !fwFile || flashing || !device.connected;
    $<HTMLButtonElement>("#fw-choose").disabled = flashing;
}

$("#fw-refresh").addEventListener("click", loadFirmware);

$("#fw-choose").addEventListener("click", async () => {
    try {
        const f = await KB.ChooseFirmware();
        if (!f.path) return;
        fwFile = f;
        $("#fw-file").innerHTML = `${esc(f.name)} <span class="muted small">v${esc(f.version)} · ${(f.size / 1024).toFixed(0)} ${t("КБ")}</span>`;
    } catch (e) {
        toast(errText(e), true);
    } finally {
        renderFlashButton();
    }
});

$("#fw-flash").addEventListener("click", async () => {
    if (!fwFile) return;
    const slow = device.connection === "bluetooth" ? t(" По Bluetooth загрузка займёт несколько минут.") : "";
    if (!confirm(t("Прошить {name} (v{version})? Не отключайте клавиатуру до её перезагрузки.", {name: fwFile.name, version: fwFile.version}) + slow)) return;
    flashing = true;
    renderFlashButton();
    $("#fw-progress").hidden = false;
    $("#fw-bar").style.width = "0";
    try {
        await KB.FlashFirmware(fwFile.path);
        toast(t("Прошивка загружена, клавиатура перезагружается"));
        setTimeout(loadFirmware, 8000);
    } catch (e) {
        toast(errText(e), true);
    } finally {
        flashing = false;
        renderFlashButton();
    }
});

Events.On("firmware:progress", (ev) => {
    const p = ev.data as Progress;
    if (p.total > 0) $("#fw-bar").style.width = `${(p.done / p.total) * 100}%`;
    const pct = p.total > 0 ? ` — ${Math.floor((p.done / p.total) * 100)}%` : "";
    $("#fw-stage").textContent = p.stage === "upload" ? `${p.text}${pct}` : p.text;
    $("#fw-progress").classList.toggle("error", p.stage === "error");
});

function renderLangSwitch() {
    $$("#lang-switch button").forEach((b) => b.classList.toggle("active", b.dataset.lang === getLang()));
}

$$<HTMLButtonElement>("#lang-switch button").forEach((b) =>
    b.addEventListener("click", () => {
        const l = b.dataset.lang as Lang;
        if (l === getLang()) return;
        setLang(l);
        update("language", l);
    }),
);

// Приложение переключает язык, когда сохранит настройки, — тогда и
// перезапрашиваем то, что переводит оно само.
Events.On("language:changed", async () => {
    effects = (await KB.Effects()) ?? [];
    buildEffects();
    renderLighting(lighting);
    renderDevice(await KB.Status());
    renderBattery(await KB.Battery());
    if (page === "firmware") loadFirmware();
});

onLang(() => {
    renderLangSwitch();
    $$<HTMLElement>(".preset").forEach((b) => (b.title = t(b.dataset.name!)));
    renderDevice(device);
    renderLighting(lighting);
    refreshTesterLabels();
    if (tsBoard && !tsLog.length) resetTester();
    renderPomodoro();
    renderUpdate();
});

let upd: UpdateInfo | null = null;
let updating = false;

function renderUpdate() {
    const card = $("#update-card");
    card.hidden = !upd?.available;
    if (!upd?.available) return;
    $("#update-version").textContent = upd.release.version;
    $<HTMLButtonElement>("#update-install").disabled = updating;
}

Events.On("update:available", (ev) => {
    upd = ev.data as UpdateInfo;
    renderUpdate();
});

Events.On("update:progress", (ev) => {
    const p = ev.data as UpdateProgress;
    const pct = p.total > 0 ? Math.floor((p.done / p.total) * 100) : 0;
    $("#update-progress").hidden = false;
    $("#update-bar").style.width = `${pct}%`;
    $("#update-install").textContent = t("Загрузка обновления… {pct}%", {pct});
});

$("#update-check").addEventListener("click", async () => {
    const btn = $("#update-check");
    btn.classList.add("spin");
    try {
        upd = await KB.CheckUpdate();
        renderUpdate();
        if (!upd.available) toast(t("Установлена последняя версия"));
    } catch (e) {
        toast(errText(e), true);
    } finally {
        btn.classList.remove("spin");
    }
});

$("#update-notes").addEventListener("click", () => upd && Browser.OpenURL(upd.release.url));

$("#update-install").addEventListener("click", async () => {
    updating = true;
    renderUpdate();
    try {
        await KB.InstallUpdate();
        toast(t("Обновление установлено — приложение перезапускается"));
    } catch (e) {
        toast(errText(e), true);
        $("#update-progress").hidden = true;
        $("#update-install").textContent = t("Обновить");
    } finally {
        updating = false;
        renderUpdate();
    }
});

async function init() {
    const lang = (await KB.Language()) as Lang;
    if (lang !== getLang()) setLang(lang);
    renderLangSwitch();
    $("#app-version").textContent = await KB.Version();
    setColor(color);
    layout = await KB.Layout();
    effects = (await KB.Effects()) ?? [];
    buildEffects();
    initPaint();
    animBoard = renderBoard($("#anim-board"), layout);
    paintBoardKeys();
    tsBoard = renderBoard($("#ts-board"), layout);
    refreshTesterLabels();
    await initSettings({toast, errText});
    renderFlashCmd();
    autostartBox.checked = await KB.Autostart();
    renderDevice(await KB.Status());
    renderLighting(await KB.LightingState());
    setPomodoro(await KB.PomodoroState());
    renderBattery(await KB.Battery());
}

init().catch((e) => toast(errText(e), true));
