# Guideline 2.1 reply (tvOS 1.0)

Apple asked for information (Guideline 2.1, limited account history), not a defect. The owner pastes the reply below into the Resolution Center and records the videos. Do not send the reply from here.

Version: tvOS 1.0, submission `7946ed56-284d-4a5c-9fd3-3d8cfa421c96`. iOS 1.0 is still waiting for review.

## Reply

Thank you for reviewing Broadwave. Here is the information you asked for.

Purpose and audience. Broadwave is for people who watch free over-the-air television with their own antenna and a tuner, such as an HDHomeRun. The iPhone, iPad, and Apple TV apps are the player for a server the user runs at home. The app hosts and provides no content. It plays the broadcasts the user's tuner receives, the way a television does. There are no accounts, no user-generated content, and no paid features.

Setup. The user runs the server (Docker, Unraid, or a Mac) on their network. The app finds it with Bonjour, or the user types an address. The app asks for no credentials. A tuner on the same network is found automatically. A Schedules Direct account or an XMLTV link is optional, and it is entered on the server only if the user wants a longer guide. No sample files are required.

To try it: on a Mac with Docker, run `docker run -p 8477:8477 ghcr.io/wolfebase/broadwave:latest`, open http://localhost:8477, add a playlist under Settings > Sources > M3U URL, then open the app and enter the Mac's address. The videos already attached show Home, the guide, live playback, and two channels side by side. Those demo channels are Blender Foundation open movies (CC BY), which is also what the screenshots show. We can provide a reachable test server on request.

External services. The apps talk only to the user's own Broadwave server. That server, on the user's machine, may contact:

- the user's tuner on the local network;
- a guide the user turns on: the SiliconDust guide that comes with an HDHomeRun, Schedules Direct, or an XMLTV address the user adds;
- public ESPN scoreboards for NFL, college football, NBA, WNBA, college basketball, MLB, NHL, MLS, NWSL, and the Premier League, plus schedules for F1 and NASCAR;
- a playlist or channel source the user adds.

There are no authentication, payment, analytics, or AI services. Wolfe Up LLC receives none of this data. The privacy policy is in the repository at PRIVACY.md: the app does not collect data.

Regional differences. The app is listed in the United States and Canada. It works the same in both. Channels depend on the broadcasters the user's antenna can receive (ATSC). Nothing in the app changes by country.

Third-party material. The app does not supply broadcast television, team logos, or other programming. What is on screen is the signal from the user's antenna, or a source the user added. Screenshots and the attached videos use Blender Foundation open movies under CC BY, not broadcast television. Sports scores come from a public scoreboard so a game the user is already receiving can be labeled. The app does not stream those games from the internet.

A screen recording from a physical device, starting at launch, will follow. The path is below.

## Shot list

Record on a physical iPhone and a physical Apple TV, on the latest system, from the moment the app opens. One take each is enough if it stays on this path.

1. Launch from the Home Screen.
2. First run finds the server on the local network.
3. Home.
4. The guide. Open one program.
5. A live channel, playing.
6. The stream panel (what is being sent, and how far behind live).
7. Schedule a recording.
8. Play a recording.
9. Two channels side by side.
10. Settings.

How to record:

- iPhone: Control Center, then the screen recording button. Stop it from Control Center or the red status bar. AirDrop or save the video, then attach it in App Store Connect.
- Apple TV: connect the Apple TV to the Mac with a USB or network pair in Xcode (Window > Devices and Simulators). In QuickTime Player, choose File > New Movie Recording, then the Apple TV as the camera. Record the path above.

## Review notes

The same facts, kept short enough for the 4000-character App Review Notes field, are in `review-notes.txt` next to this file. Those notes were submitted for tvOS. The iOS result is recorded in `docs/plan/BLOCKERS.md`.
