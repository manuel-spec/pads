"use strict";

// Shared helpers for talking to the PADS native-messaging host. Loaded by both
// the background script and the popup; native messaging is available from any
// extension context that holds the permission, so neither has to proxy for the
// other.

const PADS_HOST = "pads";

const PADS_DEFAULTS = {
  capture: true,
  minBytes: 0,
  notify: true,
};

// callHost never rejects. Every caller wants to show the failure in the UI
// rather than handle an exception, so errors come back as {ok:false, error}.
async function callHost(message) {
  try {
    const reply = await browser.runtime.sendNativeMessage(PADS_HOST, message);
    if (!reply || typeof reply !== "object") {
      return { ok: false, error: "The PADS host sent a malformed reply." };
    }
    return reply;
  } catch (err) {
    return { ok: false, error: describeHostError(err) };
  }
}

// describeHostError turns Firefox's native-messaging errors into something a
// user can act on. The common one by far is a host that was never registered.
function describeHostError(err) {
  const text = String((err && err.message) || err || "unknown error");
  if (/no such native application|not found|failed to start/i.test(text)) {
    return "PADS native host is not registered. Run: pads nativehost install";
  }
  if (/attempt to postMessage on disconnected port|terminated/i.test(text)) {
    return "The PADS host stopped unexpectedly.";
  }
  return text;
}

async function loadSettings() {
  try {
    const stored = await browser.storage.local.get(PADS_DEFAULTS);
    return { ...PADS_DEFAULTS, ...stored };
  } catch (err) {
    return { ...PADS_DEFAULTS };
  }
}

// baseName strips any directory part a suggested filename carries. The host
// sanitises it again; this is only so the popup shows something sensible.
function baseName(name) {
  if (typeof name !== "string" || name === "") {
    return "";
  }
  const parts = name.replace(/\\/g, "/").split("/");
  return parts[parts.length - 1] || "";
}

function formatBytes(bytes) {
  const value = Number(bytes) || 0;
  if (value < 1024) {
    return `${value} B`;
  }
  const units = ["KiB", "MiB", "GiB", "TiB"];
  let scaled = value / 1024;
  let unit = 0;
  while (scaled >= 1024 && unit < units.length - 1) {
    scaled /= 1024;
    unit += 1;
  }
  return `${scaled.toFixed(1)} ${units[unit]}`;
}
