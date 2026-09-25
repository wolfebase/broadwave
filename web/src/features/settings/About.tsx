import { blenderCredit } from "../../legal";

export function AboutPage() {
  return (
    <div className="page-wrap">
      <header className="page-header">
        <h1>About</h1>
      </header>
      <article className="quiet-card wide">
        <p>{blenderCredit}</p>
      </article>
    </div>
  );
}
