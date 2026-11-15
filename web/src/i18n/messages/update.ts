import type { Message } from "../runtime";

// How to update. Loads with the update notice's steps, not with the shell.
export const updateMessages = {
  "update.lede": "Settings, recordings, and paired devices stay. Pick the way the server was installed.",
  "update.ledeKnown": "Settings, recordings, and paired devices stay.",
  "update.unraid.label": "Unraid",
  "update.unraid.text": "Docker tab, Check for Updates, then Apply Update on Broadwave.",
  "update.docker.label": "Docker",
  "update.docker.text": "Pull the new image and recreate the container with the same folders: docker compose pull && docker compose up -d (or docker pull ghcr.io/wolfebase/broadwave:latest, then remove and run the container again).",
  "update.updater.label": "With the Broadwave updater",
  "update.updater.text": "Nothing to do. It updates at 03:30, on a night when nothing is recording.",
  "update.binary.label": "Binary",
  "update.binary.text": "Download the new release for your system from the releases page, stop Broadwave, replace the file, and start it again.",
} as const satisfies Record<string, Message>;
