# Soulseek Automation for Umbrel

This is a **starter package** for a custom Umbrel store. Tested on Umbrel with slskd, Soularr, and Lidarr. It intentionally does not contain any credentials.

## Install
1. Copy this directory into your custom store repository as `ranciid-soulseek/` (the directory name must match the manifest `id`). Commit and push, then refresh the custom app store in Umbrel.
2. Before installation, confirm the `umbrel_main_network` Docker network exists, and the host path `/home/umbrel/umbrel/Home/Downloads` is correct. Both were seen in your Lidarr configuration.
3. Create the staging folders and app configuration folders on the Umbrel host:
   `sudo mkdir -p /home/umbrel/umbrel/Home/Downloads/{complete,incomplete}`
   Umbrel normally creates app-data folders on installation. After installation, create `data/slskd` and `data/soularr` under `/home/umbrel/umbrel/app-data/ranciid-soulseek/`, if needed, and grant UID 1000 write access.
4. In `data/slskd/slskd.yml`, configure your Soulseek username/password, web authentication, and a unique API key (see https://github.com/slskd/slskd/blob/master/docs/config.md). Do not commit secrets to GitHub. Restart the app after editing.
5. Copy `config.ini.example` into `/home/umbrel/umbrel/app-data/ranciid-soulseek/data/soularr/config.ini`, then insert your Lidarr API key and the slskd API key. Keep this file private.
6. Open slskd at `http://192.168.1.11:5030` and Soularr at `http://192.168.1.11:8265` (local LAN). Restrict remote access to Tailscale; these web ports are not authenticated by Umbrel's app proxy in this starter compose. Check Soularr UI access controls before exposing it to other networks.
7. In Lidarr, use `/media/Music` as the library root. Its `/downloads` mount points to the same host directory as slskd. Soularr's `[Lidarr] download_dir` must be `/downloads/complete`.

## Important
- Soularr has a web UI but may start before credentials are configured, resulting in errors until configuration is complete.
- slskd web login, Soulseek account, and API key must be configured. The default UI credentials must not be relied upon.
- `50300/tcp` is exposed for Soulseek peers, but router port forwarding is optional and may improve inbound connectivity. Do **not** forward the web UI ports `5030` or `8265`.
- Image tags use `latest` and are not pinned. Pin tested versions for long-term stability.
- The shared staging folder is on Umbrel Home/Downloads, not your Seagate drive. Large downloads will consume space on that filesystem. You can relocate it later by updating all mounts and the corresponding Lidarr paths together.
- Download only material you have rights to obtain and share.
