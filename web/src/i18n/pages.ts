/** Routes the shell can show. The pseudo-locale test renders each one. */
export const appPages = [
  { name: "Home", path: "/" },
  { name: "Guide", path: "/guide" },
  { name: "Search", path: "/search" },
  { name: "Sports", path: "/sports" },
  { name: "Recordings", path: "/recordings" },
  // One title is its own page: Library renders seasons instead of the show grid.
  { name: "Show", path: "/recordings?show=Jeopardy%21" },
  { name: "Library", path: "/library" },
  { name: "Schedule", path: "/schedule" },
  { name: "Settings", path: "/settings" },
  { name: "Sources", path: "/sources" },
  { name: "About", path: "/about" },
  { name: "Setup", path: "/setup" },
  { name: "Diagnostics", path: "/diagnostics" },
  { name: "Pair", path: "/pair" },
  { name: "Play", path: "/play?recording=1" },
  { name: "Watch", path: "/watch?channel=1" },
  { name: "Virtual", path: "/watch?virtual=1" },
  { name: "Multiview", path: "/multiview" },
] as const;
