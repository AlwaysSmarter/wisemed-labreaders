const loginView = document.getElementById("login");
const appView = document.getElementById("app");
const loginMsg = document.getElementById("login-msg");
const settingsMsg = document.getElementById("settings-msg");
const form = document.getElementById("settings-form");
const printerSelects = Array.from(document.querySelectorAll("[data-printer-select]"));
const endpointSelector = document.getElementById("settings-endpoint-selector");
const statsBody = document.querySelector("#stats-table tbody");
const jobsBody = document.querySelector("#jobs-table tbody");
const dateFrom = document.getElementById("date-from");
const dateTo = document.getElementById("date-to");

const barcodeFieldNames = [
  "bcp_type",
  "othercfg_sel_printer",
  "othercfg_printer_resolution",
  "othercfg_printer_barcode",
  "othercfg_label_width",
  "othercfg_label_height",
  "othercfg_print_bcodex",
  "othercfg_print_bcodey",
  "othercfg_print_bcodeopt_w",
  "othercfg_bc_widenarowr",
  "othercfg_print_bcodeopt_h",
  "othercfg_print_bcodeopt_o",
  "othercfg_print_bcodeopt_e",
  "othercfg_print_bcodeopt_f",
  "othercfg_print_bcodeopt_g",
  "othercfg_print_bcodetxtx",
  "othercfg_print_bcodetxty",
  "othercfg_print_bcodetxtf",
  "othercfg_print_bcodetxto",
  "othercfg_print_bcodetxth",
  "othercfg_print_bcodetxtw",
  "othercfg_print_namex",
  "othercfg_print_namey",
  "othercfg_print_namef",
  "othercfg_print_nameo",
  "othercfg_print_nameh",
  "othercfg_print_namew",
  "othercfg_print_tubecodex",
  "othercfg_print_tubecodey",
  "othercfg_print_tubecodef",
  "othercfg_print_tubecodeo",
  "othercfg_print_tubecodeh",
  "othercfg_print_tubecodew",
  "othercfg_print_tubecode_boxw",
  "othercfg_print_tubecode_boxh",
  "othercfg_print_tubecode_boxt",
  "othercfg_print_tubecode_boxc",
  "othercfg_print_tubecode_boxr",
];

const postaFieldNames = [
  "pr_sel_printer",
  "pr_resolution",
  "pr_orientation",
  "pr_label_width_mm",
  "pr_label_height_mm",
  "pr_start_x_mm",
  "pr_start_y_mm",
  "pr_outer_padding_mm",
  "pr_section_gap_mm",
  "pr_section_header_h_mm",
  "pr_section_title_font_h",
  "pr_section_title_font_w",
  "pr_body_font_h",
  "pr_body_font_w",
  "pr_body_line_gap",
  "pr_small_font_h",
  "pr_small_font_w",
  "pr_stamp_box_w_mm",
  "pr_stamp_box_h_mm",
  "pr_stamp_font_h",
  "pr_stamp_font_w",
  "pr_sender_name",
  "pr_sender_address1",
  "pr_sender_address2",
  "pr_sender_city",
  "pr_sender_postal_code",
  "shipping_prepaid_stamp",
];

const endpointProfiles = [
  { profile: "barcode-legacy", prefix: "ep_barcode__", fields: barcodeFieldNames },
  { profile: "barcode-api", prefix: "ep_barcode__", fields: barcodeFieldNames },
  { profile: "posta-romana-legacy", prefix: "ep_posta__", fields: postaFieldNames },
  { profile: "posta-romana-api", prefix: "ep_posta__", fields: postaFieldNames },
];

const defaultSettings = {
  bcp_type: "zebrazpl",
  othercfg_printer_resolution: "200",
  othercfg_printer_barcode: "B3",
  othercfg_label_width: "50",
  othercfg_label_height: "25",
  othercfg_print_bcodex: "5",
  othercfg_print_bcodey: "5",
  othercfg_print_bcodeopt_w: "2",
  othercfg_bc_widenarowr: "3.0",
  othercfg_print_bcodeopt_h: "50",
  othercfg_print_bcodeopt_o: "N",
  othercfg_print_bcodeopt_e: "N",
  othercfg_print_bcodeopt_f: "N",
  othercfg_print_bcodeopt_g: "N",
  othercfg_print_bcodetxtx: "20",
  othercfg_print_bcodetxty: "40",
  othercfg_print_bcodetxtf: "D",
  othercfg_print_bcodetxto: "N",
  othercfg_print_bcodetxth: "6",
  othercfg_print_bcodetxtw: "6",
  othercfg_print_namex: "5",
  othercfg_print_namey: "12",
  othercfg_print_namef: "B",
  othercfg_print_nameo: "N",
  othercfg_print_nameh: "6",
  othercfg_print_namew: "6",
  othercfg_print_tubecodex: "40",
  othercfg_print_tubecodey: "22",
  othercfg_print_tubecodef: "B",
  othercfg_print_tubecodeo: "N",
  othercfg_print_tubecodeh: "6",
  othercfg_print_tubecodew: "6",
  othercfg_print_tubecode_boxw: "3.81",
  othercfg_print_tubecode_boxh: "3.81",
  othercfg_print_tubecode_boxt: "1",
  othercfg_print_tubecode_boxc: "B",
  othercfg_print_tubecode_boxr: "4",
  shipping_prepaid_stamp: "FRANCARE ULTERIOARA",
  pr_resolution: "203",
  pr_orientation: "landscape",
  pr_label_width_mm: "100",
  pr_label_height_mm: "150",
  pr_start_x_mm: "3",
  pr_start_y_mm: "3",
  pr_outer_padding_mm: "4",
  pr_section_gap_mm: "3",
  pr_section_header_h_mm: "8",
  pr_section_title_font_h: "34",
  pr_section_title_font_w: "24",
  pr_body_font_h: "32",
  pr_body_font_w: "24",
  pr_body_line_gap: "8",
  pr_small_font_h: "26",
  pr_small_font_w: "20",
  pr_stamp_box_w_mm: "42",
  pr_stamp_box_h_mm: "28",
  pr_stamp_font_h: "24",
  pr_stamp_font_w: "18",
  pr_sender_name: "INSTITUTUL NATIONAL DE SANATATE PUBLICA",
  pr_sender_address1: "Str. Dr. Leonte Anastasievici, Nr. 1-3",
  pr_sender_address2: "Cod postal 077042",
  pr_sender_city: "Loc. Bucuresti Sector 5",
  pr_sender_postal_code: "077042",
};

const views = {
  dashboard: document.getElementById("view-dashboard"),
  settings: document.getElementById("view-settings"),
  history: document.getElementById("view-history"),
};

let readerSettings = {};

document.getElementById("login-btn").addEventListener("click", onLogin);
document.getElementById("logout-btn").addEventListener("click", onLogout);
document.getElementById("save-settings").addEventListener("click", saveSettings);
document.getElementById("reload-settings").addEventListener("click", loadSettings);
document.getElementById("refresh-stats").addEventListener("click", refreshStatsAndJobs);
document.getElementById("selected-test-print").addEventListener("click", () => onTestPrint(endpointSelector.value));
document.querySelectorAll(".menu-btn[data-view]").forEach((btn) => btn.addEventListener("click", () => activateView(btn.dataset.view)));
document.querySelectorAll("[data-test-profile]").forEach((btn) => {
  btn.addEventListener("click", () => onTestPrint(btn.dataset.testProfile));
});
endpointSelector.addEventListener("change", onEndpointSelectionChange);

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
  Object.entries(views).forEach(([key, node]) => {
    node.hidden = key !== name;
  });
  document.querySelectorAll(".menu-btn[data-view]").forEach((btn) => {
    btn.classList.toggle("active", btn.dataset.view === name);
  });
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
  printerSelects.forEach((select) => {
    select.innerHTML = `<option value="">(default)</option>`;
    printers.forEach((name) => {
      const opt = document.createElement("option");
      opt.value = name;
      opt.textContent = name;
      select.appendChild(opt);
    });
  });
}

async function loadSettings() {
  const [barcodeData, readerData] = await Promise.all([
    api("/api/barcode/settings"),
    api("/api/reader-settings"),
  ]);
  readerSettings = readerData.settings || {};
  const settings = materializeEndpointSettings({
    ...defaultSettings,
    local_http_address: readerSettings.local_http_address || "",
    local_http_language: readerSettings.local_http_language || "ro",
    local_http_tls: readerSettings.local_http_tls || "false",
    local_http_cors_allowed_origins: readerSettings.local_http_cors_allowed_origins || "https://ldse.wisemed.eu",
    ...(barcodeData.settings || {}),
  });
  for (const [key, value] of Object.entries(settings)) {
    const field = form.elements.namedItem(key);
    if (field) {
      field.value = value || "";
    }
  }
}

async function saveSettings() {
  settingsMsg.textContent = "";
  settingsMsg.style.color = "#8f1d1d";
  const all = {};
  Array.from(form.elements).forEach((el) => {
    if (el.name) {
      all[el.name] = el.value;
    }
  });
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

async function onTestPrint(profile) {
  settingsMsg.textContent = "";
  const data = await api("/api/barcode/test-print", {
    method: "POST",
    body: JSON.stringify({ profile: profile || "barcode-legacy" }),
    allowFail: true,
  });
  if (!data || data.success === false) {
    settingsMsg.textContent = data?.error || "Test print failed";
    settingsMsg.style.color = "#8f1d1d";
    return;
  }
  settingsMsg.textContent = "Test print trimis";
  settingsMsg.style.color = "#1c7b32";
  await refreshStatsAndJobs();
}

function onEndpointSelectionChange() {
  const profile = endpointSelector.value || "barcode-legacy";
  const card = document.querySelector(`[data-endpoint-profile="${profile}"]`);
  if (card) {
    card.scrollIntoView({ behavior: "smooth", block: "start" });
  }
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

function materializeEndpointSettings(settings) {
  const next = { ...settings };
  endpointProfiles.forEach(({ prefix, fields }) => {
    fields.forEach((field) => {
      const key = prefix + field;
      if (!next[key]) {
        next[key] = next[field] || defaultSettings[field] || "";
      }
    });
  });
  return next;
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
