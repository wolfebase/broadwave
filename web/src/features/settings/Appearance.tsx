import { useState } from "react";
import type { Settings } from "../../types";
import { settingsUiMessages } from "../../i18n/messages/settingsUi.ts";
import { settingsHubMessages } from "../../i18n/messages/settingsHub.ts";
import { bind } from "../../i18n/runtime.ts";
import { copy } from "../../strings";
import { accents, readAccent, readTheme, saveAccent, saveTheme, themeChoices } from "../../theme/look";
import { ChoiceRow, Group, Row } from "./rows";

const t = bind(settingsUiMessages);
const h = bind(settingsHubMessages);

// The accent names live here, not in look.ts, so the first script stays small.
const accentKeys = {
  blue: "accent.blue",
  teal: "accent.teal",
  green: "accent.green",
  orange: "accent.orange",
  purple: "accent.purple",
  rose: "accent.rose",
} as const;

export function accentName(id: keyof typeof accentKeys): string {
  return t(accentKeys[id]);
}

/** Theme and accent are this device's. Layout is the server's setting for every browser. */
export function Appearance({ settings, onChange }: { settings: Settings; onChange: (values: Partial<Settings>) => void }) {
  const [theme, setTheme] = useState(readTheme);
  const [accent, setAccent] = useState(readAccent);
  return (
    <>
      <Group lede={h("appearance.thisDevice")}>
        <ChoiceRow
          label={h("appearance.theme")}
          hint={h("appearance.themeHint")}
          value={theme}
          options={themeChoices.map((item) => ({ value: item, label: t(`appearance.${item}`) }))}
          onChange={(item) => setTheme(saveTheme(item))}
        />
        <Row label={h("appearance.accentLabel")} hint={copy.settings.accentHint} stack>
          <div className="accent-row" role="radiogroup" aria-label={h("appearance.accentLabel")}>
            {accents.map((item) => (
              <label key={item.id} className="accent-choice">
                <input type="radio" name="accent" value={item.id} checked={accent === item.id} onChange={() => setAccent(saveAccent(item.id))} />
                <span className="accent-dot" data-accent={item.id} />
                <span>{t(accentKeys[item.id])}</span>
              </label>
            ))}
          </div>
        </Row>
      </Group>
      <Group title={h("appearance.layoutGroup")}>
        <ChoiceRow
          label={copy.settings.layout}
          hint={h("appearance.layoutHint")}
          value={settings.layout === "desktop" || settings.layout === "tv" || settings.layout === "phone" ? settings.layout : "auto"}
          options={[
            { value: "auto", label: t("settings.layoutAuto") },
            { value: "desktop", label: t("settings.layoutDesktop") },
            { value: "tv", label: t("settings.layoutTv") },
            { value: "phone", label: t("settings.layoutPhone") },
          ]}
          onChange={(layout) => onChange({ layout })}
        />
      </Group>
    </>
  );
}
