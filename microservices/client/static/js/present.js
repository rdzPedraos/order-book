// Turns what the API answers into the rows the page paints.

import { DECIMALS, formatAmount, groupThousands, parseUnits } from './amount.js';

const MOVEMENTS = {
  DEPOSIT: { label: 'Depósito', isPositive: true },
  WITHDRAWAL: { label: 'Retiro', isPositive: false },
  RESERVE: { label: 'Reservado para una orden', isPositive: false },
  RELEASE: { label: 'Fondos liberados', isPositive: true },
  TRADE_PAID: { label: 'Trade: pagaste', isPositive: false },
  TRADE_RECEIVED: { label: 'Trade: recibiste', isPositive: true },
};

const EMPTY_WALLET = [
  { currency: 'BRL', total: '0.00', reserved: '0.00' },
  { currency: 'VIB', total: '0', reserved: '0' },
];

export function formatMoney(currency, decimalString) {
  const text = groupThousands(decimalString);

  return currency === 'BRL' ? `R$ ${text}` : `${text} VIB`;
}

export function formatLocalTime(createdAt) {
  return new Date(createdAt).toLocaleString('es', {
    day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit',
  });
}

const EMPTY_ROW = { price: '', volume: '', total: '', barPercent: 0 };

// Levels come best price first; total is the volume from the best price to
// that level, which the bar shows against the whole side. With a size the list
// always has that many rows, so it does not change height when the book does.
export function getLevelRows(levels, size = levels.length) {
  const shown = levels.slice(0, size);
  const volumes = shown.map((level) => BigInt(level.volume));
  let running = 0n;
  const totals = volumes.map((volume) => (running += volume));
  const largest = totals.length === 0 ? 1n : totals[totals.length - 1];

  const rows = shown.map((level, index) => ({
    price: groupThousands(level.price),
    volume: groupThousands(level.volume),
    total: groupThousands(totals[index].toString()),
    barPercent: Number((totals[index] * 100n) / largest),
  }));

  return rows.concat(Array.from({ length: size - rows.length }, () => ({ ...EMPTY_ROW })));
}

export function getSpread(book) {
  const bestBid = book?.bids?.[0];
  const bestAsk = book?.asks?.[0];
  if (!bestBid || !bestAsk) return '';

  const gap = parseUnits(bestAsk.price, DECIMALS.BRL) - parseUnits(bestBid.price, DECIMALS.BRL);

  return formatMoney('BRL', formatAmount(gap, DECIMALS.BRL));
}

export function getBalanceRows(wallet) {
  const balances = wallet ?? EMPTY_WALLET;

  return balances.map((balance) => ({
    currency: balance.currency,
    total: formatMoney(balance.currency, balance.total),
    reserved: formatMoney(balance.currency, balance.reserved),
  }));
}

export function getAvailableFunds(wallet) {
  const funds = { BRL: '0', VIB: '0' };

  for (const balance of wallet ?? []) funds[balance.currency] = balance.available;

  return funds;
}

export function describeMovement(movement, formatTime = formatLocalTime) {
  const known = MOVEMENTS[movement.type];
  const sign = known === undefined ? '' : known.isPositive ? '+' : '-';

  return {
    label: known === undefined ? movement.type : known.label,
    currency: movement.currency,
    amount: sign + formatMoney(movement.currency, movement.amount),
    isPositive: known?.isPositive ?? false,
    time: formatTime(movement.createdAt),
  };
}
