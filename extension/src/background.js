"use strict";

// Chromium runs this as a service worker and loads only this file, so it pulls
// in the shared helpers itself. Firefox lists both scripts in its manifest and
// has no importScripts, which is what the guard tests for.
if (typeof importScripts === "function") {
  importScripts("host.js");
}

// Captures browser downloads and hands them to the PADS daemon.
//
// The download is paused the moment Firefox creates it, before the handoff is
// attempted. Pausing is reversible, so neither outcome loses anything: on
// success the paused download is cancelled and PADS has the file, and on
// failure it resumes exactly where it stopped. Handing off first and cancelling
// afterwards would download the opening bytes twice; cancelling first would
// throw the download away if the daemon turned out to be unreachable.

let settings = { ...PADS_DEFAULTS };

// URLs this extension asked the browser to download itself, after a handoff
// failed and the paused download could not be resumed. Skipped once so the
// retry is not captured straight back into the same failure.
//
// Chromium can stop the service worker between the re-issue and the download
// that follows it, so this lives in session storage where available; an
// in-memory set alone would let that pair loop.
const HAND_BACK_KEY = "padsHandBack";
const HAND_BACK_TTL_MS = 5 * 60 * 1000;
const handBackMemory = new Set();

function sessionStore() {
  return (browser.storage && browser.storage.session) || null;
}

async function markHandBack(url) {
  const store = sessionStore();
  if (!store) {
    handBackMemory.add(url);
    return;
  }
  try {
    const held = (await store.get(HAND_BACK_KEY))[HAND_BACK_KEY] || {};
    held[url] = Date.now();
    await store.set({ [HAND_BACK_KEY]: pruneHandBack(held) });
  } catch (err) {
    handBackMemory.add(url);
  }
}

// takeHandBack reports whether this URL was one the extension re-issued, and
// clears it so only that one download is let through.
async function takeHandBack(url) {
  if (handBackMemory.delete(url)) {
    return true;
  }
  const store = sessionStore();
  if (!store) {
    return false;
  }
  try {
    const held = (await store.get(HAND_BACK_KEY))[HAND_BACK_KEY] || {};
    if (!(url in held)) {
      return false;
    }
    delete held[url];
    await store.set({ [HAND_BACK_KEY]: pruneHandBack(held) });
    return true;
  } catch (err) {
    return false;
  }
}

// pruneHandBack drops stale entries so a re-issue that never arrived does not
// exempt that URL from capture for the rest of the session.
function pruneHandBack(held) {
  const cutoff = Date.now() - HAND_BACK_TTL_MS;
  const kept = {};
  for (const [url, at] of Object.entries(held)) {
    if (at >= cutoff) {
      kept[url] = at;
    }
  }
  return kept;
}

async function refreshSettings() {
  settings = await loadSettings();
}

browser.storage.onChanged.addListener((changes, area) => {
  if (area === "local") {
    refreshSettings();
  }
});

browser.downloads.onCreated.addListener(handleCreated);

async function handleCreated(item) {
  await refreshSettings();
  if (!shouldCapture(item)) {
    return;
  }
  if (await takeHandBack(item.url)) {
    return;
  }

  // Stop the bytes first. Everything after this is reversible.
  const paused = await pauseDownload(item.id);

  const reply = await callHost({
    type: "start",
    url: item.url,
    filename: baseName(item.filename),
    headers: await sessionHeaders(item),
  });

  if (!reply.ok) {
    await handBackToBrowser(item, paused);
    await notify("PADS handoff failed", `${reply.error}\nThe browser is downloading it instead.`);
    return;
  }

  await discardBrowserDownload(item.id);
  await notify("Sent to PADS", reply.output || item.url);
}

function shouldCapture(item) {
  if (!settings.capture || !item || typeof item.url !== "string") {
    return false;
  }
  // blob: and data: URLs exist only inside the page; there is nothing for the
  // daemon to re-fetch.
  if (!/^https?:\/\//i.test(item.url)) {
    return false;
  }
  if (item.state === "complete" || item.state === "interrupted") {
    return false;
  }
  // A known-small file is not worth a segmented downloader. fileSize is -1
  // until the server reports one.
  if (settings.minBytes > 0 && item.fileSize > 0 && item.fileSize < settings.minBytes) {
    return false;
  }
  return true;
}

async function pauseDownload(id) {
  try {
    await browser.downloads.pause(id);
    return true;
  } catch (err) {
    // Too fast to pause, or already finished. The handoff still runs; the
    // duplicate is bounded by whatever Firefox managed in that window.
    return false;
  }
}

// handBackToBrowser restores the download the extension interfered with.
// Resuming is preferred because it keeps the bytes already on disk; a download
// the server will not resume has to be started again from scratch.
async function handBackToBrowser(item, paused) {
  if (paused) {
    try {
      await browser.downloads.resume(item.id);
      return;
    } catch (err) {
      // Not resumable; fall through and re-issue it.
    }
  }

  try {
    const current = await browser.downloads.search({ id: item.id });
    if (current.length > 0 && current[0].state === "in_progress" && !current[0].paused) {
      return;
    }
  } catch (err) {
    // Fall through and re-issue.
  }

  await markHandBack(item.url);
  try {
    await browser.downloads.download({ url: item.url });
  } catch (err) {
    await takeHandBack(item.url);
  }
}

// discardBrowserDownload tears down Firefox's copy once PADS owns the transfer.
// Each step is optional: the download may have finished or been removed while
// the handoff was in flight.
async function discardBrowserDownload(id) {
  try {
    await browser.downloads.cancel(id);
  } catch (err) {
    // Already finished or cancelled by the user.
  }
  try {
    await browser.downloads.removeFile(id);
  } catch (err) {
    // Only a completed download has a file to remove.
  }
  try {
    await browser.downloads.erase({ id });
  } catch (err) {
    // A leftover history entry is harmless.
  }
}

// sessionHeaders collects what the daemon needs to fetch a file that is behind
// a login. It returns nothing unless the user has granted the optional cookie
// permission, so the default install sends no session data anywhere.
async function sessionHeaders(item) {
  if (!(await hasSessionPermission())) {
    return {};
  }

  const headers = {};
  const cookie = await cookieHeader(item.url);
  if (cookie) {
    headers["Cookie"] = cookie;
  }
  if (item.referrer && /^https?:\/\//i.test(item.referrer)) {
    headers["Referer"] = item.referrer;
  }
  if (navigator.userAgent) {
    headers["User-Agent"] = navigator.userAgent;
  }
  return headers;
}

async function cookieHeader(url) {
  try {
    // firstPartyDomain: null returns cookies whether or not first-party
    // isolation is on; without it an isolated profile yields nothing.
    let cookies = [];
    try {
      cookies = await browser.cookies.getAll({ url, firstPartyDomain: null });
    } catch (err) {
      cookies = await browser.cookies.getAll({ url });
    }
    return cookies
      .map((cookie) => `${cookie.name}=${cookie.value}`)
      .join("; ");
  } catch (err) {
    return "";
  }
}

browser.runtime.onInstalled.addListener(setupMenus);
browser.runtime.onStartup.addListener(setupMenus);

async function setupMenus() {
  await refreshSettings();
  try {
    await browser.contextMenus.removeAll();
    browser.contextMenus.create({
      id: "pads-download",
      title: "Download with PADS",
      contexts: ["link", "image", "video", "audio"],
    });
  } catch (err) {
    // A missing context menu is not worth failing startup over.
  }
}

browser.contextMenus.onClicked.addListener(async (info) => {
  if (info.menuItemId !== "pads-download") {
    return;
  }
  const url = info.linkUrl || info.srcUrl;
  if (!url) {
    return;
  }

  const reply = await callHost({
    type: "start",
    url,
    filename: "",
    headers: await sessionHeaders({ url, referrer: info.pageUrl }),
  });
  if (!reply.ok) {
    await notify("PADS could not start the download", reply.error);
    return;
  }
  await notify("Sent to PADS", reply.output || url);
});

async function notify(title, message) {
  if (!settings.notify) {
    return;
  }
  try {
    await browser.notifications.create({
      type: "basic",
      iconUrl: browser.runtime.getURL(PADS_ICON),
      title,
      message: String(message).slice(0, 300),
    });
  } catch (err) {
    // Notifications are a convenience, never a requirement.
  }
}

refreshSettings();
