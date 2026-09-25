import { useEffect, useState } from "react";
import { discover, getHome, type HomePlace } from "../../api";
import { useData } from "../../app/data";
import "./setup.css";

/** Tuners, screens, and servers on this network. One action each. Nothing is added until a tap. */
export function HomeList({ hideAdded = false }: { hideAdded?: boolean }) {
  const { refresh } = useData();
  const [places, setPlaces] = useState<HomePlace[]>([]);
  const [tunerAddress, setTunerAddress] = useState("");
  const [sharing, setSharing] = useState(false);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [ready, setReady] = useState(false);

  function load(fresh: boolean) {
    return getHome(fresh)
      .then((res) => {
        setPlaces(res.places ?? []);
        setTunerAddress(res.tunerAddress);
        setSharing(res.sharing);
      })
      .catch(() => setNote("This network did not answer."))
      .finally(() => setReady(true));
  }

  useEffect(() => {
    void load(true);
  }, []);

  const shown = hideAdded ? places.filter((place) => !(place.group === "tuner" && place.action === "added")) : places;

  return (
    <div className="home-list">
      <h3>Your home</h3>
      <p className="dim">Tuners, screens, and servers on this network. Nothing is added until you tap.</p>
      {shown.length === 0 ? <p className="dim">{ready ? "Nothing else answered yet." : "Searching…"}</p> : null}
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
                  .then(() => setNote("Tuner added."))
                  .catch((e: unknown) => setNote(e instanceof Error ? e.message : "That tuner did not answer."))
                  .finally(() => setBusy(false));
              }}
              onUse={() => {
                const line = sharing
                  ? `Add an HDHomeRun at ${tunerAddress}.`
                  : `Turn on Act as an HDHomeRun, then add ${tunerAddress}.`;
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
        Scan again
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
      <button type="button" className="btn" disabled={busy} onClick={onAdd}>
        Add
      </button>
    );
  }
  if (place.action === "use") {
    return (
      <button type="button" className="btn" onClick={onUse}>
        Use as tuner
      </button>
    );
  }
  const label = place.action === "added" ? "Added" : place.action === "here" ? "On this server" : "On this network";
  return <span className={place.action === "added" ? "ok" : "dim"}>{label}</span>;
}
