import { useState } from "react";
import { copy } from "../../strings";
import { accents, readAccent, readTheme, saveAccent, saveTheme, themeChoices } from "../../theme/look";

const themeLabel = { system: "System", light: "Light", dark: "Dark" } as const;

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
              {themeLabel[item]}
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
              <span>{item.label}</span>
            </label>
          ))}
        </div>
        <span className="hint">{copy.settings.accentHint}</span>
      </div>
    </section>
  );
}
