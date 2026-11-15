# Submitting Broadwave to Unraid Community Applications

What has to happen, by a person, to list Broadwave in the Unraid Apps tab. Checked against the submission site on 2026-11-14.

## How Community Applications works now

- Submissions go through **https://ca.unraid.net/submit**. Unraid's own docs (https://docs.unraid.net/community-applications/) name that site as the source of truth, and its help pages (https://ca.unraid.net/submit/help) are the rules of record.
- The site scans a **public GitHub repository** that has an OSI license and a `ca_profile.xml` at its root, and one XML file per app. Each template's `TemplateURL` must be the raw URL of that exact file. **Validate** and **Scan** run in the submit flow; submit once every scan check passes. Approved apps appear after the next catalog build.
- Policy (https://forums.unraid.net/topic/87144-ca-application-policies/): the GitHub account and the image registry must have **two-factor authentication** on; an app the maintainer made must be open source; no two templates may use the same image; no animated icons; a template that tries to run extra commands is blacklisted with the whole repository.
- A forum support thread is not required by the site, but Unraid users look for one, and `ca_profile.xml` has a `<Forum>` field for it.

## What is ready in this repository

| File | Purpose |
| --- | --- |
| `deploy/unraid/ca/` | The template repository, file for file: `ca_profile.xml`, `LICENSE` (Apache 2.0), `README.md`, `icon.svg`, and `templates/broadwave.xml`. |
| `deploy/unraid/ca/templates/broadwave.xml` | The listed template. It is `deploy/unraid/broadwave.xml` with one change: its `TemplateURL` is `https://raw.githubusercontent.com/wolfebase/unraid-templates/main/templates/broadwave.xml`. `go test ./...` in `docs/sitegen` fails when the two drift apart. |
| `deploy/unraid/broadwave.xml` | The same template for adding by hand. Keep editing this one; then copy it (step 6). |
| `deploy/unraid/broadwave-updater.xml` | The optional updater. **Not submitted**: Community Apps already lists `nickfedor/watchtower` (the duplicate-image rule), and it mounts the Docker socket. It stays a by-hand template. |
| `deploy/unraid/support-thread.md` | The support thread post, ready to paste. |

Why a separate repository: the scan parses the template XML files in the repository it is given, and this one also carries the updater template, which must not be listed. A template repository also keeps `ca_profile.xml` out of the server's root, and can hold more Wolfe Up templates later. The scan's identity check wants each `TemplateURL` to be the raw URL of the file it scanned, so the listed copy points at `wolfebase/unraid-templates`.

## Steps

### 1. Check two-factor authentication

On the `wolfebase` GitHub account: https://github.com/settings/security. Two-factor authentication must be on. The image is on GHCR under the same account, so that covers the registry too. (A command-line check could not read this setting.)

### 2. Make the template repository

Create a **public** repository named `unraid-templates` under `wolfebase`, with no README, license, or .gitignore. Then, from a checkout of `main` that has `deploy/unraid/ca/`:

```bash
tmp=$(mktemp -d)
cp -R deploy/unraid/ca/. "$tmp/"
cd "$tmp"
git init -b main
git add .
git commit -m "Broadwave template for Community Applications"
git remote add origin https://github.com/wolfebase/unraid-templates.git
git push -u origin main
```

Check that these open in a browser:

- https://raw.githubusercontent.com/wolfebase/unraid-templates/main/templates/broadwave.xml
- https://raw.githubusercontent.com/wolfebase/unraid-templates/main/ca_profile.xml

### 3. Post the support thread

1. Sign in to https://forums.unraid.net (make an account if needed).
2. Open **Docker Containers**: https://forums.unraid.net/forum/47-docker-containers/ and choose **Start new topic**.
3. Title: `[Support] Broadwave - live TV and DVR for an antenna`
4. Body: everything between the two `---` lines in `deploy/unraid/support-thread.md`. The forum editor takes the Markdown-style bold and lists as plain text; fix any formatting it drops.
5. Post it and copy the thread's address, such as `https://forums.unraid.net/topic/123456-support-broadwave-live-tv-and-dvr-for-an-antenna/`.

### 4. Point Support at the thread

In this repository, with the thread address in `THREAD`:

```bash
THREAD='https://forums.unraid.net/topic/123456-support-broadwave-live-tv-and-dvr-for-an-antenna/'
sed -i '' "s|<Support>https://github.com/wolfebase/broadwave/issues</Support>|<Support>$THREAD</Support>|" \
  deploy/unraid/broadwave.xml deploy/unraid/ca/templates/broadwave.xml
sed -i '' "s|  <!-- Add once the Unraid forum support thread exists: <Forum>.*</Forum> -->|  <Forum>$THREAD</Forum>|" \
  deploy/unraid/ca/ca_profile.xml
(cd docs/sitegen && go test ./...)
```

(`sed -i ''` is the macOS form; on Linux use `sed -i`.) Commit, push to `main`, and copy the changed files into `wolfebase/unraid-templates` (step 6). The updater template keeps GitHub issues as its Support.

### 5. Submit

1. Open https://ca.unraid.net/submit/new and sign in as the site asks.
2. Repository: `https://github.com/wolfebase/unraid-templates` (branch `main`).
3. Run **Validate**, then **Scan**. Expect one Docker app, **Broadwave**, from `templates/broadwave.xml`, with the profile from `ca_profile.xml`. If a check fails, fix it in `deploy/unraid/` here, copy it over (step 6), push, and scan again.
4. Where the form asks, use these values:

| Field | Value |
| --- | --- |
| App name | Broadwave |
| Image | `ghcr.io/wolfebase/broadwave:latest` (public, linux/amd64 and linux/arm64) |
| Category | Media Applications: Video, Media Servers: Video (`MediaApp:Video MediaServer:Video`) |
| Project | https://github.com/wolfebase/broadwave |
| Support | the forum thread from step 3 |
| License | Apache-2.0 |
| Icon | https://raw.githubusercontent.com/wolfebase/broadwave/main/web/public/icons/icon-512.png (512 × 512 PNG, no transparency) |
| Template URL | https://raw.githubusercontent.com/wolfebase/unraid-templates/main/templates/broadwave.xml |
| Description | the template's Overview |
| Contact | the address in `SUPPORT.md` |

The form itself is behind the site's sign-in, so its exact field names could not be read in advance. The scan fills most of these from the XML.

5. Submit for review once every scan check passes.

### 6. Keep the template repository in step

Whenever `deploy/unraid/broadwave.xml` changes:

```bash
sed 's|<TemplateURL>https://raw.githubusercontent.com/wolfebase/broadwave/main/deploy/unraid/broadwave.xml</TemplateURL>|<TemplateURL>https://raw.githubusercontent.com/wolfebase/unraid-templates/main/templates/broadwave.xml</TemplateURL>|' \
  deploy/unraid/broadwave.xml > deploy/unraid/ca/templates/broadwave.xml
(cd docs/sitegen && go test ./...)
```

Commit, then copy `deploy/unraid/ca/` over a checkout of `wolfebase/unraid-templates` and push. Community Apps picks the change up on its next catalog build.

### 7. After it is listed

- `docs/guide/install.md` (Unraid section and the table at the top), `deploy/unraid/support-thread.md`, `site/index.html`, and the README install table: say "Search for Broadwave in the Apps tab" first, and keep the by-hand `wget` as the fallback. Remove "Broadwave isn't in Community Applications yet."
- Edit the forum post the same way.
- `scripts/install.sh` points Unraid users at `docs/guide/install.md#unraid`, so it needs no change.
