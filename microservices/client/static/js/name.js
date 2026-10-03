// The made-up name of the person, sent as X-User-ID. The storage comes in as
// a parameter so the page can run when the browser blocks it.

const STORAGE_KEY = 'vibranium.name';

export function chooseName(draft, current) {
  const name = draft.trim();

  return name === '' ? current : name;
}

export function loadName(storage) {
  try {
    return storage.getItem(STORAGE_KEY) ?? '';
  } catch {
    return '';
  }
}

export function saveName(storage, name) {
  try {
    storage.setItem(STORAGE_KEY, name);
  } catch {
    // A blocked storage only means the name is asked again on the next visit.
  }
}
