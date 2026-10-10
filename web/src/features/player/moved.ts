// The player that sent its channel away is gone by the time the notice shows,
// so the app-wide listener says it.
type Listener = (name: string) => void;

const listeners = new Set<Listener>();

export function movedTo(name: string) {
  listeners.forEach((fn) => fn(name));
}

export function onMoved(fn: Listener) {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

/** Who sent a channel here, for the player. It can arrive before the player has loaded. */
export type SentNote = { id: number; text: string };

let pending: SentNote | null = null;
const players = new Set<(note: SentNote) => void>();

export function sentHere(note: SentNote) {
  if (players.size) players.forEach((fn) => fn(note));
  else pending = note;
}

export function onSentHere(fn: (note: SentNote) => void) {
  if (pending) fn(pending);
  pending = null;
  players.add(fn);
  return () => {
    players.delete(fn);
  };
}
