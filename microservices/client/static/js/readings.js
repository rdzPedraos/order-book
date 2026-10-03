// What the page shows, replaced by each answer of the polling. A reading that
// failed or was not asked for keeps what was shown before.

export function applyReadings(state, readings) {
  const next = { ...state };

  for (const name of Object.keys(readings)) {
    if (readings[name].status === 'fulfilled') next[name] = readings[name].value;
  }

  return next;
}
