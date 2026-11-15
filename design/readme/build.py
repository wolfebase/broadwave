#!/usr/bin/env python3
"""Builds the README art in .github/readme/ from code, fonts, and demo screenshots.

    pip install pillow fonttools brotli uharfbuzz
    python3 design/readme/build.py            # every SVG
    python3 design/readme/build.py --png      # also the social preview (needs Google Chrome)

Design system
- Canvas: every graphic is 1280 units wide, so all of them show at one scale in the README
  (about 0.74 at GitHub's 946 px column). Feature tiles are 628 x 400, two to a row, gap 24.
- Cards: radius 24, night gradient (#0B1222 to #070A12), soft radial light (never blurred
  blobs), fine grain against banding, a hairline (white 10%) that reads on GitHub dark and
  sits inside the edge on white. Nothing draws outside a card: no halos on light pages.
- Type: Inter (OFL), drawn as paths so every browser shows the same letters. Display 800,
  titles 30/700, body 19/450, labels 15/650 caps +6%. Mono: JetBrains Mono (OFL) 18.
  Body text is #AEB7C5 or brighter on the card (AA at body size).
- Color: design/tokens.json. Signal blue #3D7BFF, violet #8B5CF6, cyan #22D3EE, tally #FF3B30,
  pale strand #C9D6FF. Category colors for guide cells.
- Motion: one language. Loops of 6 to 20 s, ease EASE for moves and linear for flows,
  staggers in beats of 0.5 s. Every loop starts on its complete state, so a renderer that
  shows only the first frame (or no animation at all) still shows the whole idea.
  Under prefers-reduced-motion the gentle ambient motion keeps running (flows, soft
  pulses, slow fades, ticking digits); large or fast travel is dropped or slowed there
  (Doc.reduce). Nothing flashes more than once a second. CSS animations, plus SMIL for
  the terminal's typing (a clip that grows); GitHub strips scripts.
- Pictures: real screenshots of the apps' demo (Blender open movies, made-up stations),
  embedded as JPEG at about 2x their shown size. Never painted over.
"""

import base64
import io
import math
import os
import re
import subprocess
import sys
import tempfile

import uharfbuzz as hb
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.basePen import DecomposingPen
from fontTools.pens.transformPen import TransformPen
from fontTools.ttLib import TTFont
from fontTools.varLib.instancer import instantiateVariableFont
from PIL import Image

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))
OUT = os.path.join(ROOT, ".github", "readme")
SHOTS = os.path.join(HERE, "shots")

BG0, BG1 = "#0B1222", "#070A12"
TEXT, TEXT2, TEXT3 = "#F5F7FA", "#AEB7C5", "#8D99AC"
BLUE, VIOLET, CYAN, TALLY, PALE = "#3D7BFF", "#8B5CF6", "#22D3EE", "#FF3B30", "#C9D6FF"
GREEN, AMBER = "#2FBF71", "#FFB020"
R = 24
EASE = "cubic-bezier(.45,0,.25,1)"
# The pattern in scripts/check-names.sh, split the same way so this file passes it.
RETIRED = re.compile("wave" "guide|ota[ -]?viewer|ota" "kit|ota" "ui", re.I)


def fmt(v):
    s = f"{v:.2f}".rstrip("0").rstrip(".")
    return "0" if s in ("-0", "") else s


# ---------------------------------------------------------------- fonts


class Face:
    """One variable font at fixed axes: harfbuzz shapes, fontTools draws."""

    cache = {}

    def __init__(self, file, key, **axes):
        tt = TTFont(os.path.join(HERE, "fonts", file))
        tt.flavor = None
        buf = io.BytesIO()
        tt.save(buf)
        self.blob = buf.getvalue()
        self.key = key
        self.axes = axes
        static = instantiateVariableFont(TTFont(io.BytesIO(self.blob)), axes)
        self.glyphs = static.getGlyphSet()
        self.order = static.getGlyphOrder()
        self.upem = static["head"].unitsPerEm
        face = hb.Face(self.blob)
        self.font = hb.Font(face)
        self.font.set_variations(axes)

    @classmethod
    def get(cls, name):
        if name not in cls.cache:
            spec = FACES[name]
            cls.cache[name] = Face(spec[0], name, **spec[1])
        return cls.cache[name]

    def shape(self, text):
        buf = hb.Buffer()
        buf.add_str(text)
        buf.guess_segment_properties()
        hb.shape(self.font, buf, {"kern": True, "liga": True, "calt": True})
        out, x = [], 0
        for info, pos in zip(buf.glyph_infos, buf.glyph_positions):
            out.append((info.codepoint, x + pos.x_offset, pos.y_offset))
            x += pos.x_advance
        return out, x

    def outline(self, gid, transform=(1, 0, 0, 1, 0, 0)):
        pen = SVGPathPen(self.glyphs, ntos=lambda v: fmt(v))
        self.glyphs[self.order[gid]].draw(DecomposingTransform(pen, transform, self.glyphs))
        return pen.getCommands()


class DecomposingTransform(DecomposingPen):
    """Draws components as outlines, through a transform."""

    def __init__(self, out, transform, glyphs):
        super().__init__(glyphs)
        self.t = TransformPen(out, transform)

    def moveTo(self, p):
        self.t.moveTo(p)

    def lineTo(self, p):
        self.t.lineTo(p)

    def curveTo(self, *p):
        self.t.curveTo(*p)

    def qCurveTo(self, *p):
        self.t.qCurveTo(*p)

    def closePath(self):
        self.t.closePath()

    def endPath(self):
        self.t.endPath()


FACES = {
    "display": ("Inter.woff2", {"wght": 820, "opsz": 32}),
    "bold": ("Inter.woff2", {"wght": 700, "opsz": 28}),
    "semi": ("Inter.woff2", {"wght": 620, "opsz": 20}),
    "body": ("Inter.woff2", {"wght": 460, "opsz": 18}),
    "mono": ("JetBrainsMono.woff2", {"wght": 450}),
    "monob": ("JetBrainsMono.woff2", {"wght": 700}),
}


# ---------------------------------------------------------------- svg document


class Doc:
    def __init__(self, w, h, label):
        self.w, self.h, self.label = w, h, label
        self.defs, self.body, self.css, self.rm = [], [], [], []
        self.glyphs = {}
        self.ids = set()
        self.n = 0

    def uid(self, p):
        self.n += 1
        return f"{p}{self.n}"

    def add(self, s):
        self.body.append(s)

    def define(self, key, s):
        if key not in self.ids:
            self.ids.add(key)
            self.defs.append(s)

    def style(self, s):
        self.css.append(s)

    def reduce(self, s):
        """CSS that applies only under prefers-reduced-motion."""
        self.rm.append(s)

    # Text: per-glyph <use> of shared outlines. Returns the width.
    def text(self, x, y, s, size, face="body", fill=TEXT, anchor="start", track=0.0, opacity=None, cls=None, ret=False):
        f = Face.get(face)
        run, adv = f.shape(s)
        scale = size / f.upem
        width = adv * scale + track * size * max(0, len(run) - 1)
        if anchor == "middle":
            x -= width / 2
        elif anchor == "end":
            x -= width
        parts = []
        for i, (gid, gx, gy) in enumerate(run):
            key = f"{f.key[0]}{f.key[-1]}{gid}"
            if key not in self.glyphs:
                d = f.outline(gid)
                self.glyphs[key] = d
            if not self.glyphs[key]:
                continue
            px = x + gx * scale + i * track * size
            sc = f"{scale:.6f}".rstrip("0")
            parts.append(f'<use href="#{key}" transform="matrix({sc} 0 0 -{sc} {fmt(px)} {fmt(y - gy * scale)})"/>')
        attrs = f' fill="{fill}"'
        if opacity is not None:
            attrs += f' opacity="{opacity}"'
        if cls:
            attrs += f' class="{cls}"'
        markup = f"<g{attrs}>{''.join(parts)}</g>"
        if ret:
            return markup
        self.add(markup)
        return width

    def measure(self, s, size, face="body", track=0.0):
        f = Face.get(face)
        run, adv = f.shape(s)
        return adv * size / f.upem + track * size * max(0, len(run) - 1)

    # Text as one absolute path, for gradient fills (the gradient spans the whole word).
    def text_path(self, x, y, s, size, face="display", track=0.0, anchor="start"):
        f = Face.get(face)
        run, adv = f.shape(s)
        scale = size / f.upem
        width = adv * scale + track * size * max(0, len(run) - 1)
        if anchor == "middle":
            x -= width / 2
        d = []
        for i, (gid, gx, gy) in enumerate(run):
            px = x + gx * scale + i * track * size
            d.append(f.outline(gid, (scale, 0, 0, -scale, px, y - gy * scale)))
        return "".join(d), width

    def image(self, key, path, w, h, quality=84, crop=None):
        """Embeds a screenshot once, resized to w x h pixels; returns its id."""
        if key in self.ids:
            return key
        im = Image.open(path).convert("RGB")
        if crop:
            im = im.crop(crop)
        im = im.resize((w, h), Image.LANCZOS)
        # scripts/check-names.sh greps every file for retired product names, base64 too.
        while True:
            buf = io.BytesIO()
            im.save(buf, "JPEG", quality=quality, subsampling=0, optimize=True, progressive=True)
            b64 = base64.b64encode(buf.getvalue()).decode()
            if not RETIRED.search(b64):
                break
            quality -= 1
        self.define(key, f'<image id="{key}" width="{w}" height="{h}" href="data:image/jpeg;base64,{b64}"/>')
        return key

    def render(self):
        glyphs = "".join(f'<path id="{k}" d="{d}"/>' for k, d in self.glyphs.items() if d)
        css = "".join(self.css)
        if self.rm:
            css += "@media (prefers-reduced-motion:reduce){" + "".join(self.rm) + "}"
        return (
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {self.w} {self.h}" width="{self.w}" height="{self.h}" '
            f'role="img" aria-label="{self.label}"><style>{css}</style><defs>{glyphs}{"".join(self.defs)}</defs>'
            f'{"".join(self.body)}</svg>'
        )

    def save(self, name):
        data = self.render()
        with open(os.path.join(OUT, name), "w") as fh:
            fh.write(data)
        print(f"{name:28} {len(data) / 1024:7.1f} KB")


# ---------------------------------------------------------------- shared pieces


def common_defs(d):
    d.define(
        "bg",
        f'<linearGradient id="bg" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="{BG0}"/><stop offset="1" stop-color="{BG1}"/></linearGradient>',
    )
    # Grain: breaks up gradient banding. Static, so it is drawn once.
    d.define(
        "grain",
        '<filter id="grain" x="0" y="0" width="100%" height="100%" color-interpolation-filters="sRGB">'
        '<feTurbulence type="fractalNoise" baseFrequency=".85" numOctaves="2" seed="7" stitchTiles="stitch"/>'
        '<feColorMatrix values="0 0 0 0 1 0 0 0 0 1 0 0 0 0 1 0 0 0 1.6 -.62"/></filter>',
    )


def glow(d, cx, cy, rx, ry, color, strength, cls=None):
    """A soft light: many stops, so it falls off without visible rings."""
    gid = d.uid("gl")
    stops = "".join(
        f'<stop offset="{fmt(t)}" stop-color="{color}" stop-opacity="{fmt(strength * (1 - t) ** 2.2)}"/>' for t in (0, 0.15, 0.3, 0.45, 0.6, 0.75, 0.9, 1)
    )
    d.define(gid, f'<radialGradient id="{gid}">{stops}</radialGradient>')
    c = f' class="{cls}"' if cls else ""
    d.add(f'<ellipse{c} cx="{fmt(cx)}" cy="{fmt(cy)}" rx="{fmt(rx)}" ry="{fmt(ry)}" fill="url(#{gid})"/>')


def card_open(d, x, y, w, h, lights=()):
    """Starts a clipped card. lights: (cx, cy, rx, ry, color, strength) relative to the card."""
    common_defs(d)
    cid = d.uid("c")
    d.define(cid, f'<clipPath id="{cid}"><rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{R}"/></clipPath>')
    d.add(f'<g clip-path="url(#{cid})"><rect x="{x}" y="{y}" width="{w}" height="{h}" fill="url(#bg)"/>')
    for cx, cy, rx, ry, color, s in lights:
        glow(d, x + cx, y + cy, rx, ry, color, s)
    return (x, y, w, h)


def card_close(d, box, grain=0.05):
    x, y, w, h = box
    d.add(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" filter="url(#grain)" opacity="{grain}"/></g>')
    d.add(
        f'<rect x="{x + 0.5}" y="{y + 0.5}" width="{w - 1}" height="{h - 1}" rx="{R - 0.5}" fill="none" stroke="#FFFFFF" stroke-opacity=".11"/>'
    )


def tile_caption(d, box, title, line):
    x, y, w, h = box
    d.text(x + 36, y + h - 60, title, 30, "bold", TEXT, track=-0.012)
    d.text(x + 36, y + h - 28, line, 19, "body", TEXT2)


def wave_points(x0, x1, base, amp, period, phase, step=10):
    pts = []
    x = x0
    while x <= x1 + 0.1:
        t = (x / period) * 2 * math.pi + phase
        y = base + amp * (math.sin(t) * 0.78 + math.sin(2 * t + 0.9) * 0.22)
        pts.append((x, y))
        x += step
    return pts


def smooth(pts):
    """Catmull-Rom through the points, as cubic Beziers."""
    d = [f"M{fmt(pts[0][0])} {fmt(pts[0][1])}"]
    for i in range(len(pts) - 1):
        p0 = pts[max(0, i - 1)]
        p1, p2 = pts[i], pts[i + 1]
        p3 = pts[min(len(pts) - 1, i + 2)]
        c1 = (p1[0] + (p2[0] - p0[0]) / 6, p1[1] + (p2[1] - p0[1]) / 6)
        c2 = (p2[0] - (p3[0] - p1[0]) / 6, p2[1] - (p3[1] - p1[1]) / 6)
        d.append(f"C{fmt(c1[0])} {fmt(c1[1])} {fmt(c2[0])} {fmt(c2[1])} {fmt(p2[0])} {fmt(p2[1])}")
    return "".join(d)


def path_length(pts):
    return sum(math.hypot(b[0] - a[0], b[1] - a[1]) for a, b in zip(pts, pts[1:]))


def brand_wave(d, x0, x1, base, amp, period, dur=10, ribbon=True, scale=1.0, opacity=1.0, angle=0.0, comet=True):
    """The Broadwave wave: a pale strand, a signal-blue strand, and a fine ribbon, as on the
    Apple TV's Top Shelf. The strands travel in opposite directions, so the crossings flow
    along the wave, and a soft light rides the blue strand. All seamless: each layer moves
    exactly one period per loop."""
    pad = 160 if angle else 0
    span = (x0 - period - pad, x1 + period + pad)
    k = period
    d.style(
        f"@keyframes wf{k}{{to{{transform:translateX({period}px)}}}}@keyframes wr{k}{{to{{transform:translateX(-{period}px)}}}}"
        f".wa{k}{{animation:wf{k} {dur}s linear infinite}}.wb{k}{{animation:wr{k} {dur * 1.4:.2f}s linear infinite}}"
        f".wc{k}{{animation:wf{k} {dur * 2.2:.2f}s linear infinite}}"
    )
    turn = f' transform="rotate({fmt(angle)} {fmt((x0 + x1) / 2)} {fmt(base)})"' if angle else ""
    g = [f'<g opacity="{opacity}"{turn}>']
    if ribbon:
        lines = []
        for i in range(10):
            ph = 0.5 + i * 0.07
            a = amp * (0.62 + 0.04 * i)
            lines.append(f'<path d="{smooth(wave_points(*span, base + 6 * scale + i * 1.6 * scale, a, period, ph, 12))}"/>')
        g.append(f'<g class="wc{k}" fill="none" stroke="{BLUE}" stroke-width="{fmt(0.8 * scale)}" opacity=".32">{"".join(lines)}</g>')
    pale_pts = wave_points(*span, base, amp, period, 0.0, 10)
    blue_pts = wave_points(*span, base + 10 * scale, amp * 0.86, period, 1.05, 10)
    # Each strand is one path, drawn several times through <use>.
    pid, bid = d.uid("wp"), d.uid("wb")
    d.define(pid, f'<path id="{pid}" d="{smooth(pale_pts)}"/>')
    d.define(bid, f'<path id="{bid}" d="{smooth(blue_pts)}"/>')

    def use(ref, color, width, op=1, extra=""):
        o = f' opacity="{op}"' if op != 1 else ""
        return f'<use href="#{ref}" stroke="{color}" stroke-width="{fmt(width * scale)}"{o}{extra}/>'

    g.append(f'<g class="wa{k}" fill="none" stroke-linecap="round">{use(pid, PALE, 22, ".07")}{use(pid, PALE, 5.5, ".9")}</g>')
    g.append(f'<g class="wb{k}" fill="none" stroke-linecap="round">')
    g.append(use(bid, BLUE, 26, ".12") + use(bid, BLUE, 11, ".28") + use(bid, "#5B92FF", 4) + use(bid, "#CFE0FF", 1.2, ".7"))
    if comet:
        # Two lights on the strand; the dash pattern repeats every half length.
        rep = path_length(blue_pts) / 2
        dash = 110 * scale
        t = rep / 230.0
        d.style(f"@keyframes cm{k}{{to{{stroke-dashoffset:-{rep:.1f}}}}}.cm{k}{{animation:cm{k} {t:.2f}s linear infinite}}")
        da = f' stroke-dasharray="{dash:.1f} {rep - dash:.1f}" class="cm{k}"'
        g.append(use(bid, "#9FC2FF", 14, ".28", da) + use(bid, "#FFFFFF", 2.6, ".95", da))
    g.append("</g></g>")
    d.add("".join(g))


def tally_pill(d, x, y, text="ON AIR", size=15):
    w = d.measure(text, size, "semi", 0.12) + 50
    h = size * 2.2
    d.style("@keyframes tp{0%,100%{opacity:1}50%{opacity:.35}}.tp{animation:tp 2.4s ease-in-out infinite}")
    d.add(
        f'<rect x="{x}" y="{y}" width="{fmt(w)}" height="{fmt(h)}" rx="{fmt(h / 2)}" fill="{TALLY}" fill-opacity=".14" stroke="{TALLY}" stroke-opacity=".55"/>'
        f'<circle class="tp" cx="{fmt(x + 19)}" cy="{fmt(y + h / 2)}" r="5.5" fill="{TALLY}"/>'
        f'<circle class="tp" cx="{fmt(x + 19)}" cy="{fmt(y + h / 2)}" r="11" fill="{TALLY}" opacity=".25"/>'
    )
    d.text(x + 35, y + h / 2 + size * 0.36, text, size, "semi", "#FF6B61", track=0.12)
    return w


# ---------------------------------------------------------------- devices (vector bezels)


def tv(d, x, y, w, img, sweep=False, stand=True, delay=0.0, alt=None):
    """A thin-bezel TV. The screen is w wide and 16:9; returns the outer box."""
    h = w * 9 / 16
    b = max(5.0, w * 0.011)
    ox, oy, ow, oh = x - b, y - b, w + 2 * b, h + 2 * b
    gid = d.uid("tvr")
    d.define(
        gid,
        f'<linearGradient id="{gid}" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#5A6272"/><stop offset=".08" stop-color="#2A2F38"/><stop offset="1" stop-color="#14171C"/></linearGradient>',
    )
    if stand:
        nw, pw = w * 0.05, w * 0.3
        d.add(
            f'<rect x="{fmt(x + w / 2 - nw / 2)}" y="{fmt(oy + oh - 2)}" width="{fmt(nw)}" height="{fmt(w * 0.045)}" fill="#1B1F26"/>'
            f'<rect x="{fmt(x + w / 2 - pw / 2)}" y="{fmt(oy + oh + w * 0.04)}" width="{fmt(pw)}" height="{fmt(max(4, w * 0.009))}" rx="{fmt(max(2, w * 0.0045))}" fill="#252A33"/>'
            f'<rect x="{fmt(x + w / 2 - pw / 2)}" y="{fmt(oy + oh + w * 0.04)}" width="{fmt(pw)}" height="1" fill="#5A6272" opacity=".7"/>'
        )
    d.add(
        f'<rect x="{fmt(ox)}" y="{fmt(oy)}" width="{fmt(ow)}" height="{fmt(oh)}" rx="{fmt(b * 0.9)}" fill="#0A0C10"/>'
        f'<rect x="{fmt(ox + 0.5)}" y="{fmt(oy + 0.5)}" width="{fmt(ow - 1)}" height="{fmt(oh - 1)}" rx="{fmt(b * 0.9)}" fill="none" stroke="url(#{gid})"/>'
    )
    screen(d, x, y, w, h, img, 1.5, sweep, delay, alt)
    return (ox, oy, ow, oh + (w * 0.05 if stand else 0))


def screen(d, x, y, w, h, img, r, sweep=False, delay=0.0, alt=None):
    """A picture behind glass. sweep: a slow light passes now and then. alt: a second
    picture that fades in and out (16 s), starting hidden."""
    cid = d.uid("sc")
    d.define(cid, f'<clipPath id="{cid}"><rect x="{fmt(x)}" y="{fmt(y)}" width="{fmt(w)}" height="{fmt(h)}" rx="{fmt(r)}"/></clipPath>')
    d.define(
        "glass",
        '<linearGradient id="glass" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#FFFFFF" stop-opacity=".09"/>'
        '<stop offset=".35" stop-color="#FFFFFF" stop-opacity=".02"/><stop offset=".36" stop-color="#FFFFFF" stop-opacity="0"/></linearGradient>',
    )
    inner = [f'<g clip-path="url(#{cid})"><rect x="{fmt(x)}" y="{fmt(y)}" width="{fmt(w)}" height="{fmt(h)}" fill="#000"/>']
    for pic, cls in ((img, None), (alt, "xf")):
        if not pic:
            continue
        c = f' class="{cls}" opacity="0"' if cls else ""
        inner.append(f'<use{c} href="#{pic[0]}" transform="matrix({w / pic[1]:.5f} 0 0 {h / pic[2]:.5f} {fmt(x)} {fmt(y)})"/>')
    if alt:
        d.style(f"@keyframes xf{{0%,44%{{opacity:0}}50%,94%{{opacity:1}}100%{{opacity:0}}}}.xf{{animation:xf 16s {EASE} infinite}}")
    inner.append(f'<rect x="{fmt(x)}" y="{fmt(y)}" width="{fmt(w)}" height="{fmt(h)}" fill="url(#glass)"/>')
    if sweep:
        d.define(
            "sweep",
            '<linearGradient id="sweep" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#FFFFFF" stop-opacity="0"/>'
            '<stop offset=".5" stop-color="#FFFFFF" stop-opacity=".11"/><stop offset="1" stop-color="#FFFFFF" stop-opacity="0"/></linearGradient>',
        )
        dist = w * 1.9
        name = f"sw{int(w)}"
        d.style(
            f"@keyframes {name}{{0%,25%{{transform:translateX(0)}}100%{{transform:translateX({fmt(dist)}px)}}}}"
            f".{name}{{animation:{name} 10s cubic-bezier(.4,0,.6,1) infinite}}"
        )
        bw = w * 0.32
        st = f' style="animation-delay:{delay:.2f}s"' if delay else ""
        inner.append(
            f'<g class="{name}"{st}><rect x="{fmt(x - bw * 1.6)}" y="{fmt(y - h * 0.2)}" width="{fmt(bw)}" height="{fmt(h * 1.4)}" fill="url(#sweep)" transform="skewX(-18)" transform-origin="{fmt(x)} {fmt(y + h / 2)}"/></g>'
        )
    inner.append("</g>")
    d.add("".join(inner))


def ipad(d, x, y, w, img, sweep=False, delay=0.0):
    """iPad in landscape; w is the screen width (4:3). Returns the outer box."""
    h = w * 2064 / 2752
    b = w * 0.032
    ox, oy, ow, oh = x - b, y - b, w + 2 * b, h + 2 * b
    gid = d.uid("pr")
    d.define(
        gid,
        f'<linearGradient id="{gid}" x1="0" y1="0" x2=".3" y2="1"><stop offset="0" stop-color="#7C8494"/><stop offset=".15" stop-color="#3A3F48"/><stop offset="1" stop-color="#1E2127"/></linearGradient>',
    )
    d.add(
        f'<rect x="{fmt(ox - 1.5)}" y="{fmt(oy - 1.5)}" width="{fmt(ow + 3)}" height="{fmt(oh + 3)}" rx="{fmt(b * 1.9)}" fill="url(#{gid})"/>'
        f'<rect x="{fmt(ox)}" y="{fmt(oy)}" width="{fmt(ow)}" height="{fmt(oh)}" rx="{fmt(b * 1.75)}" fill="#050608"/>'
        f'<circle cx="{fmt(x + w / 2)}" cy="{fmt(oy + b / 2)}" r="{fmt(b * 0.16)}" fill="#1C2230"/>'
    )
    screen(d, x, y, w, h, img, b * 0.75, sweep, delay)
    return (ox, oy, ow, oh)


def iphone(d, x, y, w, img, sweep=False, delay=0.0):
    """iPhone in portrait; w is the screen width. Returns the outer box."""
    h = w * 2868 / 1320
    b = w * 0.04
    ox, oy, ow, oh = x - b, y - b, w + 2 * b, h + 2 * b
    gid = d.uid("fr")
    d.define(
        gid,
        f'<linearGradient id="{gid}" x1="0" y1="0" x2=".4" y2="1"><stop offset="0" stop-color="#8A919E"/><stop offset=".2" stop-color="#454B55"/><stop offset="1" stop-color="#23262C"/></linearGradient>',
    )
    rr = w * 0.17
    d.add(
        f'<rect x="{fmt(ox - w * 0.012)}" y="{fmt(oy + h * 0.2)}" width="{fmt(w * 0.02)}" height="{fmt(h * 0.06)}" rx="1" fill="#3A3F48"/>'
        f'<rect x="{fmt(ox - w * 0.012)}" y="{fmt(oy + h * 0.29)}" width="{fmt(w * 0.02)}" height="{fmt(h * 0.06)}" rx="1" fill="#3A3F48"/>'
        f'<rect x="{fmt(ox + ow - w * 0.008)}" y="{fmt(oy + h * 0.25)}" width="{fmt(w * 0.02)}" height="{fmt(h * 0.09)}" rx="1" fill="#3A3F48"/>'
        f'<rect x="{fmt(ox - 1.5)}" y="{fmt(oy - 1.5)}" width="{fmt(ow + 3)}" height="{fmt(oh + 3)}" rx="{fmt(rr + b + 1.5)}" fill="url(#{gid})"/>'
        f'<rect x="{fmt(ox)}" y="{fmt(oy)}" width="{fmt(ow)}" height="{fmt(oh)}" rx="{fmt(rr + b)}" fill="#050608"/>'
    )
    screen(d, x, y, w, h, img, rr, sweep, delay)
    iw, ih = w * 0.29, w * 0.085
    d.add(f'<rect x="{fmt(x + w / 2 - iw / 2)}" y="{fmt(y + w * 0.03)}" width="{fmt(iw)}" height="{fmt(ih)}" rx="{fmt(ih / 2)}" fill="#000"/>')
    return (ox, oy, ow, oh)


def shot(d, name, w, h, quality=84, crop=None):
    key = "i" + name.replace("-", "")
    d.image(key, os.path.join(SHOTS, name + ".jpg"), w, h, quality, crop)
    return (key, w, h)


# ---------------------------------------------------------------- hero


def hero(social=False):
    W, H = 1280, 640
    d = Doc(W, H, "Broadwave: free TV from the sky, live on every screen you own")
    common_defs(d)
    if social:
        d.add(f'<rect width="{W}" height="{H}" fill="url(#bg)"/>')
        box = None
    else:
        box = card_open(d, 0, 0, W, H)
    glow(d, 940, 330, 520, 330, BLUE, 0.42)
    glow(d, 1180, 40, 380, 240, VIOLET, 0.30)
    glow(d, 120, 640, 520, 220, CYAN, 0.10)
    d.define(
        "scan",
        '<pattern id="scan" width="4" height="4" patternUnits="userSpaceOnUse"><rect width="4" height="1" fill="#FFFFFF" opacity=".03"/></pattern>',
    )
    if not social:
        d.add(f'<rect width="{W}" height="{H}" fill="url(#scan)"/>')
    # Floor: a faint horizon the TV stands on.
    d.define(
        "floor",
        f'<linearGradient id="floor" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="{BLUE}" stop-opacity=".10"/><stop offset="1" stop-color="{BLUE}" stop-opacity="0"/></linearGradient>',
    )
    # The TV carries the right half, centred on the card; its stand sets the floor.
    sw = 644
    sh = sw * 9 / 16
    sx = 600
    sy = 312 - sh / 2
    fy = round(sy + sh + max(5.0, sw * 0.011) + sw * 0.04 + max(4, sw * 0.009))
    d.add(f'<rect x="0" y="{fy}" width="{W}" height="{H - fy}" fill="url(#floor)"/>')
    d.define(
        "hz",
        '<linearGradient id="hz" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#9DB8FF" stop-opacity="0"/>'
        '<stop offset=".55" stop-color="#9DB8FF" stop-opacity=".35"/><stop offset="1" stop-color="#9DB8FF" stop-opacity="0"/></linearGradient>',
    )
    d.add(f'<rect x="0" y="{fy}" width="{W}" height="1" fill="url(#hz)"/>')

    # The wave rises left to right: under the words, then behind the screen's lower third.
    brand_wave(d, 0, W, 474, 44, 640, dur=11, scale=1.3, angle=-10)

    img = shot(d, "tv-foryou", 1120, 630, 82)
    d.style(f"@keyframes br{{0%,100%{{opacity:1}}50%{{opacity:.6}}}}.br{{animation:br 8s {EASE} infinite}}")
    glow(d, sx + sw / 2, sy + sw * 9 / 32, sw * 0.75, sw * 0.5, "#6E9BFF", 0.24, "br")
    d.add('<g id="hero-tv">')
    tv(d, sx, sy, sw, img, sweep=not social, alt=None if social else shot(d, "tv-guide", 1120, 630, 78))
    d.add("</g>")
    d.define(
        "rfade",
        '<linearGradient id="rfade" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#FFFFFF" stop-opacity=".16"/><stop offset=".55" stop-color="#FFFFFF" stop-opacity="0"/></linearGradient>'
        f'<mask id="rmask" maskContentUnits="userSpaceOnUse"><rect x="0" y="{fy}" width="1280" height="{H - fy}" fill="url(#rfade)"/></mask>',
    )
    d.add(f'<g mask="url(#rmask)"><use href="#hero-tv" transform="matrix(1 0 0 -1 0 {2 * fy})"/></g>')

    # Words.
    x = 60
    tally_pill(d, x, 132)
    d.text(x + 128, 132 + 23, "FREE  ·  OPEN SOURCE  ·  SELF-HOSTED", 14, "semi", TEXT3, track=0.1)
    path, ww = d.text_path(x - 4, 270, "Broadwave", 100, "display", track=-0.035)
    d.define(
        "word",
        f'<linearGradient id="word" x1="0" y1="0" x2="1" y2=".25"><stop offset="0" stop-color="#FFFFFF"/><stop offset=".55" stop-color="#DCE6FF"/><stop offset="1" stop-color="#9DB8FF"/></linearGradient>',
    )
    d.add(f'<path d="{path}" fill="url(#word)"/>')
    d.text(x, 340, "Free TV from the sky.", 36, "bold", TEXT, track=-0.006)
    line, lw = d.text_path(x, 386, "Live on every screen you own.", 36, "bold", track=-0.006)
    d.define(
        "line",
        f'<linearGradient id="line" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="{CYAN}"/><stop offset=".5" stop-color="#7FA6FF"/><stop offset="1" stop-color="#B794FF"/></linearGradient>',
    )
    d.add(f'<path d="{line}" fill="url(#line)"/>')
    d.text(x, 428, "Live TV, DVR, and multiview from your antenna.", 19, "body", TEXT2)
    d.text(x, 454, "No subscription, no account.", 19, "body", TEXT2)
    if box:
        card_close(d, box, 0.05)
    else:
        d.add(f'<rect width="{W}" height="{H}" filter="url(#grain)" opacity=".02"/>')
    return d


def keyframes(name, frames):
    """frames: [(percent, css)] -> one @keyframes rule."""
    return f"@keyframes {name}{{" + "".join(f"{fmt(pc)}%{{{css}}}" for pc, css in frames) + "}"


def windows(name, start, end, n, period, hold_last=True):
    """n crossfading states across [start, end] percent; the last one also shows from end
    through 100% and from 0% to start, so the loop opens on it. Returns the class names."""
    names = []
    step = (end - start) / (n - 1)
    for i in range(n):
        a = start + i * step
        b = a + step
        if i == n - 1:
            fr = [(0, "opacity:1"), (start, "opacity:1"), (start + 0.8, "opacity:0"), (a - 0.4, "opacity:0"), (a, "opacity:1"), (100, "opacity:1")]
        else:
            fr = [(0, "opacity:0"), (max(0.01, a - 0.01), "opacity:0"), (a, "opacity:1"), (b - 0.4, "opacity:1"), (b, "opacity:0"), (100, "opacity:0")]
            if i == 0:
                fr = [(0, "opacity:0"), (start - 0.01, "opacity:0"), (start + 0.8, "opacity:1"), (b - 0.4, "opacity:1"), (b, "opacity:0"), (100, "opacity:0")]
        cls = f"{name}{i}"
        names.append((cls, keyframes(cls, fr) + f".{cls}{{animation:{cls} {period}s linear infinite}}"))
    return names


def digit_strip(d, key, size, digits, gap):
    """A column of digits in defs, gap apart, for a ticking counter."""
    if key in d.ids:
        return key
    rows = "".join(d.text(0, i * gap, ch, size, "monob", TEXT, ret=True) for i, ch in enumerate(digits))
    d.define(key, f'<g id="{key}">{rows}</g>')
    return key


# ---------------------------------------------------------------- stills and small screens

# Single frames of the demo's films, cut from the screenshots (no app chrome in them).
FRAMES = {
    "bbb": ("phone-watch", (0, 1063, 1320, 1805)),
    "sintel": ("pad-quad", (1546, 382, 2566, 956)),
    "tears": ("pad-quad", (186, 1160, 1206, 1722)),
    "eleph": ("pad-quad", (1546, 1160, 2566, 1722)),
}


def frame(d, name, w=480, h=270, quality=82):
    src, crop = FRAMES[name]
    key = f"f{name}{w}"
    d.image(key, os.path.join(SHOTS, src + ".jpg"), w, h, quality, crop)
    return (key, w, h)


def picture(d, x, y, w, h, img, r=4, fit="cover"):
    """An image filling a box (cover, centered)."""
    cid = d.uid("pc")
    d.define(cid, f'<clipPath id="{cid}"><rect x="{fmt(x)}" y="{fmt(y)}" width="{fmt(w)}" height="{fmt(h)}" rx="{fmt(r)}"/></clipPath>')
    key, iw, ih = img
    sc = max(w / iw, h / ih)
    tx = x + (w - iw * sc) / 2
    ty = y + (h - ih * sc) / 2
    d.add(f'<g clip-path="url(#{cid})"><use href="#{key}" transform="matrix({sc:.5f} 0 0 {sc:.5f} {fmt(tx)} {fmt(ty)})"/></g>')


def mini(d, kind, x, y, w, h, img, label=None):
    """A small screen for the diagrams: tv, laptop, tablet, or phone, bottom at y + h."""
    b = 4 if kind != "phone" else 3.5
    r = {"tv": 5, "laptop": 6, "tablet": 9, "phone": 11}[kind]
    d.add(
        f'<rect x="{fmt(x - b)}" y="{fmt(y - b)}" width="{fmt(w + 2 * b)}" height="{fmt(h + 2 * b)}" rx="{fmt(r + b * 0.6)}" fill="#05070B" stroke="#FFFFFF" stroke-opacity=".16"/>'
    )
    picture(d, x, y, w, h, img, r * 0.6)
    if kind == "laptop":
        d.add(
            f'<path d="M{fmt(x - b - 12)} {fmt(y + h + b)}h{fmt(w + 2 * b + 24)}l-6 7h{fmt(-(w + 2 * b + 12))}z" fill="#2A303B"/>'
            f'<rect x="{fmt(x - b - 12)}" y="{fmt(y + h + b)}" width="{fmt(w + 2 * b + 24)}" height="1" fill="#8B95A7" opacity=".6"/>'
        )
    if kind == "tv":
        d.add(f'<rect x="{fmt(x + w / 2 - w * 0.17)}" y="{fmt(y + h + b + 5)}" width="{fmt(w * 0.34)}" height="3" rx="1.5" fill="#2A303B"/>')
    if label:
        d.text(x + w / 2, y + h + 34, label, 15, "semi", TEXT3, anchor="middle")


def chip(d, x, y, text, size=13, fill="#000000", fill_op=0.55, color=TEXT, face="semi", dot=None, pad=9):
    w = d.measure(text, size, face) + pad * 2 + (12 if dot else 0)
    h = size + 12
    d.add(f'<rect x="{fmt(x)}" y="{fmt(y)}" width="{fmt(w)}" height="{fmt(h)}" rx="{fmt(h / 2)}" fill="{fill}" fill-opacity="{fill_op}"/>')
    tx = x + pad
    if dot:
        d.add(f'<circle cx="{fmt(x + pad + 3)}" cy="{fmt(y + h / 2)}" r="3.5" fill="{dot}"/>')
        tx += 12
    d.text(tx, y + h / 2 + size * 0.36, text, size, face, color)
    return w


# ---------------------------------------------------------------- devices family portrait


def devices():
    W, H = 1280, 680
    d = Doc(W, H, "Broadwave on Apple TV, iPad, and iPhone")
    box = card_open(
        d, 0, 0, W, H,
        lights=[(140, 60, 420, 260, VIOLET, 0.22), (1180, 600, 420, 220, CYAN, 0.10)],
    )
    d.style(f"@keyframes br{{0%,100%{{opacity:1}}50%{{opacity:.55}}}}.br{{animation:br 8s {EASE} infinite}}")
    glow(d, 640, 300, 640, 360, BLUE, 0.36, "br")
    fy = 604
    d.add(f'<rect x="0" y="{fy}" width="{W}" height="{H - fy}" fill="url(#floor)"/>')
    d.define(
        "floor",
        f'<linearGradient id="floor" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="{BLUE}" stop-opacity=".10"/><stop offset="1" stop-color="{BLUE}" stop-opacity="0"/></linearGradient>',
    )
    d.define(
        "hz",
        '<linearGradient id="hz" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#9DB8FF" stop-opacity="0"/>'
        '<stop offset=".5" stop-color="#9DB8FF" stop-opacity=".35"/><stop offset="1" stop-color="#9DB8FF" stop-opacity="0"/></linearGradient>',
    )
    d.add(f'<rect x="0" y="{fy}" width="{W}" height="1" fill="url(#hz)"/>')
    brand_wave(d, 0, W, 250, 60, 640, dur=12, opacity=0.6)

    tvw = 780
    tvx = (W - tvw) / 2 - 10
    tvy = fy - tvw * 9 / 16 - 8 - tvw * 0.04 - max(4, tvw * 0.009) - 1
    padw = 392
    padh = padw * 2064 / 2752
    phw = 150
    phh = phw * 2868 / 1320
    # Contact shadows.
    for cx, rw in ((tvx + tvw / 2, tvw * 0.22), (870 + padw / 2, padw * 0.55), (186 + phw / 2, phw * 0.7)):
        glow(d, cx, fy, rw, 10, "#000000", 0.9)
    d.add('<g id="family">')
    tv(d, tvx, tvy, tvw, shot(d, "tv-quad", 1240, 698, 74), sweep=True)
    ipad(d, 870, fy - padh - padw * 0.032, padw, shot(d, "pad-foryou", 720, 540, 74), sweep=True, delay=0.6)
    iphone(d, 186, fy - phh - phw * 0.04, phw, shot(d, "phone-foryou", 300, 652, 78), sweep=True, delay=-0.6)
    d.add("</g>")
    d.define(
        "dfade",
        f'<linearGradient id="dfade" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#FFFFFF" stop-opacity=".2"/><stop offset="1" stop-color="#FFFFFF" stop-opacity="0"/></linearGradient>'
        f'<mask id="dmask" maskContentUnits="userSpaceOnUse"><rect x="0" y="{fy}" width="{W}" height="{H - fy}" fill="url(#dfade)"/></mask>',
    )
    d.add(f'<g mask="url(#dmask)"><use href="#family" transform="matrix(1 0 0 -1 0 {2 * fy})"/></g>')
    card_close(d, box)
    return d


# ---------------------------------------------------------------- Apple TV and phone screens


def tv_tile(d, x0, name, title, line, H):
    box = card_open(d, x0, 0, 628, H)
    d.style(f"@keyframes br{{0%,100%{{opacity:1}}50%{{opacity:.5}}}}.br{{animation:br 8s {EASE} infinite}}")
    glow(d, x0 + 314, 190, 420, 260, BLUE, 0.32, "br")
    tv(d, x0 + 40, 40, 548, shot(d, name, 1100, 619, 80), stand=False, sweep=True, delay=x0 / 652 * 5)
    tile_caption(d, box, title, line)
    card_close(d, box)


def tv_row_1():
    H = 480
    d = Doc(1280, H, "Apple TV: the Live Guide with your channels along the top, and For You")
    tv_tile(d, 0, "tv-guide", "Live Guide", "Your channels along the top, a live preview below.", H)
    tv_tile(d, 652, "tv-foryou-row", "For You", "What's on now, your shows, and today's games.", H)
    return d


def tv_row_2():
    H = 480
    d = Doc(1280, H, "Apple TV: Watch together on the player's Screens tab. iPad with four channels, iPhone with the guide")
    tv_tile(d, 0, "tv-screens", "Watch together", "Pause or rewind on one screen, and every screen follows.", H)
    x0 = 652
    box = card_open(d, x0, 0, 628, H, lights=[(260, 190, 360, 240, VIOLET, 0.22)])
    glow(d, x0 + 480, 200, 260, 240, BLUE, 0.28, "br")
    padw = 384
    padh = padw * 2064 / 2752
    py = 44 + padw * 0.032
    ipad(d, x0 + 52, py, padw, shot(d, "pad-quad", 800, 600, 80), sweep=True, delay=2.5)
    phw = 122
    phh = phw * 2868 / 1320
    iphone(d, x0 + 470, py + padh - phh + padw * 0.032 - phw * 0.04, phw, shot(d, "phone-guide", 280, 608, 84), sweep=True, delay=3.5)
    tile_caption(d, box, "iPhone and iPad", "Four channels at once on iPad, the guide on iPhone.")
    card_close(d, box)
    return d


# ---------------------------------------------------------------- feature tiles


def tile_sync(d, x0):
    box = card_open(d, x0, 0, 628, 400, lights=[(314, 150, 400, 220, BLUE, 0.30)])
    img = frame(d, "bbb", 480, 270)
    y0 = 52
    specs = [("tv", 36, 196, 110, "Living room"), ("laptop", 256, 140, 88, "Office"), ("tablet", 420, 104, 78, "Kitchen"), ("phone", 546, 46, 100, "Bedroom")]
    bottom = y0 + 160
    for kind, x, w, h, label in specs:
        mini(d, kind, x0 + x, bottom - h, w, h, img, label)
    # The same clock on every screen, ticking in step: 42:1x, a second at a time.
    size, gap = 11, 16
    cw = d.measure("0", size, "monob")
    ones = digit_strip(d, "dg10", size, "0123456789", gap)
    tens = digit_strip(d, "dg6", size, "012345", gap)
    d.style(
        f"@keyframes tk1{{to{{transform:translateY(-{10 * gap}px)}}}}.tk1{{animation:tk1 10s steps(10,end) infinite}}"
        f"@keyframes tk6{{to{{transform:translateY(-{6 * gap}px)}}}}.tk6{{animation:tk6 60s steps(6,end) infinite}}"
        "@keyframes beat{0%{opacity:.9}35%,100%{opacity:0}}.beat{animation:beat 2s ease-out infinite}"
    )
    rings = []
    for kind, x, w, h, label in specs:
        cx = x0 + x + w / 2
        tw = cw * 5 + 10
        cy = bottom - 6
        d.add(f'<rect x="{fmt(cx - tw / 2)}" y="{fmt(cy - 15)}" width="{fmt(tw)}" height="17" rx="5" fill="#000" fill-opacity=".62"/>')
        tx = cx - cw * 2.5
        d.text(tx, cy - 2, "42:", size, "monob", TEXT)
        win = d.uid("tw")
        d.define(win, f'<clipPath id="{win}"><rect x="{fmt(tx + cw * 3)}" y="{fmt(cy - 14)}" width="{fmt(cw * 2 + 1)}" height="15"/></clipPath>')
        d.add(
            f'<g clip-path="url(#{win})"><g transform="translate({fmt(tx + cw * 3)} {fmt(cy - 2)})"><use class="tk6" href="#{tens}"/></g>'
            f'<g transform="translate({fmt(tx + cw * 4)} {fmt(cy - 2)})"><use class="tk1" href="#{ones}"/></g></g>'
        )
        r = {"tv": 5, "laptop": 6, "tablet": 9, "phone": 11}[kind] + 2.4
        rings.append(f'<rect x="{fmt(x0 + x - 4)}" y="{fmt(bottom - h - 4)}" width="{fmt(w + 8)}" height="{fmt(h + 8)}" rx="{fmt(r)}"/>')
    d.add(f'<g class="beat" fill="none" stroke="{CYAN}" stroke-width="2" opacity="0">{"".join(rings)}</g>')
    # One shared moment: a bracket under the rooms.
    by = bottom + 58
    d.add(f'<path d="M{x0 + 36} {by - 6}v6h556v-6" fill="none" stroke="{CYAN}" stroke-opacity=".5" stroke-width="1.5"/>')
    w = d.measure("Same frame · 15 ms apart", 14, "semi") + 28
    d.add(f'<rect x="{fmt(x0 + 314 - w / 2)}" y="{by - 12}" width="{fmt(w)}" height="24" rx="12" fill="#0A1426" stroke="{CYAN}" stroke-opacity=".5"/>')
    d.text(x0 + 314, by + 5, "Same frame · 15 ms apart", 14, "semi", "#8FE9F7", anchor="middle")
    tile_caption(d, box, "Every room, the same frame", "Whole-Home Sync keeps every screen on one moment.")
    card_close(d, box)


def pause_glyph(d, cx, cy, s, op=1):
    return (
        f'<g opacity="{op}"><circle cx="{fmt(cx)}" cy="{fmt(cy)}" r="{fmt(s)}" fill="#000" fill-opacity=".55" stroke="#FFFFFF" stroke-opacity=".5"/>'
        f'<rect x="{fmt(cx - s * 0.36)}" y="{fmt(cy - s * 0.4)}" width="{fmt(s * 0.24)}" height="{fmt(s * 0.8)}" rx="1.5" fill="#FFFFFF"/>'
        f'<rect x="{fmt(cx + s * 0.12)}" y="{fmt(cy - s * 0.4)}" width="{fmt(s * 0.24)}" height="{fmt(s * 0.8)}" rx="1.5" fill="#FFFFFF"/></g>'
    )


def tile_together(d, x0):
    """8 s: paused everywhere; the phone plays and every screen follows a beat later; the
    phone pauses, a ripple goes out, and every screen pauses a beat later."""
    box = card_open(d, x0, 0, 628, 400, lights=[(160, 150, 300, 220, VIOLET, 0.26), (440, 150, 300, 220, BLUE, 0.26)])
    img = frame(d, "bbb", 480, 270)
    T = 8
    px, py, pw, ph = x0 + 70, 40, 104, 218
    d.add(f'<rect x="{px - 5}" y="{py - 5}" width="{pw + 10}" height="{ph + 10}" rx="20" fill="#05070B" stroke="#FFFFFF" stroke-opacity=".2"/>')
    d.add(f'<rect x="{px + 36}" y="{py + 7}" width="32" height="9" rx="4.5" fill="#000" stroke="#FFFFFF" stroke-opacity=".08"/>')
    picture(d, px, py + 26, pw, 58, img, 0)
    d.text(px + 10, py + 104, "4.1", 11, "bold", TEXT)
    d.text(px + 10, py + 120, "Big Buck Bunny", 11, "semi", TEXT2)
    d.add(f'<rect x="{px + 10}" y="{py + 150}" width="{pw - 20}" height="3" rx="1.5" fill="#FFFFFF" opacity=".25"/><rect x="{px + 10}" y="{py + 150}" width="{(pw - 20) * 0.7}" height="3" rx="1.5" fill="#FFFFFF"/>')
    cx, cy = px + pw / 2, py + 186
    d.style(
        keyframes("press", [(0, "transform:scale(1)"), (28, "transform:scale(1)"), (30, "transform:scale(.84)"), (33, "transform:scale(1)"),
                            (68, "transform:scale(1)"), (70, "transform:scale(.84)"), (73, "transform:scale(1)"), (100, "transform:scale(1)")])
        + f".press{{transform-box:fill-box;transform-origin:center;animation:press {T}s {EASE} infinite}}"
        # Paused: the phone shows Play, the screens are dim with a pause mark.
        + keyframes("isplay", [(0, "opacity:1"), (29.5, "opacity:1"), (30.5, "opacity:0"), (69.5, "opacity:0"), (70.5, "opacity:1"), (100, "opacity:1")])
        + keyframes("ispause", [(0, "opacity:0"), (29.5, "opacity:0"), (30.5, "opacity:1"), (69.5, "opacity:1"), (70.5, "opacity:0"), (100, "opacity:0")])
        + f".isplay{{animation:isplay {T}s linear infinite}}.ispause{{animation:ispause {T}s linear infinite}}"
        + keyframes("held", [(0, "opacity:1"), (35, "opacity:1"), (38, "opacity:0"), (75, "opacity:0"), (78, "opacity:1"), (100, "opacity:1")])
        + f".held{{animation:held {T}s {EASE} infinite}}"
        + keyframes("rip", [(0, "transform:scale(.3);opacity:0"), (69, "transform:scale(.3);opacity:0"), (71, "opacity:.8"), (86, "transform:scale(1);opacity:0"), (100, "transform:scale(1);opacity:0")])
        + keyframes("rip0", [(0, "transform:scale(.3);opacity:0"), (29, "transform:scale(.3);opacity:0"), (31, "opacity:.6"), (46, "transform:scale(1);opacity:0"), (100, "transform:scale(1);opacity:0")])
        + f".rip,.rip0{{transform-box:view-box;transform-origin:{fmt(cx)}px {fmt(cy)}px}}.rip{{animation:rip {T}s ease-out infinite}}.rip0{{animation:rip0 {T}s ease-out infinite}}"
    )
    d.reduce(".rip,.rip0{animation:none}")
    for c in ("rip", "rip0"):
        d.add(f'<circle class="{c}" cx="{fmt(cx)}" cy="{fmt(cy)}" r="280" fill="none" stroke="{CYAN}" stroke-width="2" opacity="0"/>')
    play = f'<path class="isplay" d="M{fmt(cx - 4)} {fmt(cy - 7)}l11 7l-11 7z" fill="#0A0F1A"/>'
    pause = (
        f'<g class="ispause" opacity="0"><rect x="{fmt(cx - 6)}" y="{fmt(cy - 7)}" width="4" height="14" rx="1" fill="#0A0F1A"/>'
        f'<rect x="{fmt(cx + 2)}" y="{fmt(cy - 7)}" width="4" height="14" rx="1" fill="#0A0F1A"/></g>'
    )
    d.add(f'<g class="press"><circle cx="{fmt(cx)}" cy="{fmt(cy)}" r="17" fill="#FFFFFF"/>{play}{pause}</g>')
    screens = [("tv", x0 + 262, 40, 230, 129), ("tablet", x0 + 262, 196, 124, 93), ("laptop", x0 + 418, 202, 136, 85)]
    dims, glyphs = [], []
    for kind, x, y, w, h in screens:
        mini(d, kind, x, y, w, h, img)
        dims.append(f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="3" fill="#000" fill-opacity=".45"/>')
        glyphs.append(pause_glyph(d, x + w / 2, y + h / 2, min(w, h) * 0.2))
    d.add(f'<g class="held">{"".join(dims)}{"".join(glyphs)}</g>')
    tile_caption(d, box, "Watch together", "Pause or rewind on one screen, and every screen follows.")
    card_close(d, box)


GUIDE = [
    ("4.1", "bbb", [("Morning News", 0, 1.0, "news"), ("Kitchen Live", 1.0, 2.0, "series"), ("Garden Hour", 2.0, 3.2, "series")]),
    ("5.1", "sintel", [("The Lost Dragon", -0.6, 1.6, "movies"), ("Quiz Night", 1.6, 2.6, "series"), ("Local Weather", 2.6, 3.2, "news")]),
    ("7.1", None, [("Harbor at Valley", -0.4, 2.3, "sports"), ("Postgame", 2.3, 3.2, "sports")]),
    ("9.1", "tears", [("City Lights", 0, 0.5, "series"), ("Science Hour", 0.5, 1.5, "series"), ("Evening News", 1.5, 2.5, "news"), ("Late Show", 2.5, 3.2, "series")]),
    ("11.1", "eleph", [("Cartoon Club", -0.2, 1.0, "kids"), ("Puzzle Time", 1.0, 2.0, "kids"), ("Nature", 2.0, 3.2, "series")]),
]
CATS = {"news": "#3D9BFF", "sports": "#2FBF71", "movies": "#B36BFF", "kids": "#FFB020", "series": "#5E7BA8"}


def tile_guide(d, x0):
    """12 s: the focus moves across three programmes; the now line creeps along."""
    box = card_open(d, x0, 0, 628, 400, lights=[(420, 120, 380, 220, BLUE, 0.24), (80, 260, 260, 200, GREEN, 0.10)])
    gx, gy, cw, rh, gap = x0 + 36, 64, 92, 40, 5
    left = gx + cw + gap
    slot = (x0 + 592 - left) / 2.6
    for i, t in enumerate(("8:00", "8:30", "9:00")):
        d.text(left + i * slot + 6, gy - 14, t, 13, "semi", TEXT3)
    clip = d.uid("gc")
    d.define(clip, f'<clipPath id="{clip}"><rect x="{left}" y="{gy - 2}" width="{x0 + 628 - left}" height="{5 * (rh + gap)}"/></clipPath>')
    for r, (num, fr, cells) in enumerate(GUIDE):
        y = gy + r * (rh + gap)
        d.add(f'<rect x="{gx}" y="{y}" width="{cw}" height="{rh}" rx="7" fill="#FFFFFF" fill-opacity=".06"/>')
        if fr:
            picture(d, gx + 7, y + 9, 38, 22, frame(d, fr, 160, 90, 78), 3)
        else:
            d.add(
                f'<rect x="{gx + 7}" y="{y + 9}" width="38" height="22" rx="3" fill="#1E7A47"/>'
                f'<path d="M{gx + 26} {y + 9}v22M{gx + 14} {y + 9}v22M{gx + 38} {y + 9}v22" stroke="#FFFFFF" stroke-opacity=".45"/>'
                f'<circle cx="{gx + 26}" cy="{y + 20}" r="4" fill="none" stroke="#FFFFFF" stroke-opacity=".6"/>'
            )
        d.text(gx + 52, y + 26, num, 15, "bold", TEXT)
        d.add(f'<g clip-path="url(#{clip})">')
        for title, a, b, cat in cells:
            cx = left + a * slot
            w = (b - a) * slot - 4
            d.add(f'<rect x="{fmt(cx)}" y="{y}" width="{fmt(w)}" height="{rh}" rx="7" fill="#FFFFFF" fill-opacity=".07"/>')
            d.add(f'<rect x="{fmt(cx)}" y="{y + 9}" width="3" height="{rh - 18}" rx="1.5" fill="{CATS[cat]}"/>')
            d.text(max(cx, left) + 12, y + 25, title, 14, "semi", "#DCE3EE")
        d.add("</g>")
    # Now line: creeps 36 units over 15 s, then fades and starts again where it began.
    nx = left + 0.7 * slot
    d.style(
        keyframes("nowl", [(0, "transform:translateX(0);opacity:1"), (94, "transform:translateX(36px);opacity:1"), (96.5, "transform:translateX(36px);opacity:0"),
                           (96.6, "transform:translateX(0);opacity:0"), (100, "transform:translateX(0);opacity:1")])
        + ".nowl{animation:nowl 16s linear infinite}"
    )
    d.add(f'<g class="nowl"><rect x="{fmt(nx - 1)}" y="{gy - 4}" width="2" height="{5 * (rh + gap) + 2}" fill="{TALLY}"/>')
    d.add(f'<rect x="{fmt(nx - 30)}" y="{gy - 30}" width="60" height="20" rx="10" fill="{TALLY}"/>')
    d.text(nx, gy - 16, "8:21", 12, "bold", "#FFFFFF", anchor="middle")
    d.add("</g>")
    # Three focus states that hand over to each other.
    focus = [
        (2, 0, "Harbor Gulls at Valley Larks", "Football · Q4"),
        (3, 2, "Evening News", "News · Live"),
        (1, 1, "Quiz Night", "Season 4 · New"),
    ]
    T = 12
    states = [
        [(0, "opacity:1"), (30, "opacity:1"), (33, "opacity:0"), (97, "opacity:0"), (100, "opacity:1")],
        [(0, "opacity:0"), (30, "opacity:0"), (33, "opacity:1"), (63, "opacity:1"), (66, "opacity:0"), (100, "opacity:0")],
        [(0, "opacity:0"), (63, "opacity:0"), (66, "opacity:1"), (97, "opacity:1"), (100, "opacity:0")],
    ]
    for i, ((r, c, title, sub_), fr) in enumerate(zip(focus, states)):
        _, a, b, cat = GUIDE[r][2][c]
        y = gy + r * (rh + gap)
        fx = left + max(a, 0) * slot
        fw = (b - max(a, 0)) * slot - 4
        cls = f"fo{i}"
        d.style(keyframes(cls, fr) + f".{cls}{{animation:{cls} {T}s {EASE} infinite}}")
        hidden = "" if i == 0 else ' opacity="0"'
        d.add(f'<g class="{cls}"{hidden}>')
        glow(d, fx + fw / 2, y + rh / 2 + 8, fw * 0.7, 34, "#000000", 0.6)
        d.add(f'<rect x="{fmt(fx - 4)}" y="{y - 4}" width="{fmt(fw + 8)}" height="{rh + 8}" rx="9" fill="#F5F7FA"/>')
        d.add(f'<rect x="{fmt(fx + 2)}" y="{y + 7}" width="3" height="{rh - 14}" rx="1.5" fill="{CATS[cat]}"/>')
        d.text(fx + 12, y + 19, title, 14, "bold", "#0A0F1A")
        d.text(fx + 12, y + 34, sub_, 11.5, "semi", "#3A4556")
        d.add("</g>")
    tile_caption(d, box, "The guide is the centerpiece", "Artwork, a live preview, and search across it all.")
    card_close(d, box)


def tile_multiview(d, x0):
    box = card_open(d, x0, 0, 628, 400, lights=[(314, 150, 420, 240, VIOLET, 0.28)])
    tw, th, g = 224, 126, 8
    left = x0 + (628 - (2 * tw + g)) / 2
    top = 32
    names = [("bbb", "4.1"), ("sintel", "5.1"), ("tears", "9.1"), ("eleph", "11.1")]
    for i, (n, num) in enumerate(names):
        x = left + (i % 2) * (tw + g)
        y = top + (i // 2) * (th + g)
        picture(d, x, y, tw, th, frame(d, n, 480, 270), 8)
        chip(d, x + 8, y + th - 30, num, 12, fill_op=0.6)
    # Sound and 60 fps follow the picked tile.
    hop = tw + g
    vh = th + g
    d.style(
        "@keyframes pick{0%,22%{transform:translate(0,0)}25%,47%" + f"{{transform:translate({hop}px,0)}}"
        f"50%,72%{{transform:translate({hop}px,{vh}px)}}75%,97%{{transform:translate(0,{vh}px)}}100%{{transform:translate(0,0)}}}}"
        ".pick{animation:pick 12s cubic-bezier(.6,0,.2,1) infinite}"
    )
    # The ring and its badge, drawn once; the hop moves it, the reduced-motion version fades it.
    bx = left + tw - 84
    sx, sy = bx + 12, top + 20
    d.define(
        "mvpick",
        f'<g id="mvpick"><rect x="{fmt(left - 3)}" y="{top - 3}" width="{tw + 6}" height="{th + 6}" rx="10" fill="none" stroke="#FFFFFF" stroke-width="3"/>'
        f'<rect x="{fmt(bx)}" y="{top + 8}" width="76" height="24" rx="12" fill="#FFFFFF"/>'
        f'<path d="M{fmt(sx)} {sy - 3}h3l4-4v14l-4-4h-3z" fill="#0A0F1A"/>'
        f'<path d="M{fmt(sx + 10)} {sy - 4}q3 4 0 8" fill="none" stroke="#0A0F1A" stroke-width="1.6" stroke-linecap="round"/>'
        + d.text(bx + 30, top + 25, "60 fps", 12, "bold", "#0A0F1A", ret=True) + "</g>",
    )
    d.reduce(".pick{animation:none;opacity:0}")
    for i in range(4):
        dx, dy = (i % 2) * hop, (i // 2) * vh
        a = i * 25
        fr = [(0, "opacity:0"), (max(0.01, a), "opacity:0"), (a + 2, "opacity:1"), (a + 23, "opacity:1"), (a + 25, "opacity:0"), (100, "opacity:0")]
        if i == 0:
            fr = [(0, "opacity:1"), (23, "opacity:1"), (25, "opacity:0"), (98, "opacity:0"), (100, "opacity:1")]
        d.style(keyframes(f"pf{i}", fr))
        d.reduce(f".pf{i}{{animation:pf{i} 12s {EASE} infinite}}")
        d.add(f'<g class="pf{i}" opacity="0"><use href="#mvpick" transform="translate({fmt(dx)} {fmt(dy)})"/></g>')
    d.add('<use class="pick" href="#mvpick"/>')
    tile_caption(d, box, "Four channels at once", "Sound and 60 fps follow whichever tile you pick.")
    card_close(d, box)


def tile_sports(d, x0):
    box = card_open(d, x0, 0, 628, 400, lights=[(200, 120, 360, 220, GREEN, 0.16), (520, 200, 300, 220, TALLY, 0.14)])
    # Scoreboard.
    sx, sy = x0 + 36, 34
    d.add(f'<rect x="{sx}" y="{sy}" width="300" height="44" rx="12" fill="#FFFFFF" fill-opacity=".07" stroke="#FFFFFF" stroke-opacity=".1"/>')
    d.add(f'<rect x="{sx + 12}" y="{sy + 13}" width="18" height="18" rx="5" fill="#3D7BFF"/>')
    d.text(sx + 38, sy + 28, "HAR 27", 15, "bold", TEXT)
    d.add(f'<rect x="{sx + 112}" y="{sy + 13}" width="18" height="18" rx="5" fill="{AMBER}"/>')
    d.text(sx + 138, sy + 28, "VAL 27", 15, "bold", TEXT)
    d.text(sx + 222, sy + 28, "OT 4:51", 15, "semi", "#FF8A80")
    # Timeline: 7 PM to 11 PM.
    tl, tr = x0 + 36, x0 + 592
    hx = lambda h: tl + (h - 7) / 4 * (tr - tl)
    ty = 128
    for h, lab in ((7, "7 PM"), (8, "8"), (9, "9"), (10, "10"), (11, "11")):
        d.add(f'<rect x="{fmt(hx(h))}" y="{ty}" width="1" height="128" fill="#FFFFFF" opacity=".08"/>')
        d.text(hx(h) + (5 if h < 11 else -5), ty - 8, lab, 13, "semi", TEXT3, anchor="start" if h < 11 else "end")
    # Scheduled.
    d.add(f'<rect x="{fmt(hx(7))}" y="{ty + 14}" width="{fmt(hx(10) - hx(7))}" height="40" rx="9" fill="#FFFFFF" fill-opacity=".08"/>')
    d.add(f'<rect x="{fmt(hx(7))}" y="{ty + 23}" width="3" height="22" rx="1.5" fill="{GREEN}"/>')
    d.text(hx(7) + 14, ty + 39, "Harbor Gulls at Valley Larks", 14, "semi", "#DCE3EE")
    # Scheduled end.
    ex = hx(10)
    d.add(f'<path d="M{fmt(ex)} {ty + 6}V{ty + 132}" stroke="#FFFFFF" stroke-opacity=".55" stroke-dasharray="3 4"/>')
    d.text(ex - 6, ty + 128, "scheduled end", 12, "semi", TEXT3, anchor="end")
    # Recording, past the end.
    ry = ty + 70
    over = hx(10.55) - ex
    d.add(f'<rect x="{fmt(hx(7))}" y="{ry}" width="{fmt(ex - hx(7))}" height="30" rx="8" fill="{TALLY}" fill-opacity=".85"/>')
    T = 10
    d.style(
        keyframes("grow", [(0, "transform:scaleX(1);opacity:1"), (30, "transform:scaleX(1);opacity:1"), (34, "transform:scaleX(1);opacity:0"),
                           (35, "transform:scaleX(0);opacity:0"), (36, "transform:scaleX(0);opacity:1"), (78, "transform:scaleX(1);opacity:1"),
                           (100, "transform:scaleX(1);opacity:1")])
        + f".grow{{transform-box:fill-box;transform-origin:left center;animation:grow {T}s cubic-bezier(.33,0,.25,1) infinite}}"
    )
    d.add(f'<rect class="grow" x="{fmt(ex - 8)}" y="{ry}" width="{fmt(over + 8)}" height="30" rx="8" fill="{TALLY}"/>')
    d.add(f'<rect x="{fmt(ex - 1)}" y="{ry}" width="2" height="30" fill="#FFFFFF" opacity=".6"/>')
    d.add(f'<circle class="tp" cx="{fmt(hx(7) + 16)}" cy="{ry + 15}" r="5" fill="#FFFFFF"/>')
    d.style("@keyframes tp{0%,100%{opacity:1}50%{opacity:.35}}.tp{animation:tp 2.4s ease-in-out infinite}")
    d.text(hx(7) + 30, ry + 20, "Recording", 14, "bold", "#FFFFFF")
    # The minutes over, counting up as the bar grows (a new number about every 0.6 s).
    counts = ["+5", "+10", "+15", "+20", "+25", "+30", "+33"]
    for i, (cls, css) in enumerate(windows("mn", 36, 78, len(counts), T)):
        d.style(css)
        hidden = "" if i == len(counts) - 1 else ' opacity="0"'
        d.add(f'<g class="{cls}"{hidden}>')
        d.text(ex + over + 10, ry + 20, counts[i] + " min", 15, "bold", "#FF8A80")
        d.add("</g>")
    tile_caption(d, box, "A DVR that gets sports", "A game that runs long keeps recording until it's final.")
    card_close(d, box)


def tile_motion(d, x0):
    """7 s: each lane's ball crosses on its arc. 60 fps glides; 30 fps holds every other
    position, so it steps. Slow motion, so the difference shows."""
    box = card_open(d, x0, 0, 628, 400, lights=[(160, 120, 300, 200, GREEN, 0.14), (460, 220, 320, 200, BLUE, 0.22)])
    lanes = [(60, "60 fps", TEXT, GREEN, 30, "Broadwave"), (30, "30 fps", TEXT3, "#AEB7C5", 15, "half the frames")]
    T, t0, t1 = 7, 12, 80
    for li, (fps, label, color, dot, n, note) in enumerate(lanes):
        lx, ly, lw, lh = x0 + 36, 34 + li * 132, 556, 118
        d.add(f'<rect x="{lx}" y="{ly}" width="{lw}" height="{lh}" rx="16" fill="#FFFFFF" fill-opacity=".05" stroke="#FFFFFF" stroke-opacity=".08"/>')
        d.text(lx + 18, ly + 28, label, 15, "bold", color)
        d.text(lx + 18 + d.measure(label, 15, "bold") + 10, ly + 28, note, 15, "semi", TEXT3)
        x1, x2 = lx + 120, lx + lw - 36
        pos = []
        for k in range(n + 1):
            t = k / n
            pos.append((x1 + (x2 - x1) * t, ly + lh - 26 - math.sin(t * math.pi) * 52))
        d.add("".join(f'<circle cx="{fmt(x)}" cy="{fmt(y)}" r="3.6" fill="{dot}" opacity=".32"/>' for x, y in pos[:-1]))
        # The ball. Keyframes in absolute positions; the loop opens with it at the end.
        end = pos[-1]
        fr = [(0, f"transform:translate({fmt(end[0])}px,{fmt(end[1])}px);opacity:1"), (t0 - 6, f"transform:translate({fmt(end[0])}px,{fmt(end[1])}px);opacity:1"),
              (t0 - 3, f"transform:translate({fmt(end[0])}px,{fmt(end[1])}px);opacity:0"), (t0 - 2.9, f"transform:translate({fmt(pos[0][0])}px,{fmt(pos[0][1])}px);opacity:0"),
              (t0, f"transform:translate({fmt(pos[0][0])}px,{fmt(pos[0][1])}px);opacity:1")]
        for k, (x, y) in enumerate(pos[1:], 1):
            pc = t0 + (t1 - t0) * k / n
            if fps == 30:
                px_, py_ = pos[k - 1]
                fr.append((pc - 0.01, f"transform:translate({fmt(px_)}px,{fmt(py_)}px);opacity:1"))
            fr.append((pc, f"transform:translate({fmt(x)}px,{fmt(y)}px);opacity:1"))
        fr.append((100, f"transform:translate({fmt(end[0])}px,{fmt(end[1])}px);opacity:1"))
        cls = f"ball{fps}"
        d.style(keyframes(cls, fr) + f".{cls}{{animation:{cls} {T}s linear infinite}}")
        d.reduce(f".{cls}{{animation-duration:{T * 1.6:.1f}s}}")
        d.add(f'<circle class="{cls}" cx="0" cy="0" r="7.5" fill="{dot}" transform="translate({fmt(end[0])} {fmt(end[1])})"/>')
    tile_caption(d, box, "True 60 fps", "Sports at the broadcast's full frame rate. Film back at 24p.")
    card_close(d, box)


def tiles_row(name, label, a, b):
    d = Doc(1280, 400, label)
    a(d, 0)
    b(d, 652)
    return d


# ---------------------------------------------------------------- one tuner


def antenna(d, x, y):
    d.add(
        f'<g fill="none" stroke="#DCE6FF" stroke-width="5" stroke-linecap="round" stroke-linejoin="round">'
        f'<path d="M{x} {y + 112}L{x + 34} {y}L{x + 68} {y + 112}"/><path d="M{x + 13} {y + 70}h42M{x + 22} {y + 40}h24"/></g>'
        f'<circle class="tp" cx="{x + 34}" cy="{y - 2}" r="7" fill="{TALLY}"/>'
    )
    d.style(
        "@keyframes air{0%{transform:scale(.4);opacity:.0}20%{opacity:.7}100%{transform:scale(1.3);opacity:0}}"
        f".air{{transform-box:view-box;transform-origin:{x + 34}px {y - 2}px;animation:air 3s ease-out infinite}}.air2{{animation-delay:1.5s}}"
    )
    for c in ("air", "air air2"):
        d.add(f'<circle class="{c}" cx="{x + 34}" cy="{y - 2}" r="40" fill="none" stroke="{TALLY}" stroke-width="2" opacity="0"/>')


def one_tuner():
    W, H = 1280, 500
    d = Doc(W, H, "Antenna to tuner to Broadwave to every screen in the house, all on the same frame")
    box = card_open(d, 0, 0, W, H, lights=[(140, 260, 300, 240, CYAN, 0.12), (560, 230, 380, 260, BLUE, 0.30), (1030, 230, 360, 280, VIOLET, 0.18)])
    d.style("@keyframes tp{0%,100%{opacity:1}50%{opacity:.35}}.tp{animation:tp 2.4s ease-in-out infinite}")
    cy = 214
    # Antenna.
    antenna(d, 104, cy - 58)
    d.text(138, cy + 104, "Your antenna", 19, "bold", TEXT, anchor="middle")
    d.text(138, cy + 128, "free, over the air", 15, "body", TEXT3, anchor="middle")
    # Tuner.
    tx, tw, th = 268, 150, 56
    d.add(f'<rect x="{tx}" y="{cy - th / 2}" width="{tw}" height="{th}" rx="14" fill="#141B28" stroke="#FFFFFF" stroke-opacity=".16"/>')
    d.add(f'<circle cx="{tx + 22}" cy="{cy}" r="4" fill="#32D74B"/><circle cx="{tx + 36}" cy="{cy}" r="4" fill="#32D74B" opacity=".45"/>')
    d.text(tx + 52, cy + 6, "Tuner", 17, "semi", "#DCE3EE")
    d.text(tx + tw / 2, cy + 104, "One tuner", 19, "bold", TEXT, anchor="middle")
    d.text(tx + tw / 2, cy + 128, "one tune per channel", 15, "body", TEXT3, anchor="middle")
    # Broadwave.
    bx, bw, bh = 496, 236, 104
    d.define(
        "bwrim",
        f'<linearGradient id="bwrim" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="{CYAN}"/><stop offset=".5" stop-color="{BLUE}"/><stop offset="1" stop-color="{VIOLET}"/></linearGradient>',
    )
    d.style(f"@keyframes br{{0%,100%{{opacity:1}}50%{{opacity:.45}}}}.br{{animation:br 6s {EASE} infinite}}")
    glow(d, bx + bw / 2, cy, 220, 120, BLUE, 0.4, "br")
    d.add(f'<rect x="{bx}" y="{cy - bh / 2}" width="{bw}" height="{bh}" rx="20" fill="#0C1426" stroke="url(#bwrim)" stroke-width="2"/>')
    # Small wave mark.
    pts = wave_points(0, 44, 0, 7, 44, 0.2, 4)
    mark = smooth([(bx + 24 + px, cy - 10 + py) for px, py in pts])
    mark2 = smooth([(bx + 24 + px, cy - 6 + py) for px, py in wave_points(0, 44, 0, 6, 44, 1.2, 4)])
    d.add(f'<path d="{mark}" fill="none" stroke="{PALE}" stroke-width="3.5" stroke-linecap="round"/><path d="{mark2}" fill="none" stroke="{BLUE}" stroke-width="3.5" stroke-linecap="round"/>')
    d.text(bx + 80, cy - 2, "Broadwave", 24, "bold", TEXT, track=-0.01)
    d.text(bx + 80, cy + 24, "on your server", 15, "body", TEXT2)
    d.text(bx + bw / 2, cy + 104, "Docker, a NAS, or a mini PC", 15, "body", TEXT3, anchor="middle")
    # Links.
    d.style("@keyframes flow{to{stroke-dashoffset:-24}}.flow{animation:flow 2.4s linear infinite}")
    d.add(
        f'<path class="flow" d="M178 {cy}H{tx - 10}M{tx + tw + 10} {cy}H{bx - 10}" stroke="#9DB8FF" stroke-width="2" stroke-dasharray="3 9" stroke-linecap="round" fill="none"/>'
    )
    # Screens.
    img = frame(d, "bbb", 480, 270)
    rooms = [("tv", 900, 54, 168, 94, "TV", "Living room"), ("laptop", 900, 182, 112, 70, "Laptop", "Office"), ("tablet", 900, 290, 96, 72, "Tablet", "Kitchen"), ("phone", 900, 396, 38, 80, "Phone", "Bedroom")]
    paths = []
    for kind, x, y, w, h, name, room in rooms:
        mini(d, kind, x, y, w, h, img)
        my = y + h / 2
        paths.append(f"M{bx + bw + 10} {cy}C{bx + bw + 90} {cy} {x - 90} {fmt(my)} {x - 14} {fmt(my)}")
        lx = x + w + 30
        d.text(lx, my - 2, name, 19, "bold", TEXT)
        d.text(lx, my + 21, room, 15, "body", TEXT3)
    d.add(f'<path class="flow" d="{"".join(paths)}" stroke="#9DB8FF" stroke-width="2" stroke-dasharray="3 9" stroke-linecap="round" fill="none"/>')
    links = [(f"M178 {cy}H{tx - 10}", tx - 10 - 178, 0.0), (f"M{tx + tw + 10} {cy}H{bx - 10}", bx - 10 - tx - tw - 10, 0.5)]
    for path in paths:
        nums = [float(v) for v in re.findall(r"-?[\d.]+", path)]
        p0, c1, c2, p1 = (nums[0], nums[1]), (nums[2], nums[3]), (nums[4], nums[5]), (nums[6], nums[7])
        pts = [tuple((1 - t) ** 3 * p0[j] + 3 * (1 - t) ** 2 * t * c1[j] + 3 * (1 - t) * t * t * c2[j] + t ** 3 * p1[j] for j in (0, 1)) for t in [i / 40 for i in range(41)]]
        links.append((path, path_length(pts), 1.0))
    T = 3.0
    for i, (path, length, delay) in enumerate(links):
        # One packet per loop: a dash, then a gap longer than the link.
        cls = f"pk{i}"
        span = length + 40
        d.style(keyframes(cls, [(0, f"stroke-dashoffset:{fmt(20)}"), (45, f"stroke-dashoffset:{fmt(-length - 20)}"), (100, f"stroke-dashoffset:{fmt(-length - 20)}")])
                + f".{cls}{{animation:{cls} {T}s {EASE} {delay}s infinite}}")
        d.reduce(f".{cls}{{animation-duration:{T * 2}s;animation-delay:{delay * 2}s}}")
        da = f'stroke-dasharray="22 {fmt(span + 200)}" stroke-dashoffset="{fmt(-length - 20)}"'
        d.add(
            f'<path class="{cls}" d="{path}" fill="none" stroke="{CYAN}" stroke-width="14" stroke-linecap="round" opacity=".3" {da}/>'
            f'<path class="{cls}" d="{path}" fill="none" stroke="#E6FBFF" stroke-width="5" stroke-linecap="round" {da}/>'
        )
    d.text(76, H - 40, "Everyone on a channel shares one tune, and every room plays the same frame.", 19, "body", TEXT2)
    card_close(d, box)
    return d


# ---------------------------------------------------------------- terminal

CMD = "curl -fsSL https://raw.githubusercontent.com/wolfebase/broadwave/main/scripts/install.sh | bash"
# What scripts/install.sh prints on a fresh Linux box (see scripts/install.test.sh), with
# docker's own lines between. Long lines run off the window, as in a real terminal.
OUTPUT = [
    ("out", "Installing Broadwave (ghcr.io/wolfebase/broadwave:latest)."),
    ("cmd", "+ mkdir -p /home/pi/broadwave"),
    ("cmd", "+ mkdir -p /home/pi/broadwave/recordings"),
    ("cmd", "+ docker pull ghcr.io/wolfebase/broadwave:latest"),
    ("dock", "latest: Pulling from wolfebase/broadwave"),
    ("dock", "Status: Downloaded newer image for ghcr.io/wolfebase/broadwave:latest"),
    ("cmd", "+ docker run -d --name broadwave --restart unless-stopped --network host -e TZ=America/New_York -e PUID=1000 -e PGID=1000 --device /dev/dri"),
    ("dock", "7c1e93b0a5d2f4e86b1c09d7e3a5f2b84c6d1e0f9a7b3c5d2e4f6a8b0c1d3e5f"),
    ("blank", ""),
    ("ok", "Broadwave is running."),
    ("open", "Open http://192.168.0.42:8477"),
    ("out", "  Config:     /home/pi/broadwave"),
    ("out", "  Recordings: /home/pi/broadwave/recordings"),
    ("out", "Run this again to update. To remove it (folders stay): curl -fsSL https://raw.githubusercontent.com/wolfebase/broadwave/main/scripts/install.sh | bash -s -- --uninstall"),
]


def terminal():
    """16 s: opens on the finished install; fades; the command types itself; the output
    comes in order; it holds."""
    W = 1280
    lh, top, size = 26, 104, 16.5
    H = top + lh * (len(OUTPUT) + 1) + 34
    T = 16.0
    d = Doc(W, H, "One command installs Broadwave; then it finds your tuner, scans channels, and fills the guide")
    box = card_open(d, 0, 0, W, H, lights=[(260, 0, 420, 260, BLUE, 0.20), (1100, H, 420, 260, VIOLET, 0.16)])
    d.add(f'<rect x="0" y="0" width="{W}" height="52" fill="#FFFFFF" fill-opacity=".04"/><rect x="0" y="52" width="{W}" height="1" fill="#FFFFFF" fill-opacity=".08"/>')
    for i, c in enumerate(("#FF5F57", "#FEBC2E", "#28C840")):
        d.add(f'<circle cx="{32 + i * 22}" cy="26" r="6.5" fill="{c}"/>')
    d.text(W / 2, 31, "bash — install Broadwave", 14, "semi", TEXT3, anchor="middle")
    cw = d.measure("0", size, "mono")
    x = 40
    clip = d.uid("tc")
    d.define(clip, f'<clipPath id="{clip}"><rect x="0" y="53" width="{W - 34}" height="{H}"/></clipPath>')
    d.add(f'<g clip-path="url(#{clip})">')
    y = top - 14
    fade_out, type_from, type_to = 18.0, 22.0, 38.0
    d.style(
        keyframes("cmdin", [(0, "opacity:1"), (fade_out, "opacity:1"), (fade_out + 2, "opacity:0"), (type_from - 0.5, "opacity:0"), (type_from, "opacity:1"), (100, "opacity:1")])
        + f".cmdin{{animation:cmdin {T}s linear infinite}}"
        + "@keyframes blink{50%{opacity:0}}.blink{animation:blink 1s steps(1) infinite}"
    )
    d.text(x, y, "$", size, "monob", CYAN)
    # Typing: a clip over the command grows two letters at a time (SMIL, discrete).
    n = len(CMD)
    steps = list(range(0, n + 1, 2)) + ([n] if n % 2 else [])
    keys, widths, cursor = ["0"], [f"{cw * n + 4:.1f}"], [f"{x + cw * (n + 3):.1f}"]
    def at(pc, chars):
        keys.append(f"{pc / 100:.4f}")
        widths.append(f"{cw * chars + (4 if chars == n else 0):.1f}")
        cursor.append(f"{x + cw * (chars + 2 + (1 if chars == n else 0)):.1f}")
    at(type_from - 0.6, 0)
    for i, c in enumerate(steps):
        at(type_from + (type_to - type_from) * i / (len(steps) - 1), c)
    tclip = d.uid("ty")
    d.define(
        tclip,
        f'<clipPath id="{tclip}"><rect x="{fmt(x + cw * 2)}" y="{y - size}" width="{fmt(cw * n + 4)}" height="{size + 8}">'
        f'<animate attributeName="width" dur="{T}s" repeatCount="indefinite" calcMode="discrete" keyTimes="{";".join(keys)}" values="{";".join(widths)}"/></rect></clipPath>',
    )
    d.add(f'<g clip-path="url(#{tclip})">')
    d.text(x + cw * 2, y, CMD, size, "mono", TEXT, cls="cmdin")
    d.add("</g>")
    d.add(
        f'<rect class="blink" x="{fmt(x + cw * (n + 3))}" y="{y - size + 2}" width="{fmt(cw)}" height="{size + 3}" fill="{CYAN}" opacity=".8">'
        f'<animate attributeName="x" dur="{T}s" repeatCount="indefinite" calcMode="discrete" keyTimes="{";".join(keys)}" values="{";".join(cursor)}"/></rect>'
    )
    colors = {"out": "#DCE3EE", "cmd": TEXT3, "dock": "#7D899C", "ok": "#32D74B", "open": TEXT}
    shown = 0
    for i, (kind, line) in enumerate(OUTPUT):
        y = top - 14 + lh * (i + 1)
        if kind == "blank":
            continue
        a = type_to + 2.5 + shown * 2.0 + (4 if i >= 4 else 0) + (3 if i >= 7 else 0)
        shown += 1
        cls = f"ln{i}"
        d.style(
            keyframes(cls, [(0, "opacity:1"), (fade_out, "opacity:1"), (fade_out + 2, "opacity:0"), (a, "opacity:0"), (a + 0.8, "opacity:1"), (100, "opacity:1")])
            + f".{cls}{{animation:{cls} {T}s linear infinite}}"
        )
        d.add(f'<g class="{cls}">')
        if kind == "open":
            d.text(x, y, "Open ", size, "mono", TEXT)
            u = line[5:]
            d.text(x + cw * 5, y, u, size, "monob", "#7FA6FF")
            d.add(f'<rect x="{fmt(x + cw * 5)}" y="{y + 4}" width="{fmt(cw * len(u))}" height="1.2" fill="#7FA6FF" opacity=".7"/>')
        else:
            d.text(x, y, line, size, "monob" if kind == "ok" else "mono", colors[kind])
        d.add("</g>")
    d.add("</g>")
    d.define(
        "tfade",
        f'<linearGradient id="tfade" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="{BG1}" stop-opacity="0"/><stop offset="1" stop-color="{BG1}"/></linearGradient>',
    )
    d.add(f'<rect x="{W - 150}" y="53" width="150" height="{H - 53}" fill="url(#tfade)"/>')
    card_close(d, box, 0.04)
    return d


TICKER = [
    "Live TV", "Whole-Home Sync", "Watch together", "One tuner, every screen", "Follow a team",
    "Records until it's final", "Multiview", "True 60 fps", "5.1 surround", "ATSC 3.0",
    "Plex, Jellyfin, Emby, Channels", "No account", "Open source",
]


def ticker():
    """A slim strip of what it does, scrolling left. 22 s a lap; slower under reduced motion."""
    W, H = 1280, 64
    d = Doc(W, H, "Live TV, Whole-Home Sync, Watch together, one tuner for every screen, team recordings, multiview, true 60 fps, 5.1, ATSC 3.0, and more")
    box = card_open(d, 0, 0, W, H, lights=[(200, 32, 400, 90, BLUE, 0.22), (1080, 32, 400, 90, VIOLET, 0.18)])
    size, gap = 17, 30
    items, x = [], 0.0
    for word in TICKER:
        items.append(f'<circle cx="{fmt(x + 4)}" cy="{H / 2}" r="3.5" fill="{BLUE}"/>')
        items.append(d.text(x + 18, H / 2 + size * 0.36, word, size, "semi", "#DCE3EE", ret=True))
        x += 18 + d.measure(word, size, "semi") + gap
    lap = x
    run = "".join(items)
    d.define("tk", f'<g id="tk">{run}</g>')
    dur = lap / 95
    d.style(f"@keyframes tkr{{to{{transform:translateX(-{lap:.1f}px)}}}}.tkr{{animation:tkr {dur:.1f}s linear infinite}}")
    d.reduce(f".tkr{{animation-duration:{dur * 1.8:.1f}s}}")
    d.define(
        "tkf",
        '<linearGradient id="tkf" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#FFF" stop-opacity="0"/><stop offset=".08" stop-color="#FFF"/>'
        f'<stop offset=".92" stop-color="#FFF"/><stop offset="1" stop-color="#FFF" stop-opacity="0"/></linearGradient><mask id="tkm" maskContentUnits="userSpaceOnUse"><rect width="{W}" height="{H}" fill="url(#tkf)"/></mask>',
    )
    copies = int(W // lap) + 2
    d.add(f'<g mask="url(#tkm)"><g class="tkr" transform-origin="0 0">' + "".join(f'<use href="#tk" x="{fmt(24 + i * lap)}"/>' for i in range(copies)) + "</g></g>")
    card_close(d, box, 0.04)
    return d


# ---------------------------------------------------------------- footer and social


def footer():
    """Two strands that read on white and on GitHub dark, fading out at both ends."""
    W, H = 1280, 120
    d = Doc(W, H, "")
    P = 640
    d.style(
        f"@keyframes fd{{to{{transform:translateX({P}px)}}}}@keyframes fr{{to{{transform:translateX(-{P}px)}}}}"
        ".fd{animation:fd 12s linear infinite}.fr{animation:fr 17s linear infinite}"
    )
    d.define(
        "ff",
        '<linearGradient id="ff" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#FFF" stop-opacity="0"/><stop offset=".2" stop-color="#FFF"/>'
        '<stop offset=".8" stop-color="#FFF"/><stop offset="1" stop-color="#FFF" stop-opacity="0"/></linearGradient>'
        f'<mask id="fm" maskContentUnits="userSpaceOnUse"><rect width="{W}" height="{H}" fill="url(#ff)"/></mask>',
    )
    a = smooth(wave_points(-P, W + P, 58, 22, P, 0.0, 16))
    b = smooth(wave_points(-P, W + P, 64, 19, P, 1.05, 16))
    d.add(
        f'<g mask="url(#fm)" fill="none" stroke-linecap="round"><g class="fd"><path d="{a}" stroke="{VIOLET}" stroke-width="3" opacity=".75"/></g>'
        f'<g class="fr"><path d="{b}" stroke="{BLUE}" stroke-width="10" opacity=".14"/><path d="{b}" stroke="{BLUE}" stroke-width="3.5"/></g></g>'
    )
    return d


def social_png(path):
    """The repository's social preview: the hero, full bleed, rendered by Chrome."""
    chrome = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
    svg = hero(social=True).render()
    with tempfile.TemporaryDirectory() as tmp:
        html = os.path.join(tmp, "s.html")
        with open(html, "w") as fh:
            fh.write(f'<html><body style="margin:0;background:#070A12">{svg}</body></html>')
        png = os.path.join(tmp, "s.png")
        subprocess.run(
            [chrome, "--headless=new", "--disable-gpu", "--hide-scrollbars", "--window-size=1280,640", "--force-prefers-reduced-motion",
             f"--screenshot={png}", "file://" + html],
            check=True, capture_output=True,
        )
        im = Image.open(png).convert("RGB").crop((0, 0, 1280, 640))
        im.save(path, optimize=True)
    print(f"{os.path.basename(path):28} {os.path.getsize(path) / 1024:7.1f} KB")


def main():
    os.makedirs(OUT, exist_ok=True)
    hero().save("hero.svg")
    devices().save("devices.svg")
    tv_row_1().save("apple-tv-1.svg")
    tv_row_2().save("apple-tv-2.svg")
    tiles_row("tiles-1.svg", "Every room, the same frame. Watch together.", tile_sync, tile_together).save("tiles-1.svg")
    tiles_row("tiles-2.svg", "The guide is the centerpiece. Four channels at once.", tile_guide, tile_multiview).save("tiles-2.svg")
    tiles_row("tiles-3.svg", "A DVR that gets sports. True 60 fps.", tile_sports, tile_motion).save("tiles-3.svg")
    one_tuner().save("one-tuner.svg")
    terminal().save("terminal.svg")
    footer().save("footer.svg")
    ticker().save("ticker.svg")
    if "--png" in sys.argv:
        social_png(os.path.join(OUT, "social-preview.png"))


if __name__ == "__main__":
    main()
