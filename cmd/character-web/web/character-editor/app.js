"use strict";

const $ = (id) => document.getElementById(id);

async function api(path, body) {
  const r = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  let j = {};
  try {
    j = await r.json();
  } catch (_) {}
  if (!r.ok) {
    const msg = j.error || r.statusText || String(r.status);
    throw new Error(msg);
  }
  return j;
}

function uid() {
  const n = $("userId").value.trim();
  const v = parseInt(n, 10);
  if (!Number.isFinite(v) || v <= 0) throw new Error("user_id должен быть положительным числом");
  return v;
}

function defaultPlayData() {
  return {
    schema_version: 2,
    x: 0,
    y: 0,
    hp: 10,
    face_dx: 1,
    face_dy: 0,
    stats: { str: 10, dex: 10, con: 10, int: 10, wis: 10, cha: 10 },
    sprite: "Male 01-1",
  };
}

/** Из поля ввода: имя набора или путь anim/Name/Name.png → id Name */
function normalizeSpriteInput(raw) {
  if (!raw || typeof raw !== "string") return "";
  let s = raw.trim().replace(/\\/g, "/");
  s = s.replace(/^\.?\/?anim\//i, "");
  if (s.toLowerCase().endsWith(".png")) {
    s = s.slice(0, -4).trim();
  }
  const parts = s.split("/").filter(Boolean);
  if (parts.length >= 2) {
    const a = parts[parts.length - 2];
    const b = parts[parts.length - 1];
    if (a === b) return a;
    return b;
  }
  if (parts.length === 1) return parts[0];
  return s;
}

function getSpriteId() {
  const v = normalizeSpriteInput($("sprite").value);
  if (v) return v;
  return "Male 01-1";
}

function readPlayDataFromForm() {
  const g = (id) => parseInt($(id).value, 10);
  return {
    schema_version: 2,
    x: g("x"),
    y: g("y"),
    hp: g("hp"),
    face_dx: g("face_dx"),
    face_dy: g("face_dy"),
    stats: {
      str: g("str"),
      dex: g("dex"),
      con: g("con"),
      int: g("int"),
      wis: g("wis"),
      cha: g("cha"),
    },
    sprite: getSpriteId(),
  };
}

function setSpriteField(id) {
  let v = normalizeSpriteInput(id || "Male 01-1");
  if (!v) v = "Male 01-1";
  $("sprite").value = v;
  reloadPreview();
}

function fillFormFromPlayData(p) {
  $("x").value = p.x ?? 0;
  $("y").value = p.y ?? 0;
  $("hp").value = p.hp ?? 10;
  $("face_dx").value = p.face_dx ?? 1;
  $("face_dy").value = p.face_dy ?? 0;
  const s = p.stats || {};
  $("str").value = s.str ?? 10;
  $("dex").value = s.dex ?? 10;
  $("con").value = s.con ?? 10;
  $("int").value = s.int ?? 10;
  $("wis").value = s.wis ?? 10;
  $("cha").value = s.cha ?? 10;
  setSpriteField(p.sprite || "Male 01-1");
}

function fillFormFromCharacter(c) {
  $("characterId").value = c.id || "";
  $("displayName").value = c.display_name || "";
  $("description").value = c.description || "";
  $("version").value = c.version != null ? String(c.version) : "0";
  fillFormFromPlayData(c.play_data || defaultPlayData());
}

function showErr(el, msg) {
  if (!msg) {
    el.hidden = true;
    el.textContent = "";
    return;
  }
  el.hidden = false;
  el.textContent = msg;
}

async function refreshList() {
  const err = $("listErr");
  showErr(err, "");
  try {
    const j = await api("/api/list", { user_id: uid(), limit: 100, offset: 0 });
    const tb = $("listBody");
    tb.replaceChildren();
    for (const c of j.characters || []) {
      const tr = document.createElement("tr");
      tr.dataset.selectable = "1";
      tr.innerHTML = `<td class="mono">${escapeHtml(c.id)}</td><td>${escapeHtml(c.display_name || "")}</td><td>${c.version ?? ""}</td>`;
      tr.addEventListener("click", () => {
        tb.querySelectorAll("tr.selected").forEach((r) => r.classList.remove("selected"));
        tr.classList.add("selected");
        $("characterId").value = c.id;
        $("displayName").value = c.display_name || "";
      });
      tb.appendChild(tr);
    }
  } catch (e) {
    showErr(err, e.message);
  }
}

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

async function loadOne() {
  const err = $("formErr");
  showErr(err, "");
  const id = $("characterId").value.trim();
  if (!id) {
    showErr(err, "Укажите character_id");
    return;
  }
  try {
    const j = await api("/api/get", { user_id: uid(), character_id: id });
    fillFormFromCharacter(j.character);
  } catch (e) {
    showErr(err, e.message);
  }
}

async function createOne() {
  const err = $("formErr");
  showErr(err, "");
  const name = $("displayName").value.trim();
  if (!name) {
    showErr(err, "display_name обязателен при создании");
    return;
  }
  try {
    const body = {
      user_id: uid(),
      display_name: name,
      description: $("description").value,
      character_id: $("characterId").value.trim(),
      play_data: readPlayDataFromForm(),
    };
    const j = await api("/api/create", body);
    fillFormFromCharacter(j.character);
    await refreshList();
  } catch (e) {
    showErr(err, e.message);
  }
}

async function saveData() {
  const err = $("formErr");
  showErr(err, "");
  const id = $("characterId").value.trim();
  if (!id) {
    showErr(err, "character_id нужен для сохранения");
    return;
  }
  try {
    const ev = parseInt($("version").value, 10);
    const j = await api("/api/save-data", {
      user_id: uid(),
      character_id: id,
      play_data: readPlayDataFromForm(),
      expected_version: Number.isFinite(ev) ? ev : 0,
    });
    fillFormFromCharacter(j.character);
    await refreshList();
  } catch (e) {
    showErr(err, e.message);
  }
}

async function deleteOne() {
  const err = $("formErr");
  showErr(err, "");
  const id = $("characterId").value.trim();
  if (!id) {
    showErr(err, "character_id");
    return;
  }
  if (!confirm("Удалить персонажа " + id + "?")) return;
  try {
    await api("/api/delete", { user_id: uid(), character_id: id });
    $("btnNew").click();
    await refreshList();
  } catch (e) {
    showErr(err, e.message);
  }
}

function newTemplate() {
  showErr($("formErr"), "");
  $("characterId").value = "";
  $("displayName").value = "";
  $("description").value = "";
  $("version").value = "0";
  fillFormFromPlayData(defaultPlayData());
  $("listBody").querySelectorAll("tr.selected").forEach((r) => r.classList.remove("selected"));
}

// —— превью спрайта (ходьба на юг), картинка с /assets/anim/... ——
let previewImg = null;
let previewLoadingFor = "";
let previewPhase = 0;

function previewURL(spriteId) {
  const s = encodeURIComponent(spriteId.trim());
  return `/assets/anim/${s}/${s}.png`;
}

function reloadPreview() {
  const id = getSpriteId();
  previewLoadingFor = id;
  const img = new Image();
  img.onload = () => {
    if (previewLoadingFor === id) previewImg = img;
  };
  img.onerror = () => {
    if (previewLoadingFor === id) previewImg = null;
  };
  img.src = previewURL(id);
}

function previewTick() {
  requestAnimationFrame(previewTick);
  const canvas = $("spritePreview");
  if (!canvas) return;
  const ctx = canvas.getContext("2d");
  ctx.imageSmoothingEnabled = false;
  ctx.fillStyle = "#0a0a10";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  if (!previewImg || !previewImg.complete || previewImg.naturalWidth === 0) {
    ctx.fillStyle = "#6a6a78";
    ctx.font = "13px system-ui,sans-serif";
    ctx.fillText("Нет файла по пути anim/…", 14, 58);
    ctx.font = "11px system-ui,sans-serif";
    ctx.fillText("(запусти character-web с -data)", 14, 76);
    return;
  }
  previewPhase += 0.15;
  const W = 32;
  const H = 32;
  const row = 0;
  const i = Math.floor(previewPhase) % 4;
  const col = [0, 1, 2, 1][i];
  const scale = 3;
  const dw = W * scale;
  const dh = H * scale;
  const dx = (canvas.width - dw) / 2;
  const dy = (canvas.height - dh) / 2;
  ctx.drawImage(previewImg, col * W, row * H, W, H, dx, dy, dw, dh);
}

async function fetchAnimSprites() {
  const r = await fetch("/api/anims");
  if (!r.ok) return [];
  const j = await r.json();
  return Array.isArray(j.sprites) ? j.sprites : [];
}

function populateAnimDatalist(names) {
  const dl = $("animDatalist");
  if (!dl) return;
  dl.replaceChildren();
  for (const n of names) {
    const o = document.createElement("option");
    o.value = n;
    dl.appendChild(o);
  }
}

async function refreshAnimList() {
  try {
    const names = await fetchAnimSprites();
    populateAnimDatalist(names);
  } catch (_) {
    populateAnimDatalist([]);
  }
}

$("sprite").addEventListener("input", () => {
  reloadPreview();
});

$("btnAnimRefresh").addEventListener("click", () => {
  refreshAnimList();
});

$("btnList").addEventListener("click", () => refreshList());
$("btnLoad").addEventListener("click", () => loadOne());
$("btnCreate").addEventListener("click", () => createOne());
$("btnSave").addEventListener("click", () => saveData());
$("btnDelete").addEventListener("click", () => deleteOne());
$("btnNew").addEventListener("click", () => newTemplate());

(async () => {
  await refreshAnimList();
  newTemplate();
  await refreshList();
  reloadPreview();
  requestAnimationFrame(previewTick);
})();
