"use strict";

// Captures Firefox downloads and hands them to the PADS daemon.
//
// The handoff happens before Firefox's download is cancelled, not after. If
// PADS is unreachable the browser's own download simply carries on, which costs
// a few duplicated bytes but never loses a download to a daemon that is down.

let settings = { ...PADS_DEFAULTS };

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

  const reply = await callHost({
    type: "start",
    url: item.url,
    filename: baseName(item.filename),
  });

  if (!reply.ok) {
    await notify("PADS handoff failed", `${reply.error}\nFirefox is keeping this download.`);
    return;
  }

  await stopBrowserDownload(item.id);
  await notify("Sent to PADS", reply.output || item.url);
}

function shouldCapture(item) {
  if (!settings.capture || !item || typeof item.url !== "string") {
    return false;
  }
  // blob:, data: and ftp: downloads have no URL the daemon could re-fetch.
  if (!/^https?:\/\//i.test(item.url)) {
    return false;
  }
  if (item.state === "complete" || item.state === "interrupted") {
    return false;
  }
  // A known-small file is not worth the round trip through a segmented
  // downloader; fileSize is -1 until the server reports one.
  if (settings.minBytes > 0 && item.fileSize > 0 && item.fileSize < settings.minBytes) {
    return false;
  }
  return true;
}

// stopBrowserDownload tears down Firefox's copy once PADS owns the transfer.
// Each step is optional: the download may have finished or been removed in the
// time the handoff took.
async function stopBrowserDownload(id) {
  try {
    await browser.downloads.cancel(id);
  } catch (err) {
    // Already finished or cancelled by the user.
  }
  try {
    await browser.downloads.removeFile(id);
  } catch (err) {
    // Only completed downloads have a file to remove.
  }
  try {
    await browser.downloads.erase({ id });
  } catch (err) {
    // Leaving a history entry behind is harmless.
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

  const reply = await callHost({ type: "start", url, filename: "" });
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
      title,
      message: String(message).slice(0, 300),
    });
  } catch (err) {
    // Notifications are a convenience, never a requirement.
  }
}

refreshSettings();
