# Broadwave support

Broadwave is live TV and DVR for your antenna. One person in the house runs the Broadwave server on an always-on computer that runs Docker (a NAS, a mini PC, an Unraid box, or a Docker Desktop computer), with a network tuner such as an HDHomeRun. Everyone else just watches.

## Get started

1. Run the server on Linux:

   ```bash
   docker run -d --name broadwave --network host --restart unless-stopped \
     -e PUID=$(id -u) -e PGID=$(id -g) \
     -v ~/broadwave:/config \
     ghcr.io/wolfebase/broadwave:latest
   ```

   Unraid: add the Broadwave template and leave PUID at 99 and PGID at 100. Docker Desktop on a Mac or Windows PC: publish port 8477 and set `HDHR_HOST` to the tuner's address. The README has every command. [Set up a server](https://wolfebase.github.io/broadwave/docs/server.html) walks through it.
2. Open `http://<server>:8477` and follow setup. It finds an HDHomeRun on your network by itself. When none appears, enter the tuner's address.
3. Watch.

## Getting the apps

- **Web app, now, on every device.** Open `http://<server>:8477` in any browser: phone, tablet, computer, or a TV's browser. Live TV, the guide, recordings, and multiview all work. On a phone, add it to the home screen.
- **iPhone, iPad, and Apple TV apps: coming soon to the App Store.** On the same network they find the server on their own, or you type its address. Until then, use the web app.

## Away from home

1. **Sign in with Cloudflare** (best). In Settings › Devices and away from home, choose Sign in with Cloudflare. You need a free Cloudflare account with a domain on it. The server gets its own address, such as `tv.example.com`, and only devices you pair or invite can open it. Then use Invite someone to send family a code.
2. **Tailscale** (no domain needed). Install Tailscale on the server and on each phone or computer, then open the server's Tailscale address with port 8477.

Never forward port 8477 on your router. [Away from home](https://wolfebase.github.io/broadwave/docs/remote.html) has the steps.

## Common questions

- **The app doesn't find my server.** Make sure the phone or Apple TV is on the same network, and that Local Network access is on (Settings > Privacy & Security > Local Network > Broadwave). Or enter the server's address.
- **Some channels have no guide.** Broadwave reads listings from your tuner's guide, from the broadcast itself, and from any guide source you add in Settings > Guide.
- **Every page asks for a code, and no admin device is left.** Restart the server once with `BROADWAVE_RESET_SIGNIN=1`, pair a device, then remove the setting.

## Contact

Open an issue at https://github.com/wolfebase/broadwave/issues or email twolfekc@gmail.com.

Download a support bundle from Settings › About, or at `http://<server>:8477/api/v1/support`.
