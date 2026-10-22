/** What the player says while a channel starts, by how long it has run (ms). */
const tuneSteps = [
  { at: 0, id: "player.tuneAntenna" },
  { at: 3000, id: "player.tunePicture" },
  { at: 7000, id: "player.tuneAlmost" },
  { at: 18000, id: "player.tuneWeak" },
] as const;

/** The words for a start that has run this long. Lining up is said only while Line up automatically is on, as on the Apple apps. */
export function tuneStepId(elapsed: number, lining: boolean) {
  const step = [...tuneSteps].reverse().find((s) => elapsed >= s.at) ?? tuneSteps[0];
  return step.id === "player.tuneAlmost" && lining ? "player.tuneLive" : step.id;
}

