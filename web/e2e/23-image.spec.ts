import { execFileSync } from "node:child_process";
import { lstatSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { expect, test } from "./fixture";

// Checks that only mean something against the container (E2E_IMAGE): the
// health check, PUID/PGID, the time zone, and the software fallback.
const here = path.dirname(fileURLToPath(import.meta.url));
const image = process.env.E2E_IMAGE || "";
const port = Number(process.env.E2E_PORT || 18731);
const container = `broadwave-e2e-${port}`;

function run() {
  return JSON.parse(readFileSync(path.join(here, ".run/server.json"), "utf8")) as { base: string; config: string };
}

function docker(...args: string[]) {
  return execFileSync("docker", args, { encoding: "utf8" }).trim();
}

function strangers(dir: string, uid: number, gid: number, out: string[] = []) {
  for (const name of readdirSync(dir)) {
    const full = path.join(dir, name);
    const st = lstatSync(full);
    if (st.uid !== uid || st.gid !== gid) out.push(`${full} ${st.uid}:${st.gid}`);
    if (st.isDirectory()) strangers(full, uid, gid, out);
  }
  return out;
}

test.describe("the container", () => {
  test.skip(!image, "runs against the image only (E2E_IMAGE)");

  test("runs as PUID and PGID, owns its folder, keeps the time zone, and reports healthy", async () => {
    test.setTimeout(120_000);
    const uid = process.getuid!();
    const gid = process.getgid!();

    // Docker's health check runs `broadwave -healthcheck` every 30 s.
    await expect
      .poll(() => docker("inspect", "-f", "{{.State.Health.Status}}", container), { timeout: 90_000, intervals: [2_000] })
      .toBe("healthy");

    const status = docker("exec", container, "cat", "/proc/1/status");
    const field = (name: string) => (status.match(new RegExp(`^${name}:\\s*(.*)$`, "m"))?.[1] ?? "").trim().split(/\s+/);
    expect(field("Uid")).toEqual([uid, uid, uid, uid].map(String));
    expect(field("Gid")).toEqual([gid, gid, gid, gid].map(String));
    // Root's group goes with root. The runner has no GPU, so no device group stays either.
    expect(field("Groups")).not.toContain("0");

    expect(strangers(run().config, uid, gid)).toEqual([]);

    const zone = docker("exec", container, "printenv", "TZ");
    expect(zone).toBe("America/Chicago");
    docker("exec", container, "test", "-f", `/usr/share/zoneinfo/${zone}`);

    const res = await fetch(`${run().base}/api/v1/diagnostics`);
    expect(res.ok).toBe(true);
    const diag = (await res.json()) as { doctor?: { id: string }[]; encoder?: { name: string; hardware: boolean } };
    const notes = (diag.doctor ?? []).map((n) => n.id);
    expect(notes).not.toContain("owner");
    expect(notes).not.toContain("tz");
    // No /dev/dri on the runner: the picture comes from the processor.
    expect(diag.encoder?.hardware).toBe(false);
    expect(diag.encoder?.name).toBe("libx264");
  });
});
