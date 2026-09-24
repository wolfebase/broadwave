import { readdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { brotliCompressSync, constants, gzipSync } from "node:zlib";
import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";

// The server serves these when the browser accepts them. Hashed bundles stay immutable.
function precompress(): Plugin {
  return {
    name: "precompress",
    apply: "build",
    async closeBundle() {
      const root = path.resolve("../server/cmd/broadwave/assets/web");
      const files = await walk(root);
      await Promise.all(
        files.map(async (file) => {
          if (!/\.(html|js|css|svg|json)$/.test(file)) return;
          const body = await readFile(file);
          await writeFile(file + ".gz", gzipSync(body, { level: 9 }));
          await writeFile(file + ".br", brotliCompressSync(body, { params: { [constants.BROTLI_PARAM_QUALITY]: 6 } }));
        }),
      );
    },
  };
}

async function walk(dir: string): Promise<string[]> {
  const entries = await readdir(dir, { withFileTypes: true });
  const out: string[] = [];
  for (const entry of entries) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) out.push(...(await walk(full)));
    else out.push(full);
  }
  return out;
}

export default defineConfig({
  plugins: [react(), precompress()],
  build: {
    outDir: "../server/cmd/broadwave/assets/web",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:8477",
      "/media": "http://127.0.0.1:8477",
    },
  },
});
