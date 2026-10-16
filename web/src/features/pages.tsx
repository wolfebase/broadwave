import { useCallback, useMemo, useRef, useState } from "react";
import { createVirtual, deleteRecording, setKeep, setWatched, stopRecording } from "../api";
import { pagesMessages } from "../i18n/messages/pagesCopy.ts";
import { bind } from "../i18n/runtime.ts";
import { useData } from "../app/data";
import { copy } from "../strings";
import { focusRing } from "../app/remote";
import { navigate, useRoute } from "../app/router";
import { Library } from "./library/Library";
import { StoragePlanView } from "./library/StoragePlanView";
import { nextEpisode } from "./library/model";
import { Play } from "./recordings/Play";
import { useTvStartFocus } from "./tvStart";
import { VirtualPlay } from "./recordings/Virtual";
import { Schedule } from "./schedule/Schedule";

const t = bind(pagesMessages);

export function RecordingsPage() {
  const { recordings, virtuals, refresh } = useData();
  const { params } = useRoute();
  const show = params.get("show")?.trim() ?? "";
  const storage = params.get("view") === "storage";
  const libraryRef = useRef<HTMLButtonElement>(null);
  const storageRef = useRef<HTMLButtonElement>(null);
  const openView = (next: "library" | "storage") => {
    const query = new URLSearchParams(params);
    if (next === "storage") query.set("view", "storage");
    else query.delete("view");
    const search = query.toString();
    navigate(search ? `/recordings?${search}` : "/recordings");
    focusRing(next === "storage" ? storageRef.current : libraryRef.current);
  };
  const listed =
    show === ""
      ? recordings
      : recordings.filter((rec) => rec.title.trim().toLocaleLowerCase() === show.toLocaleLowerCase());
  const [note, setNote] = useState("");
  const pageRef = useRef<HTMLDivElement>(null);
  useTvStartFocus(pageRef, ".media-card .btn.primary, .library-empty .btn, .library-chips .chip.on, .storage-plan button", `${storage}:${recordings.length > 0}`);
  return (
    <div className="page-wrap" ref={pageRef}>
      <header className="page-header">
        <h1>{t("pages.recordings")}</h1>
        <div className="guide-chips">
          <div className="segmented" role="group" aria-label={t("pages.recordings")}>
            <button ref={libraryRef} type="button" className={storage ? "seg" : "seg on"} aria-pressed={!storage} onClick={() => openView("library")}>
              {t("pages.library")}
            </button>
            <button ref={storageRef} type="button" className={storage ? "seg on" : "seg"} aria-pressed={storage} onClick={() => openView("storage")}>
              {t("pages.storage")}
            </button>
          </div>
        </div>
      </header>
      {storage ? (
        <StoragePlanView />
      ) : (
        <Library
          key={show}
          recordings={listed}
          show={show}
          note={note}
          onMany={async (action, recs) => {
            const results = await Promise.allSettled(recs.map((r) => (action === "delete" ? deleteRecording(r.id) : setWatched(r.id, action === "watched"))));
            const failed = results.filter((r) => r.status === "rejected") as PromiseRejectedResult[];
            const done = recs.length - failed.length;
            const said = action === "delete" ? t("pages.deletedMany", done) : action === "watched" ? t("pages.markedWatched", done) : t("pages.markedUnwatched", done);
            setNote(failed.length === 0 ? said : `${done > 0 ? `${said} ` : ""}${t("pages.couldNotChange", { count: failed.length, reason: String(failed[0].reason?.message ?? failed[0].reason) })}`);
            await refresh(["recordings"]);
          }}
          onPlay={(r) => navigate(`/play?recording=${r.id}`)}
          onWatched={(r, flag) => void setWatched(r.id, flag).then(() => refresh(["recordings"]))}
          onKeep={(r, keep) =>
            void setKeep(r.id, keep).then(
              () => refresh(["recordings"]),
              (err: unknown) => setNote(err instanceof Error ? err.message : String(err)),
            )
          }
          onDelete={(r) =>
            void deleteRecording(r.id).then(
              () => {
                setNote(t("pages.deleted", { title: r.title }));
                return refresh(["recordings"]);
              },
              (err: unknown) => setNote(err instanceof Error ? err.message : String(err)),
            )
          }
          onStop={(r) =>
            void stopRecording(r.id).then(() => {
              setNote(t("pages.stopped", { title: r.title }));
              return refresh(["recordings"]);
            })
          }
          onVirtual={(r) => {
            const used = new Set(virtuals.map((v) => v.number));
            let n = 900;
            while (used.has(String(n))) n++;
            void createVirtual(String(n), t("pages.virtualName", { title: r.title }), [r.id]).then(() => {
              setNote(t("pages.virtualNote", { number: n, title: r.title }));
              return refresh(["virtuals"]);
            });
          }}
        />
      )}
    </div>
  );
}

export function SchedulePage() {
  const { recordings, passes, refresh } = useData();
  const pageRef = useRef<HTMLDivElement>(null);
  useTvStartFocus(pageRef, ".page-wrap section button:not([disabled]), .page-wrap button:not([disabled])", passes.length);
  return (
    <div className="page-wrap" ref={pageRef}>
      <header className="page-header">
        <h1>{copy.schedule.title}</h1>
      </header>
      <Schedule recordings={recordings} passes={passes} onStop={() => void refresh(["recordings"])} onPasses={() => void refresh(["passes"])} />
    </div>
  );
}

export function PlayPage() {
  const { params } = useRoute();
  const { recordings, settings } = useData();
  const id = Number(params.get("recording") || 0);
  const rec = recordings.find((r) => r.id === id);
  const next = useMemo(() => (rec ? nextEpisode(recordings, rec) : undefined), [recordings, rec]);
  const onNext = useCallback(() => {
    if (next) navigate(`/play?recording=${next.id}`, true);
  }, [next]);
  if (!rec) return null;
  return (
    <div className="play-page">
      <Play
        recording={rec}
        pictureMode={settings.pictureMode ?? "broadcast"}
        autoplay={settings.autoplay !== "0"}
        next={next}
        onNext={onNext}
        onBack={() => window.history.back()}
      />
    </div>
  );
}

export function VirtualPage() {
  const { params } = useRoute();
  const { settings } = useData();
  return (
    <div className="play-page">
      <VirtualPlay id={Number(params.get("virtual") || 0)} pictureMode={settings.pictureMode ?? "broadcast"} onBack={() => navigate("/guide")} />
    </div>
  );
}
