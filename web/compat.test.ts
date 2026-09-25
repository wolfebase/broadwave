import assert from "node:assert/strict";
import { test } from "node:test";
import { gateApp, gateFeature, updateApp, updateServer } from "./src/lib/compat.ts";

const older = {
  id: "s",
  name: "Home",
  version: "0.9.0",
  apiVersion: 1,
  features: ["live", "renditions", "events", "dvr", "passes", "virtualChannels", "commercialDetection", "export"],
};

test("a server missing a feature hides that control", () => {
  assert.equal(gateFeature(older, "hdhrEmulation"), updateServer);
  assert.equal(updateServer, "Update your Broadwave server to use this");
  assert.equal(gateFeature({ features: ["hdhrEmulation", "live"] }, "hdhrEmulation"), null);
  assert.equal(gateFeature(null, "hdhrEmulation"), null);
  assert.equal(gateFeature({ apiVersion: 1 }, "hdhrEmulation"), null);
});

test("an older app is told to update, and a newer api does not block by itself", () => {
  assert.equal(gateApp({ minAppVersion: "1.1" }, "1.0"), updateApp);
  assert.equal(updateApp, "Update Broadwave to use this server");
  assert.equal(gateApp({ minAppVersion: "1.0" }, "1.0"), null);
  assert.equal(gateApp({ minAppVersion: "1.0.0" }, "1.0"), null);
  assert.equal(gateApp({ minAppVersion: "1.0" }, "1.0.1"), null);
  assert.equal(gateApp({ minAppVersion: "1.10" }, "1.9"), updateApp);
  assert.equal(gateApp({ minAppVersion: "1.9" }, "1.10"), null);
  assert.equal(gateApp(older, "1.0"), null);
  assert.equal(gateApp({ apiVersion: 99, version: "0.1.0", minAppVersion: "1.0" }, "1.0"), null);
  assert.equal(gateApp(null, "1.0"), null);
});
