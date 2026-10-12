import type { Message } from "../runtime";

export const legalMessages = {
  "legal.blender":
    "Store art and the demo use Big Buck Bunny, Sintel, Tears of Steel, and Elephants Dream. They are Blender Foundation films under CC BY.",
} as const satisfies Record<string, Message>;
