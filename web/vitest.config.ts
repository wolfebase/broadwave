import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    include: [
      "hook.test.tsx",
      "prime.test.ts",
      "dialogHook.test.tsx",
      "framesHook.test.tsx",
      "layoutHook.test.tsx",
      "screensApi.test.ts",
      "captionsCue.test.ts",
      "captionsFollow.test.ts",
      "startLine.test.ts",
      "glass.test.ts",
      "passLabel.test.ts",
      "scoreLine.test.ts",
      "livePill.test.ts",
      "tvRemote.test.ts",
      "playerChrome.test.ts",
    ],
  },
});
