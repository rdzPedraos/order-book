// The rules of a deposit: the body the API expects and why it cannot be sent.

import { DECIMALS, parseAmount } from './amount.js';

const PROBLEMS = {
  BRL: 'Escribe un monto mayor que 0, con máximo 2 decimales.',
  VIB: 'Escribe un monto entero de VIB mayor que 0.',
};

export function buildDepositBody(currency, amount) {
  return { currency, amount: amount.trim() };
}

export function validateDeposit(currency, amount) {
  return parseAmount(amount, DECIMALS[currency]) === null ? PROBLEMS[currency] : '';
}
