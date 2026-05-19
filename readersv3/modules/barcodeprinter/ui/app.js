const loginView = document.getElementById("login");
const appView = document.getElementById("app");
const loginMsg = document.getElementById("login-msg");
const settingsMsg = document.getElementById("settings-msg");
const form = document.getElementById("settings-form");
const printerSelect = document.getElementById("printer-select");
const statsBody = document.querySelector("#stats-table tbody");
const jobsBody = document.querySelector("#jobs-table tbody");
const dateFrom = document.getElementById("date-from");
const dateTo = document.getElementById("date-to");
let readerSettings = {};
const views = {
  dashboard: document.getElementById("view-dashboard"),
  settings: document.getElementById("view-settings"),
  history: document.getElementById("view-history"),
};

document.getElementById("login-btn").addEventListener("click", onLogin);
document.getElementById("logout-btn").addEventListener("click", onLogout);
document.getElementById("save-settings").addEventListener("click", saveSettings);
document.getElementById("reload-settings").addEventListener("click", loadSettings);
document.getElementById("refresh-stats").addEventListener("click", refreshStatsAndJobs);
document.getElementById("test-print").addEventListener("click", onTestPrint);
document.querySelectorAll(".menu-btn[data-view]").forEach((btn) => btn.addEventListener("click", () => activateView(btn.dataset.view)));

initDates();
bootstrap();

async function bootstrap() {
  const session = await api("/api/session", { allowFail: true });
  if (session?.authenticated) {
    showApp();
    await loadAll();
    return;
  }
  showLogin();
}

function initDates() {
  const now = new Date();
  dateTo.value = now.toISOString().slice(0, 10);
  now.setDate(now.getDate() - 7);
  dateFrom.value = now.toISOString().slice(0, 10);
}

function showLogin() {
  loginView.hidden = false;
  appView.hidden = true;
}

function showApp() {
  loginView.hidden = true;
  appView.hidden = false;
}

function activateView(name) {
  Object.entries(views).forEach(([key, node]) => { node.hidden = key !== name; });
  document.querySelectorAll(".menu-btn[data-view]").forEach((btn) => btn.classList.toggle("active", btn.dataset.view === name));
}

async function onLogin() {
  loginMsg.textContent = "";
  const username = document.getElementById("username").value || "";
  const password = document.getElementById("password").value || "";
  const resp = await api("/api/session/login", {
    method: "POST",
    body: JSON.stringify({ username, password }),
    allowFail: true,
  });
  if (!resp || resp.ok === false) {
    loginMsg.textContent = resp?.error || "Login failed";
    return;
  }
  showApp();
  await loadAll();
}

async function onLogout() {
  await api("/api/session/logout", { method: "POST", allowFail: true });
  showLogin();
}

async function loadAll() {
  activateView("dashboard");
  await loadPrinters();
  await loadSettings();
  await refreshStatsAndJobs();
}

async function loadPrinters() {
  const data = await api("/api/barcode/printers");
  const printers = data.printers || [];
  printerSelect.innerHTML = `<option value="">(default)</option>`;
  printers.forEach((name) => {
    const opt = document.createElement("option");
    opt.value = name;
    opt.textContent = name;
    printerSelect.appendChild(opt);
  });
}

async function loadSettings() {
  const [barcodeData, readerData] = await Promise.all([
    api("/api/barcode/settings"),
    api("/api/reader-settings"),
  ]);
  readerSettings = readerData.settings || {};
  const settings = {
    local_http_address: readerSettings.local_http_address || "",
    local_http_language: readerSettings.local_http_language || "ro",
    local_http_tls: readerSettings.local_http_tls || "false",
    local_http_cors_allowed_origins: readerSettings.local_http_cors_allowed_origins || "https://ldse.wisemed.eu",
    ...(barcodeData.settings || {}),
  };
  for (const [key, value] of Object.entries(settings)) {
    const field = form.elements.namedItem(key);
    if (field) field.value = value || "";
  }
}

async function saveSettings() {
  settingsMsg.textContent = "";
  settingsMsg.style.color = "#8f1d1d";
  const all = {};
  Array.from(form.elements).forEach((el) => { if (el.name) all[el.name] = el.value; });
  const readerPayload = {
    repeat_mode: readerSettings.repeat_mode || "individual",
    reader_id: readerSettings.reader_id || "",
    reader_label: readerSettings.reader_label || "",
    analyzer_name: readerSettings.analyzer_name || "",
    analyzer_code: readerSettings.analyzer_code || "",
    db_name: readerSettings.db_name || "",
    sqlite_path: readerSettings.sqlite_path || "",
    local_http_address: all.local_http_address || "",
    local_http_language: all.local_http_language || "ro",
    local_http_tls: all.local_http_tls || "false",
    local_http_cors_allowed_origins: all.local_http_cors_allowed_origins || "https://ldse.wisemed.eu",
    analyzer_comm_type: readerSettings.analyzer_comm_type || "",
    analyzer_protocol: readerSettings.analyzer_protocol || "",
    app_updates_enabled: readerSettings.app_updates_enabled || "true",
    app_updates_app_id: readerSettings.app_updates_app_id || "",
    app_updates_current_version: readerSettings.app_updates_current_version || "",
    app_updates_channel: readerSettings.app_updates_channel || "stable",
    app_updates_base_url: readerSettings.app_updates_base_url || "",
    app_updates_auto_download: readerSettings.app_updates_auto_download || "true",
    app_updates_download_dir: readerSettings.app_updates_download_dir || "./updates",
    result_sync_enabled: readerSettings.result_sync_enabled || "false",
    result_sync_interval_minutes: readerSettings.result_sync_interval_minutes || "5",
    result_sync_sample_prefixes: readerSettings.result_sync_sample_prefixes || "",
    result_sync_sample_suffixes: readerSettings.result_sync_sample_suffixes || "",
    result_sync_separators: readerSettings.result_sync_separators || "-",
    result_sync_qc_prefixes: readerSettings.result_sync_qc_prefixes || "",
  };
  const barcodePayload = { ...all };
  [
    "local_http_address",
    "local_http_language",
    "local_http_tls",
    "local_http_cors_allowed_origins",
  ].forEach((key) => delete barcodePayload[key]);

  const [readerResp, barcodeResp] = await Promise.all([
    api("/api/reader-settings", {
      method: "PUT",
      body: JSON.stringify(readerPayload),
      allowFail: true,
    }),
    api("/api/barcode/settings", {
      method: "PUT",
      body: JSON.stringify(barcodePayload),
      allowFail: true,
    }),
  ]);
  if (!readerResp || readerResp.ok === false) {
    settingsMsg.textContent = readerResp?.error || "Save failed";
    return;
  }
  readerSettings = readerResp.settings || readerPayload;
  if (!barcodeResp || barcodeResp.success === false) {
    settingsMsg.textContent = barcodeResp?.error || "Save failed";
    return;
  }
  settingsMsg.textContent = "Setarile au fost salvate in config.yaml";
  settingsMsg.style.color = "#1c7b32";
}

async function onTestPrint() {
  settingsMsg.textContent = "";
  const data = await api("/api/barcode/test-print", { method: "POST", allowFail: true });
  if (!data || data.success === false) {
    settingsMsg.textContent = data?.error || "Test print failed";
    settingsMsg.style.color = "#8f1d1d";
    return;
  }
  settingsMsg.textContent = "Test print trimis";
  settingsMsg.style.color = "#1c7b32";
  await refreshStatsAndJobs();
}

async function refreshStatsAndJobs() {
  const qs = `?date_from=${encodeURIComponent(dateFrom.value)}&date_to=${encodeURIComponent(dateTo.value)}`;
  const statsData = await api("/api/barcode/stats/daily" + qs);
  const jobsData = await api("/api/barcode/jobs" + qs + "&limit=300");
  renderStats(statsData.daily || []);
  renderJobs(jobsData.jobs || []);
}

function renderStats(rows) {
  statsBody.innerHTML = "";
  if (!rows.length) {
    statsBody.innerHTML = `<tr><td colspan="5">Nu exista date</td></tr>`;
    return;
  }
  rows.forEach((row) => {
    const tr = document.createElement("tr");
    tr.innerHTML = `<td>${esc(row.day)}</td><td>${num(row.prints)}</td><td>${num(row.labels)}</td><td>${num(row.ok)}</td><td>${num(row.fail)}</td>`;
    statsBody.appendChild(tr);
  });
}

function renderJobs(rows) {
  jobsBody.innerHTML = "";
  if (!rows.length) {
    jobsBody.innerHTML = `<tr><td colspan="8">Nu exista tipariri</td></tr>`;
    return;
  }
  rows.forEach((row) => {
    const tr = document.createElement("tr");
    tr.innerHTML = `<td>${esc(row.created_at)}</td><td>${esc(row.client_ip)}</td><td>${esc(row.file_id)}</td><td>${esc(row.name)}</td><td>${esc(row.bc_type)}</td><td>${num(row.labels_count)}</td><td>${esc(row.status)}</td><td>${esc(row.error)}</td>`;
    jobsBody.appendChild(tr);
  });
}

async function api(url, opts = {}) {
  const res = await fetch(url, {
    method: opts.method || "GET",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(opts.headers || {}) },
    body: opts.body,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok && !opts.allowFail) {
    throw new Error(data.error || `HTTP ${res.status}`);
  }
  return data;
}

function esc(v) {
  return String(v || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function num(v) {
  const n = Number(v || 0);
  return Number.isFinite(n) ? String(n) : "0";
}
