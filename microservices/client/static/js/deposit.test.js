import { test } from 'node:test';
import assert from 'node:assert/strict';

import { buildDepositBody, validateDeposit } from './deposit.js';

test('Cargar BRL: the body has the currency and the decimal string', () => {
  assert.deepEqual(buildDepositBody('BRL', '150.00'), { currency: 'BRL', amount: '150.00' });
});

test('Cargar VIB: the body has the currency and the decimal string', () => {
  assert.deepEqual(buildDepositBody('VIB', '10'), { currency: 'VIB', amount: '10' });
});

test('Monto inválido: zero is not a deposit', () => {
  assert.equal(validateDeposit('BRL', '0'), 'Escribe un monto mayor que 0, con máximo 2 decimales.');
});

test('Monto inválido: BRL with three decimals', () => {
  assert.equal(validateDeposit('BRL', '10.555'), 'Escribe un monto mayor que 0, con máximo 2 decimales.');
});

test('Monto inválido: VIB with decimals', () => {
  assert.equal(validateDeposit('VIB', '1.5'), 'Escribe un monto entero de VIB mayor que 0.');
});

test('validateDeposit accepts a valid amount', () => {
  assert.equal(validateDeposit('BRL', '150.00'), '');
  assert.equal(validateDeposit('VIB', '10'), '');
});
