import { useEffect, useId, useRef, useState } from "react";
import type { Channel, Device, TunerStatus } from "../../types";
import { addFree, addPlaylistFile, addSource, checkSignals, discover, findFree, getSignals, getTuners, lookHarder, request, sourceStatuses, startScan, type ChannelSignal, type FreeFeed, type SourceAdded, type SourceStatus } from "../../api";
import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { settingsHubMessages } from "../../i18n/messages/settingsHub.ts";
import { bind } from "../../i18n/runtime.ts";
import { actionName } from "../../lib/actionName";
import { copy } from "../../strings";
import { lastSeenPhrase } from "../../time";
import { navigate } from "../../app/router";
import { deviceScans, showFirmware } from "./deviceCard";
import { applySignalPoll, applyTunerPoll } from "./sourcePoll";
import { guideAfterFree, hitsAfterLook } from "../setup/lookHits";
import { playlistKeep } from "./playlistKeep";
import { signalWord, tunerHost } from "./tunerHealth";
import { Confirm } from "./Confirm";
import { Lineup } from "./Lineup";
import { Group, Row } from "./rows";
import { AntennaIcon } from "./icons";

const t = bind(settingsUiMessages);
const h = bind(settingsHubMessages);

type Patch = { enabled?: boolean; hidden?: boolean; customName?: string; customNumber?: string; guideKey?: string; twinChoice?: string };

/** A tuner taken away on purpose is retired: it stops counting as offline, and comes back by itself if it answers. */
function retireDevice(deviceId: string, retired: boolean) {
  return request<{ devices: Device[] }>(`/api/v1/devices/${encodeURIComponent(deviceId)}`, { method: "PATCH", body: JSON.stringify({ retired }) });
}

export function Sources({
  devices,
  channels,
  busy,
  error,
  onDiscover,
  onLookup,
  onPatch,
  onRemove,
  onRefresh,
}: {
  devices: Device[];
  channels: Channel[];
  busy: boolean;
  error: string;
  onDiscover: () => void;
  onLookup: (ip: string) => void;
  onPatch: (channel: Channel, patch: Patch) => void;
  onRemove: (deviceId: string) => void;
  onRefresh?: () => Promise<void> | void;
}) {
  const [tuners, setTuners] = useState<TunerStatus[]>([]);
  const [statuses, setStatuses] = useState<SourceStatus[]>([]);
  const [sourcesReady, setSourcesReady] = useState(false);
  const [scanning, setScanning] = useState("");
  const [checking, setChecking] = useState("");
  const [deviceNote, setDeviceNote] = useState<{ id: string; text: string } | null>(null);
  const [removing, setRemoving] = useState<Device | null>(null);
  useEffect(() => {
    let ticket = 0;
    let shown: TunerStatus[] = [];
    async function load() {
      const mine = ++ticket;
      let got: TunerStatus[] | null = null;
      let reported: SourceStatus[] | null;
      try {
        const res = await getTuners();
        got = res.tuners ?? [];
        const sources = await sourceStatuses();
        reported = sources.sources ?? [];
      } catch {
        reported = null;
      }
      const next = applyTunerPoll(mine, ticket, shown, got, reported);
      if (!next) return;
      shown = next.tuners;
      setTuners(next.tuners);
      if (next.statuses != null) setStatuses(next.statuses);
      setSourcesReady(true);
    }
    void load();
    const id = window.setInterval(() => void load(), 5000);
    return () => {
      ticket += 1;
      window.clearInterval(id);
    };
  }, []);

  async function itsBack(device: Device) {
    const name = device.friendlyName || device.modelNumber || device.deviceId;
    const host = tunerHost(device.baseUrl);
    setChecking(device.deviceId);
    setDeviceNote(null);
    try {
      if (device.retired) await retireDevice(device.deviceId, false);
      const res = await discover(host);
      const again = (res.devices ?? []).find((item) => item.deviceId === device.deviceId);
      setDeviceNote({ id: device.deviceId, text: again && !again.offline ? h("tuners.foundAgain", { name }) : h("tuners.stillGone", { name, address: host }) });
      await onRefresh?.();
    } catch {
      setDeviceNote({ id: device.deviceId, text: h("tuners.stillGone", { name, address: host }) });
    } finally {
      setChecking("");
    }
  }

  // An offline tuner is retired, so its channels and passes stay for when it returns.
  // One that still answers would come straight back, so it is forgotten instead.
  async function remove(device: Device) {
    if (device.offline) {
      try {
        await retireDevice(device.deviceId, true);
        await onRefresh?.();
      } catch {
        onRemove(device.deviceId);
      }
    } else {
      onRemove(device.deviceId);
    }
    setRemoving(null);
  }

  const others = unmatchedSources(statuses, devices);
  const count = devices.length;
  return (
    <>
      <Group
        title={h("tuners.group")}
        actions={
          <button type="button" className="btn small" onClick={onDiscover} disabled={busy}>
            {h("tuners.find")}
          </button>
        }
      >
        {count === 0 && others.length === 0 ? <p className="set-note">{h("tuners.none")}</p> : null}
        {devices.map((device) => {
          const status = statuses.find((item) => item.deviceId === device.deviceId);
          const name = device.friendlyName || device.modelNumber || device.deviceId;
          const retired = Boolean(device.retired);
          const offline = !retired && (Boolean(status?.health) || Boolean(device.offline));
          const scans = sourcesReady && deviceScans(status ? status.kind : undefined);
          const facts = [device.modelNumber, device.tunerCount > 0 ? h("tuners.count", device.tunerCount) : "", tunerHost(device.baseUrl)].filter(Boolean).join(" · ");
          const seen = lastSeenPhrase(device.lastSeen);
          return (
            <div key={device.deviceId} className={retired ? "tuner-row is-retired" : offline ? "tuner-row is-offline" : "tuner-row"}>
              <span className="tuner-icon" aria-hidden="true">
                <AntennaIcon />
              </span>
              <div className="tuner-text">
                <span className="tuner-name">
                  {name}
                  <span className={retired ? "state-pill" : offline ? "state-pill warn" : "state-pill ok"}>{retired ? h("tuners.retired") : offline ? h("tuners.offline") : h("tuners.working")}</span>
                </span>
                <span className="tuner-facts">{facts}</span>
                {retired ? (
                  <span className="tuner-help">{h("tuners.retiredHelp")}</span>
                ) : offline ? (
                  <span className="tuner-help">
                    {seen ? `${h("tuners.lastSeen", { when: seen })} ` : ""}
                    {h("tuners.offlineHelp")}
                  </span>
                ) : (
                  <>
                    {device.note ? <span className="tuner-facts">{device.note}</span> : null}
                    {status?.streamLimit ? <span className="tuner-facts">{h("tuners.streams", { used: status.streamsInUse ?? 0, limit: status.streamLimit })}</span> : null}
                    {showFirmware(device.firmwareVersion) ? (
                      <span className="tuner-facts">
                        {device.upgradeAvailable ? h("tuners.firmwareNewer", { version: device.firmwareVersion ?? "", newer: device.upgradeAvailable }) : h("tuners.firmware", { version: device.firmwareVersion ?? "" })}
                      </span>
                    ) : null}
                  </>
                )}
                {deviceNote?.id === device.deviceId ? (
                  <span className="tuner-help" role="status">
                    {deviceNote.text}
                  </span>
                ) : null}
              </div>
              <div className="tuner-actions">
                {offline || retired ? (
                  <button type="button" className="btn small" disabled={busy || checking !== ""} aria-label={h("tuners.backAria", { name })} onClick={() => void itsBack(device)}>
                    {checking === device.deviceId ? h("tuners.lookingBack") : h("tuners.back")}
                  </button>
                ) : scans ? (
                  <button
                    type="button"
                    className="btn small"
                    disabled={busy || scanning === device.deviceId}
                    aria-label={h("tuners.scanAria", { name })}
                    onClick={() => {
                      setScanning(device.deviceId);
                      void startScan(device.deviceId).finally(() => setScanning(""));
                    }}
                  >
                    {scanning === device.deviceId ? h("tuners.scanning") : h("tuners.scan")}
                  </button>
                ) : null}
                {retired ? null : (
                  <button type="button" className={offline ? "btn small" : "btn small ghost"} disabled={busy} aria-label={copy.sources.removeLabel(name)} onClick={() => setRemoving(device)}>
                    {h("tuners.remove")}
                  </button>
                )}
              </div>
            </div>
          );
        })}
        {others.map((item) => (
          <div key={item.id} className={item.health ? "tuner-row is-offline" : "tuner-row"}>
            <span className="tuner-icon" aria-hidden="true">
              <AntennaIcon />
            </span>
            <div className="tuner-text">
              <span className="tuner-name">
                {item.name}
                <span className={item.health ? "state-pill warn" : "state-pill ok"}>{item.health ? h("tuners.offline") : h("tuners.working")}</span>
              </span>
              <span className="tuner-facts">
                {item.streamLimit ? h("tuners.streams", { used: item.streamsInUse ?? 0, limit: item.streamLimit }) : t("sources.noStreamLimit")}
              </span>
              {item.health && !item.health.includes("://") ? <span className="tuner-help">{item.health}</span> : null}
            </div>
          </div>
        ))}
        {tuners.length > 0 ? <TunerUse tuners={tuners} /> : null}
        {error ? (
          <p className="set-note error" role="alert">
            {error}
          </p>
        ) : null}
      </Group>
      <Lineup channels={channels} devices={devices} onPatch={onPatch} />
      <SignalCheck />
      <FindTuners busy={busy} onLookup={onLookup} />
      <details className="set-disclosure">
        <summary>
          <span className="set-row-label">{h("tuners.otherSources")}</span>
          <span className="set-row-hint">{h("tuners.otherSourcesHint")}</span>
        </summary>
        <div className="set-card">
          <SourceAdd />
        </div>
      </details>
      <Group>
        <Row label={h("tuners.diagnostics")} hint={h("about.diagnosticsHint")}>
          <button type="button" className="btn small" onClick={() => navigate("/diagnostics")}>
            {h("tuners.diagnostics")}
          </button>
        </Row>
        <Row label={h("about.setup")} hint={h("about.setupHint")}>
          <button type="button" className="btn small" onClick={() => navigate("/setup")}>
            {h("tuners.setupAgain")}
          </button>
        </Row>
      </Group>
      {removing ? (
        <Confirm
          title={h("tuners.removeAria", { name: removing.friendlyName || removing.modelNumber || removing.deviceId })}
          body={<p>{removing.offline ? h("tuners.removeRetire", { name: removing.friendlyName || removing.deviceId }) : h("tuners.removeConfirm", { name: removing.friendlyName || removing.deviceId })}</p>}
          confirm={h("tuners.remove")}
          cancel={h("backups.cancel")}
          busy={busy}
          onConfirm={() => void remove(removing)}
          onCancel={() => setRemoving(null)}
        />
      ) : null}
    </>
  );
}

/** What each tuner is doing this minute. */
function TunerUse({ tuners }: { tuners: TunerStatus[] }) {
  return (
    <div className="tuner-use" aria-label={h("tuners.inUse")}>
      <span className="set-row-hint">{h("tuners.inUse")}</span>
      <ul>
        {tuners.map((tuner) => {
          const busy = Boolean(tuner.guide || tuner.target);
          const word = busy ? signalWord(tuner.strength, tuner.quality) : "";
          return (
            <li key={tuner.index} className={busy ? "busy" : ""}>
              <span className="tuner-slot">{h("tuners.slot", { n: tuner.index + 1 })}</span>
              <span className="tuner-doing">{tunerDoing(tuner)}</span>
              {word ? (
                <span className="tuner-signal" title={h("tuners.signalPercent", { strength: tuner.strength ?? 0, quality: tuner.quality ?? 0 })}>
                  {h("tuners.signal", { word: h(word) })}
                </span>
              ) : null}
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function tunerDoing(tuner: TunerStatus) {
  const number = tuner.guide ?? "";
  const name = tuner.name ?? "";
  if (tuner.ours) return number ? h("tuners.watching", { number, name }) : t("settings.thisServer");
  if (tuner.target) return number ? h("tuners.otherAppChannel", { number, name }) : h("tuners.otherApp");
  if (number) return h("tuners.watching", { number, name });
  return h("tuners.free");
}

function FindTuners({ busy, onLookup }: { busy: boolean; onLookup: (ip: string) => void }) {
  const [ip, setIp] = useState("");
  const [looking, setLooking] = useState(false);
  const [looked, setLooked] = useState(false);
  const [hits, setHits] = useState<{ kind: string; name: string; addr: string }[]>([]);
  const [freeFeeds, setFreeFeeds] = useState<FreeFeed[]>([]);
  const [freeGuide, setFreeGuide] = useState("");
  const [freeNote, setFreeNote] = useState("");
  const [findingFree, setFindingFree] = useState(false);
  const lookN = useRef(0);
  const freeN = useRef(0);
  const addressId = useId();
  return (
    <Group title={h("tuners.findGroup")} lede={h("tuners.findHint")}>
      <form
        className="set-row stack"
        onSubmit={(event) => {
          event.preventDefault();
          onLookup(ip.trim());
        }}
      >
        <div className="set-row-text">
          <label htmlFor={addressId} className="set-row-label">
            {h("tuners.addressLabel")}
          </label>
          <span className="set-row-hint">{copy.sources.addressHint}</span>
        </div>
        <div className="set-inline">
          <input id={addressId} className="set-input" value={ip} onChange={(event) => setIp(event.target.value)} placeholder={copy.sources.addressPlaceholder} autoComplete="off" spellCheck={false} />
          <button type="submit" className="btn" disabled={busy || ip.trim() === ""}>
            {copy.sources.lookup}
          </button>
        </div>
      </form>
      <div className="set-row">
        <div className="set-row-text">
          <span className="set-row-label">{copy.sources.look}</span>
          <span className="set-row-hint">{copy.sources.lookHint}</span>
        </div>
        <button
          type="button"
          className="btn small"
          disabled={busy || looking}
          onClick={() => {
            const mine = ++lookN.current;
            setLooking(true);
            setLooked(false);
            void lookHarder()
              .then((res) => {
                if (mine !== lookN.current) return;
                setHits((shown) => hitsAfterLook(mine, lookN.current, shown, true, res.found) ?? shown);
                setLooked(true);
              })
              .catch(() => {
                setHits((shown) => hitsAfterLook(mine, lookN.current, shown, false, undefined) ?? shown);
              })
              .finally(() => {
                if (mine === lookN.current) setLooking(false);
              });
          }}
        >
          {looking ? h("tuners.lookingBack") : copy.sources.look}
        </button>
      </div>
      {looked && hits.length === 0 ? <p className="set-note">{copy.sources.lookEmpty}</p> : null}
      {hits.map((hit) => (
        <div key={`${hit.kind}-${hit.addr}`} className="set-row">
          <div className="set-row-text">
            <span className="set-row-label">{hit.name}</span>
            <span className="set-row-hint">{hit.addr}</span>
          </div>
          <button type="button" className="btn small" disabled={busy} aria-label={actionName(copy.sources.add, hit.name)} onClick={() => onLookup(hit.addr)}>
            {copy.sources.add}
          </button>
        </div>
      ))}
      <div className="set-row">
        <div className="set-row-text">
          <span className="set-row-label">{copy.sources.free}</span>
          <span className="set-row-hint">{copy.sources.freeHint}</span>
        </div>
        <button
          type="button"
          className="btn small"
          disabled={busy || findingFree}
          onClick={() => {
            const mine = ++freeN.current;
            setFindingFree(true);
            void findFree()
              .then((res) => {
                if (mine !== freeN.current) return;
                setFreeFeeds((shown) => hitsAfterLook(mine, freeN.current, shown, true, res.found) ?? shown);
                const guide = guideAfterFree(true, res.found, res.guide);
                if (guide != null) setFreeGuide(guide);
                setFreeNote(res.found?.length ? "" : copy.sources.freeEmpty);
              })
              .catch(() => {
                setFreeFeeds((shown) => hitsAfterLook(mine, freeN.current, shown, false, undefined) ?? shown);
                if (mine === freeN.current) setFreeNote(copy.sources.freeEmpty);
              })
              .finally(() => {
                if (mine === freeN.current) setFindingFree(false);
              });
          }}
        >
          {findingFree ? h("tuners.lookingBack") : copy.sources.free}
        </button>
      </div>
      {freeFeeds.map((feed) => (
        <div key={feed.playlist} className="set-row">
          <div className="set-row-text">
            <span className="set-row-label">{feed.name}</span>
            <span className="set-row-hint">{feed.addr}</span>
          </div>
          <button
            type="button"
            className="btn small"
            disabled={busy}
            aria-label={actionName(copy.sources.add, feed.name)}
            onClick={() => {
              setFreeNote("");
              void addFree(feed)
                .then((res) => setFreeNote(res.message || t("sources.added")))
                .catch((err: unknown) => setFreeNote(err instanceof Error ? err.message : copy.sources.freeEmpty));
            }}
          >
            {copy.sources.add}
          </button>
        </div>
      ))}
      {freeGuide ? <pre className="set-note">{freeGuide}</pre> : null}
      {freeNote ? (
        <p className="set-note" role="status">
          {freeNote}
        </p>
      ) : null}
    </Group>
  );
}

// A source that already has a device row reports there. This list is only
// what the rows cannot show.
function unmatchedSources(statuses: SourceStatus[], devices: Device[]) {
  const known = new Set(devices.map((device) => device.deviceId));
  return statuses.filter((item) => !item.deviceId || !known.has(item.deviceId));
}

function SourceAdd() {
  const [kind, setKind] = useState("m3u");
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [groups, setGroups] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [note, setNote] = useState("");
  const [pick, setPick] = useState<SourceAdded | null>(null);
  const [chosen, setChosen] = useState<number[]>([]);
  const ids = { kind: useId(), name: useId(), url: useId(), user: useId(), pass: useId(), file: useId(), groups: useId() };
  const options = pick?.groups?.length ? pick.groups : (pick?.channels ?? []).map((ch) => ch.name);
  const login = kind === "xtream" || kind === "tvheadend";
  return (
    <form
      className="source-add"
      onSubmit={(event) => {
        event.preventDefault();
        const fail = (err: unknown) => setNote(err instanceof Error ? err.message : t("sources.addFailed"));
        const finish = (res: SourceAdded) => {
          if (res.pick) {
            setPick(res);
            setChosen([]);
            setNote(res.message || copy.sources.pick);
            return;
          }
          setPick(null);
          setChosen([]);
          setNote(kind === "folder" ? t("sources.addedFiles", { count: res.added ?? 0 }) : t("sources.added"));
        };
        let useGroups = groups;
        let useKeep = "";
        if (pick) {
          if (chosen.length === 0) {
            setNote(copy.sources.pick);
            return;
          }
          const pickedGroups = pick.groups ?? [];
          if (pickedGroups.length > 0) {
            useGroups = [...chosen]
              .filter((i) => i >= 0 && i < pickedGroups.length)
              .sort((a, b) => a - b)
              .map((i) => pickedGroups[i])
              .join(", ");
          } else {
            useKeep = playlistKeep(pick.channels ?? [], chosen);
          }
        }
        if (kind === "m3u" && file) {
          void addPlaylistFile(name, useGroups, file, useKeep).then(finish).catch(fail);
          return;
        }
        void addSource(kind, name, url, "", useGroups, useKeep, username, password).then(finish).catch(fail);
      }}
    >
      <div className="source-add-grid">
        <label className="field" htmlFor={ids.kind}>
          {t("sources.addSource")}
          <select id={ids.kind} value={kind} onChange={(event) => setKind(event.target.value)}>
            <option value="m3u">{t("sources.kind.playlist")}</option>
            <option value="xtream">{t("sources.kind.xtream")}</option>
            <option value="tvheadend">{t("sources.kind.tvheadend")}</option>
            <option value="channels">{t("sources.kind.channels")}</option>
            <option value="threadfin">{t("sources.kind.threadfin")}</option>
            <option value="xteve">{t("sources.kind.xteve")}</option>
            <option value="ersatztv">{t("sources.kind.ersatztv")}</option>
            <option value="dispatcharr">{t("sources.kind.dispatcharr")}</option>
            <option value="link">{t("sources.kind.link")}</option>
            <option value="folder">{t("sources.kind.folder")}</option>
          </select>
        </label>
        <label className="field" htmlFor={ids.name}>
          {t("sources.name")}
          <input id={ids.name} value={name} onChange={(event) => setName(event.target.value)} />
        </label>
        <label className="field wide" htmlFor={ids.url}>
          {copy.sources.address}
          <input
            id={ids.url}
            value={url}
            onChange={(event) => setUrl(event.target.value)}
            placeholder={kind === "folder" ? t("sources.folderPlaceholder") : kind === "xtream" ? t("sources.xtreamPlaceholder") : t("sources.playlistPlaceholder")}
            spellCheck={false}
          />
        </label>
        {login ? (
          <>
            <label className="field" htmlFor={ids.user}>
              {t("sources.username")}
              <input id={ids.user} value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="off" />
            </label>
            <label className="field" htmlFor={ids.pass}>
              {t("sources.password")}
              <input id={ids.pass} value={password} onChange={(event) => setPassword(event.target.value)} type="password" autoComplete="off" />
            </label>
          </>
        ) : null}
        {kind === "m3u" ? (
          <label className="field" htmlFor={ids.file}>
            {copy.sources.file}
            <input id={ids.file} type="file" accept=".m3u,.m3u8,.gz,application/gzip,application/vnd.apple.mpegurl" onChange={(event) => setFile(event.target.files?.[0] ?? null)} />
          </label>
        ) : null}
        {kind === "m3u" || kind === "xtream" || kind === "tvheadend" || kind === "channels" ? (
          <label className="field" htmlFor={ids.groups}>
            {copy.sources.groups}
            <input id={ids.groups} value={groups} onChange={(event) => setGroups(event.target.value)} placeholder={t("sources.groupsPlaceholder")} spellCheck={false} />
            {kind === "m3u" || kind === "xtream" ? <span className="hint">{copy.sources.groupsHint}</span> : null}
          </label>
        ) : null}
      </div>
      {kind === "channels" ? <p className="hint">{t("sources.channelsHint")}</p> : null}
      {options.length > 0 ? (
        <fieldset className="source-pick">
          <legend>{copy.sources.pick}</legend>
          {options.map((option, index) => (
            <label className="check" key={`${index}:${option}`}>
              <input
                type="checkbox"
                checked={chosen.includes(index)}
                onChange={(event) => setChosen((prev) => (event.target.checked ? [...prev, index] : prev.filter((item) => item !== index)))}
              />
              {option}
            </label>
          ))}
        </fieldset>
      ) : null}
      <div className="source-add-foot">
        <button type="submit" className="btn" disabled={(url.trim() === "" && file == null) || (login && (username.trim() === "" || password === ""))}>
          {copy.sources.add}
        </button>
        {note ? (
          <p className="hint" role="status">
            {note}
          </p>
        ) : null}
      </div>
    </form>
  );
}

function SignalCheck() {
  const [rows, setRows] = useState<ChannelSignal[]>([]);
  const [running, setRunning] = useState(false);
  const [note, setNote] = useState("");
  const [all, setAll] = useState(false);
  useEffect(() => {
    let ticket = 0;
    const load = () => {
      const mine = ++ticket;
      void getSignals()
        .then((res) => {
          const next = applySignalPoll(mine, ticket, res.running, res.channels ?? []);
          if (!next) return;
          setRows(next.rows);
          setRunning(next.running);
        })
        .catch((err: Error) => {
          if (mine !== ticket) return;
          setNote(err.message);
        });
    };
    load();
    const timer = window.setInterval(load, running ? 2000 : 15000);
    return () => {
      ticket += 1;
      window.clearInterval(timer);
    };
  }, [running]);
  const done = rows.filter((row) => row.verdict);
  const limit = 6;
  const shown = all ? done : done.slice(0, limit);
  return (
    <Group
      title={h("signal.group")}
      lede={h("signal.hint")}
      actions={
        <button
          type="button"
          className="btn small"
          disabled={running}
          onClick={() => {
            setNote("");
            void checkSignals()
              .then((res) => {
                setRunning(true);
                setNote(res.message ?? "");
              })
              .catch((err: Error) => setNote(err.message));
          }}
        >
          {running ? h("signal.running") : h("signal.run")}
        </button>
      }
    >
      {note ? (
        <p className="set-note" role="status">
          {note}
        </p>
      ) : null}
      {done.length === 0 ? <p className="set-note">{h("signal.empty")}</p> : null}
      {shown.length > 0 ? (
        <ul className="set-list">
          {shown.map((row) => (
            <li key={row.channelId} className="set-row">
              <div className="set-row-text">
                <span className="set-row-label">
                  <span className="lineup-num">{row.number}</span> {row.name}
                </span>
                <span className="set-row-hint">
                  {row.verdict}
                  {row.strength != null ? ` · ${h("tuners.signalPercent", { strength: row.strength, quality: row.quality ?? 0 })}` : ""}
                  {row.tip ? ` · ${row.tip}` : ""}
                </span>
              </div>
            </li>
          ))}
        </ul>
      ) : null}
      {done.length > limit ? (
        <div className="set-row more">
          <button type="button" className="btn small ghost" aria-expanded={all} onClick={() => setAll((v) => !v)}>
            {all ? h("backups.fewer") : h("rec.byShowMore", done.length - limit)}
          </button>
        </div>
      ) : null}
    </Group>
  );
}
