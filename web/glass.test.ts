import { afterEach, expect, test } from "vitest";
import { chooseGlass, drawsInSoftware } from "./src/lib/glass.ts";

// jsdom must not open a real WebGL context. These tests replace getContext.
const originalGetContext = HTMLCanvasElement.prototype.getContext;

const maskedRenderer = 0x1f01;
const unmaskedRenderer = 0x9246;

type Context = {
  RENDERER: number;
  getExtension(name: string): { UNMASKED_RENDERER_WEBGL: number } | { loseContext(): void } | null;
  getParameter(pname: number): string;
};

function stubContext(gl: Context | null) {
  const asked: string[] = [];
  HTMLCanvasElement.prototype.getContext = function (contextId: string) {
    asked.push(contextId);
    if (contextId !== "webgl2" && contextId !== "webgl") return null;
    return gl;
  } as unknown as HTMLCanvasElement["getContext"];
  return asked;
}

function stubRenderer(options: { debug: boolean; masked?: string; unmasked?: string }) {
  const masked = options.masked ?? "Apple M";
  const unmasked = options.unmasked ?? "Apple M";
  const parameters: number[] = [];
  let lost = false;
  const gl: Context = {
    RENDERER: maskedRenderer,
    getExtension(name: string) {
      if (name === "WEBGL_debug_renderer_info") {
        return options.debug ? { UNMASKED_RENDERER_WEBGL: unmaskedRenderer } : null;
      }
      if (name === "WEBGL_lose_context") {
        return {
          loseContext() {
            lost = true;
          },
        };
      }
      return null;
    },
    getParameter(pname: number) {
      parameters.push(pname);
      if (pname === unmaskedRenderer) return unmasked;
      if (pname === maskedRenderer) return masked;
      return "Apple M";
    },
  };
  stubContext(gl);
  return { parameters, lost: () => lost };
}

afterEach(() => {
  HTMLCanvasElement.prototype.getContext = originalGetContext;
  delete document.documentElement.dataset.glass;
});

test("no webgl context is software and the page goes flat", () => {
  const asked = stubContext(null);
  expect(drawsInSoftware()).toBe(true);
  expect(asked).toEqual(["webgl2", "webgl"]);
  chooseGlass();
  expect(document.documentElement.dataset.glass).toBe("flat");
});

test("LLVMPipe and Basic Render Driver are software, and Apple M is not", () => {
  for (const name of ["LLVMPipe", "Basic Render Driver"]) {
    const seen = stubRenderer({ debug: true, unmasked: name });
    expect(drawsInSoftware()).toBe(true);
    expect(seen.parameters).toEqual([unmaskedRenderer]);
  }

  const seen = stubRenderer({ debug: true, unmasked: "Apple M" });
  expect(drawsInSoftware()).toBe(false);
  expect(seen.parameters).toEqual([unmaskedRenderer]);
  chooseGlass();
  expect(document.documentElement.dataset.glass).toBeUndefined();
});

test("a context that throws is software and the page still goes flat", () => {
  HTMLCanvasElement.prototype.getContext = function () {
    throw new Error("context failed");
  } as unknown as HTMLCanvasElement["getContext"];
  expect(drawsInSoftware()).toBe(true);
  chooseGlass();
  expect(document.documentElement.dataset.glass).toBe("flat");
});

test("a missing debug extension reads the masked renderer", () => {
  const seen = stubRenderer({ debug: false, masked: "softpipe" });
  expect(drawsInSoftware()).toBe(true);
  expect(seen.parameters).toEqual([maskedRenderer]);
  expect(seen.lost()).toBe(true);
});

test("the unmasked renderer decides and the context is released", () => {
  const seen = stubRenderer({ debug: true, masked: "Apple M", unmasked: "llvmpipe" });
  expect(drawsInSoftware()).toBe(true);
  expect(seen.parameters).toEqual([unmaskedRenderer]);
  expect(seen.lost()).toBe(true);
});
