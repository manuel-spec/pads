"use strict";

const daemonEl = document.getElementById("daemon");
const captureEl = document.getElementById("capture");
const hintEl = document.getElementById("hint");
const jobsEl = document.getElementById("jobs");
const emptyEl = document.getElementById("empty");

let timer = null;

document.addEventListener("DOMContentLoaded", async () => {
  const settings = await loadSettings();
  captureEl.checked = settings.capture;
  captureEl.addEventListener("change", () => {
    browser.storage.local.set({ capture: captureEl.checked });
  });

  await refresh();
  timer = setInterval(refresh, 1000);
});

window.addEventListener("unload", () => {
  if (timer !== null) {
    clearInterval(timer);
  }
});

async function refresh() {
  const reply = await callHost({ type: "jobs" });

  if (!reply.ok) {
    setDaemon("unknown", "host error");
    showHint(reply.error);
    render([]);
    return;
  }
  if (!reply.running) {
    setDaemon("down", "daemon down");
    showHint("No daemon is running.\nStart one with: pads daemon run");
    render([]);
    return;
  }

  setDaemon("up", "daemon up");
  hideHint();
  render(reply.jobs || []);
}

function setDaemon(state, label) {
  daemonEl.className = `status status--${state}`;
  daemonEl.textContent = label;
}

function showHint(text) {
  hintEl.textContent = text;
  hintEl.hidden = false;
}

function hideHint() {
  hintEl.hidden = true;
}

function render(jobs) {
  emptyEl.hidden = jobs.length > 0;
  jobsEl.replaceChildren(...jobs.map(renderJob));
}

function renderJob(job) {
  const li = document.createElement("li");
  li.className = "job";

  const top = document.createElement("div");
  top.className = "job__top";

  const name = document.createElement("span");
  name.className = "job__name";
  name.textContent = baseName(job.output) || job.url || job.id;
  name.title = job.url || "";

  const state = document.createElement("span");
  state.className = `job__state job__state--${job.status}`;
  state.textContent = job.status;

  top.append(name, state);

  const track = document.createElement("div");
  track.className = "job__track";
  const fill = document.createElement("div");
  fill.className = "job__fill";
  fill.style.width = `${percent(job)}%`;
  track.append(fill);

  const meta = document.createElement("div");
  meta.className = "job__meta";

  const counts = document.createElement("span");
  counts.textContent = job.total > 0
    ? `${formatBytes(job.bytes_done)} / ${formatBytes(job.total_size)}`
    : formatBytes(job.bytes_done);
  meta.append(counts);

  const action = actionFor(job);
  if (action) {
    meta.append(action);
  }

  li.append(top, track, meta);

  if (job.error) {
    const error = document.createElement("div");
    error.className = "job__meta job__error";
    error.textContent = job.error;
    li.append(error);
  }
  return li;
}

// percent falls back to 0 rather than NaN while the total size is still unknown.
function percent(job) {
  const total = Number(job.total_size) || 0;
  const done = Number(job.bytes_done) || 0;
  if (total <= 0) {
    return job.status === "complete" ? 100 : 0;
  }
  return Math.min(100, Math.round((done / total) * 100));
}

function actionFor(job) {
  if (job.status === "running") {
    return button("Pause", () => act({ type: "pause", id: job.id }));
  }
  if (job.status === "paused" || job.status === "failed") {
    return button("Resume", () => act({ type: "resume", id: job.id }));
  }
  return null;
}

function button(label, onClick) {
  const el = document.createElement("button");
  el.type = "button";
  el.textContent = label;
  el.addEventListener("click", onClick);
  return el;
}

async function act(message) {
  const reply = await callHost(message);
  if (!reply.ok) {
    showHint(reply.error);
    return;
  }
  await refresh();
}
