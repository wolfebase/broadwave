import { useState } from "react";
import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { bind } from "../../i18n/runtime.ts";
import { copy } from "../../strings";
import { accents, readAccent, readTheme, saveAccent, saveTheme, themeChoices } from "../../theme/look";

const t = bind(settingsUiMessages);

// The accent names live here, not in look.ts, so the first script stays small.
const accentKeys = {
  blue: "accent.blue",
  teal: "accent.teal",
  green: "accent.green",
  orange: "accent.orange",
  purple: "accent.purple",
  rose: "accent.rose",
} as const;

/** Appearance is this device's. It is not one of the server's settings. */
export function Appearance() {
  const [theme, setTheme] = useState(readTheme);
  const [accent, setAccent] = useState(readAccent);
  return (
    <section className="settings-section appearance">
      <h2 id="appearance-title">{copy.settings.appearance}</h2>
      <div className="field">
        <div className="segmented" role="group" aria-labelledby="appearance-title">
          {themeChoices.map((item) => (
            <button
              key={item}
              type="button"
              className={theme === item ? "seg on" : "seg"}
              aria-pressed={theme === item}
              onClick={() => setTheme(saveTheme(item))}
            >
              {t(`appearance.${item}`)}
            </button>
          ))}
        </div>
        <span className="hint">{copy.settings.appearanceHint}</span>
      </div>
      <div className="field">
        <span id="accent-label">{copy.settings.accent}</span>
        <div className="accent-row" role="radiogroup" aria-labelledby="accent-label">
          {accents.map((item) => (
            <label key={item.id} className="accent-choice">
              <input type="radio" name="accent" value={item.id} checked={accent === item.id} onChange={() => setAccent(saveAccent(item.id))} />
              <span className="accent-dot" data-accent={item.id} />
              <span>{t(accentKeys[item.id])}</span>
            </label>
          ))}
        </div>
        <span className="hint">{copy.settings.accentHint}</span>
      </div>
    </section>
  );
}
