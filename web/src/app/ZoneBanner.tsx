import { useState } from "react";
import { shellMessages } from "../i18n/messages/shell";
import { zoneMessages } from "../i18n/messages/zone";
import { bind } from "../i18n/runtime";
import { focusPastNotice } from "./remote";
import { browserZone, zoneProblem, zoneProblemKey, type ServerZone } from "../lib/serverZone";

const shell = bind(shellMessages);
const zone = bind(zoneMessages);
const hiddenKey = "broadwave-zone-hidden";

function readHidden(): string {
  try {
    return localStorage.getItem(hiddenKey) ?? "";
  } catch {
    return "";
  }
}

/** For an admin: the server's time zone is mistyped, missing, or not this browser's. */
export default function ZoneBanner({ server }: { server: ServerZone | null }) {
  const [hidden, setHidden] = useState(readHidden);
  const here = browserZone();
  const problem = zoneProblem(server, here.zone, here.offset);
  if (!problem || hidden === zoneProblemKey(problem)) return null;
  const text =
    problem.kind === "invalid"
      ? zone("zone.invalid", { zone: problem.zone, suggest: problem.suggest })
      : problem.kind === "unset"
        ? zone("zone.unset", { suggest: problem.suggest })
        : zone("zone.differs", { zone: problem.zone, hours: String(problem.hours), suggest: problem.suggest });
  const dismiss = () => {
    const key = zoneProblemKey(problem);
    setHidden(key);
    try {
      localStorage.setItem(hiddenKey, key);
    } catch {
      // A private window keeps it hidden for this visit.
    }
  };
  return (
    <div className="banner-update banner-zone" role="status">
      <p>{text}</p>
      <button
        type="button"
        className="btn small ghost"
        onClick={(event) => {
          focusPastNotice(event);
          dismiss();
        }}
      >
        {shell("shell.notNow")}
      </button>
    </div>
  );
}
