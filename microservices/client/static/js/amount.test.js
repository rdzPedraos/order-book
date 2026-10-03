import { test } from 'node:test';
import assert from 'node:assert/strict';

import { formatAmount, groupThousands, parseAmount, parseUnits } from './amount.js';

test('parseAmount reads BRL in cents', () => {
  assert.equal(parseAmount('90.00', 2), 9000n);
  assert.equal(parseAmount('90', 2), 9000n);
  assert.equal(parseAmount(' 0.5 ', 2), 50n);
});

test('parseAmount reads VIB in whole units', () => {
  assert.equal(parseAmount('10', 0), 10n);
});

test('parseAmount rejects text that is not a number', () => {
  assert.equal(parseAmount('abc', 2), null);
  assert.equal(parseAmount('', 2), null);
  assert.equal(parseAmount('-5', 2), null);
});

test('Precio inválido: parseAmount rejects too many decimals', () => {
  assert.equal(parseAmount('90.123', 2), null);
});

test('Cantidad con decimales: parseAmount rejects decimals in VIB', () => {
  assert.equal(parseAmount('2.5', 0), null);
});

test('Monto inválido: parseAmount rejects zero', () => {
  assert.equal(parseAmount('0', 2), null);
  assert.equal(parseAmount('0.00', 2), null);
});

test('parseUnits accepts zero for balances', () => {
  assert.equal(parseUnits('0.00', 2), 0n);
  assert.equal(parseUnits('100.00', 2), 10000n);
  assert.equal(parseUnits('x', 2), null);
});

test('formatAmount writes the decimal string of the currency', () => {
  assert.equal(formatAmount(9000n, 2), '90.00');
  assert.equal(formatAmount(5n, 2), '0.05');
  assert.equal(formatAmount(10n, 0), '10');
});

test('groupThousands separates the integer part', () => {
  assert.equal(groupThousands('1000.00'), '1,000.00');
  assert.equal(groupThousands('1234567'), '1,234,567');
  assert.equal(groupThousands('90.00'), '90.00');
});
