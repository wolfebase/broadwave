# Broadwave support

Broadwave is live TV and DVR for your antenna. You run the Broadwave server at home (Docker, Unraid, or a Mac) with a tuner such as an HDHomeRun, then watch on iPhone, iPad, Apple TV, or any web browser.

## Get started

1. Run the server: `docker run -d --name Broadwave --network host -v /path/to/config:/config ghcr.io/wolfebase/broadwave:latest` (Unraid: add the Broadwave template). Host networking lets it find your tuner.
2. Open `http://<server>:8477` and follow setup. It finds an HDHomeRun on your network by itself.
3. Open the Broadwave app on the same network. It finds the server on its own. You can also enter the address.

## Common questions

- **The app doesn't find my server.** Make sure the phone or Apple TV is on the same network, and that Local Network access is on (Settings > Privacy & Security > Local Network > Broadwave). Or enter the server's address.
- **Some channels have no guide.** Broadwave reads listings from your tuner's guide, from the broadcast itself, and from any guide source you add in Settings > Guide.
- **Can I watch away from home?** Yes, over a VPN such as Tailscale. Enter the server's VPN address in the app.

## Contact

Open an issue at https://github.com/wolfebase/broadwave/issues or email twolfekc@gmail.com.
