import { afterEach, beforeEach, describe, expect, test, vi, type Mock } from "vitest";

// events() opens a WebSocket. This stand-in does not.
// browserName() here is not the real user-agent check: no argument yields "Browser",
// and any user agent yields "Custom". saveScreenName calls it with no arguments
// when the cleaned name is blank.
vi.mock("./src/lib/events.ts", () => {
  const here = vi.fn();
  return {
    browserName: (ua?: string) => (ua ? "Custom" : "Browser"),
    events: () => ({ here }),
    __here: here,
  };
});

import { cleanScreenName, saveScreenName } from "./src/features/settings/screenName.ts";
import * as eventsModule from "./src/lib/events.ts";

const here = (eventsModule as unknown as { __here: Mock }).__here;
const { browserName } = eventsModule;
const key = "broadwave-screen-name";

const restoreStorage: Array<() => void> = [];

function blockStorage() {
  const fail = () => {
    throw new Error("storage blocked");
  };
  const setItem = vi.spyOn(Storage.prototype, "setItem").mockImplementation(fail);
  const removeItem = vi.spyOn(Storage.prototype, "removeItem").mockImplementation(fail);
  restoreStorage.push(() => {
    removeItem.mockRestore();
    setItem.mockRestore();
  });
  return { setItem, removeItem };
}

describe("cleanScreenName", () => {
  test("trims the ends and drops control characters, including DEL", () => {
    expect(cleanScreenName("  Den TV  ")).toBe("Den TV");
    expect(cleanScreenName("\tDen TV\n")).toBe("Den TV");
    expect(cleanScreenName("Den\u0001 TV")).toBe("Den TV");
    expect(cleanScreenName("Den\u001f TV")).toBe("Den TV");
    expect(cleanScreenName("Den\u0020TV")).toBe("Den TV");
    expect(cleanScreenName("City\u007e News")).toBe("City~ News");
    expect(cleanScreenName("City\u007f News")).toBe("City News");
    expect(cleanScreenName("City\u0080 News")).toBe("City\u0080 News");
    expect(cleanScreenName("\u0001  City News  \u0002")).toBe("City News");
    expect(cleanScreenName("")).toBe("");
    expect(cleanScreenName(" \n\t ")).toBe("");
    expect(cleanScreenName("\u0000\u001f\u007f")).toBe("");
    expect(cleanScreenName("\u0001   \u0002")).toBe("");
  });

  test("keeps at most 40 characters and trims again after the cut", () => {
    const capped = `${"City News".repeat(4)}City`;
    expect(capped).toHaveLength(40);
    expect(cleanScreenName(`${capped} News`)).toBe(capped);
    expect(cleanScreenName(`  ${capped} News  `)).toBe(capped);
    expect(cleanScreenName(`\u0001${capped}\u007fX`)).toBe(capped);

    const almost = `${"City News".repeat(4)}Cit`;
    expect(almost).toHaveLength(39);
    // The 40th character is a space, then further text is dropped, then that space is trimmed.
    expect(cleanScreenName(`${almost} Den TV`)).toBe(almost);
  });
});

describe("saveScreenName", () => {
  beforeEach(() => {
    localStorage.clear();
    here.mockClear();
  });

  afterEach(() => {
    for (const restore of restoreStorage.splice(0).reverse()) restore();
    localStorage.clear();
    here.mockClear();
  });

  test("a non-blank cleaned name is stored under the screen key and returned as itself", () => {
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    const removeItem = vi.spyOn(Storage.prototype, "removeItem");
    restoreStorage.push(() => {
      removeItem.mockRestore();
      setItem.mockRestore();
    });

    const saved = saveScreenName("  \u0001Den TV\u007f  ");
    expect(saved).toBe("Den TV");
    expect(saved).not.toBe(browserName());
    expect(saved).not.toBe(browserName("City News"));
    expect(setItem).toHaveBeenCalledTimes(1);
    expect(setItem).toHaveBeenCalledWith(key, "Den TV");
    expect(removeItem).not.toHaveBeenCalled();
    expect(localStorage.getItem(key)).toBe("Den TV");
    expect(here).toHaveBeenCalledTimes(1);
    expect(here).toHaveBeenCalledWith();
  });

  test("a name past 40 characters is stored at the cap", () => {
    const capped = `${"City News".repeat(4)}City`;
    expect(saveScreenName(`\n${capped} News\t`)).toBe(capped);
    expect(capped).not.toBe(browserName());
    expect(localStorage.getItem(key)).toBe(capped);
    expect(here).toHaveBeenCalledTimes(1);
    expect(here).toHaveBeenCalledWith();
  });

  test("a new name replaces the stored one", () => {
    expect(saveScreenName("Den TV")).toBe("Den TV");
    expect(saveScreenName("City News")).toBe("City News");
    expect(localStorage.getItem(key)).toBe("City News");
    expect(here).toHaveBeenCalledTimes(2);
  });

  test.each(["", "   ", " \n\t ", "\u0000\u001f\u007f", "\u0001   \u0002"])(
    "blank %j is removed and the no-argument browser name is returned",
    (raw) => {
      localStorage.setItem(key, "Den TV");
      const setItem = vi.spyOn(Storage.prototype, "setItem");
      const removeItem = vi.spyOn(Storage.prototype, "removeItem");
      restoreStorage.push(() => {
        removeItem.mockRestore();
        setItem.mockRestore();
      });

      const saved = saveScreenName(raw);
      expect(saved).toBe("Browser");
      expect(saved).toBe(browserName());
      expect(saved).not.toBe(browserName("City News"));
      expect(removeItem).toHaveBeenCalledTimes(1);
      expect(removeItem).toHaveBeenCalledWith(key);
      expect(setItem).not.toHaveBeenCalled();
      expect(localStorage.getItem(key)).toBeNull();
      expect(here).toHaveBeenCalledTimes(1);
      expect(here).toHaveBeenCalledWith();
    },
  );

  test("a setItem throw is swallowed, the typed name comes back, and here runs once", () => {
    const { setItem, removeItem } = blockStorage();
    expect(saveScreenName("  City News  ")).toBe("City News");
    expect(setItem).toHaveBeenCalledTimes(1);
    expect(setItem).toHaveBeenCalledWith(key, "City News");
    expect(removeItem).not.toHaveBeenCalled();
    expect(localStorage.getItem(key)).toBeNull();
    expect(here).toHaveBeenCalledTimes(1);
    expect(here).toHaveBeenCalledWith();
  });

  test("a removeItem throw is swallowed, the browser name comes back, and here runs once", () => {
    localStorage.setItem(key, "Den TV");
    const { setItem, removeItem } = blockStorage();
    // The stand-in ignores storage. The real browserName() would still see "Den TV"
    // if this delete failed, because a saved name wins over the user-agent name.
    expect(saveScreenName(" \u0001 ")).toBe(browserName());
    expect(browserName()).toBe("Browser");
    expect(removeItem).toHaveBeenCalledTimes(1);
    expect(removeItem).toHaveBeenCalledWith(key);
    expect(setItem).not.toHaveBeenCalled();
    expect(localStorage.getItem(key)).toBe("Den TV");
    expect(here).toHaveBeenCalledTimes(1);
    expect(here).toHaveBeenCalledWith();
  });

  test("each save calls here once, whether storage accepts it or throws", () => {
    expect(saveScreenName("Den TV")).toBe("Den TV");
    expect(here).toHaveBeenCalledTimes(1);
    expect(saveScreenName("   ")).toBe("Browser");
    expect(here).toHaveBeenCalledTimes(2);
    blockStorage();
    expect(saveScreenName("City News")).toBe("City News");
    expect(here).toHaveBeenCalledTimes(3);
    expect(saveScreenName("\u007f")).toBe("Browser");
    expect(here).toHaveBeenCalledTimes(4);
  });
});
