import { useEffect, useRef, useState, type ReactNode } from "react";
import { discover, getHome, type HomePlace } from "../../api";
import { useData } from "../../app/data";
import { setupMessages } from "../../i18n/messages/setup.ts";
import { settingsHubMessages } from "../../i18n/messages/settingsHub.ts";
import { bind } from "../../i18n/runtime.ts";
import { noteAfterFetch } from "./fetchNote";
import { AntennaIcon, PhoneIcon, ServerIcon, SpeakerIcon, TvIcon } from "../settings/icons";
import "./HomeList.css";

const t = bind(setupMessages);
const h = bind(settingsHubMessages);

type Group = "tuner" | "server" | "screen";

const kinds: Record<string, Parameters<typeof h>[0]> = {
  airplay: "home.kind.airplay",
  chromecast: "home.kind.chromecast",
  androidtv: "home.kind.androidtv",
  appletv: "home.kind.appletv",
  iphone: "home.kind.iphone",
  ipad: "home.kind.ipad",
  roku: "home.kind.roku",
  sonos: "home.kind.sonos",
};

function kindName(place: HomePlace): string {
  const key = kinds[place.kind];
  return key ? h(key as "home.kind.airplay") : place.detail || h("home.kind.other");
}

function PlaceIcon({ place }: { place: HomePlace }) {
  if (place.group === "tuner") return <AntennaIcon />;
  if (place.group === "server") return <ServerIcon />;
  if (place.kind === "iphone" || place.kind === "ipad") return <PhoneIcon />;
  if (/sonos|soundbar|speaker|homepod/i.test(`${place.kind} ${place.name}`)) return <SpeakerIcon />;
  return <TvIcon />;
}

/** A name in a "New iPhone found: …" notice marks that row as new. */
export function isNewPlace(place: HomePlace, notices: readonly string[]): boolean {
  const name = place.name.trim().toLocaleLowerCase();
  if (!name) return false;
  return notices.some((line) => line.toLocaleLowerCase().includes(name));
}

/**
 * Tuners, screens, and servers on this network. One action each. Nothing is added until a tap.
 * `groups` limits the list, so Settings can show tuners in one place and screens in another.
 */
export function HomeList({
  hideAdded = false,
  heading: Heading = "h3",
  groups,
  title,
  lede,
  limit = 6,
}: {
  hideAdded?: boolean;
  heading?: "h2" | "h3";
  groups?: readonly Group[];
  title?: string;
  lede?: string | null;
  limit?: number;
}) {
  const { refresh, notices, dismissNotice } = useData();
  const [places, setPlaces] = useState<HomePlace[]>([]);
  const [tunerAddress, setTunerAddress] = useState("");
  const [sharing, setSharing] = useState(false);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [scanning, setScanning] = useState(false);
  const [ready, setReady] = useState(false);
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [seen, setSeen] = useState<string[]>(notices);
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

  // The list says what is new now, so the notice that floats over the page can go.
  const [heard, setHeard] = useState(notices);
  if (heard !== notices) {
    setHeard(notices);
    if (notices.length > 0) setSeen((cur) => [...cur, ...notices]);
  }
  useEffect(() => {
    for (let i = 0; i < notices.length; i++) dismissNotice();
  }, [notices, dismissNotice]);

  const wanted = (place: HomePlace) => (!groups || groups.includes(place.group as Group)) && !(hideAdded && place.group === "tuner" && place.action === "added");
  const shown = places.filter(wanted);
  const order: Group[] = ["tuner", "server", "screen"];
  const sections = order.filter((group) => shown.some((place) => place.group === group));
  const header = title ?? t("setup.home");

  return (
    <div className="home-places">
      <div className="home-places-head">
        <div>
          <Heading>{header}</Heading>
          {lede === null ? null : <p className="dim">{lede ?? t("setup.homeLead")}</p>}
        </div>
        <button
          type="button"
          className="btn small ghost"
          disabled={busy || scanning}
          onClick={() => {
            setScanning(true);
            void load(true).finally(() => setScanning(false));
          }}
        >
          {scanning ? h("home.scanning") : t("setup.scanAgain")}
        </button>
      </div>
      {shown.length === 0 ? <p className="home-places-empty">{ready ? h("home.nothing") : h("home.searching")}</p> : null}
      {sections.map((group) => {
        const rows = shown.filter((place) => place.group === group);
        const all = open[group] ?? false;
        const visible = all || rows.length <= limit + 1 ? rows : rows.slice(0, limit);
        return (
          <section key={group} className={`home-group ${group}`} aria-label={group === "tuner" ? h("home.tuners") : group === "server" ? h("home.servers") : h("home.screens")}>
            {sections.length > 1 || !groups ? <p className="home-group-title">{group === "tuner" ? h("home.tuners") : group === "server" ? h("home.servers") : h("home.screens")}</p> : null}
            <ul className="home-rows">
              {visible.map((place) => (
                <Place key={place.id} place={place} fresh={isNewPlace(place, seen)}>
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
                </Place>
              ))}
            </ul>
            {rows.length > limit + 1 ? (
              <button type="button" className="btn small ghost home-more" aria-expanded={all} onClick={() => setOpen((cur) => ({ ...cur, [group]: !all }))}>
                {all ? h("home.showLess") : h("home.showAll", rows.length - limit)}
              </button>
            ) : null}
          </section>
        );
      })}
      {note ? (
        <p className="dim" role="status">
          {note}
        </p>
      ) : null}
    </div>
  );
}

function Place({ place, fresh, children }: { place: HomePlace; fresh: boolean; children: ReactNode }) {
  // A browser on the server's own machine reaches it over loopback (::1, 127.0.0.1): say so, not the address.
  const detail = isLoopback(place.addr)
    ? h("home.thisComputer")
    : [place.group === "screen" ? kindName(place) : place.detail, place.addr].filter(Boolean).join(" · ");
  return (
    <li className={fresh ? "home-row fresh" : "home-row"}>
      <span className="home-icon" aria-hidden="true">
        <PlaceIcon place={place} />
      </span>
      <span className="home-text">
        <span className="home-name">
          {place.name}
          {fresh ? <span className="tag new">{h("home.new")}</span> : null}
        </span>
        <span className="home-detail">{detail}</span>
      </span>
      {children}
    </li>
  );
}

function PlaceAction({ place, busy, onAdd, onUse }: { place: HomePlace; busy: boolean; onAdd: () => void; onUse: () => void }) {
  if (place.action === "add") {
    return (
      <button type="button" className="btn small" disabled={busy} aria-label={t("setup.addNamed", { name: place.name })} onClick={onAdd}>
        {h("home.add")}
      </button>
    );
  }
  if (place.action === "use") {
    return (
      <button type="button" className="btn small ghost" aria-label={t("setup.useNamed", { name: place.name })} onClick={onUse}>
        {h("home.use")}
      </button>
    );
  }
  if (place.action === "added") return <span className="home-state ok">{h("home.added")}</span>;
  if (place.action === "here") return <span className="home-state">{t("setup.onServer")}</span>;
  return null;
}

function isLoopback(addr: string | undefined): boolean {
  const host = (addr ?? "").trim().replace(/^\[|\]$/g, "").toLowerCase();
  return host === "::1" || host === "localhost" || /^127\./.test(host) || host === "::ffff:127.0.0.1";
}
