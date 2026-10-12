import type { Message } from "../runtime";

export const mvMessages = {
  "mv.side": "Side by side",
  "mv.oneTwo": "One big and two",
  "mv.oneThree": "One big and three",
  "mv.quad": "Quad",
  "mv.pip": "Small over big",
} as const satisfies Record<string, Message>;
