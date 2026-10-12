import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { bind } from "../../i18n/runtime.ts";
import { blenderCreditText } from "../../legal";

const t = bind(settingsUiMessages);

export function AboutPage() {
  return (
    <div className="page-wrap">
      <header className="page-header">
        <h1>{t("about.title")}</h1>
      </header>
      <article className="quiet-card wide">
        <p>{blenderCreditText()}</p>
      </article>
    </div>
  );
}
