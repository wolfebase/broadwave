// A pause that lines this screen up with the room. The picture resumes when
// the drift has elapsed. Leaving cancels that resume: the timer otherwise
// starts a video the viewer already left, or one a newer pause replaced.

type Schedule = (fn: () => void, ms: number) => number;
type Cancel = (id: number) => void;

export class PauseHold {
  private id = 0;
  private generation = 0;
  private schedule: Schedule;
  private cancel: Cancel;

  constructor(schedule?: Schedule, cancel?: Cancel) {
    this.schedule = schedule ?? ((fn, ms) => window.setTimeout(fn, ms));
    this.cancel = cancel ?? ((id) => window.clearTimeout(id));
  }

  /** Resume runs after holdMs unless stop() ran, or a later arm replaced this one. */
  arm(holdMs: number, resume: () => void) {
    const generation = ++this.generation;
    this.cancel(this.id);
    this.id = this.schedule(() => {
      if (generation !== this.generation) return;
      this.id = 0;
      this.generation++;
      resume();
    }, holdMs);
  }

  stop() {
    this.generation++;
    this.cancel(this.id);
    this.id = 0;
  }
}
