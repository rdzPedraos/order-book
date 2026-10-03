// Turns what the API answers into the rows the page paints.

import { groupThousands } from './amount.js';

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

function formatMoney(currency, decimalString) {
  const text = groupThousands(decimalString);

  return currency === 'BRL' ? `R$ ${text}` : `${text} VIB`;
}

function formatLocalTime(createdAt) {
  return new Date(createdAt).toLocaleString('es', {
    day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit',
  });
}

export function getLevelRows(levels) {
  const volumes = levels.map((level) => BigInt(level.volume));
  const largest = volumes.reduce((max, volume) => (volume > max ? volume : max), 1n);

  return levels.map((level, index) => ({
    price: formatMoney('BRL', level.price),
    volume: level.volume,
    barPercent: Number((volumes[index] * 100n) / largest),
  }));
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
