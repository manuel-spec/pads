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
- Turn on **Forward session cookies** for anything behind a login.
- Right-click a link, image, video, or audio element for **Download with PADS**.
- The popup lists the daemon's jobs with live progress and pause/resume buttons.

Files land in the daemon's `download_dir`, `~/Downloads` by default:

```bash
./pads config set download_dir /data/downloads
```

An existing file is never overwritten; PADS adds ` (1)`, ` (2)`, and so on.

## Behaviour worth knowing

**A captured download is paused, not cancelled, until PADS accepts it.** Firefox
creates the download, the extension pauses it immediately, and only once the
daemon confirms the handoff is the paused download discarded. If the daemon is
unreachable the download resumes in Firefox from where it stopped. Neither
outcome loses bytes: handing off first and cancelling afterwards would fetch the
opening bytes twice, and cancelling first would throw the download away when the
daemon turned out to be down.

A download that finishes faster than the pause can land is simply left to
Firefox; the duplicate is bounded by that window.

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
| `manifest.json` | MV3 manifest; the `gecko.id` must match the host manifest's `allowed_extensions` |
| `host.js` | Native-messaging helpers shared by the background script and popup |
| `background.js` | Download capture and the context menu |
| `popup.html` / `popup.css` / `popup.js` | Job list UI |

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
