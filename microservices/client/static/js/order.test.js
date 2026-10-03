import { test } from 'node:test';
import assert from 'node:assert/strict';

import { buildOrderBody, describeUnits, getReserve, getVisibleFields, validateOrder } from './order.js';

const funds = { BRL: '1000.00', VIB: '5' };

test('Compra a límite: the body has limit and quantity', () => {
  const body = buildOrderBody('BUY', 'LIMIT', { limit: '90.00', quantity: '10', amount: '500.00' });

  assert.deepEqual(body, { book: 'BRL-VIB', side: 'BUY', limit: '90.00', quantity: '10' });
});

test('Venta a límite: the body has limit and quantity', () => {
  const body = buildOrderBody('SELL', 'LIMIT', { limit: '95.00', quantity: '3', amount: '' });

  assert.deepEqual(body, { book: 'BRL-VIB', side: 'SELL', limit: '95.00', quantity: '3' });
});

test('Compra a mercado: the body has only amount', () => {
  const body = buildOrderBody('BUY', 'MARKET', { limit: '90.00', quantity: '10', amount: '500.00' });

  assert.deepEqual(body, { book: 'BRL-VIB', side: 'BUY', amount: '500.00' });
});

test('Venta a mercado: the body has only quantity', () => {
  const body = buildOrderBody('SELL', 'MARKET', { limit: '90.00', quantity: '5', amount: '500.00' });

  assert.deepEqual(body, { book: 'BRL-VIB', side: 'SELL', quantity: '5' });
});

test('Campos según el caso: a limit order shows price and quantity', () => {
  assert.deepEqual(getVisibleFields('BUY', 'LIMIT'), { limit: true, quantity: true, amount: false });
  assert.deepEqual(getVisibleFields('SELL', 'LIMIT'), { limit: true, quantity: true, amount: false });
});

test('Campos según el caso: a market buy shows only the amount', () => {
  assert.deepEqual(getVisibleFields('BUY', 'MARKET'), { limit: false, quantity: false, amount: true });
});

test('Campos según el caso: a market sell shows only the quantity', () => {
  assert.deepEqual(getVisibleFields('SELL', 'MARKET'), { limit: false, quantity: true, amount: false });
});

test('getReserve of a limit buy is quantity times limit in BRL', () => {
  const reserve = getReserve('BUY', 'LIMIT', { limit: '90.00', quantity: '10', amount: '' });

  assert.deepEqual(reserve, { currency: 'BRL', units: 90000n });
});

test('getReserve of a market buy is the amount in BRL', () => {
  const reserve = getReserve('BUY', 'MARKET', { limit: '', quantity: '', amount: '500.00' });

  assert.deepEqual(reserve, { currency: 'BRL', units: 50000n });
});

test('getReserve of a sell is the quantity in VIB', () => {
  const reserve = getReserve('SELL', 'LIMIT', { limit: '95.00', quantity: '3', amount: '' });

  assert.deepEqual(reserve, { currency: 'VIB', units: 3n });
});

test('getReserve is null while a value is not valid', () => {
  assert.equal(getReserve('BUY', 'LIMIT', { limit: 'abc', quantity: '10', amount: '' }), null);
});

test('Precio inválido: the order is not sent', () => {
  const problem = validateOrder('BUY', 'LIMIT', { limit: '90.123', quantity: '10', amount: '' }, funds);

  assert.equal(problem, 'Revisa el precio: máximo 2 decimales y mayor que 0.');
});

test('Precio inválido: text is not a price', () => {
  const problem = validateOrder('BUY', 'LIMIT', { limit: 'abc', quantity: '10', amount: '' }, funds);

  assert.equal(problem, 'Revisa el precio: máximo 2 decimales y mayor que 0.');
});

test('Cantidad con decimales: the order is not sent', () => {
  const problem = validateOrder('SELL', 'LIMIT', { limit: '90.00', quantity: '2.5', amount: '' }, funds);

  assert.equal(problem, 'La cantidad debe ser un número entero de VIB.');
});

test('validateOrder checks the amount of a market buy', () => {
  const problem = validateOrder('BUY', 'MARKET', { limit: '', quantity: '', amount: '0' }, funds);

  assert.equal(problem, 'Revisa el monto: máximo 2 decimales y mayor que 0.');
});

test('Saldo insuficiente: the reserve is more than the available BRL', () => {
  const available = { BRL: '100.00', VIB: '0' };
  const problem = validateOrder('BUY', 'LIMIT', { limit: '90.00', quantity: '10', amount: '' }, available);

  assert.equal(problem, 'Saldo insuficiente: necesitas R$ 900.00 y tienes R$ 100.00.');
});

test('Saldo insuficiente: a sell needs the VIB', () => {
  const problem = validateOrder('SELL', 'MARKET', { limit: '', quantity: '6', amount: '' }, funds);

  assert.equal(problem, 'Saldo insuficiente: necesitas 6 VIB y tienes 5 VIB.');
});

test('validateOrder accepts an order the account can pay', () => {
  const problem = validateOrder('BUY', 'LIMIT', { limit: '90.00', quantity: '10', amount: '' }, funds);

  assert.equal(problem, '');
});

test('Orden enviada: describeUnits writes the reserve of a BRL order', () => {
  assert.equal(describeUnits('BRL', 90000n), 'R$ 900.00');
  assert.equal(describeUnits('BRL', 123456789n), 'R$ 1,234,567.89');
});

test('Orden enviada: describeUnits writes the reserve of a VIB order', () => {
  assert.equal(describeUnits('VIB', 6n), '6 VIB');
});
