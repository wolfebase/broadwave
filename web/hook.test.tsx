import { act, useState, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, expect, test, vi } from "vitest";
import { inAppDepth, navigate, useRoute } from "./src/app/router.ts";
import { useDiagnostics } from "./src/features/setup/useDiagnostics.ts";
import { groupRoom, peopleSentence, personLabel, useGroup } from "./src/features/player/together.ts";

type Pending = { url: string; resolve: (response: Response) => void };

const pending: Pending[] = [];

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } });
}

function mount(node: ReactNode) {
  const el = document.createElement("div");
  document.body.appendChild(el);
  const root: Root = createRoot(el);
  act(() => {
    root.render(node);
  });
  return {
    el,
    unmount() {
      act(() => root.unmount());
      el.remove();
    },
  };
}

afterEach(() => {
  pending.splice(0);
  vi.unstubAllGlobals();
  vi.useRealTimers();
  document.body.replaceChildren();
  window.history.replaceState(null, "", "/");
});

test("the route follows a push and a replace keeps the depth", async () => {
  function Show() {
    const route = useRoute();
    return (
      <p>
        {route.path} {route.params.get("channel")}
      </p>
    );
  }
  const view = mount(<Show />);
  expect(view.el.textContent).toBe("/ ");
  await act(async () => {
    navigate("/guide?channel=4");
  });
  expect(view.el.textContent).toBe("/guide 4");
  expect(inAppDepth()).toBe(1);
  await act(async () => {
    navigate("/guide?channel=4");
  });
  expect(inAppDepth()).toBe(1);
  await act(async () => {
    navigate("/sports?channel=7", true);
  });
  expect(view.el.textContent).toBe("/sports 7");
  expect(inAppDepth()).toBe(1);
  view.unmount();
});

test("diagnostics shows the newest body and drops one from an old mount", async () => {
  vi.stubGlobal("fetch", (url: string) => {
    return new Promise<Response>((resolve) => {
      pending.push({ url, resolve });
    });
  });
  let setKey: (key: number) => void = () => undefined;
  function Show() {
    const [key, set] = useState(0);
    setKey = set;
    const diag = useDiagnostics(key, 60_000);
    return <p>{diag?.version ?? "none"}</p>;
  }
  const view = mount(<Show />);
  expect(pending.map((item) => item.url)).toEqual(["/api/v1/diagnostics"]);
  const first = pending[0];
  await act(async () => {
    setKey(1);
  });
  const second = pending[1];
  await act(async () => {
    first.resolve(json({ version: "old", os: "test" }));
    await Promise.resolve();
  });
  expect(view.el.textContent).toBe("none");
  await act(async () => {
    second.resolve(json({ version: "0.13.0", os: "test" }));
    await Promise.resolve();
  });
  expect(view.el.textContent).toBe("0.13.0");
  view.unmount();
});

test("a failed diagnostics read leaves the last body", async () => {
  vi.stubGlobal("fetch", () => Promise.reject(new Error("offline")));
  const view = mount(<Diag />);
  await act(async () => {
    await Promise.resolve();
  });
  expect(view.el.textContent).toBe("none");
  view.unmount();
});

function Diag() {
  const diag = useDiagnostics("once", 60_000);
  return <p>{diag?.version ?? "none"}</p>;
}

test("a group is named by the channel, and a long list stops at two names", () => {
  expect(groupRoom(4)).toBe("group:ch4");
  expect(personLabel({ name: "Den TV", kind: "tv" })).toBe("Den TV");
  expect(personLabel({ name: "appletv", kind: "appletv" })).toBe("Apple TV");
  expect(personLabel({ name: "", kind: "phone" })).toBe("Another screen");
  expect(peopleSentence([])).toBe("");
  expect(peopleSentence([{ name: "Den TV", kind: "tv" }, { name: "web", kind: "web" }])).toBe("Den TV and Browser");
  const many = [
    { name: "Den TV", kind: "tv" },
    { name: "Kitchen", kind: "ipad" },
    { name: "a", kind: "web" },
    { name: "b", kind: "web" },
  ];
  expect(peopleSentence(many)).toBe("Den TV, Kitchen, and 2 more");
});

test("the group hook keeps the room for this channel", async () => {
  class SilentSocket {
    onopen: ((event: unknown) => void) | null = null;
    onmessage: ((event: unknown) => void) | null = null;
    onclose: ((event: unknown) => void) | null = null;
    onerror: ((event: unknown) => void) | null = null;
    constructor(readonly url: string) {}
    send() {}
    close() {}
  }
  vi.stubGlobal("WebSocket", SilentSocket);
  vi.useFakeTimers();
  vi.stubGlobal(
    "fetch",
    () =>
      Promise.resolve(
        json({
          groups: [
            {
              room: "group:ch9",
              channelId: 9,
              mode: "group",
              anchorServer: 1,
              anchorMedia: 2,
              rate: 1,
              latency: "balanced",
              version: 1,
              members: 2,
              people: [{ name: "Den TV", kind: "tv" }],
            },
          ],
        }),
      ),
  );
  function Show() {
    const group = useGroup(9);
    return <p>{group ? peopleSentence(group.people ?? []) : "alone"}</p>;
  }
  const view = mount(<Show />);
  await act(async () => {
    await Promise.resolve();
  });
  expect(view.el.textContent).toBe("Den TV");
  view.unmount();
});
