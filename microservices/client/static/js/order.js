// The rules of an order: which fields each case shows, the body the API
// expects, what the order freezes and why it cannot be sent.

import { DECIMALS, formatAmount, groupThousands, parseAmount, parseUnits } from './amount.js';

const BOOK = 'BRL-VIB';

const PROBLEMS = {
  limit: 'Revisa el precio: máximo 2 decimales y mayor que 0.',
  quantity: 'La cantidad debe ser un número entero de VIB.',
  amount: 'Revisa el monto: máximo 2 decimales y mayor que 0.',
};

const FIELD_DECIMALS = { limit: DECIMALS.BRL, quantity: DECIMALS.VIB, amount: DECIMALS.BRL };

export function getVisibleFields(side, type) {
  if (type === 'LIMIT') return { limit: true, quantity: true, amount: false };
  if (side === 'BUY') return { limit: false, quantity: false, amount: true };

  return { limit: false, quantity: true, amount: false };
}

export function buildOrderBody(side, type, values) {
  const body = { book: BOOK, side };
  const visible = getVisibleFields(side, type);

  for (const field of Object.keys(visible)) {
    if (visible[field]) body[field] = values[field].trim();
  }

  return body;
}

export function getReserve(side, type, values) {
  const limit = parseAmount(values.limit, DECIMALS.BRL);
  const quantity = parseAmount(values.quantity, DECIMALS.VIB);
  const amount = parseAmount(values.amount, DECIMALS.BRL);

  if (side === 'SELL') return quantity === null ? null : { currency: 'VIB', units: quantity };
  if (type === 'MARKET') return amount === null ? null : { currency: 'BRL', units: amount };
  if (limit === null || quantity === null) return null;

  return { currency: 'BRL', units: limit * quantity };
}

function findInvalidField(side, type, values) {
  const visible = getVisibleFields(side, type);

  return Object.keys(visible).find(
    (field) => visible[field] && parseAmount(values[field], FIELD_DECIMALS[field]) === null,
  );
}

export function describeUnits(currency, units) {
  const text = groupThousands(formatAmount(units, DECIMALS[currency]));

  return currency === 'BRL' ? `R$ ${text}` : `${text} VIB`;
}

export function validateOrder(side, type, values, available) {
  const invalidField = findInvalidField(side, type, values);
  if (invalidField) return PROBLEMS[invalidField];

  const reserve = getReserve(side, type, values);
  const have = parseUnits(available[reserve.currency], DECIMALS[reserve.currency]) ?? 0n;
  if (reserve.units <= have) return '';

  return `Saldo insuficiente: necesitas ${describeUnits(reserve.currency, reserve.units)} y tienes ${describeUnits(reserve.currency, have)}.`;
}
