# Set up a server

Installed the Broadwave app first? The app needs a Broadwave server at home. One person in the house sets it up once. Everyone else just opens the app.

Want to look around first? Choose **Try the demo** in the app. It plays sample films with no server.

## What you need

- **An antenna** and a **network TV tuner**, such as an HDHomeRun. The tuner plugs into the antenna and your router.
- **An always-on computer that runs Docker**: a NAS, a mini PC, an Unraid server, or a computer with Docker Desktop. The server is free.

## 1. Start the server

On a Linux computer or a NAS:

```bash
docker run -d --name broadwave --network host --restart unless-stopped \
  -e PUID=$(id -u) -e PGID=$(id -g) \
  -v ~/broadwave:/config \
  ghcr.io/wolfebase/broadwave:latest
```

On Unraid, add the Broadwave template. On Docker Desktop for Mac or Windows, [Install](install.md) has the command that publishes the port.

## 2. Open setup

In a browser, open `http://<the computer's address>:8477`. Setup finds the tuner, scans the channels, and fills the guide. [First run](first-run.md) shows each screen.

## 3. Open the app

On the same Wi-Fi, the app finds the server on its own. Pick it. If it doesn't show up, type the address from step 2.

Every browser in the house can watch too, at the same address. No install needed.

## Next

- [Away from home](remote.md): watch outside the house with Sign in with Cloudflare, or Tailscale.
- [Install](install.md): graphics, time zone, recordings folder, and updates.
- [Common problems](problems.md): when a tuner or the app doesn't show up.
