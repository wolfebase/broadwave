/** Smooth scrolling is motion. Reduced motion jumps to the spot instead. */
export function scrollBehavior(reduced: boolean, behavior: ScrollBehavior = "smooth"): ScrollBehavior {
  return reduced ? "auto" : behavior;
}
