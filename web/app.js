const $ = (id) => document.getElementById(id);

const osEl = $("os");
const identifierEl = $("identifier");
const verRangeEl = $("verRange");
const buildEl = $("build");
const boardEl = $("board");
const form = $("form");
const statusEl = $("status");
const logEl = $("log");
const offsetsEl = $("offsets");
const metaEl = $("meta");
const refreshEl = $("refresh");
const skipExistingEl = $("skipExisting");
const runBtn = $("run");
const exportPlistBtn = $("exportPlist");
const historyEl = $("history");
const historyWrap = $("historyWrap");
const detailWrap = $("detailWrap");

let devices = [];
let allBuilds = [];
let library = [];
let lastResult = null;
let extracting = false;

function setStatus(kind, text) {
  statusEl.hidden = !text;
  statusEl.className = "status" + (kind ? " " + kind : "");
  statusEl.textContent = text || "";
}

function logLine(msg) {
  logEl.hidden = false;
  logEl.textContent += msg + "\n";
  logEl.scrollTop = logEl.scrollHeight;
}

function setBusy(busy) {
  extracting = busy;
  runBtn.disabled = busy;
}

async function getJSON(url) {
  const res = await fetch(url);
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

function versionMajor(v) {
  const m = String(v || "").trim().match(/^(\d+)/);
  return m ? parseInt(m[1], 10) : 0;
}

function parseVersionRange(raw) {
  const s = String(raw || "").trim().replace(/\s+/g, "").replace(/[–—]/g, "-");
  if (!s) return null;
  const m = s.match(/^(\d+)(?:-(\d+))?$/);
  if (!m) return { error: "Use 15 or 15-17" };
  let from = parseInt(m[1], 10);
  let to = m[2] ? parseInt(m[2], 10) : from;
  if (from > to) [from, to] = [to, from];
  return { from, to };
}

function syncExtractMode() {
  const range = parseVersionRange(verRangeEl.value);
  const usingRange = !!(range && !range.error);
  buildEl.disabled = usingRange;
  buildEl.required = !usingRange;
}

function buildsInRange(from, to) {
  return allBuilds.filter((b) => {
    const major = versionMajor(b.version);
    return major >= from && major <= to;
  });
}

function fillBuilds() {
  const prev = buildEl.value;
  buildEl.innerHTML = "";
  for (const b of allBuilds) {
    const opt = document.createElement("option");
    opt.value = b.build;
    opt.dataset.version = b.version || "";
    opt.dataset.os = b.os || osEl.value;
    const tags = [b.version, b.build];
    if (b.beta) tags.push("beta");
    if (b.rc) tags.push("rc");
    opt.textContent = tags.filter(Boolean).join(" · ");
    buildEl.appendChild(opt);
  }
  if ([...buildEl.options].some((o) => o.value === prev)) buildEl.value = prev;
}

function fillDevices() {
  const prev = identifierEl.value;
  identifierEl.innerHTML = "";
  for (const d of devices) {
    const opt = document.createElement("option");
    opt.value = d.identifier;
    opt.textContent = `${d.name} (${d.identifier})`;
    identifierEl.appendChild(opt);
  }
  if ([...identifierEl.options].some((o) => o.value === prev)) identifierEl.value = prev;
}

function fillBoards(dev) {
  const prev = boardEl.value;
  boardEl.innerHTML = "";
  const boards = (dev && dev.boards) || [];
  for (const b of boards) {
    const opt = document.createElement("option");
    opt.value = b;
    opt.textContent = b;
    boardEl.appendChild(opt);
  }
  if ([...boardEl.options].some((o) => o.value === prev)) boardEl.value = prev;
}

async function loadDevices() {
  devices = await getJSON("/api/devices?os=" + encodeURIComponent(osEl.value));
  fillDevices();
  await onDeviceChange();
}

async function onDeviceChange() {
  const id = identifierEl.value;
  const dev = devices.find((d) => d.identifier === id);
  fillBoards(dev);
  allBuilds = [];
  buildEl.innerHTML = "";
  if (!id) return;
  allBuilds = await getJSON(
    "/api/builds?os=" + encodeURIComponent(osEl.value) + "&identifier=" + encodeURIComponent(id)
  );
  fillBuilds();
  syncExtractMode();
}

function resultFromManifest(m) {
  return {
    cached: true,
    manifest: m,
    kernelVersion: m.kernelVersion,
    kernelBase: m.kernelBase,
    kernelEntry: m.kernelEntry,
    offsets: m.offsets || {},
  };
}

function renderMeta(result) {
  metaEl.innerHTML = "";
  const m = result.manifest || {};
  const rows = [
    ["Device", m.identifier],
    ["Build", m.build],
    ["Board", m.board],
    ["Version", m.version],
    ["Kernel", result.kernelVersion],
    ["Base", result.kernelBase],
    ["Entry", result.kernelEntry],
    ["Files", Object.keys(m.files || {}).join(", ")],
    ["XPF", result.elapsedSec ? result.elapsedSec + "s" : ""],
  ];
  for (const [k, v] of rows) {
    if (!v) continue;
    const dt = document.createElement("dt");
    dt.textContent = k;
    const dd = document.createElement("dd");
    dd.textContent = v;
    metaEl.appendChild(dt);
    metaEl.appendChild(dd);
  }
}

function xmlEscape(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function plistDict(entries, indent) {
  const pad = "\t".repeat(indent);
  const lines = [`${pad}<dict>`];
  for (const [key, value] of entries) {
    lines.push(`${pad}\t<key>${xmlEscape(key)}</key>`);
    if (typeof value === "string") {
      lines.push(`${pad}\t<string>${xmlEscape(value)}</string>`);
    } else {
      lines.push(...plistDict(value, indent + 1));
    }
  }
  lines.push(`${pad}</dict>`);
  return lines;
}

function libraryToPlist(items) {
  const tree = {};
  for (const m of items) {
    if (!hasOffsets(m) || !m.identifier || !m.build) continue;
    if (!tree[m.identifier]) tree[m.identifier] = {};
    if (tree[m.identifier][m.build]) continue;
    const offsets = Object.keys(m.offsets)
      .sort()
      .map((k) => [k, String(m.offsets[k])]);
    tree[m.identifier][m.build] = offsets;
  }
  const devices = Object.keys(tree).sort().map((id) => {
    const builds = Object.keys(tree[id]).sort().map((build) => [build, tree[id][build]]);
    return [id, builds];
  });
  return [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">',
    '<plist version="1.0">',
    ...plistDict(devices, 0),
    "</plist>",
    "",
  ].join("\n");
}

function exportPlist() {
  const items = library.filter(hasOffsets);
  if (!items.length) return;
  const blob = new Blob([libraryToPlist(items)], { type: "application/x-plist" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "offsets.plist";
  a.click();
  URL.revokeObjectURL(url);
}

function renderOffsets() {
  const entries = Object.entries(lastResult?.offsets || {}).sort((a, b) => a[0].localeCompare(b[0]));
  offsetsEl.innerHTML = "";
  for (const [name, value] of entries) {
    const tr = document.createElement("tr");
    tr.innerHTML = `<td>${name}</td><td>${value}</td>`;
    offsetsEl.appendChild(tr);
  }
}

function clearDetail() {
  lastResult = null;
  offsetsEl.innerHTML = "";
  metaEl.innerHTML = "";
  detailWrap.hidden = true;
}

function openDetail(result) {
  lastResult = result;
  detailWrap.hidden = false;
  renderMeta(result);
  renderOffsets();
  renderHistory();
}

function hasOffsets(m) {
  return m && m.offsets && Object.keys(m.offsets).length > 0;
}

function sameEntry(a, b) {
  return a && b && a.identifier === b.identifier && a.build === b.build && a.board === b.board;
}

function upsertLibrary(m) {
  if (!m) return;
  const i = library.findIndex((x) => sameEntry(x, m));
  if (i >= 0) library[i] = m;
  else library.unshift(m);
}

let historyOpen = null;
let historyRendering = false;

function deviceLabel(identifier) {
  const d = devices.find((x) => x.identifier === identifier);
  return d ? `${d.name} (${identifier})` : identifier;
}

function renderHistory() {
  const items = library.filter(hasOffsets);
  historyWrap.hidden = items.length === 0;
  historyEl.innerHTML = "";
  const current = lastResult?.manifest;
  if (historyOpen && current?.identifier) historyOpen.add(current.identifier);
  const order = [];
  const groups = new Map();
  for (const m of items) {
    if (!groups.has(m.identifier)) {
      groups.set(m.identifier, []);
      order.push(m.identifier);
    }
    groups.get(m.identifier).push(m);
  }

  historyRendering = true;
  for (const id of order) {
    const rows = groups.get(id);
    const details = document.createElement("details");
    details.className = "history-group";
    const isCurrent = current && current.identifier === id;
    details.open = historyOpen ? historyOpen.has(id) : isCurrent;
    details.addEventListener("toggle", () => {
      if (historyRendering) return;
      if (!historyOpen) {
        historyOpen = new Set();
        if (current?.identifier) historyOpen.add(current.identifier);
      }
      if (details.open) historyOpen.add(id);
      else historyOpen.delete(id);
    });
    const summary = document.createElement("summary");
    summary.innerHTML = `<span class="name">${deviceLabel(id)}</span><span class="sub">${rows.length}</span>`;
    details.appendChild(summary);
    const ul = document.createElement("ul");
    for (const m of rows) {
      const li = document.createElement("li");
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "history-item" + (sameEntry(current, m) ? " active" : "");
      const title = [m.version, m.build].filter(Boolean).join(" · ");
      btn.innerHTML = `<span>${title}</span><span class="sub">${m.board}</span>`;
      btn.addEventListener("click", () => showStored(m));
      li.appendChild(btn);
      ul.appendChild(li);
    }
    details.appendChild(ul);
    historyEl.appendChild(details);
  }
  historyRendering = false;
}

function showStored(m) {
  if (!hasOffsets(m)) {
    clearDetail();
    renderHistory();
    return;
  }
  openDetail(resultFromManifest(m));
}

function currentItem() {
  const opt = buildEl.selectedOptions[0];
  if (!opt) return null;
  return {
    os: opt.dataset.os || osEl.value,
    identifier: identifierEl.value,
    build: opt.value,
    board: boardEl.value,
    version: opt.dataset.version || "",
    refresh: refreshEl.checked,
  };
}

function rangeItems(from, to) {
  return buildsInRange(from, to).map((b) => ({
    os: b.os || osEl.value,
    identifier: identifierEl.value,
    build: b.build,
    board: boardEl.value,
    version: b.version || "",
    refresh: refreshEl.checked,
  }));
}

async function readSSE(res, onEvent) {
  const reader = res.body.getReader();
  const dec = new TextDecoder();
  let buf = "";
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += dec.decode(value, { stream: true });
    let idx;
    while ((idx = buf.indexOf("\n\n")) >= 0) {
      const chunk = buf.slice(0, idx);
      buf = buf.slice(idx + 2);
      let event = "message";
      const dataLines = [];
      for (const line of chunk.split("\n")) {
        if (line.startsWith("event:")) event = line.slice(6).trim();
        else if (line.startsWith("data:")) dataLines.push(line.slice(5).trim());
      }
      if (dataLines.length) onEvent(event, JSON.parse(dataLines.join("\n")));
    }
  }
}

async function runExtract(items) {
  if (!items.length) throw new Error("nothing to extract");
  logEl.textContent = "";
  logEl.hidden = true;
  setBusy(true);
  setStatus("run", "Running…");

  try {
    const res = await fetch("/api/extract/batch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ items, skipExisting: skipExistingEl.checked }),
    });
    if (!res.ok || !res.body) {
      const data = await res.json().catch(() => ({}));
      throw new Error(data.error || res.statusText);
    }
    let sawResult = false;
    await readSSE(res, (event, data) => {
      if (event === "progress") {
        const prefix = data.total ? `[${data.index + 1}/${data.total}] ` : "";
        logLine(prefix + data.stage + "  " + data.message);
        setStatus("run", prefix + data.message);
      } else if (event === "error") {
        throw new Error(data.error);
      } else if (event === "item") {
        if (data.status === "start") {
          logLine(`[${data.index + 1}/${data.total}] start  ${data.identifier} ${data.build}`);
        } else if (data.status === "error") {
          logLine(`[${data.index + 1}/${data.total}] error  ${data.error}`);
        } else if (data.result) {
          sawResult = true;
          upsertLibrary(data.result.manifest);
          renderHistory();
        }
      } else if (event === "done") {
        sawResult = true;
        setStatus(data.failed ? "err" : "ok", data.failed ? `${data.failed} failed` : "Done");
      }
    });
    if (!sawResult) throw new Error("no result from server");
  } finally {
    setBusy(false);
  }
}

form.addEventListener("submit", async (e) => {
  e.preventDefault();
  const range = parseVersionRange(verRangeEl.value);
  let items;
  if (range) {
    if (range.error) {
      setStatus("err", range.error);
      return;
    }
    items = rangeItems(range.from, range.to);
    if (!items.length) {
      setStatus("err", `No builds in ${range.from === range.to ? range.from : range.from + "-" + range.to}`);
      return;
    }
  } else {
    const item = currentItem();
    if (!item) {
      setStatus("err", "Select a build or enter a version range");
      return;
    }
    items = [item];
  }
  try {
    await runExtract(items);
  } catch (err) {
    logLine("error  " + err.message);
    setStatus("err", err.message);
  }
});

verRangeEl.addEventListener("input", syncExtractMode);

osEl.addEventListener("change", () => {
  loadDevices().catch((e) => setStatus("err", e.message));
});
identifierEl.addEventListener("change", () => {
  onDeviceChange().catch((e) => setStatus("err", e.message));
});
exportPlistBtn.addEventListener("click", exportPlist);

async function init() {
  try {
    library = await getJSON("/api/library");
  } catch {
    library = [];
  }
  if (!Array.isArray(library)) library = [];
  renderHistory();
  await loadDevices();
  renderHistory();
}

init().catch((e) => setStatus("err", e.message));
