// Loads the background entry point the way each browser does, under stub APIs,
// and reports what it registered.
//
// Chromium runs it as a service worker that pulls in host.js with
// importScripts; Firefox loads both files from the manifest's scripts array and
// has no importScripts at all. Neither path is exercised by a syntax check,
// and getting either wrong means an extension that installs and does nothing.
//
// Run with: make extension-check

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import vm from "node:vm";

const here = dirname(fileURLToPath(import.meta.url));
const src = join(here, "src");

function stubAPI(registered) {
  const listener = (name) => ({
    addListener: () => registered.listeners.push(name),
  });
  return {
    runtime: {
      onInstalled: listener("runtime.onInstalled"),
      onStartup: listener("runtime.onStartup"),
      getURL: (path) => `chrome-extension://stub/${path}`,
      sendNativeMessage: async () => ({ ok: true, running: false }),
      lastError: null,
    },
    downloads: {
      onCreated: listener("downloads.onCreated"),
      pause: async () => {},
      resume: async () => {},
      cancel: async () => {},
      erase: async () => {},
      removeFile: async () => {},
      search: async () => [],
      download: async () => 1,
    },
    storage: {
      local: { get: async (d) => d ?? {}, set: async () => {} },
      session: { get: async () => ({}), set: async () => {} },
      onChanged: listener("storage.onChanged"),
    },
    contextMenus: {
      onClicked: listener("contextMenus.onClicked"),
      removeAll: async () => {},
      create: () => {},
    },
    notifications: { create: async () => "id" },
    permissions: { contains: async () => false, request: async () => false, remove: async () => true },
  };
}

function run(target) {
  const registered = { listeners: [] };
  const api = stubAPI(registered);

  const sandbox = {
    navigator: { userAgent: "stub" },
    console,
    setTimeout,
    clearTimeout,
    setInterval,
    clearInterval,
  };
  sandbox.globalThis = sandbox;

  if (target === "chrome") {
    // Chromium: chrome.* only, and the worker loads host.js itself.
    sandbox.chrome = api;
    sandbox.importScripts = (...files) => {
      for (const file of files) {
        vm.runInContext(readFileSync(join(src, file), "utf8"), sandbox, { filename: file });
      }
    };
  } else {
    // Firefox: browser.* is already present and both files are loaded for it.
    sandbox.browser = api;
  }

  const context = vm.createContext(sandbox);
  if (target === "firefox") {
    vm.runInContext(readFileSync(join(src, "host.js"), "utf8"), context, { filename: "host.js" });
  }
  vm.runInContext(readFileSync(join(src, "background.js"), "utf8"), context, { filename: "background.js" });

  // The extension is inert without these two.
  const required = ["downloads.onCreated", "contextMenus.onClicked"];
  const missing = required.filter((name) => !registered.listeners.includes(name));
  if (missing.length > 0) {
    throw new Error(`${target}: never registered ${missing.join(", ")}`);
  }
  if (typeof context.callHost !== "function") {
    throw new Error(`${target}: host.js helpers were not loaded`);
  }

  console.log(`${target.padEnd(8)} ok  (${registered.listeners.length} listeners)`);
}

let failed = false;
for (const target of ["chrome", "firefox"]) {
  try {
    run(target);
  } catch (err) {
    failed = true;
    console.error(`${target.padEnd(8)} FAIL  ${err.message}`);
  }
}
process.exit(failed ? 1 : 0);
