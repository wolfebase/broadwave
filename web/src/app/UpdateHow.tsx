import { updateMessages } from "../i18n/messages/update";
import { bind } from "../i18n/runtime";
import { installKind, updateHint, updateSteps, type UpdateInstall } from "../lib/updateHow";

const t = bind(updateMessages);

/** The ways to update, or only this server's way when it says how it was installed. */
export function UpdateHow({ update }: { update: UpdateInstall }) {
  const hint = updateHint(update);
  const kind = installKind(update);
  const steps = updateSteps(kind);
  return (
    <div id="update-how" className="update-how">
      {hint ? <p>{hint}</p> : null}
      <p className="hint">{t(kind || hint ? "update.ledeKnown" : "update.lede")}</p>
      {hint ? null : (
        <dl>
          {steps.map((step) => (
            <div key={step.label}>
              <dt>{step.label}</dt>
              <dd>{step.text}</dd>
            </div>
          ))}
        </dl>
      )}
    </div>
  );
}
