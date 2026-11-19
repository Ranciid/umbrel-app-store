# Bindery for Ranciid's Ports (Umbrel)

A native Umbrel community-store package for the Kindle-enabled edition of [Bindery](https://github.com/vavallee/bindery), an ebook and audiobook library automation service and Readarr alternative.

## Included

- `umbrel-app.yml`: app-store listing, gallery, permissions, and launch details.
- `docker-compose.yml`: Custom Bindery v1.52.0 + Kindle integration, the native Umbrel app proxy, a one-shot permission initializer, persistent configuration, shared storage, and networking to installed Umbrel services.
- `icon.svg`: bundled store artwork.
- `README.md`: integration, troubleshooting, and storage guidance.

This installs **Bindery only**, rather than duplicating existing Prowlarr, SABnzbd or Audiobookshelf installations. No API keys or passwords are included.

## Install

1. Add `https://github.com/Ranciid/umbrel-app-store` as a custom app store in Umbrel if it is not already added.
2. After this directory is merged into the repo's `main` branch, refresh the custom store.
3. Install **Bindery** from the store and launch its tile. You can also open `http://<your-umbrel-ip>:8787` on your local network.
4. Complete the first-run page to create a Bindery administrator account. There are **no default credentials**.
5. Configure an indexer and a download client in Bindery's **Settings** before searching.

Umbrel injects `APP_DATA_DIR` and `UMBREL_ROOT` during an app-store install. A standalone `docker compose up` from a shell requires setting these yourself and does not supply Umbrel's `app_proxy` service.

## First-run permissions and upgrading from the original package

Umbrel can create new Docker bind-mount directories owned by `root:root` with permissions `0755`. Because Bindery runs as UID/GID `1000:1000`, this causes a crash loop with `database parent directory "/config" is not writable`.

**Fixed in Umbrel packaging revision 1.52.1** (the Bindery application itself remains pinned to upstream v1.52.0):

- Before Bindery starts, a root-owned, one-shot Alpine init container creates `/config`, `/downloads/books`, and `/downloads/audiobooks`.
- It assigns UID/GID `1000:1000` to the private `/config` tree and **only** the two shared library root directories.
- It does **not** recursively change ownership of the entire shared `/downloads` tree or existing book/audiobook content.
- The application server starts only after the initialization container successfully completes and continues running non-root.

**Existing installations:** After merging this update, refresh the custom app store in Umbrel and update Bindery to package version 1.52.1. A data-preserving app update should rerun the initializer automatically. Do not uninstall the app simply to fix permissions, since uninstalling may remove its private configuration.

If Umbrel does not offer an update, fix your currently installed volume directly on the Umbrel host. The following commands act only on the already-created Bindery private directory and the two book library roots (not the rest of Downloads):

```bash
ROOT=/home/umbrel/umbrel
sudo mkdir -p "$ROOT/app-data/ranciid-bindery/data/config" \
  "$ROOT/data/storage/downloads/books" \
  "$ROOT/data/storage/downloads/audiobooks"
sudo chown -R 1000:1000 "$ROOT/app-data/ranciid-bindery/data/config"
sudo chown 1000:1000 \
  "$ROOT/data/storage/downloads/books" \
  "$ROOT/data/storage/downloads/audiobooks"
sudo docker restart ranciid-bindery_server_1
sudo docker logs --tail=40 ranciid-bindery_server_1
```

**Verify host mount paths before running this workaround** on customized, migrated, or external-drive installs. Run `sudo docker inspect ranciid-bindery_server_1 --format '{{range .Mounts}}{{println .Destination "=>" .Source}}{{end}}'` and compare the `/config` and `/downloads` sources. The commands above assume the conventional `/home/umbrel/umbrel` root.

## Shared storage

This package uses the current official Umbrel shared Downloads mount:

| Path in Bindery | Host path (relative to `UMBREL_ROOT`) | Purpose |
| --- | --- | --- |
| `/config` | App-specific `app-data/ranciid-bindery/data/config` | SQLite DB, cache, backups |
| `/downloads` | `data/storage/downloads` | Existing downloads and shared storage |
| `/downloads/books` | `data/storage/downloads/books` | Imported ebooks |
| `/downloads/audiobooks` | `data/storage/downloads/audiobooks` | Imported audiobooks |

Bindery and its import targets live under **one bind mount** so importing can hardlink files on the same filesystem. Actual downloads are tracked through the download client's API; Bindery does not blindly import everything that happens to land in `/downloads`.

If your install still uses the **legacy** `/home/umbrel/umbrel/Home/Downloads` tree, or if your SABnzbd downloads are stored on a separate external drive, update the shared-volume source in this package before installing. A path with similar spelling is *not* necessarily the same storage. Completed files must be visible in Bindery at the same paths that the download client reports, or you must configure a remote path mapping in Bindery under **Settings → Download clients**. For current official Umbrel SABnzbd installs, both containers use `/downloads`, so no remap is normally needed.

## Native Send to Kindle (Ranciid edition)

This Umbrel package uses a custom build of **Bindery v1.52.0**, plus a source-level native Kindle integration. It is still **one Bindery container**, not a second sidecar app.

- Open **Settings → Kindle** in Bindery (admin).
- Enter your verified SMTP provider settings, e.g. SMTP2GO host `mail.smtp2go.com`, port `587` (STARTTLS), generated SMTP username/password, a verified sending email, and one or more `@kindle.com` recipients.
- Ensure Amazon **Personal Document Settings → Approved Personal Document E-mail List** includes the exact From address.
- **Save**, then use **Send SMTP test to From address**. This tests the mail server; the Kindle provider does not acknowledge receipt of arbitrary documents.
- For a one-time send, open a Bindery book page and click **Send to Kindle**.
- To send future imports automatically, enable **Automatically send newly imported EPUB/PDF books**. This starts from the next book file registered after you enable it, so existing books are **not** bulk-emailed.

**Avoid the old CWA duplicate loop:** Change **General → File Naming → Ebook import mode** from `External` to `Auto`, clear the External Drop folder and Calibre-tab CWA ingest path, set Calibre **Write integration** to `Off`, and disable Calibre library import/sync if you no longer use Calibre. Bindery then writes the final ebook under `/downloads/books`. Keep Audiobook import mode `Auto` as appropriate. Do not delete your original Calibre library until you confirm the Kindle setup works.

**Security and delivery guarantees:**

- The SMTP password lives in Bindery's existing private SQLite settings storage, which is **plaintext at rest**, accessible to the Bindery process and anyone with filesystem/database access. It is never included in API responses or this GitHub repository. Use a dedicated SMTP2GO credential.
- The Kindle API is protected by Bindery's **administrator authentication and CSRF middleware**. Only `@kindle.com` addresses can be recipients. SMTP always uses verified TLS (port 587 STARTTLS or 465 implicit TLS).
- The sender accepts **EPUB and PDF** files only and caps each attachment at 24 MiB. Other formats need conversion outside this feature.
- Each file+recipient receives one persisted send attempt. An SMTP accepted result is **not** confirmation of Amazon processing, and failures/uncertain attempts are not automatically resent. Use the book's **Send to Kindle** button to explicitly confirm a retry, preventing inadvertent duplicates.
- The new DB table `kindle_sends` stores send status and history. It is in the same private `/config/bindery.db` volume and is included in normal DB backups.
- Restarting Bindery does not resend previous files automatically.
- This feature is **off by default** for everyone using the public app store.

### Custom image build and publication

The source-level changes live under [`bindery-kindle/`](https://github.com/Ranciid/umbrel-app-store/tree/main/bindery-kindle) alongside a strict patch tool. GitHub Actions clones upstream Bindery **v1.52.0**, applies the overlay, runs Go tests, and builds both AMD64 and ARM64 images. After the PR merges and its `Build native Bindery Kindle image` workflow succeeds, the image is published at:

`ghcr.io/ranciid/bindery-kindle:1.52.0-kindle.1`

**The first release may need a GitHub Packages visibility change to Public** so all Umbrel users can pull it without signing in. Go to the GHCR package settings under the Ranciid GitHub profile and check its visibility. Do not merge a manifest pointing to an unpublished/private package before confirming the build.

This is a maintained downstream variant of Bindery, not an upstream official Kindle feature. Future upstream versions require rebasing the overlay and rerunning tests before updating the image.

## Connect Prowlarr

1. In Prowlarr, find its API key in **Settings → General**.
2. In Bindery, open **Settings → Indexers → Add Prowlarr**.
3. URL: `http://prowlarr_server_1:9696` (existing Umbrel container on `umbrel_main_network`).
4. Paste the Prowlarr API key, save and run **Sync now**, then test the imported indexers.
5. Ensure your indexers include book categories (usually **7020** for ebooks and **3030** for audiobooks); support varies by indexer.

You can add Newznab or Torznab indexers directly in Bindery instead. Bindery does not require Prowlarr.

## Connect SABnzbd

1. Open SABnzbd and create a download category called `books`, if it does not already exist. Bindery **does not create categories** in SABnzbd.
2. Copy the API key from SABnzbd **Config → General**.
3. In Bindery open **Settings → Download Clients → Add Client → SABnzbd**.
4. Host: `sabnzbd_web_1`, port: `8080`, SSL: **off** (internal Docker network). Enter your API key and set the category to `books`.
5. Save and select **Test**. Configure a separate audiobook category if desired.

If SABnzbd uses a nonstandard destination outside Umbrel's shared Downloads, mount that location and set a download-client path remap. Bindery must be able to read the *completed* files, not just call SABnzbd's API.

## Audiobookshelf integration

The official Umbrel Audiobookshelf app mounts the same host folder, `data/storage/downloads/audiobooks`, as `/audiobooks` inside Audiobookshelf.

- In Audiobookshelf, create or point your audiobook library at `/audiobooks`.
- Bindery imports audiobook files into `/downloads/audiobooks`.
- If using Bindery's Audiobookshelf integration, its container URL on Umbrel's shared network is `http://audiobookshelf_web_1:80`; configure its credentials/token through Bindery's settings. Test the integration.

For ebooks, you can use Bindery's OPDS catalogue or point a separate ebook reader/library app at the host `data/storage/downloads/books` folder.

## Security, operations and maintenance

- **Direct TCP port 8787 is exposed to your LAN** to enable OPDS and API access without Umbrel's app-tile proxy. Bindery handles its own accounts, but its first-run setup page is available until you create an admin. Complete setup promptly.
- **Do not** port-forward 8787 to the Internet. For remote access, prefer Tailscale or an authenticated reverse proxy.
- API credentials entered in Bindery are stored in its private configuration volume. Do **not** commit secrets to this repository.
- For OpenLibrary metadata rate limits, optionally set `BINDERY_CONTACT` to your own contact address in the container environment, following [upstream instructions](https://github.com/vavallee/bindery/blob/main/docs/QUICKSTART.md).
- The Docker image is **pinned to `1.52.0-kindle.1`**, built from upstream v1.52.0 and the reviewed patch, rather than `latest`, so Umbrel updates will be intentional and reviewable. Read [Bindery release notes](https://github.com/vavallee/bindery/releases) before changing the image tag. The manifest version can also be incremented separately for an Umbrel packaging-only fix.
- Back up both the private `data/config` directory and shared `books` / `audiobooks` directories. Removing the app may remove private app data; shared storage is separate.

To check the container on Umbrel, use `sudo docker logs --tail=100 ranciid-bindery_server_1`. Bindery's upstream image includes a built-in Docker health check; there is no shell inside its distroless container.

Upstream guides: [Quickstart](https://github.com/vavallee/bindery/blob/main/docs/QUICKSTART.md), [Deployment and path remapping](https://github.com/vavallee/bindery/blob/main/docs/DEPLOYMENT.md).

Only download and share books for which you have the necessary rights.
