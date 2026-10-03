import { test } from 'node:test';
import assert from 'node:assert/strict';

import { describeMovement, getAvailableFunds, getBalanceRows, getLevelRows } from './present.js';

const formatTime = (createdAt) => `at ${createdAt}`;

test('Book con niveles: each row has price, volume and bar', () => {
  const rows = getLevelRows([
    { price: '100.00', volume: '30', orders: 2 },
    { price: '99.00', volume: '15', orders: 1 },
  ]);

  assert.deepEqual(rows, [
    { price: 'R$ 100.00', volume: '30', barPercent: 100 },
    { price: 'R$ 99.00', volume: '15', barPercent: 50 },
  ]);
});

test('Un lado vacío: no levels give no rows', () => {
  assert.deepEqual(getLevelRows([]), []);
});

test('Saldos: total and reserved of each currency', () => {
  const rows = getBalanceRows([
    { currency: 'BRL', available: '100.00', reserved: '900.00', total: '1000.00' },
    { currency: 'VIB', available: '5', reserved: '0', total: '5' },
  ]);

  assert.deepEqual(rows, [
    { currency: 'BRL', total: 'R$ 1,000.00', reserved: 'R$ 900.00' },
    { currency: 'VIB', total: '5 VIB', reserved: '0 VIB' },
  ]);
});

test('Cuenta nueva: a wallet not read yet shows zeros', () => {
  assert.deepEqual(getBalanceRows(null), [
    { currency: 'BRL', total: 'R$ 0.00', reserved: 'R$ 0.00' },
    { currency: 'VIB', total: '0 VIB', reserved: '0 VIB' },
  ]);
});

test('Movimientos recientes: a deposit is positive', () => {
  const movement = { type: 'DEPOSIT', currency: 'BRL', amount: '1500.00', createdAt: 't1' };

  assert.deepEqual(describeMovement(movement, formatTime), {
    label: 'Depósito', currency: 'BRL', amount: '+R$ 1,500.00', isPositive: true, time: 'at t1',
  });
});

test('Movimientos recientes: a reserve is negative', () => {
  const movement = { type: 'RESERVE', currency: 'BRL', amount: '900.00', createdAt: 't2' };

  assert.deepEqual(describeMovement(movement, formatTime), {
    label: 'Reservado para una orden', currency: 'BRL', amount: '-R$ 900.00', isPositive: false, time: 'at t2',
  });
});

test('Movimientos recientes: a release is positive', () => {
  const movement = { type: 'RELEASE', currency: 'BRL', amount: '27.00', createdAt: 't3' };

  const described = describeMovement(movement, formatTime);

  assert.equal(described.label, 'Fondos liberados');
  assert.equal(described.amount, '+R$ 27.00');
});

test('Movimientos recientes: the two sides of a trade', () => {
  const paid = describeMovement({ type: 'TRADE_PAID', currency: 'BRL', amount: '500.00', createdAt: 't4' }, formatTime);
  const received = describeMovement({ type: 'TRADE_RECEIVED', currency: 'VIB', amount: '5', createdAt: 't4' }, formatTime);

  assert.equal(paid.label, 'Trade: pagaste');
  assert.equal(paid.amount, '-R$ 500.00');
  assert.equal(received.label, 'Trade: recibiste');
  assert.equal(received.amount, '+5 VIB');
});

test('Movimientos recientes: a withdrawal is negative', () => {
  const movement = { type: 'WITHDRAWAL', currency: 'BRL', amount: '20.00', createdAt: 't5' };

  const described = describeMovement(movement, formatTime);

  assert.equal(described.label, 'Retiro');
  assert.equal(described.amount, '-R$ 20.00');
});

test('describeMovement keeps an unknown type as it came', () => {
  const movement = { type: 'FEE', currency: 'BRL', amount: '1.00', createdAt: 't6' };

  const described = describeMovement(movement, formatTime);

  assert.equal(described.label, 'FEE');
  assert.equal(described.amount, 'R$ 1.00');
});

test('getAvailableFunds reads what each currency has available', () => {
  const funds = getAvailableFunds([
    { currency: 'BRL', available: '100.00', reserved: '900.00', total: '1000.00' },
    { currency: 'VIB', available: '5', reserved: '0', total: '5' },
  ]);

  assert.deepEqual(funds, { BRL: '100.00', VIB: '5' });
});

test('getAvailableFunds is zero before the wallet is read', () => {
  assert.deepEqual(getAvailableFunds(null), { BRL: '0', VIB: '0' });
});
