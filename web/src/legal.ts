import { legalMessages } from "./i18n/messages/legal.ts";
import { translate } from "./i18n/runtime.ts";

// Keep the Apple About text in apple/App/Shared/SportsView.swift in step with this string.
// tsc fails if Blender or CC BY is removed.
export const blenderCredit = legalMessages["legal.blender"];

export function blenderCreditText(): string {
  return translate(blenderCredit);
}

type Contains<Haystack extends string, Needle extends string> = Haystack extends `${string}${Needle}${string}` ? true : false;

export const blenderCreditGuard: Contains<typeof blenderCredit, "Blender"> & Contains<typeof blenderCredit, "CC BY"> = true;
