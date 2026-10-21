import type { Channel, PassPreview } from "../../types";
import { scheduleMessages } from "../../i18n/messages/schedule.ts";
import { bind, weekdayShortNames } from "../../i18n/runtime.ts";
import { formatClock } from "../../time";

const t = bind(scheduleMessages);

/** What a rule would record in the next two weeks, and what it would push out. A timed recording counts times, not airings. */
export function PreviewLines({ preview, note, channels, timed = false }: { preview: PassPreview | null; note: string; channels: Channel[]; timed?: boolean }) {
  if (note) return <p className="hint" role="status">{note}</p>;
  if (!preview) return null;
  const records = preview.items.filter((item) => !item.skipped);
  const number = (id: number) => channels.find((ch) => ch.id === id)?.displayNumber ?? "";
  let head = timed ? t("schedule.timedPreviewNone") : t("schedule.previewNone");
  if (preview.items.length > 0) {
    if (records.length === preview.items.length) head = timed ? t("schedule.timedPreviewRecords", records.length) : t("schedule.previewRecords", records.length);
    else if (records.length === 0) head = timed ? t("schedule.timedPreviewNoneRecord", preview.items.length) : t("schedule.previewNoneRecord", preview.items.length);
    else head = timed ? t("schedule.timedPreviewSome", preview.items.length, { done: records.length }) : t("schedule.previewSome", preview.items.length, { done: records.length });
  }
  return (
    <div className="pass-preview" role="status" aria-live="polite">
      <p>{head}</p>
      {preview.items.length > 0 ? (
        <ul className="marker-list">
          {preview.items.slice(0, 5).map((item) => (
            <li key={`${item.airing.channelId}-${item.airing.start}`} className="codec">
              {formatDay(item.airing.start)} · {number(item.airing.channelId)} {item.airing.title}
              {item.skipped ? ` · ${item.conflict ? t("schedule.skippedTuner") : item.reason || t("schedule.skipped")}` : ""}
            </li>
          ))}
          {preview.items.length > 5 ? <li className="codec">{t("schedule.andMore", { count: preview.items.length - 5 })}</li> : null}
        </ul>
      ) : null}
      {preview.bumps.length > 0 ? (
        <>
          <p>{t("schedule.previewBump")}</p>
          <ul className="marker-list">
            {preview.bumps.map((item) => (
              <li key={`${item.passId}-${item.airing.channelId}-${item.airing.start}`} className="codec">
                {formatDay(item.airing.start)} · {number(item.airing.channelId)} {item.airing.title}
              </li>
            ))}
          </ul>
        </>
      ) : null}
    </div>
  );
}

/** HH:MM on this device's clock format. */
export function clockLabel(hhmm: string) {
  const [h, m] = hhmm.split(":").map(Number);
  const d = new Date();
  d.setHours(h, m, 0, 0);
  return formatClock(d);
}

export function formatDay(iso: string) {
  const date = new Date(iso);
  return `${weekdayShortNames()[date.getDay()]} ${date.getMonth() + 1}/${date.getDate()}, ${formatClock(date)}`;
}
