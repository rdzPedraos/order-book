// Turns the orders the API answers into the rows of "Mis órdenes".

import { formatLocalTime, formatMoney } from './present.js';

const ACTIVE_STATUSES = ['PENDING', 'OPEN', 'PARTIALLY_FILLED'];

const STATUS_LABELS = {
  PENDING: 'Pendiente',
  OPEN: 'Abierta',
  PARTIALLY_FILLED: 'Parcial',
  FILLED: 'Ejecutada',
  CANCELLED: 'Cancelada',
  REJECTED: 'Rechazada',
};

const REASONS = {
  no_liquidity: 'Sin liquidez en el mercado',
  insufficient_funds: 'Fondos insuficientes',
  invalid_amount: 'Monto inválido',
};

export function isActive(order) {
  return ACTIVE_STATUSES.includes(order.status);
}

export function filterOrders(orders, filter) {
  const all = orders ?? [];

  return filter === 'ACTIVE' ? all.filter(isActive) : all;
}

function describeTitle(order) {
  const action = order.side === 'BUY' ? 'Compra' : 'Venta';
  if (order.type === 'LIMIT') return `${action} ${order.quantity} VIB · límite ${formatMoney('BRL', order.limit)}`;
  if (order.side === 'BUY') return `${action} de mercado · gasta ${formatMoney('BRL', order.amount)}`;

  return `${action} de mercado · ${order.quantity} VIB`;
}

function describeProgress(order) {
  if (order.filledQuantity === '0') return 'Nada ejecutado todavía';

  const of = order.quantity === null ? '' : ` de ${order.quantity}`;

  return `Ejecutado ${order.filledQuantity}${of} VIB · prom. ${formatMoney('BRL', order.avgPrice)}`;
}

function describeReason(order) {
  if (order.status === 'CANCELLED') return order.reason ? (REASONS[order.reason] ?? order.reason) : 'Cancelada por ti';
  if (order.status === 'REJECTED') return REASONS[order.reason] ?? order.reason ?? '';

  return '';
}

const CANCEL_REQUESTED = 'Cancelación pedida, esperando que se aplique';

export function describeOrder(order, formatTime = formatLocalTime, isCancelRequested = false) {
  const isWaiting = isCancelRequested && isActive(order);

  return {
    orderId: order.orderId,
    shortId: `#${order.orderId.slice(-6)}`,
    sideLabel: order.side === 'BUY' ? 'Compra' : 'Venta',
    isBuy: order.side === 'BUY',
    title: describeTitle(order),
    progress: describeProgress(order),
    statusLabel: STATUS_LABELS[order.status] ?? order.status,
    status: order.status,
    reason: isWaiting ? CANCEL_REQUESTED : describeReason(order),
    canCancel: isActive(order) && !isWaiting,
    time: formatTime(order.createdAt),
  };
}
