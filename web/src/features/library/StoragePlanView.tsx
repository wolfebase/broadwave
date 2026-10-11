import { useEffect, useState } from "react";
import { getStoragePlan } from "../../api";
import { formatBytes } from "../../lib/format";
import type { Recording, StoragePlan, StoragePlanItem } from "../../types";
import { RecordAgain } from "./RecordAgain";

/** Record it again needs only the recording's id and name. */
function againRecording(item: StoragePlanItem): Recording {
  return { id: item.id, channelId: 0, guideNumber: "", title: item.title, status: "complete", startedAt: "" };
}

function Rows({ items, again }: { items: StoragePlanItem[]; again?: boolean }) {
  if (items.length === 0) return null;
  return (
    <ul className="source-list">
      {items.map((item) => (
        <li key={`${item.rule}-${item.id}`} className="source-row">
          <strong>{item.title}</strong>
          <span className="ch-tags">{item.reason}</span>
          <span className="hint">{formatBytes(item.bytes)}</span>
          {again ? <RecordAgain rec={againRecording(item)} /> : null}
        </li>
      ))}
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
      <h2 className="section-title">What would be removed</h2>
      <p className="lede">Nothing is deleted here. This is what the rules would remove.</p>
      {plan?.protectFavorites ? <p className="hint">Recordings on favorite channels stay.</p> : null}
      {plan?.roomNote ? <p className="hint">{plan.roomNote}</p> : null}
      {error ? (
        <p className="error" role="alert">
          {error}
        </p>
      ) : null}
      {plan && plan.remove.length === 0 ? <p className="quiet-card">Nothing would be removed.</p> : null}
      {plan ? <Rows items={plan.remove} /> : null}
      {plan && plan.held.length > 0 ? (
        <>
          <h2 className="section-title">Left alone</h2>
          <Rows items={plan.held} />
        </>
      ) : null}
      {plan && plan.again.length > 0 ? (
        <>
          <h2 className="section-title">Record again</h2>
          <Rows items={plan.again} again />
        </>
      ) : null}
    </section>
  );
}
