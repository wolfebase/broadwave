import { useId, useState } from "react";
import type { Settings } from "../../types";
import { settingsHubMessages } from "../../i18n/messages/settingsHub.ts";
import { bind } from "../../i18n/runtime.ts";
import { copy } from "../../strings";
import { browserName } from "../../lib/events";
import { HomeList } from "../setup/HomeList";
import { PairedDevices } from "./PairedDevices";
import { RemoteAccess } from "./RemoteAccess";
import { saveScreenName } from "./screenName";
import { Group, Row } from "./rows";

const h = bind(settingsHubMessages);

export function DevicesSection({ settings, onChange }: { settings: Settings; onChange: (values: Partial<Settings>) => Promise<void> | void }) {
  return (
    <>
      <Group title={h("screen.group")}>
        <ScreenName />
      </Group>
      <div className="set-block">
        <PairedDevices deviceAuth={settings.deviceAuth ?? "0"} onChange={onChange} />
      </div>
      {/* Away from home is its own component. Its insides belong to the remote-access work. */}
      <div className="set-block set-remote">
        <RemoteAccess onChange={onChange} />
      </div>
      <div className="set-block">
        <HomeList heading="h3" groups={["server", "screen"]} title={h("home.group")} lede={h("home.lede")} />
      </div>
    </>
  );
}

function ScreenName() {
  const id = useId();
  const [screenName, setScreenName] = useState(browserName);
  return (
    <Row label={copy.settings.screenName} hint={h("screen.nameHint")} labelFor={id} stack>
      <input
        id={id}
        className="set-input"
        value={screenName}
        maxLength={40}
        spellCheck={false}
        autoComplete="off"
        placeholder={h("screen.namePlaceholder")}
        onChange={(event) => setScreenName(event.target.value)}
        onBlur={() => {
          if (screenName !== browserName()) setScreenName(saveScreenName(screenName));
        }}
        onKeyDown={(event) => {
          if (event.key !== "Enter") return;
          event.preventDefault();
          setScreenName(saveScreenName(screenName));
        }}
      />
    </Row>
  );
}
