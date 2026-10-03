import { test } from 'node:test';
import assert from 'node:assert/strict';

import { describeOrder, filterOrders, isActive } from './orders.js';

const formatTime = (createdAt) => `at ${createdAt}`;

function makeOrder(fields) {
  return {
    orderId: '01a0fa6b-28ea-7bbb-9edb-4ad34c008538',
    book: 'BRL-VIB',
    side: 'BUY',
    type: 'LIMIT',
    limit: '90.00',
    amount: null,
    quantity: '10',
    filledQuantity: '0',
    pendingQuantity: '10',
    avgPrice: null,
    status: 'OPEN',
    reason: null,
    createdAt: 't1',
    ...fields,
  };
}

const open = makeOrder({ orderId: 'a-open', status: 'OPEN' });
const filled = makeOrder({ orderId: 'a-filled', status: 'FILLED', filledQuantity: '10', pendingQuantity: '0', avgPrice: '90.00' });
const cancelled = makeOrder({ orderId: 'a-cancelled', status: 'CANCELLED' });

test('isActive is true for pending, open and partially filled orders', () => {
  assert.equal(isActive(makeOrder({ status: 'PENDING' })), true);
  assert.equal(isActive(makeOrder({ status: 'OPEN' })), true);
  assert.equal(isActive(makeOrder({ status: 'PARTIALLY_FILLED' })), true);
});

test('Orden final: isActive is false for filled, cancelled and rejected orders', () => {
  assert.equal(isActive(makeOrder({ status: 'FILLED' })), false);
  assert.equal(isActive(makeOrder({ status: 'CANCELLED' })), false);
  assert.equal(isActive(makeOrder({ status: 'REJECTED' })), false);
});

test('Solo las activas: the ACTIVE filter keeps only the active orders', () => {
  assert.deepEqual(filterOrders([open, filled, cancelled], 'ACTIVE'), [open]);
});

test('Ver todas: the ALL filter keeps every order in the same order', () => {
  assert.deepEqual(filterOrders([open, filled, cancelled], 'ALL'), [open, filled, cancelled]);
});

test('Sin órdenes: filterOrders of nothing is empty', () => {
  assert.deepEqual(filterOrders(null, 'ACTIVE'), []);
  assert.deepEqual(filterOrders([], 'ALL'), []);
});

test('Orden sin ejecución: nothing executed yet', () => {
  const described = describeOrder(open, formatTime);

  assert.equal(described.title, 'Compra 10 VIB · límite R$ 90.00');
  assert.equal(described.progress, 'Nada ejecutado todavía');
  assert.equal(described.statusLabel, 'Abierta');
  assert.equal(described.reason, '');
  assert.equal(described.canCancel, true);
  assert.equal(described.time, 'at t1');
});

test('Orden con ejecución parcial: filled of quantity at the average price', () => {
  const partial = makeOrder({ status: 'PARTIALLY_FILLED', filledQuantity: '4', pendingQuantity: '6', avgPrice: '90.50' });

  const described = describeOrder(partial, formatTime);

  assert.equal(described.progress, 'Ejecutado 4 de 10 VIB · prom. R$ 90.50');
  assert.equal(described.statusLabel, 'Parcial');
  assert.equal(described.canCancel, true);
});

test('Compra a mercado: executed VIB without a total', () => {
  const market = makeOrder({
    type: 'MARKET', limit: null, amount: '500.00', quantity: null, pendingQuantity: null,
    status: 'PENDING', filledQuantity: '3', avgPrice: '100.10',
  });

  const described = describeOrder(market, formatTime);

  assert.equal(described.title, 'Compra de mercado · gasta R$ 500.00');
  assert.equal(described.progress, 'Ejecutado 3 VIB · prom. R$ 100.10');
  assert.equal(described.statusLabel, 'Pendiente');
});

test('Venta a mercado: the title has the quantity', () => {
  const market = makeOrder({ side: 'SELL', type: 'MARKET', limit: null, quantity: '5', status: 'PENDING' });

  const described = describeOrder(market, formatTime);

  assert.equal(described.title, 'Venta de mercado · 5 VIB');
  assert.equal(described.sideLabel, 'Venta');
  assert.equal(described.isBuy, false);
});

test('Cancelada sin liquidez: the reason of the book', () => {
  const noLiquidity = makeOrder({ status: 'CANCELLED', reason: 'no_liquidity' });

  const described = describeOrder(noLiquidity, formatTime);

  assert.equal(described.statusLabel, 'Cancelada');
  assert.equal(described.reason, 'Sin liquidez en el mercado');
});

test('Cancelada por la persona: a cancelled order without reason', () => {
  const described = describeOrder(cancelled, formatTime);

  assert.equal(described.reason, 'Cancelada por ti');
});

test('Orden rechazada: insufficient funds', () => {
  const rejected = makeOrder({ status: 'REJECTED', reason: 'insufficient_funds' });

  const described = describeOrder(rejected, formatTime);

  assert.equal(described.statusLabel, 'Rechazada');
  assert.equal(described.reason, 'Fondos insuficientes');
});

test('Orden rechazada: invalid amount and an unknown reason', () => {
  const invalid = describeOrder(makeOrder({ status: 'REJECTED', reason: 'invalid_amount' }), formatTime);
  const unknown = describeOrder(makeOrder({ status: 'REJECTED', reason: 'other' }), formatTime);

  assert.equal(invalid.reason, 'Monto inválido');
  assert.equal(unknown.reason, 'other');
});

test('Orden final: a filled order has no reason and cannot be cancelled', () => {
  const described = describeOrder(filled, formatTime);

  assert.equal(described.statusLabel, 'Ejecutada');
  assert.equal(described.progress, 'Ejecutado 10 de 10 VIB · prom. R$ 90.00');
  assert.equal(described.reason, '');
  assert.equal(described.canCancel, false);
});

test('describeOrder carries the id to cancel and a short code to show', () => {
  const described = describeOrder(open, formatTime);

  assert.equal(described.orderId, 'a-open');
  assert.equal(described.shortId, '#a-open');
});

test('Cancelación pedida: an active order waiting to be cancelled has no button and says so', () => {
  const described = describeOrder(open, formatTime, true);

  assert.equal(described.canCancel, false);
  assert.equal(described.reason, 'Cancelación pedida, esperando que se aplique');
  assert.equal(described.statusLabel, 'Abierta');
});

test('Cancelación pedida: a final order ignores the request', () => {
  const described = describeOrder(cancelled, formatTime, true);

  assert.equal(described.reason, 'Cancelada por ti');
  assert.equal(described.canCancel, false);
});

test('without a request an active order keeps its button and no note', () => {
  const described = describeOrder(open, formatTime, false);

  assert.equal(described.canCancel, true);
  assert.equal(described.reason, '');
});
