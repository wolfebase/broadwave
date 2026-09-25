// Keep the Apple About text in apple/App/Shared/SportsView.swift in step with this string.
// tsc fails if Blender or CC BY is removed.
export const blenderCredit =
  "Store art and the demo use Big Buck Bunny, Sintel, Tears of Steel, and Elephants Dream. They are Blender Foundation films under CC BY.";

type Contains<Haystack extends string, Needle extends string> = Haystack extends `${string}${Needle}${string}` ? true : false;

export const blenderCreditGuard: Contains<typeof blenderCredit, "Blender"> & Contains<typeof blenderCredit, "CC BY"> = true;
