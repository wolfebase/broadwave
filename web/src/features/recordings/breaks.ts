export type BreakMarker = { id: number; start: number; end: number };

/** What the player shows for one recording's commercial scan. */
export type BreakScan = {
  running: boolean;
  note: string;
  markers: BreakMarker[] | null;
};

export const idleBreakScan: BreakScan = { running: false, note: "", markers: null };

// The scan outlives the player page. Leaving and coming back must not start a second request.
export function createBreakScans() {
  const jobs = new Map<number, BreakScan>();
  const listeners = new Set<() => void>();
  const emit = () => {
    for (const fn of listeners) fn();
  };
  return {
    subscribe(fn: () => void) {
      listeners.add(fn);
      return () => {
        listeners.delete(fn);
      };
    },
    get(id: number): BreakScan {
      return jobs.get(id) ?? idleBreakScan;
    },
    begin(id: number): boolean {
      const current = jobs.get(id);
      if (current?.running) return false;
      jobs.set(id, { running: true, note: "", markers: current?.markers ?? null });
      emit();
      return true;
    },
    finish(id: number, markers: BreakMarker[], note: string) {
      jobs.set(id, { running: false, note, markers });
      emit();
    },
    fail(id: number, note: string) {
      const current = jobs.get(id);
      jobs.set(id, { running: false, note, markers: current?.markers ?? null });
      emit();
    },
  };
}

export const breakScans = createBreakScans();
