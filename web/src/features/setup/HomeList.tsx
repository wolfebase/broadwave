import { useEffect, useRef, useState } from "react";
import { discover, getHome, type HomePlace } from "../../api";
import { useData } from "../../app/data";
import { setupMessages } from "../../i18n/messages/setup.ts";
import { bind } from "../../i18n/runtime.ts";
import { copy } from "../../strings";
import { noteAfterFetch } from "./fetchNote";
import "./setup.css";

const t = bind(setupMessages);

/** Tuners, screens, and servers on this network. One action each. Nothing is added until a tap. */
export function HomeList({ hideAdded = false, heading: Heading = "h3" }: { hideAdded?: boolean; heading?: "h2" | "h3" }) {
  const { refresh } = useData();
  const [places, setPlaces] = useState<HomePlace[]>([]);
  const [tunerAddress, setTunerAddress] = useState("");
  const [sharing, setSharing] = useState(false);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [ready, setReady] = useState(false);
  const scanN = useRef(0);

  function load(fresh: boolean) {
    const mine = ++scanN.current;
    const silent = t("setup.networkSilent");
    return getHome(fresh)
      .then((res) => {
        if (mine !== scanN.current) return;
        setPlaces(res.places ?? []);
        setTunerAddress(res.tunerAddress);
        setSharing(res.sharing);
        setNote((n) => noteAfterFetch(n, true, silent));
      })
      .catch(() => {
        if (mine !== scanN.current) return;
        setNote((n) => noteAfterFetch(n, false, silent));
      })
      .finally(() => {
        if (mine === scanN.current) setReady(true);
      });
  }

  useEffect(() => {
    void load(true);
  }, []);

  const shown = hideAdded ? places.filter((place) => !(place.group === "tuner" && place.action === "added")) : places;

  return (
    <div className="home-list">
      <Heading>{t("setup.home")}</Heading>
      <p className="dim">{t("setup.homeLead")}</p>
      {shown.length === 0 ? <p className="dim">{ready ? t("setup.nothingElse") : t("setup.searching")}</p> : null}
      <ul className="setup-list">
        {shown.map((place) => (
          <li key={place.id}>
            <strong>{place.name}</strong>
            <span className="dim">{[place.detail, place.addr].filter(Boolean).join(" · ")}</span>
            <PlaceAction
              place={place}
              busy={busy}
              onAdd={() => {
                setBusy(true);
                setNote("");
                void discover(place.addr)
                  .then(() => refresh(["devices", "channels"]))
                  .then(() => load(true))
                  .then(() => setNote(t("setup.tunerAdded")))
                  .catch((e: unknown) => setNote(e instanceof Error ? e.message : t("setup.tunerFailed")))
                  .finally(() => setBusy(false));
              }}
              onUse={() => {
                const line = sharing ? t("setup.hdhrAt", { address: tunerAddress }) : t("setup.hdhrTurnOn", { address: tunerAddress });
                setNote(line);
                void navigator.clipboard?.writeText(tunerAddress).catch(() => undefined);
              }}
            />
          </li>
        ))}
      </ul>
      {note ? <p className="dim">{note}</p> : null}
      <button
        type="button"
        className="btn"
        disabled={busy}
        onClick={() => {
          setBusy(true);
          void load(true).finally(() => setBusy(false));
        }}
      >
        {t("setup.scanAgain")}
      </button>
    </div>
  );
}

function PlaceAction({
  place,
  busy,
  onAdd,
  onUse,
}: {
  place: HomePlace;
  busy: boolean;
  onAdd: () => void;
  onUse: () => void;
}) {
  if (place.action === "add") {
    return (
      <button type="button" className="btn" disabled={busy} aria-label={t("setup.addNamed", { name: place.name })} onClick={onAdd}>
        {copy.sources.add}
      </button>
    );
  }
  if (place.action === "use") {
    return (
      <button type="button" className="btn" aria-label={t("setup.useNamed", { name: place.name })} onClick={onUse}>
        {t("setup.useTuner")}
      </button>
    );
  }
  const label = place.action === "added" ? t("setup.added") : place.action === "here" ? t("setup.onServer") : t("setup.onNetwork");
  return <span className={place.action === "added" ? "ok" : "dim"}>{label}</span>;
}
