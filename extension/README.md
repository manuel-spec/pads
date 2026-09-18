# PADS Firefox extension

Hands Firefox downloads to the PADS daemon instead of Firefox's own downloader.

## How it fits together

```text
Firefox extension  --stdio-->  pads nativehost  --loopback HTTP-->  pads daemon
```

The extension never sees the daemon's address or bearer token. Firefox launches
`pads nativehost` as a child process and they exchange length-prefixed JSON over
stdin/stdout; the host reads `~/.pads/daemon.json` and makes the authenticated
loopback call itself.

That indirection is the point. The daemon rejects any request carrying an
`Origin` header precisely so browser-originated traffic cannot reach it, and a
token that lives in a browser profile is a token that leaks with the profile.

## Install

1. Build and register the host:

   ```bash
   go build -o pads .
   ./pads nativehost install
   ```

   This writes `~/.mozilla/native-messaging-hosts/pads.json` (macOS:
   `~/Library/Application Support/Mozilla/NativeMessagingHosts/`) and a small
   wrapper script in `~/.pads/`. Firefox runs a manifest's `path` with no
   arguments, which is the only reason the wrapper exists.

   Re-run it whenever the `pads` binary moves; the manifest records an absolute
   path.

2. Start a daemon:

   ```bash
   ./pads daemon run
   ```

3. Load the extension. For development, open `about:debugging#/runtime/this-firefox`,
   choose **Load Temporary Add-on**, and pick `extension/firefox/manifest.json`.
   A temporary add-on is removed when Firefox closes. For a permanent install the
   extension needs signing by Mozilla.

4. Restart Firefox so it picks up the newly registered host.

## Use

- Downloads started in Firefox are handed to PADS automatically. Toggle this off
  from the toolbar popup.
- Right-click a link, image, video, or audio element for **Download with PADS**.
- The popup lists the daemon's jobs with live progress and pause/resume buttons.

Files land in the daemon's `download_dir`, `~/Downloads` by default:

```bash
./pads config set download_dir /data/downloads
```

An existing file is never overwritten; PADS adds ` (1)`, ` (2)`, and so on.

## Behaviour worth knowing

**The handoff happens before Firefox's download is cancelled.** If the daemon is
down, Firefox simply keeps downloading and you get a notification. The cost is a
few duplicated bytes; the alternative — cancel first, then discover PADS is
unreachable — loses the download.

**Authenticated downloads will not work.** PADS re-fetches the URL from scratch
with no cookies, `Authorization` header, or `Referer` from your session, so
anything behind a login fails or returns an HTML error page. Turn capture off
for those, or fetch them with Firefox. Forwarding request headers would fix this
and the daemon API has no field for them yet.

**Only `http://` and `https://` URLs are captured.** `blob:` and `data:` URLs
exist only inside the page and there is nothing for the daemon to re-fetch.

## Files

| File | Role |
| --- | --- |
| `manifest.json` | MV3 manifest; the `gecko.id` must match the host manifest's `allowed_extensions` |
| `host.js` | Native-messaging helpers shared by the background script and popup |
| `background.js` | Download capture and the context menu |
| `popup.html` / `popup.css` / `popup.js` | Job list UI |

## Protocol

Requests are `{"type": ...}`; every reply carries `ok`.

| Request | Reply |
| --- | --- |
| `{"type":"health"}` | `{"ok":true,"running":bool,"addr":string}` |
| `{"type":"start","url":...,"filename":...}` | `{"ok":true,"id":...,"output":...}` |
| `{"type":"jobs"}` | `{"ok":true,"running":bool,"jobs":[...]}` |
| `{"type":"pause","id":...}` | `{"ok":true,"id":...}` |
| `{"type":"resume","id":...}` | `{"ok":true,"id":...}` |

`filename` is a suggestion from the page and is treated as untrusted: the host
keeps only its base name and confines the result to `download_dir`.
