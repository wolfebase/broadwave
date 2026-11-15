import { Component, type ReactNode } from "react";
import { shellMessages } from "../i18n/messages/shell";
import { bind } from "../i18n/runtime";
import { isChunkError } from "../lib/chunkError";

const shell = bind(shellMessages);

type Props = {
  /** The page's address: going somewhere else clears a failure. */
  path: string;
  /** The server answers. A page that failed only because the server was away loads again when it is back. */
  online: boolean;
  /** Something beside the page (an alert, the mini player): on failure it is simply left out. */
  quiet?: boolean;
  children: ReactNode;
};

type State = { error: unknown };

/**
 * Without this, a page that fails to load or throws takes the whole app with it:
 * the screen goes blank and stays blank after the server is back.
 */
export class PageBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: unknown): State {
    return { error };
  }

  componentDidUpdate(prev: Props) {
    if (this.state.error == null) return;
    if (prev.path !== this.props.path) {
      this.setState({ error: null });
      return;
    }
    // A lazy page remembers its failed fetch, so only a reload fetches it again.
    if (isChunkError(this.state.error) && this.props.online && !prev.online) window.location.reload();
  }

  render() {
    if (this.state.error == null) return this.props.children;
    if (this.props.quiet) return null;
    const away = isChunkError(this.state.error);
    return (
      <div className="not-found" role="alert">
        <h1>{shell("shell.pageFailed")}</h1>
        <p>{away ? shell("shell.pageFailedAway") : shell("shell.pageFailedBug")}</p>
        <button type="button" className="btn primary" onClick={() => window.location.reload()}>
          {shell("shell.reload")}
        </button>
      </div>
    );
  }
}
