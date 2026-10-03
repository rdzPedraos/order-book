import { test } from 'node:test';
import assert from 'node:assert/strict';

import { chooseName, loadName, saveName } from './name.js';

function fakeStorage(initial = {}) {
  const items = { ...initial };

  return {
    items,
    getItem: (key) => (key in items ? items[key] : null),
    setItem: (key, value) => {
      items[key] = value;
    },
  };
}

const blockedStorage = {
  getItem: () => {
    throw new Error('blocked');
  },
  setItem: () => {
    throw new Error('blocked');
  },
};

test('Primera visita: with nothing saved there is no name', () => {
  assert.equal(loadName(fakeStorage()), '');
});

test('Recarga de la página: the saved name comes back', () => {
  const storage = fakeStorage();

  saveName(storage, 'luna_vib');

  assert.equal(loadName(storage), 'luna_vib');
});

test('a blocked storage does not break the page', () => {
  assert.equal(loadName(blockedStorage), '');
  assert.doesNotThrow(() => saveName(blockedStorage, 'luna_vib'));
});

test('Nombre elegido: the draft is the current name', () => {
  assert.equal(chooseName('luna_vib', ''), 'luna_vib');
});

test('Cambio de nombre: the new draft replaces the current name', () => {
  assert.equal(chooseName('beto', 'luna_vib'), 'beto');
});

test('Nombre vacío: the last valid name stays', () => {
  assert.equal(chooseName('', 'luna_vib'), 'luna_vib');
  assert.equal(chooseName('   ', 'luna_vib'), 'luna_vib');
});

test('chooseName trims the spaces around the draft', () => {
  assert.equal(chooseName('  luna_vib ', ''), 'luna_vib');
});
