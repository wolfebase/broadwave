// A backdrop blur over playing video is redrawn for every frame. Drawn in
// software, the open Options panel alone dropped about half of 1080p60, so a
// browser without a graphics chip gets flat glass.
const software = /swiftshader|llvmpipe|softpipe|software|basic render/i;

export function drawsInSoftware(): boolean {
  try {
    const canvas = document.createElement("canvas");
    const gl = canvas.getContext("webgl2") ?? canvas.getContext("webgl");
    if (!gl) return true;
    const info = gl.getExtension("WEBGL_debug_renderer_info");
    const renderer = String(gl.getParameter(info ? info.UNMASKED_RENDERER_WEBGL : gl.RENDERER));
    gl.getExtension("WEBGL_lose_context")?.loseContext();
    return software.test(renderer);
  } catch {
    return true;
  }
}

export function chooseGlass() {
  if (drawsInSoftware()) document.documentElement.dataset.glass = "flat";
}
