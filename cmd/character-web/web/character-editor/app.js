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
  };
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
  };
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

$("btnList").addEventListener("click", () => refreshList());
$("btnLoad").addEventListener("click", () => loadOne());
$("btnCreate").addEventListener("click", () => createOne());
$("btnSave").addEventListener("click", () => saveData());
$("btnDelete").addEventListener("click", () => deleteOne());
$("btnNew").addEventListener("click", () => newTemplate());

newTemplate();
refreshList();
