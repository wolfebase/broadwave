import { useEffect, useRef } from "react";
import { shellMessages } from "../i18n/messages/shell";
import { bind } from "../i18n/runtime";
import { GuideIcon, HomeIcon, SearchIcon } from "../ui/icons";
import { followLink } from "./Nav";
import { focusRing } from "./remote";

const shell = bind(shellMessages);

/** A link the app does not know. Say so, and offer the places people usually mean. */
export function NotFound({ tv }: { tv: boolean }) {
  const first = useRef<HTMLAnchorElement>(null);
  useEffect(() => {
    if (tv) focusRing(first.current);
  }, [tv]);
  return (
    <div className="not-found">
      <span className="not-found-mark" aria-hidden="true">
        404
      </span>
      <h1>{shell("shell.notFoundTitle")}</h1>
      <p>{shell("shell.notFoundBody")}</p>
      <div className="not-found-links">
        <a ref={first} className="btn primary" href="/" onClick={(event) => followLink(event, "/")}>
          <HomeIcon /> {shell("nav.home")}
        </a>
        <a className="btn" href="/guide" onClick={(event) => followLink(event, "/guide")}>
          <GuideIcon /> {shell("nav.guide")}
        </a>
        <a className="btn" href="/search" onClick={(event) => followLink(event, "/search")}>
          <SearchIcon /> {shell("nav.search")}
        </a>
      </div>
    </div>
  );
}
