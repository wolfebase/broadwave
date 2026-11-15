import { Component, type ReactNode } from "react";
import { shellMessages } from "../i18n/messages/shell";
import { bind } from "../i18n/runtime";

const shell = bind(shellMessages);

type Props = { path: string; online?: boolean; quiet?: boolean; children: ReactNode };

/**
 * Without this, a page that fails to load (the server went away) or throws takes
 * the whole app down: a blank screen that stays blank. The failure belongs to the
 * page it happened on, so going elsewhere renders normally. `quiet` leaves out
 * something beside the page instead of saying so.
 */
export class PageBoundary extends Component<Props, { failed?: string }> {
  state: { failed?: string } = {};

  static getDerivedStateFromError() {
    return { failed: location.pathname };
  }

  componentDidUpdate(prev: Props) {
    // A lazy page keeps its failed fetch; only a reload asks again, once the server answers.
    if (this.state.failed && this.props.online && !prev.online) location.reload();
  }

  render() {
    if (this.state.failed !== this.props.path) return this.props.children;
    return this.props.quiet ? null : (
      <div className="not-found" role="alert">
        <p>{shell("shell.pageFailed")}</p>
        <button type="button" className="btn primary" onClick={() => location.reload()}>
          {shell("shell.reload")}
        </button>
      </div>
    );
  }
}
