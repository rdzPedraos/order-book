import { test } from 'node:test';
import assert from 'node:assert/strict';

import { applyReadings } from './readings.js';

const fulfilled = (value) => ({ status: 'fulfilled', value });
const rejected = () => ({ status: 'rejected', reason: new Error('down') });

const before = { book: 'old book', wallet: 'old wallet', movements: 'old movements' };

test('Cambio en el mercado: a new reading replaces the previous one', () => {
  const state = applyReadings(before, {
    book: fulfilled('new book'),
    wallet: fulfilled('new wallet'),
    movements: fulfilled('new movements'),
  });

  assert.deepEqual(state, { book: 'new book', wallet: 'new wallet', movements: 'new movements' });
});

test('Falla una consulta: the failed reading keeps what was shown', () => {
  const state = applyReadings(before, {
    book: fulfilled('new book'),
    wallet: rejected(),
    movements: fulfilled('new movements'),
  });

  assert.deepEqual(state, { book: 'new book', wallet: 'old wallet', movements: 'new movements' });
});

test('a reading that was not asked for keeps what was shown', () => {
  const state = applyReadings(before, { book: fulfilled('new book') });

  assert.deepEqual(state, { book: 'new book', wallet: 'old wallet', movements: 'old movements' });
});

test('applyReadings does not change the previous state', () => {
  applyReadings(before, { book: fulfilled('new book') });

  assert.equal(before.book, 'old book');
});
