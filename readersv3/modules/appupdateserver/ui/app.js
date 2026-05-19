const loginView = document.getElementById("login");
const appView = document.getElementById("app");
const loginMsg = document.getElementById("login-msg");
const appMsg = document.getElementById("app-msg");
const versionMsg = document.getElementById("version-msg");
const settingsMsg = document.getElementById("settings-msg");
const appForm = document.getElementById("app-form");
const versionForm = document.getElementById("version-form");
const settingsForm = document.getElementById("settings-form");
const appsBody = document.querySelector("#apps-table tbody");
const versionsBody = document.querySelector("#versions-table tbody");
const versionsTitle = document.getElementById("versions-title");
const versionAppSelect = document.getElementById("version-app-select");
const versionChannelFilter = document.getElementById("version-channel-filter");
const versionOSFilter = document.getElementById("version-os-filter");
const versionArchFilter = document.getElementById("version-arch-filter");
const sessionUser = document.getElementById("session-user");
const selectedAppLabel = document.getElementById("selected-app-label");
const allowedTypesLabel = document.getElementById("allowed-types-label");
const resetVersionBtn = document.getElementById("reset-version");
const makeReleaseBtn = document.getElementById("make-release");
const deleteVersionBtn = document.getElementById("delete-version");
const generateDownloadLinkBtn = document.getElementById("generate-download-link");
const downloadLinkPreview = document.getElementById("download-link-preview");
const installerLinkPreview = document.getElementById("installer-link-preview");
const releaseProgress = document.getElementById("release-progress");
const releaseProgressTitle = document.getElementById("release-progress-title");
const releaseProgressDetail = document.getElementById("release-progress-detail");
const saveVersionBtn = document.getElementById("save-version");
const refreshVersionsBtn = document.getElementById("refresh-versions");
const views = {
  dashboard: document.getElementById("view-dashboard"),
  settings: document.getElementById("view-settings"),
  history: document.getElementById("view-history"),
};

const state = {
  apps: [],
  versions: [],
  selectedAppId: 0,
  selectedVersionId: 0,
  settings: {},
  releaseBusy: false,
};

document.getElementById("login-btn").addEventListener("click", onLogin);
document.getElementById("logout-btn").addEventListener("click", onLogout);
document.getElementById("save-app").addEventListener("click", saveApp);
document.getElementById("reset-app").addEventListener("click", resetAppForm);
document.getElementById("refresh-apps").addEventListener("click", loadApps);
document.getElementById("save-version").addEventListener("click", saveVersion);
resetVersionBtn.addEventListener("click", resetVersionForm);
makeReleaseBtn.addEventListener("click", makeRelease);
deleteVersionBtn.addEventListener("click", deleteVersion);
generateDownloadLinkBtn.addEventListener("click", generateDownloadLink);
document.getElementById("refresh-versions").addEventListener("click", loadVersions);
document.getElementById("save-settings").addEventListener("click", saveSettings);
document.querySelectorAll(".nav-link[data-view]").forEach((btn) => btn.addEventListener("click", () => activateView(btn.dataset.view)));
versionAppSelect.addEventListener("change", onVersionAppChange);
versionChannelFilter.addEventListener("change", renderVersions);
versionOSFilter.addEventListener("change", renderVersions);
versionArchFilter.addEventListener("change", renderVersions);

bootstrap();

async function bootstrap() {
  const session = await api("/api/session", { allowFail: true });
  if (session?.service_config?.allowed_user_types) {
    allowedTypesLabel.textContent = session.service_config.allowed_user_types;
  }
  if (session?.authenticated) {
    sessionUser.textContent = session?.session?.username || "-";
    showApp();
    await loadAll();
    return;
  }
  showLogin();
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
  if (name === "help") {
    window.location.href = "/help/";
    return;
  }
  Object.entries(views).forEach(([key, node]) => { node.hidden = key !== name; });
  document.querySelectorAll(".nav-link[data-view]").forEach((btn) => btn.classList.toggle("active", btn.dataset.view === name));
  const titleMap = {
    dashboard: "Aplicatii",
    history: "Versiuni",
    settings: "Setari",
    help: "Ajutor",
  };
  const titleNode = document.getElementById("dashboard-title");
  if (titleNode) titleNode.textContent = titleMap[name] || "Aplicatii";
}

async function onLogin() {
  loginMsg.textContent = "";
  const resp = await api("/api/session/login", {
    method: "POST",
    body: JSON.stringify({
      username: document.getElementById("username").value || "",
      password: document.getElementById("password").value || "",
    }),
    allowFail: true,
  });
  if (!resp || resp.ok === false) {
    loginMsg.textContent = resp?.error || "Login failed";
    return;
  }
  sessionUser.textContent = resp?.session?.username || "-";
  showApp();
  await loadAll();
}

async function onLogout() {
  await api("/api/session/logout", { method: "POST", allowFail: true });
  showLogin();
}

async function loadAll() {
  activateView("dashboard");
  await loadSettings();
  await loadApps();
}

async function loadApps() {
  const resp = await api("/api/update-server/apps", { allowFail: true });
  if (!resp || resp.ok === false) {
    appMsg.textContent = resp?.error || "Load apps failed";
    return;
  }
  state.apps = resp.apps || [];
  syncVersionAppOptions();
  renderApps();
  if (state.selectedAppId) {
    await loadVersions();
  }
}

function renderApps() {
  appsBody.innerHTML = "";
  if (!state.apps.length) {
    appsBody.innerHTML = `<tr><td colspan="3">Nu exista aplicatii</td></tr>`;
    return;
  }
  state.apps.forEach((row) => {
    const tr = document.createElement("tr");
    tr.classList.toggle("active", row.id === state.selectedAppId);
    tr.innerHTML = `<td>${esc(row.app_id)}</td><td>${esc(row.display_name)}</td><td>${row.active ? "Da" : "Nu"}</td>`;
    tr.addEventListener("click", () => {
      state.selectedAppId = row.id;
      fillAppForm(row);
      selectedAppLabel.textContent = row.display_name || row.app_id || "-";
      renderApps();
      loadVersions();
    });
    appsBody.appendChild(tr);
  });
}

function fillAppForm(row) {
  appForm.elements.id.value = row.id || "";
  appForm.elements.app_id.value = row.app_id || "";
  appForm.elements.display_name.value = row.display_name || "";
  appForm.elements.description.value = row.description || "";
  appForm.elements.active.value = row.active ? "true" : "false";
  versionAppSelect.value = String(row.id || "");
}

function resetAppForm() {
  appForm.reset();
  appForm.elements.id.value = "";
  appForm.elements.active.value = "true";
  appMsg.textContent = "";
}

async function saveApp() {
  appMsg.textContent = "";
  const payload = {
    id: Number(appForm.elements.id.value || 0),
    app_id: String(appForm.elements.app_id.value || "").trim(),
    display_name: String(appForm.elements.display_name.value || "").trim(),
    description: String(appForm.elements.description.value || "").trim(),
    active: String(appForm.elements.active.value || "true") === "true",
  };
  const method = payload.id ? "PUT" : "POST";
  const url = payload.id ? `/api/update-server/apps/${payload.id}` : "/api/update-server/apps";
  const resp = await api(url, {
    method,
    body: JSON.stringify(payload),
    allowFail: true,
  });
  if (!resp || resp.ok === false) {
    appMsg.textContent = resp?.error || "Save failed";
    return;
  }
  appMsg.textContent = "Aplicatia a fost salvata";
  state.selectedAppId = resp.app?.id || state.selectedAppId;
  versionAppSelect.value = String(state.selectedAppId || "");
  await loadApps();
}

async function loadVersions() {
  versionMsg.textContent = "";
  versionsBody.innerHTML = "";
  const selectedID = Number(versionAppSelect.value || state.selectedAppId || 0);
  state.selectedAppId = selectedID;
  if (!state.selectedAppId) {
    selectedAppLabel.textContent = "-";
    versionsTitle.textContent = "Versiuni publicate";
    versionsBody.innerHTML = `<tr><td colspan="5">Selecteaza mai intai o aplicatie</td></tr>`;
    return;
  }
  const appItem = state.apps.find((item) => item.id === state.selectedAppId);
  selectedAppLabel.textContent = appItem?.display_name || appItem?.app_id || "-";
  versionsTitle.textContent = `Versiuni pentru ${appItem?.display_name || appItem?.app_id || ""}`;
  const resp = await api(`/api/update-server/apps/${state.selectedAppId}/versions`, { allowFail: true });
  if (!resp || resp.ok === false) {
    versionMsg.textContent = resp?.error || "Load versions failed";
    return;
  }
  state.versions = resp.versions || [];
  if (!state.versions.some((row) => row.id === state.selectedVersionId)) {
    state.selectedVersionId = 0;
  }
  renderVersions();
}

function renderVersions() {
  versionsBody.innerHTML = "";
  const rows = getFilteredVersions();
  if (!rows.length) {
    versionsBody.innerHTML = `<tr><td colspan="5">Nu exista versiuni</td></tr>`;
    return;
  }
  rows.forEach((row) => {
    const target = [row.channel || "-", row.target_os || "*", row.target_arch || "*"].join(" / ");
    const pack = row.file_name ? esc(row.file_name) : "-";
    const installer = row.installer_file_name ? esc(row.installer_file_name) : "-";
    const tr = document.createElement("tr");
    tr.classList.toggle("active", row.id === state.selectedVersionId);
    tr.innerHTML = `<td>${esc(row.version)}</td><td>${esc(target)}</td><td>${row.mandatory ? "Da" : "Nu"}</td><td>${pack}</td><td>${installer}</td>`;
    tr.addEventListener("click", () => {
      state.selectedVersionId = row.id;
      fillVersionForm(row);
      renderVersions();
    });
    versionsBody.appendChild(tr);
  });
}

function getFilteredVersions() {
  const channel = String(versionChannelFilter.value || "").trim();
  const targetOS = String(versionOSFilter.value || "").trim();
  const targetArch = String(versionArchFilter.value || "").trim();
  return state.versions.filter((row) => {
    if (channel && String(row.channel || "") !== channel) return false;
    if (targetOS && String(row.target_os || "") !== targetOS) return false;
    if (targetArch && String(row.target_arch || "") !== targetArch) return false;
    return true;
  });
}

async function saveVersion() {
  versionMsg.textContent = "";
  state.selectedAppId = Number(versionAppSelect.value || state.selectedAppId || 0);
  if (!state.selectedAppId) {
    versionMsg.textContent = "Selecteaza mai intai o aplicatie";
    return;
  }
  const versionID = Number(versionForm.elements.id.value || 0);
  if (versionID > 0) {
    const existing = state.versions.find((item) => item.id === versionID) || {};
    const payload = {
      id: versionID,
      application_id: state.selectedAppId,
      version: String(versionForm.elements.version.value || "").trim(),
      channel: String(versionForm.elements.channel.value || "").trim(),
      target_os: String(versionForm.elements.target_os.value || "").trim(),
      target_arch: String(versionForm.elements.target_arch.value || "").trim(),
      mandatory: String(versionForm.elements.mandatory.value || "false") === "true",
      release_notes: String(versionForm.elements.release_notes.value || "").trim(),
      download_url: existing.download_url || "",
      file_name: existing.file_name || "",
      file_path: existing.file_path || "",
      checksum_sha256: existing.checksum_sha256 || "",
      file_size: Number(existing.file_size || 0),
      installer_file_name: existing.installer_file_name || "",
      installer_file_path: existing.installer_file_path || "",
      installer_checksum_sha256: existing.installer_checksum_sha256 || "",
      installer_file_size: Number(existing.installer_file_size || 0),
      uploaded_by: existing.uploaded_by || "",
      active: existing.active !== false,
    };
    const data = await api(`/api/update-server/versions/${versionID}`, {
      method: "PUT",
      body: JSON.stringify(payload),
      allowFail: true,
    });
    if (!data || data.ok === false) {
      versionMsg.textContent = data?.error || "Save failed";
      return;
    }
  } else {
    const formData = new FormData(versionForm);
    const res = await fetch(`/api/update-server/apps/${state.selectedAppId}/versions`, {
      method: "POST",
      body: formData,
      credentials: "same-origin",
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok || data.ok === false) {
      versionMsg.textContent = data?.error || `HTTP ${res.status}`;
      return;
    }
  }
  versionMsg.textContent = "Versiunea a fost salvata";
  resetVersionForm();
  await loadVersions();
}

async function makeRelease() {
  versionMsg.textContent = "";
  state.selectedAppId = Number(versionAppSelect.value || state.selectedAppId || 0);
  if (!state.selectedAppId) {
    versionMsg.textContent = "Selecteaza mai intai o aplicatie";
    return;
  }
  const targetOS = String(versionForm.elements.target_os.value || "").trim();
  const targetArch = String(versionForm.elements.target_arch.value || "").trim();
  if (!targetOS || !targetArch) {
    versionMsg.textContent = "Selecteaza OS si arhitectura pentru release";
    return;
  }
  setReleaseBusy(true, {
    title: "Se pregateste release-ul",
    detail: `Pregatim build-ul pentru ${targetOS} / ${targetArch} si blocam modificarile concurente.`,
  });
  const payload = {
    channel: String(versionForm.elements.channel.value || "stable").trim() || "stable",
    target_os: targetOS,
    target_arch: targetArch,
    mandatory: String(versionForm.elements.mandatory.value || "false") === "true",
    release_notes: String(versionForm.elements.release_notes.value || "").trim(),
  };
  try {
    bumpReleaseProgress("Compilam binarul", `Serverul ruleaza releasectl pentru ${targetOS} / ${targetArch}. Aceasta etapa poate dura mai mult.`);
    const resp = await api(`/api/update-server/apps/${state.selectedAppId}/make-release`, {
      method: "POST",
      body: JSON.stringify(payload),
      allowFail: true,
    });
    if (!resp || resp.ok === false) {
      versionMsg.textContent = resp?.error || "Make release failed";
      return;
    }
    bumpReleaseProgress("Actualizam lista de versiuni", "Release-ul a fost creat. Reincarcam tabelul si selectam versiunea noua.");
    const saved = resp.version || {};
    versionMsg.textContent = `Release creat: ${saved.version || "-"} (${saved.target_os || "-"} / ${saved.target_arch || "-"})${saved.installer_file_name ? `, installer: ${saved.installer_file_name}` : ""}`;
    state.selectedVersionId = Number(saved.id || 0);
    await loadVersions();
    if (state.selectedVersionId) {
      const current = state.versions.find((item) => item.id === state.selectedVersionId);
      if (current) {
        fillVersionForm(current);
        renderVersions();
      }
    }
  } finally {
    setReleaseBusy(false);
  }
}

async function deleteVersion() {
  versionMsg.textContent = "";
  const versionID = Number(versionForm.elements.id.value || state.selectedVersionId || 0);
  if (!versionID) {
    versionMsg.textContent = "Selecteaza mai intai o versiune";
    return;
  }
  const row = state.versions.find((item) => item.id === versionID);
  const label = row ? `${row.version || "-"} (${row.target_os || "*"} / ${row.target_arch || "*"})` : `#${versionID}`;
  if (!window.confirm(`Stergi versiunea ${label}?`)) {
    return;
  }
  deleteVersionBtn.disabled = true;
  const resp = await api(`/api/update-server/versions/${versionID}`, {
    method: "DELETE",
    allowFail: true,
  });
  deleteVersionBtn.disabled = false;
  if (!resp || resp.ok === false) {
    versionMsg.textContent = resp?.error || "Delete failed";
    return;
  }
  versionMsg.textContent = "Versiunea a fost stearsa";
  resetVersionForm();
  await loadVersions();
}

async function loadSettings() {
  const resp = await api("/api/update-server/settings", { allowFail: true });
  if (!resp || resp.ok === false) {
    settingsMsg.textContent = resp?.error || "Load settings failed";
    return;
  }
  const settings = resp.settings || {};
  state.settings = settings;
  allowedTypesLabel.textContent = settings.allowed_user_types || "-";
  for (const [key, value] of Object.entries(settings)) {
    const field = settingsForm.elements.namedItem(key);
    if (field) field.value = value || "";
  }
}

function syncVersionAppOptions() {
  const previous = String(state.selectedAppId || versionAppSelect.value || "");
  versionAppSelect.innerHTML = `<option value="">Selecteaza aplicatia</option>`;
  state.apps.forEach((app) => {
    const opt = document.createElement("option");
    opt.value = String(app.id);
    opt.textContent = `${app.display_name || app.app_id} (${app.app_id})`;
    versionAppSelect.appendChild(opt);
  });
  if (previous && state.apps.some((app) => String(app.id) === previous)) {
    versionAppSelect.value = previous;
  }
}

function onVersionAppChange() {
  state.selectedAppId = Number(versionAppSelect.value || 0);
  const appItem = state.apps.find((item) => item.id === state.selectedAppId);
  selectedAppLabel.textContent = appItem?.display_name || appItem?.app_id || "-";
  state.selectedVersionId = 0;
  resetVersionForm(false);
  if (appItem) {
    fillAppForm(appItem);
  }
  if (views.history && !views.history.hidden) {
    loadVersions();
  }
}

function fillVersionForm(row) {
  versionForm.elements.id.value = row.id || "";
  versionForm.elements.version.value = row.version || "";
  versionForm.elements.channel.value = row.channel || "";
  versionForm.elements.target_os.value = row.target_os || "";
  versionForm.elements.target_arch.value = row.target_arch || "";
  versionForm.elements.mandatory.value = row.mandatory ? "true" : "false";
  versionForm.elements.release_notes.value = row.release_notes || "";
  versionForm.elements.download_preview.value = buildDownloadPreview(row.id);
  versionForm.elements.installer_preview.value = buildInstallerPreview(row);
  downloadLinkPreview.hidden = true;
  downloadLinkPreview.innerHTML = "";
  installerLinkPreview.hidden = !row.installer_file_name;
  installerLinkPreview.innerHTML = row.installer_file_name ? `<a href="${esc(buildInstallerPreview(row))}" target="_blank" rel="noreferrer">${esc(row.installer_file_name)}</a>` : "";
}

function resetVersionForm(resetFile = true) {
  versionForm.reset();
  versionForm.elements.id.value = "";
  versionForm.elements.channel.value = "stable";
  versionForm.elements.target_os.value = "";
  versionForm.elements.target_arch.value = "";
  versionForm.elements.mandatory.value = "false";
  versionForm.elements.download_preview.value = "";
  versionForm.elements.installer_preview.value = "";
  downloadLinkPreview.hidden = true;
  downloadLinkPreview.innerHTML = "";
  installerLinkPreview.hidden = true;
  installerLinkPreview.innerHTML = "";
  state.selectedVersionId = 0;
  if (resetFile && versionForm.elements.package) {
    versionForm.elements.package.value = "";
  }
}

function buildDownloadPreview(versionID) {
  if (!versionID) return "";
  const publicBaseURL = String(state.settings.public_base_url || "").trim().replace(/\/+$/, "");
  if (publicBaseURL) {
    return `${publicBaseURL}/api/public/download/${versionID}?token=<one-time-token>`;
  }
  const protocol = state.settings.public_protocol || "http";
  const host = state.settings.public_host || "127.0.0.1";
  const port = state.settings.public_port || "19090";
  return `${protocol}://${host}:${port}/api/public/download/${versionID}?token=<one-time-token>`;
}

function buildInstallerPreview(row) {
  if (!row || !row.id || !row.installer_file_name) return "";
  return `<one-time-token pentru installer>`;
}

async function generateDownloadLink() {
  versionMsg.textContent = "";
  const versionID = Number(versionForm.elements.id.value || state.selectedVersionId || 0);
  if (!versionID) {
    versionMsg.textContent = "Selecteaza mai intai o versiune existenta";
    return;
  }
  const resp = await api(`/api/update-server/download-link/${versionID}`, {
    method: "POST",
    allowFail: true,
  });
  if (!resp || resp.ok === false) {
    versionMsg.textContent = resp?.error || "Generate link failed";
    return;
  }
  const url = String(resp.download_url || "").trim();
  versionForm.elements.download_preview.value = url;
  downloadLinkPreview.hidden = !url;
  downloadLinkPreview.innerHTML = url ? `<a href="${esc(url)}" target="_blank" rel="noreferrer">${esc(url)}</a>` : "";
  const row = state.versions.find((item) => item.id === versionID);
  if (row && row.installer_file_name) {
    const installerResp = await api(`/api/update-server/download-link/${versionID}?artifact=installer`, {
      method: "POST",
      allowFail: true,
    });
    const installerURL = String(installerResp?.download_url || "").trim();
    versionForm.elements.installer_preview.value = installerURL;
    installerLinkPreview.hidden = !installerURL;
    installerLinkPreview.innerHTML = installerURL ? `<a href="${esc(installerURL)}" target="_blank" rel="noreferrer">${esc(row.installer_file_name)}</a>` : "";
  }
}

function setReleaseBusy(isBusy, progress = {}) {
  state.releaseBusy = isBusy;
  makeReleaseBtn.disabled = isBusy;
  saveVersionBtn.disabled = isBusy;
  deleteVersionBtn.disabled = isBusy;
  resetVersionBtn.disabled = isBusy;
  refreshVersionsBtn.disabled = isBusy;
  generateDownloadLinkBtn.disabled = isBusy;
  versionAppSelect.disabled = isBusy;
  versionChannelFilter.disabled = isBusy;
  versionOSFilter.disabled = isBusy;
  versionArchFilter.disabled = isBusy;
  releaseProgress.hidden = !isBusy;
  releaseProgress.setAttribute("aria-busy", isBusy ? "true" : "false");
  if (!isBusy) {
    releaseProgressTitle.textContent = "Se pregateste release-ul";
    releaseProgressDetail.textContent = "Operatia poate dura cateva zeci de secunde.";
    return;
  }
  bumpReleaseProgress(progress.title, progress.detail);
}

function bumpReleaseProgress(title, detail) {
  if (title) {
    releaseProgressTitle.textContent = title;
  }
  if (detail) {
    releaseProgressDetail.textContent = detail;
  }
}

async function saveSettings() {
  settingsMsg.textContent = "";
  const payload = {};
  Array.from(settingsForm.elements).forEach((el) => { if (el.name) payload[el.name] = el.value; });
  const resp = await api("/api/update-server/settings", {
    method: "PUT",
    body: JSON.stringify(payload),
    allowFail: true,
  });
  if (!resp || resp.ok === false) {
    settingsMsg.textContent = resp?.error || "Save failed";
    return;
  }
  settingsMsg.textContent = "Setarile au fost salvate in config.yaml";
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
