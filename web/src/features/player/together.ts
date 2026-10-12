import { useEffect, useState } from "react";
import { getGroups } from "../../api";
import { playerUiMessages } from "../../i18n/messages/playerUi.ts";
import { bind } from "../../i18n/runtime.ts";
import { events, type RoomState } from "../../lib/events";

const t = bind(playerUiMessages);

// Watching together is one group per channel, so every screen in the house
// finds it without a code.
export function groupRoom(channelId: number) {
  return `group:ch${channelId}`;
}

/** The watch-together group on a channel, while anyone is in it. */
export function useGroup(channelId: number): RoomState | undefined {
  const [groups, setGroups] = useState<RoomState[]>([]);
  useEffect(() => {
    let live = true;
    // One join sends a burst of changes; only the newest answer counts.
    let asked = 0;
    const load = () => {
      const n = ++asked;
      return getGroups()
        .then((r) => live && n === asked && setGroups(r.groups))
        .catch(() => undefined);
    };
    void load();
    // A burst of joins asks once, after it settles.
    let timer: ReturnType<typeof setTimeout> | undefined;
    const offChanged = events().on("groups.changed", () => {
      clearTimeout(timer);
      timer = setTimeout(() => void load(), 250);
    });
    const offRestart = events().on("restarted", () => void load());
    const offBack = events().on("reconnected", () => void load());
    return () => {
      live = false;
      clearTimeout(timer);
      offChanged();
      offRestart();
      offBack();
    };
  }, []);
  return groups.find((g) => g.room === groupRoom(channelId));
}

function kindName(kind: string): string {
  if (kind === "iphone") return t("player.kind.iphone");
  if (kind === "ipad") return t("player.kind.ipad");
  if (kind === "appletv") return t("player.kind.appletv");
  if (kind === "web") return t("player.kind.browser");
  return "";
}

// A screen that gave no name is named by its kind on the server ("appletv").
export function personLabel(p: { name: string; kind: string }) {
  return (p.name !== p.kind && p.name) || kindName(p.kind) || t("player.anotherScreen");
}

/** "Den TV and Chrome on Mac", "Den TV, Kitchen iPad, and 2 more". */
export function peopleSentence(people: { name: string; kind: string }[]) {
  const names = people.map(personLabel);
  const and = t("player.and");
  if (names.length <= 2) return names.join(` ${and} `);
  if (names.length === 3) return `${names[0]}, ${names[1]}, ${and} ${names[2]}`;
  return `${names[0]}, ${names[1]}, ${and} ${t("player.more", names.length - 2)}`;
}
