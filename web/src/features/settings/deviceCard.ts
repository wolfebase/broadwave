// A tuner found on the network has no source row. A playlist, a link, and a
// folder do, and they do not scan. The device itself has no kind field.
const scanKinds = new Set(["hdhomerun", "hdhr-compatible"]);

export function deviceScans(sourceKind: string | undefined): boolean {
  if (sourceKind == null) return true;
  return scanKinds.has(sourceKind);
}

export function showFirmware(version: string | undefined): boolean {
  return Boolean(version && version.trim() !== "");
}
