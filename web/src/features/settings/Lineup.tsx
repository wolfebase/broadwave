import { useId, useMemo, useState } from "react";
import type { Channel, Device } from "../../types";
import { useLayout } from "../../app/layout";
import { settingsHubMessages } from "../../i18n/messages/settingsHub.ts";
import { bind } from "../../i18n/runtime.ts";
import { copy } from "../../strings";
import { lineupRows, matchesLineup, switchPatch, type LineupRow } from "./lineupModel";
import { Choice, Group, Switch } from "./rows";
import { SearchSmallIcon } from "./icons";

const h = bind(settingsHubMessages);

type Patch = { enabled?: boolean; hidden?: boolean; customName?: string; customNumber?: string; guideKey?: string; twinChoice?: string };

/** Every channel the tuners carry, dense enough to scan a hundred at a glance. */
export function Lineup({ channels, devices, onPatch }: { channels: Channel[]; devices: Device[]; onPatch: (channel: Channel, patch: Patch) => void }) {
  const layout = useLayout();
  const [query, setQuery] = useState("");
  const [extra, setExtra] = useState(false);
  const searchId = useId();
  const rows = useMemo(() => lineupRows(channels, devices), [channels, devices]);
  const hiddenCount = rows.filter((row) => row.extra).length;
  const shown = rows.filter((row) => (extra || !row.extra) && matchesLineup(row.channel, query));
  const manyTuners = new Set(channels.map((channel) => channel.deviceId)).size > 1;
  const table = layout === "desktop";
  return (
    <Group title={h("lineup.group")} lede={h("lineup.count", rows.length - hiddenCount)}>
      <div className="lineup-tools">
        <label className="lineup-search" htmlFor={searchId}>
          <SearchSmallIcon />
          <span className="sr-only">{h("lineup.search")}</span>
          <input id={searchId} type="search" value={query} placeholder={h("lineup.searchPlaceholder")} autoComplete="off" spellCheck={false} onChange={(event) => setQuery(event.target.value)} />
        </label>
        {hiddenCount > 0 ? (
          <label className="lineup-extra">
            <Switch checked={extra} label={h("lineup.showExtra")} onChange={setExtra} />
            <span>
              {h("lineup.showExtra")}
              <span className="set-row-hint">{h("lineup.extraCount", hiddenCount)}</span>
            </span>
          </label>
        ) : null}
      </div>
      {rows.length === 0 ? <p className="set-note">{h("lineup.none")}</p> : shown.length === 0 ? <p className="set-note">{h("lineup.empty")}</p> : null}
      {shown.length > 0 && table ? <LineupTable rows={shown} showTuner={manyTuners} onPatch={onPatch} /> : null}
      {shown.length > 0 && !table ? <LineupList rows={shown} showTuner={manyTuners} onPatch={onPatch} /> : null}
      <p className="set-note">{h("lineup.matchHint")}</p>
    </Group>
  );
}

function format(channel: Channel) {
  return [channel.hd ? "HD" : "SD", channel.videoCodec].filter(Boolean).join(" · ");
}

function StateControl({ row, onPatch }: { row: LineupRow; onPatch: (channel: Channel, patch: Patch) => void }) {
  const { channel, state } = row;
  const name = `${channel.displayNumber} ${channel.displayName}`;
  switch (state.kind) {
    case "switch":
      return <Switch checked={state.on} label={h("lineup.toggleAria", { number: channel.displayNumber, name: channel.displayName })} onChange={(on) => onPatch(channel, switchPatch(on))} />;
    case "pair":
      return (
        <Choice
          label={`${h("lineup.twinShows")}, ${name}`}
          value={state.choice}
          options={[
            { value: "atsc3", label: h("lineup.twin3") },
            { value: "atsc1", label: h("lineup.twin1") },
            { value: "both", label: h("lineup.twinBoth") },
          ]}
          onChange={(twinChoice) => onPatch(channel, { twinChoice })}
        />
      );
    case "shownAs":
      return <span className="lineup-state">{h("lineup.twinShownAs", { number: state.number })}</span>;
    case "encrypted":
      return <span className="lineup-state">{state.playsAs ? h("lineup.encryptedPlays", { number: state.playsAs }) : h("lineup.encrypted")}</span>;
    case "offAir":
      return <span className="lineup-state">{h("lineup.offAir")}</span>;
    case "duplicate":
      return <span className="lineup-state">{h("lineup.duplicateShort")}</span>;
  }
}

function stateNote(row: LineupRow): string {
  const { state, channel } = row;
  if (state.kind === "duplicate") return h("lineup.duplicate", { number: state.of, tuner: state.tuner || "?" });
  if (state.kind === "encrypted") return channel.playsAs ? copy.sources.playsAs(state.playsAs) : copy.sources.encrypted(channel.standard === "atsc3");
  if (state.kind === "pair") return state.twin ? h("lineup.pairOf", { number: state.twin }) : "";
  return "";
}

function EditField({ value, label, className, placeholder, onSave }: { value: string; label: string; className: string; placeholder?: string; onSave: (value: string) => void }) {
  return (
    <input
      key={value}
      className={className}
      aria-label={label}
      defaultValue={value}
      placeholder={placeholder}
      spellCheck={false}
      autoComplete="off"
      onBlur={(event) => {
        if (event.target.value.trim() !== value) onSave(event.target.value);
      }}
      onKeyDown={(event) => {
        if (event.key === "Enter") (event.target as HTMLInputElement).blur();
      }}
    />
  );
}

function LineupTable({ rows, showTuner, onPatch }: { rows: LineupRow[]; showTuner: boolean; onPatch: (channel: Channel, patch: Patch) => void }) {
  return (
    <div className="lineup-scroll">
      <table className="lineup-table">
        <thead>
          <tr>
            <th scope="col" className="col-on">{h("lineup.onGuide")}</th>
            <th scope="col" className="col-num">{h("lineup.number")}</th>
            <th scope="col">{h("lineup.name")}</th>
            <th scope="col">{h("lineup.match")}</th>
            <th scope="col">{h("lineup.details")}</th>
            {showTuner ? <th scope="col">{h("lineup.tuner")}</th> : null}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const { channel } = row;
            const note = stateNote(row);
            return (
              <tr key={channel.id} className={`lineup-tr ${row.state.kind}${row.extra ? " extra" : ""}`}>
                <td className="col-on">
                  <StateControl row={row} onPatch={onPatch} />
                </td>
                <td className="col-num">
                  <EditField className="lineup-input num" value={channel.displayNumber} label={`${channel.guideName} ${copy.sources.number}`} onSave={(customNumber) => onPatch(channel, { customNumber })} />
                </td>
                <td>
                  <EditField className="lineup-input" value={channel.displayName} label={`${channel.guideNumber} ${copy.sources.name}`} onSave={(customName) => onPatch(channel, { customName })} />
                </td>
                <td>
                  <EditField className="lineup-input" value={channel.guideKey ?? ""} label={`${channel.guideNumber} ${copy.sources.match}`} placeholder={h("lineup.matchPlaceholder")} onSave={(guideKey) => onPatch(channel, { guideKey })} />
                </td>
                <td className="col-details">
                  <span className="lineup-format">
                    {format(channel)}
                    {channel.standard === "atsc3" ? <span className="tag">3.0</span> : null}
                  </span>
                  {note ? <span className="lineup-note">{note}</span> : null}
                </td>
                {showTuner ? <td className="col-tuner">{row.tuner}</td> : null}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function LineupList({ rows, showTuner, onPatch }: { rows: LineupRow[]; showTuner: boolean; onPatch: (channel: Channel, patch: Patch) => void }) {
  const [editing, setEditing] = useState<number | null>(null);
  return (
    <ul className="source-list lineup-list">
      {rows.map((row) => {
        const { channel } = row;
        const open = editing === channel.id;
        const note = stateNote(row);
        const details = [format(channel), channel.standard === "atsc3" ? "3.0" : "", showTuner || row.state.kind === "duplicate" ? h("lineup.from", { tuner: row.tuner }) : ""].filter(Boolean).join(" · ");
        return (
          <li key={channel.id} className={`lineup-item ${row.state.kind}${row.extra ? " extra" : ""}${open ? " open" : ""}`}>
            <button type="button" className="lineup-main" aria-expanded={open} aria-label={h("lineup.editAria", { number: channel.displayNumber, name: channel.displayName })} onClick={() => setEditing(open ? null : channel.id)}>
              <span className="lineup-num">{channel.displayNumber}</span>
              <span className="lineup-text">
                <span className="lineup-name">{channel.displayName}</span>
                <span className="lineup-detail">{note || details}</span>
              </span>
            </button>
            <span className="lineup-control">
              <StateControl row={row} onPatch={onPatch} />
            </span>
            {open ? (
              <div className="lineup-editor">
                <label className="field">
                  {h("lineup.number")}
                  <EditField className="lineup-input" value={channel.displayNumber} label={`${channel.guideName} ${copy.sources.number}`} onSave={(customNumber) => onPatch(channel, { customNumber })} />
                </label>
                <label className="field">
                  {h("lineup.name")}
                  <EditField className="lineup-input" value={channel.displayName} label={`${channel.guideNumber} ${copy.sources.name}`} onSave={(customName) => onPatch(channel, { customName })} />
                </label>
                <label className="field">
                  {h("lineup.match")}
                  <EditField className="lineup-input" value={channel.guideKey ?? ""} label={`${channel.guideNumber} ${copy.sources.match}`} placeholder={h("lineup.matchPlaceholder")} onSave={(guideKey) => onPatch(channel, { guideKey })} />
                </label>
                {note ? <p className="set-row-hint">{details}</p> : null}
                <button type="button" className="btn small" onClick={() => setEditing(null)}>
                  {h("lineup.done")}
                </button>
              </div>
            ) : null}
          </li>
        );
      })}
    </ul>
  );
}
