import { useEffect, useState } from "react";
import { getStoragePlan } from "../../api";
import { libraryMessages } from "../../i18n/messages/library.ts";
import { bind } from "../../i18n/runtime.ts";
import { formatBytes } from "../../lib/format";
import type { Recording, StoragePlan, StoragePlanItem } from "../../types";
import { RecordAgain } from "./RecordAgain";
import { removalWhen, spaceLine } from "./storagePlanCopy";

const t = bind(libraryMessages);

/** Record it again needs only the recording's id and name. */
function againRecording(item: StoragePlanItem): Recording {
  return { id: item.id, channelId: 0, guideNumber: "", title: item.title, status: "complete", startedAt: "" };
}

function Rows({ items, again, timed }: { items: StoragePlanItem[]; again?: boolean; timed?: boolean }) {
  if (items.length === 0) return null;
  return (
    <ul className="source-list">
      {items.map((item) => {
        const when = timed ? removalWhen(item.when) : "";
        return (
          <li key={`${item.rule}-${item.id}`} className="source-row">
            <strong>{item.title}</strong>
            <span className="ch-tags">{item.reason}</span>
            {when ? <span className="hint">{when}</span> : null}
            <span className="hint">{formatBytes(item.bytes)}</span>
            {again ? <RecordAgain rec={againRecording(item)} /> : null}
          </li>
        );
      })}
    </ul>
  );
}

export function StoragePlanView() {
  const [plan, setPlan] = useState<StoragePlan | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let gone = false;
    getStoragePlan().then(
      (next) => {
        if (!gone) setPlan(next);
      },
      (err: unknown) => {
        if (!gone) setError(err instanceof Error ? err.message : String(err));
      },
    );
    return () => {
      gone = true;
    };
  }, []);

  return (
    <section className="page">
      <h2 className="section-title">{t("storage.title")}</h2>
      <p className="lede">{t("storage.lede")}</p>
      {plan ? <p className="hint">{spaceLine(plan)}</p> : null}
      {plan?.protectFavorites ? <p className="hint">{t("storage.favorites")}</p> : null}
      {plan?.roomNote ? <p className="hint">{plan.roomNote}</p> : null}
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
      {plan && plan.remove.length === 0 ? <p className="quiet-card">{t("storage.none")}</p> : null}
      {plan ? <Rows items={plan.remove} timed /> : null}
      {plan && plan.held.length > 0 ? (
        <>
          <h2 className="section-title">{t("storage.held")}</h2>
          <Rows items={plan.held} />
        </>
      ) : null}
      {plan && plan.again.length > 0 ? (
        <>
          <h2 className="section-title">{t("storage.again")}</h2>
          <Rows items={plan.again} again />
        </>
      ) : null}
    </section>
  );
}
