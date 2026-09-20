# PADS browser extension

Hands browser downloads to the PADS daemon instead of the browser's own
downloader. Supports Firefox and the Chromium family: Chrome, Brave, Chromium,
and Edge.

## How it fits together

```text
browser extension  --stdio-->  pads nativehost  --loopback HTTP-->  pads daemon
```

The extension never sees the daemon's address or bearer token. The browser
launches `pads nativehost` as a child process and they exchange length-prefixed
JSON over stdin/stdout; the host reads `~/.pads/daemon.json` and makes the
authenticated loopback call itself.

That indirection is the point. The daemon rejects any request carrying an
`Origin` header precisely so browser-originated traffic cannot reach it, and a
token that lives in a browser profile is a token that leaks with the profile.

## Install

1. Build the binary and the extension:

   ```bash
   make build
   make extension
   ```

   `make extension` writes `dist/extension/chrome` and `dist/extension/firefox`.
   The sources in `src/` are shared; only the manifest differs, because Chromium
   runs the background as a service worker and Firefox as an event page.

2. Register the native-messaging host:

   ```bash
   ./pads nativehost install
   ```

   With no arguments this registers with every supported browser it finds on the
   machine. Name them explicitly with `--browser`:

   ```bash
   ./pads nativehost install --browser brave,firefox
   ```

   Each browser reads host manifests from its own directory:

   | Browser | Linux | macOS |
   | --- | --- | --- |
   | Firefox | `~/.mozilla/native-messaging-hosts/` | `~/Library/Application Support/Mozilla/NativeMessagingHosts/` |
   | Chrome | `~/.config/google-chrome/NativeMessagingHosts/` | `~/Library/Application Support/Google/Chrome/NativeMessagingHosts/` |
   | Brave | `~/.config/BraveSoftware/Brave-Browser/NativeMessagingHosts/` | `~/Library/Application Support/BraveSoftware/Brave-Browser/NativeMessagingHosts/` |
   | Chromium | `~/.config/chromium/NativeMessagingHosts/` | `~/Library/Application Support/Chromium/NativeMessagingHosts/` |
   | Edge | `~/.config/microsoft-edge/NativeMessagingHosts/` | `~/Library/Application Support/Microsoft Edge/NativeMessagingHosts/` |

   Re-run it whenever the `pads` binary moves; the manifest records an absolute
   path. Windows locates the manifest through a registry key and is not handled.

   **Flatpak browsers** keep their configuration under
   `~/.var/app/<app-id>/` and never read the locations above. The installer
   detects them and registers there as well, reporting them as
   `chrome (flatpak)`. A machine with both builds of one browser gets both
   registered, since they share nothing.

   A Flatpak browser is also sandboxed and cannot see the `pads` binary or
   `~/.pads`. The installer prints the grant it needs:

   ```bash
   flatpak override --user \
     --filesystem=/path/to/pads/dir:ro \
     --filesystem=~/.pads:ro \
     com.google.Chrome
   ```

   That is the whole requirement: the host reads the daemon's address and
   token, then reaches it over loopback, which a Flatpak browser already
   shares with the host. Undo it with
   `flatpak override --user --reset <app-id>`.

   The other route, `flatpak-spawn --host`, runs the host outside the sandbox
   and keeps the token out of the browser, but needs
   `--talk-name=org.freedesktop.Flatpak`, which lets the browser run arbitrary
   commands on the host. PADS does not use it.

3. Start a daemon:

   ```bash
   ./pads daemon run
   ```

4. Load the extension.

   **Chrome, Brave, Chromium, Edge** — open `chrome://extensions` (or
   `brave://extensions`), turn on Developer mode, choose **Load unpacked**, and
   pick `dist/extension/chrome`. Note that this is the *built* directory, not
   `extension/`, which holds the shared sources and one manifest per browser
   rather than a loadable `manifest.json`.

   A Flatpak browser can only read directories it has been granted, so it may
   not see `dist/` at all. Either pick the folder through the browser's own
   file chooser, which grants access through the document portal, or copy the
   built directory somewhere already shared, such as `~/Downloads`.

   **Firefox** — open `about:debugging#/runtime/this-firefox`, choose **Load
   Temporary Add-on**, and pick `dist/extension/firefox/manifest.json`. A
   temporary add-on is removed when Firefox closes; a permanent install needs
   signing by Mozilla.

5. Restart the browser so it picks up the newly registered host.

### Why the Chromium manifest pins a key

A host manifest has to name the exact extension ID allowed to connect. Chromium
normally derives that ID from the install path, so an unpacked extension would
get a different ID on every machine and the registration could never match.

The `key` field in `manifest.chrome.json` pins the ID to
`cijdklmepoblhjbjkimmkfiipdkdnini` instead. If you repack under your own key,
pass the resulting ID to the installer:

```bash
./pads nativehost install --browser chrome --chromium-id <your-id>
```

Publishing through the Chrome Web Store replaces the key with the store's own,
which changes the ID again.

## Use

- Downloads started in the browser are handed to PADS automatically. Toggle this off
  from the toolbar popup.
- Turn on **Forward session cookies** for anything behind a login.
- Right-click a link, image, video, or audio element for **Download with PADS**.
- The popup lists the daemon's jobs with live progress and pause/resume buttons.

Files land in the daemon's `download_dir`, `~/Downloads` by default:

```bash
./pads config set download_dir /data/downloads
```

An existing file is never overwritten; PADS adds ` (1)`, ` (2)`, and so on.

## Behaviour worth knowing

**A captured download is paused, not cancelled, until PADS accepts it.** The
browser creates the download, the extension pauses it immediately, and only once
the daemon confirms the handoff is the paused download discarded. If the daemon
is unreachable the download resumes in the browser from where it stopped. Neither
outcome loses bytes: handing off first and cancelling afterwards would fetch the
opening bytes twice, and cancelling first would throw the download away when the
daemon turned out to be down.

A download that finishes faster than the pause can land is simply left to the
browser; the duplicate is bounded by that window.

**Downloads behind a login need the session toggle.** PADS re-fetches the URL
itself, so by default it arrives with no cookies and gets a login page. Turn on
**Forward session cookies** in the popup and the extension sends the download
URL's cookies, the referring page, and the browser's user-agent string along
with the handoff.

That toggle asks for the `cookies` permission and host access for all sites,
because a download can come from anywhere. It is optional and off until you
grant it, so a default install sends no session data anywhere. Turning it off
revokes the permission.

Only six headers can be forwarded: `Cookie`, `Referer`, `User-Agent`,
`Authorization`, `Accept`, and `Accept-Language`. Both the native host and the
daemon enforce that list, and a request naming anything else is refused before a
job exists. The downloader manages `Range` itself, so nothing can reach in and
corrupt segmenting.

Forwarded headers are written to the download's state file so a resumed download
still authenticates. Those files are created `0600`.

**Only `http://` and `https://` URLs are captured.** `blob:` and `data:` URLs
exist only inside the page and there is nothing for the daemon to re-fetch.

## Files

| File | Role |
| --- | --- |
| `manifest.chrome.json` | Chromium manifest: service-worker background, pinned `key` |
| `manifest.firefox.json` | Firefox manifest: event-page background, `gecko.id` |
| `src/host.js` | API-namespace shim and native-messaging helpers, shared by the background script and popup |
| `src/background.js` | Download capture and the context menu |
| `src/popup.html` / `popup.css` / `popup.js` | Job list UI |
| `check.mjs` | Loads each browser's background entry under stub APIs; run with `make extension-check` |

One source tree serves both browsers. Chromium exposes the APIs as `chrome.*`
and Firefox as `browser.*`, so `host.js` normalises on `browser.*` before
anything else runs. `background.js` pulls in `host.js` with `importScripts` when
that exists, which is how a Chromium service worker loads it, and is a no-op in
Firefox where the manifest lists both files.

## Protocol

Requests are `{"type": ...}`; every reply carries `ok`.

| Request | Reply |
| --- | --- |
| `{"type":"health"}` | `{"ok":true,"running":bool,"addr":string}` |
| `{"type":"start","url":...,"filename":...,"headers":{...}}` | `{"ok":true,"id":...,"output":...}` |
| `{"type":"jobs"}` | `{"ok":true,"running":bool,"jobs":[...]}` |
| `{"type":"pause","id":...}` | `{"ok":true,"id":...}` |
| `{"type":"resume","id":...}` | `{"ok":true,"id":...}` |

`filename` is a suggestion from the page and is treated as untrusted: the host
keeps only its base name and confines the result to `download_dir`. `headers` is
checked against the forwardable list above and rejected as a whole if it names
anything else.
