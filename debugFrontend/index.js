const state = {
  data: null,
  timer: null,
  loading: false
};

const el = {
  statusDot: document.getElementById("statusDot"),
  statusText: document.getElementById("statusText"),
  subline: document.getElementById("subline"),
  metrics: document.getElementById("metrics"),
  runtimePanel: document.getElementById("runtimePanel"),
  configPanel: document.getElementById("configPanel"),
  objectsPanel: document.getElementById("objectsPanel"),
  udpPanel: document.getElementById("udpPanel"),
  featuresPanel: document.getElementById("featuresPanel"),
  scenesPanel: document.getElementById("scenesPanel"),
  rawPanel: document.getElementById("rawPanel"),
  runtimeBadge: document.getElementById("runtimeBadge"),
  objectBadge: document.getElementById("objectBadge"),
  udpBadge: document.getElementById("udpBadge"),
  featuresBadge: document.getElementById("featuresBadge"),
  scenesBadge: document.getElementById("scenesBadge"),
  autoRefresh: document.getElementById("autoRefresh"),
  refreshRate: document.getElementById("refreshRate"),
  refreshButton: document.getElementById("refreshButton"),
  objectSearch: document.getElementById("objectSearch"),
  visibilityFilter: document.getElementById("visibilityFilter")
};

function text(value) {
  if (value === null || value === undefined || value === "") return "none";
  return String(value);
}

function number(value) {
  if (typeof value === "number") return value.toLocaleString();
  return text(value);
}

function escapeHTML(value) {
  return text(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function setStatus(kind, label) {
  el.statusDot.className = "status-dot " + kind;
  el.statusText.textContent = label;
}

async function fetchState() {
  if (state.loading) return;
  state.loading = true;
  try {
    const response = await fetch("/debug/state", { cache: "no-store" });
    if (!response.ok) throw new Error(response.status + " " + response.statusText);
    state.data = await response.json();
    render();
    setStatus("ok", "Live");
  } catch (error) {
    setStatus("err", "Disconnected");
    el.subline.textContent = error.message;
  } finally {
    state.loading = false;
  }
}

function render() {
  const data = state.data;
  if (!data) return;

  const generated = new Date(data.generatedAt);
  el.subline.textContent = "Updated " + generated.toLocaleTimeString() + " from " + data.url;
  renderMetrics(data);
  renderRuntime(data.runtime);
  renderConfig(data.config);
  renderObjects(data.runtime.objects || []);
  renderUDP(data.udp);
  renderFeatures(data.features);
  renderScenes(data.features);
  el.rawPanel.textContent = JSON.stringify(data, null, 2);
}

function renderMetrics(data) {
  const summary = data.summary || {};
  const udp = data.udp || {};
  const metrics = [
    ["Tick", number(summary.tick), data.config.tickRate + "/s"],
    ["Objects", number(summary.objectCount), number(summary.objectTypeCount) + " types"],
    ["Factories", number(summary.factoryCount), "registered"],
    ["UDP Clients", number(summary.udpClientCount), number(summary.authenticatedUdpClientCount) + " authenticated"],
    ["Scenes", summary.scenesEnabled ? "enabled" : "idle", "scene filtering"],
    ["Snapshots", data.config.snapshotRate + "/s", "every " + data.config.snapshotEvery + " ticks"],
    ["Uptime", udp.uptime || "not started", udp.address || data.config.udpAddress]
  ];

  el.metrics.innerHTML = metrics.map(function(item) {
    return "<article class=\"metric\"><div class=\"label\">" + escapeHTML(item[0]) + "</div><div class=\"value\">" + escapeHTML(item[1]) + "</div><div class=\"detail\">" + escapeHTML(item[2]) + "</div></article>";
  }).join("");
}

function renderRuntime(runtime) {
  el.runtimeBadge.textContent = "tick " + number(runtime.tick);
  const auth = runtime.authentication || {};
  const authLabel = auth.required ? auth.objectType + "/" + auth.objectId : "disabled";
  const rows = [
    ["Tick", number(runtime.tick)],
    ["Objects", number(runtime.objectCount)],
    ["Object types", number(runtime.objectTypes)],
    ["Factories", number(runtime.factoryCount)],
    ["Authentication", authLabel],
    ["Auth handler", auth.goType || "none"]
  ];

  const factoryChips = (runtime.factories || []).map(function(factory) {
    return "<span class=\"chip\">" + escapeHTML(factory.objectType) + "</span>";
  }).join("") || "<span class=\"muted\">none</span>";

  el.runtimePanel.innerHTML = table(rows) + "<div style=\"height:12px\"></div><div class=\"chips\">" + factoryChips + "</div>";
}

function renderConfig(config) {
  const rows = [
    ["UDP address", config.udpAddress],
    ["Tick rate", config.tickRate + "/s"],
    ["Tick interval", config.tickInterval + " (" + config.tickIntervalMs.toFixed(3) + " ms)"],
    ["Snapshot rate", config.snapshotRate + "/s"],
    ["Snapshot every", config.snapshotEvery + " ticks"],
    ["Client timeout", config.clientTimeout],
    ["Max UDP packet", number(config.maxUdpPacketSize) + " bytes"],
    ["Debug enabled", config.debugEnabled],
    ["Debug address", config.debugAddress],
    ["Debug URL", config.debugUrl]
  ];
  el.configPanel.innerHTML = table(rows);
}

function renderObjects(groups) {
  const query = el.objectSearch.value.trim().toLowerCase();
  const visibility = el.visibilityFilter.value;
  let total = 0;
  const html = groups.map(function(group) {
    const objects = (group.objects || []).filter(function(object) {
      const haystack = [object.objectType, object.objectId, object.goType, JSON.stringify(object.snapshot || {})].join(" ").toLowerCase();
      const matchesQuery = !query || haystack.indexOf(query) !== -1;
      const matchesVisibility = visibility === "all" || (visibility === "visible" && object.visible) || (visibility === "hidden" && !object.visible);
      return matchesQuery && matchesVisibility;
    });
    if (objects.length === 0) return "";
    total += objects.length;
    return "<div class=\"object-group\"><div class=\"group-head\"><h3>" + escapeHTML(group.objectType) + "</h3><span class=\"chip\">" + objects.length + "</span></div><div class=\"object-list\">" + objects.map(renderObject).join("") + "</div></div>";
  }).join("");

  el.objectBadge.textContent = number(total) + " shown";
  el.objectsPanel.innerHTML = html || "<div class=\"empty\">No objects</div>";
}

function renderObject(object) {
  const visibleChip = object.visible ? "<span class=\"chip good\">visible</span>" : "<span class=\"chip warn\">hidden</span>";
  const errorChip = object.snapshotError ? "<span class=\"chip bad\">" + escapeHTML(object.snapshotError) + "</span>" : "";
  const caps = (object.capabilities || []).map(function(capability) {
    return "<span class=\"chip\">" + escapeHTML(capability) + "</span>";
  }).join("");
  return "<article class=\"object-card\"><div class=\"object-head\"><div class=\"object-title\"><div class=\"object-id\">" + escapeHTML(object.objectId) + "</div><div class=\"object-type\">" + escapeHTML(object.goType) + "</div></div><div class=\"chips\">" + visibleChip + errorChip + "</div></div><div class=\"section-body\"><div class=\"chips\">" + caps + "</div></div><div class=\"tree\">" + tree(object.snapshot) + "</div></article>";
}

function renderUDP(udp) {
  const started = udp && udp.started;
  el.udpBadge.textContent = started ? udp.address : "not started";
  const counters = udp.counters || {};
  const counterNames = [
    ["Datagrams", counters.datagramsReceived],
    ["Accepted", counters.requestsAccepted],
    ["Dropped", counters.requestsDropped],
    ["Errors", counters.requestErrors],
    ["Auth tries", counters.authAttempts],
    ["Auth ok", counters.authSuccesses],
    ["Snapshots", counters.snapshotsSent],
    ["Oversized", counters.oversizedOutboundPacketsDropped],
    ["Reliable queued", counters.reliableMessagesQueued],
    ["Reliable retries", counters.reliableRetransmits],
    ["Reliable drops", counters.reliableDrops],
    ["Reliable acks", counters.reliableAckRemovals],
    ["Clients made", counters.clientsCreated]
  ];
  const counterHTML = "<div class=\"mini-grid\">" + counterNames.map(function(item) {
    return "<div class=\"counter\"><b>" + escapeHTML(number(item[1] || 0)) + "</b><span>" + escapeHTML(item[0]) + "</span></div>";
  }).join("") + "</div>";

  const clients = (udp.clients || []).map(function(client) {
    return ["Address", client.address, "ID", client.id, "Connection", client.connectionId, "Authenticated", client.authenticated, "Last seq", client.lastSeq, "Reliable queued", client.reliableQueued, "Idle", client.idle];
  }).map(function(rows) {
    const pairs = [];
    for (let i = 0; i < rows.length; i += 2) pairs.push([rows[i], rows[i + 1]]);
    return "<div style=\"height:10px\"></div>" + table(pairs);
  }).join("");

  el.udpPanel.innerHTML = table([
    ["Started", started],
    ["Address", udp.address || "none"],
    ["Uptime", udp.uptime || "none"],
    ["Clients", (udp.clientCount || 0) + " total, " + (udp.authenticatedClientCount || 0) + " authenticated"],
    ["Timeout", udp.clientTimeout || "none"]
  ]) + "<div style=\"height:12px\"></div>" + counterHTML + clients;
}

function renderFeatures(features) {
  const interest = (features && features.interestManagement) || {};
  el.featuresBadge.textContent = interest.enabled ? "enabled" : "idle";
  const rows = [
    ["Configured", !!interest.configured],
    ["Enabled", !!interest.enabled],
    ["Players", interest.playerCount || 0],
    ["Objects", interest.objectCount || 0]
  ];
  const players = renderInterestList("Players", interest.players || []);
  const objects = renderInterestList("Objects", interest.objects || []);
  el.featuresPanel.innerHTML = table(rows) + players + objects;
}

function renderScenes(features) {
  const scenes = (features && features.sceneManagement) || {};
  const clientScenes = scenes.clientScenes || [];
  const objectScenes = scenes.objectScenes || [];
  el.scenesBadge.textContent = scenes.enabled ? "enabled" : "idle";

  const rows = [
    ["Configured", !!scenes.configured],
    ["Enabled", !!scenes.enabled],
    ["Clients", scenes.clientCount || 0],
    ["Objects", scenes.objectCount || 0]
  ];

  const clients = renderSceneList("Client scenes", clientScenes);
  const objects = renderSceneList("Object scenes", objectScenes);
  el.scenesPanel.innerHTML = table(rows) + clients + objects;
}

function renderSceneList(title, items) {
  if (!items.length) return "";
  return "<div class=\"subsection\"><h3>" + escapeHTML(title) + "</h3>" + table(items.map(function(item) {
    return [item.key, item.scene];
  })) + "</div>";
}

function renderInterestList(title, items) {
  if (!items.length) return "";
  return "<div class=\"subsection\"><h3>" + escapeHTML(title) + "</h3>" + items.map(function(item) {
    return table([
      ["Key", item.key],
      ["Subject", item.subjectType],
      ["Object", item.objectType + "/" + item.objectId],
      ["Fields", [item.xField, item.yField, item.zField].filter(Boolean).join(", ")],
      ["Radius", item.radius],
      ["Include self", item.includeSelf]
    ]);
  }).join("<div style=\"height:8px\"></div>") + "</div>";
}

function table(rows) {
  return "<div class=\"table\">" + rows.map(function(row) {
    return "<div class=\"row\"><div class=\"cell\">" + escapeHTML(row[0]) + "</div><div class=\"cell\">" + escapeHTML(row[1]) + "</div></div>";
  }).join("") + "</div>";
}

function tree(value) {
  if (value === null || value === undefined) return "<span class=\"muted\">null</span>";
  if (typeof value !== "object") return "<span class=\"scalar\">" + escapeHTML(value) + "</span>";
  if (Array.isArray(value)) {
    if (value.length === 0) return "<span class=\"muted\">[]</span>";
    return "<ul>" + value.map(function(item, index) {
      return "<li><span class=\"key\">" + index + "</span>: " + tree(item) + "</li>";
    }).join("") + "</ul>";
  }
  const keys = Object.keys(value).sort();
  if (keys.length === 0) return "<span class=\"muted\">{}</span>";
  return "<ul>" + keys.map(function(key) {
    return "<li><span class=\"key\">" + escapeHTML(key) + "</span>: " + tree(value[key]) + "</li>";
  }).join("") + "</ul>";
}

function restartTimer() {
  if (state.timer) clearInterval(state.timer);
  if (el.autoRefresh.checked) {
    state.timer = setInterval(fetchState, Number(el.refreshRate.value));
  }
}

el.refreshButton.addEventListener("click", fetchState);
el.autoRefresh.addEventListener("change", restartTimer);
el.refreshRate.addEventListener("change", restartTimer);
el.objectSearch.addEventListener("input", render);
el.visibilityFilter.addEventListener("change", render);

fetchState();
restartTimer();
