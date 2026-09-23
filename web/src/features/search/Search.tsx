import { useEffect, useState, type FormEvent } from "react";
import { addPass, search } from "../../api";
import { useData } from "../../app/data";
import { navigate, useRoute } from "../../app/router";
import { spanLabel } from "../../lib/guide";
import type { Recording, SearchAiring } from "../../types";
import { SearchIcon } from "../../ui/icons";
import "./search.css";

export function SearchPage() {
  const { params } = useRoute();
  const { refresh } = useData();
  const initial = params.get("q") ?? "";
  const [draft, setDraft] = useState(initial);
  const [airings, setAirings] = useState<SearchAiring[]>([]);
  const [recordings, setRecordings] = useState<Recording[]>([]);
  const [note, setNote] = useState("");

  const active = initial.trim().length >= 2;

  useEffect(() => {
    const q = initial.trim();
    if (q.length < 2) return;
    let stop = false;
    search(q)
      .then((res) => {
        if (stop) return;
        setAirings(res.airings);
        setRecordings(res.recordings);
        setNote(res.airings.length === 0 && res.recordings.length === 0 ? "Nothing matches." : "");
      })
      .catch((err: unknown) => {
        if (!stop) setNote(err instanceof Error ? err.message : "Search failed.");
      });
    return () => {
      stop = true;
    };
  }, [initial]);

  function submit(event: FormEvent) {
    event.preventDefault();
    navigate(`/search?q=${encodeURIComponent(draft.trim())}`);
  }

  return (
    <div className="page-wrap search-page">
      <header className="page-header">
        <h1>Search</h1>
      </header>
      <form className="search-form" onSubmit={submit}>
        <SearchIcon />
        <input
          type="search"
          value={draft}
          placeholder="Shows, people, recordings"
          aria-label="Search shows, people, and recordings"
          onChange={(event) => setDraft(event.target.value)}
        />
      </form>
      {active && note ? <p className="hint">{note}</p> : null}
      {active && airings.length > 0 ? (
        <section>
          <h2 className="section-title">Guide</h2>
          <ul className="search-list">
            {airings.map((airing) => (
              <li key={airing.id}>
                <div>
                  <strong>{airing.title}</strong>
                  <span>
                    {airing.guideNumber} {airing.channelName} · {spanLabel(airing)}
                  </span>
                </div>
                <button
                  type="button"
                  className="btn"
                  onClick={() =>
                    void addPass(airing.title, airing.channelId).then(() => {
                      setNote(`Recording every ${airing.title}.`);
                      return refresh(["passes"]);
                    })
                  }
                >
                  Record every airing
                </button>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {active && recordings.length > 0 ? (
        <section>
          <h2 className="section-title">Recordings</h2>
          <ul className="search-list">
            {recordings.map((rec) => (
              <li key={rec.id}>
                <button type="button" className="search-rec" onClick={() => navigate(`/play?recording=${rec.id}`)}>
                  <strong>{rec.title}</strong>
                  <span>{rec.guideNumber}</span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  );
}
