/** Accessible name for an action that appears once per show, channel, or file. */
export function actionName(action: string, subject: string): string {
  const verb = action.replace(/\s+/g, " ").trim();
  const who = subject.replace(/\s+/g, " ").trim();
  return who ? `${verb} ${who}` : verb;
}
