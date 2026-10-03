// Amounts travel as decimal strings. Here they become BigInt in the minimal
// unit of their currency (cents of BRL, whole VIB), never a floating point.

export const DECIMALS = { BRL: 2, VIB: 0 };

export function parseUnits(text, decimals) {
  const value = String(text ?? '').trim();
  const pattern = decimals === 0 ? /^\d+$/ : new RegExp(`^\\d+(\\.\\d{1,${decimals}})?$`);
  if (!pattern.test(value)) return null;

  const [whole, fraction = ''] = value.split('.');
  return BigInt(whole + fraction.padEnd(decimals, '0'));
}

export function parseAmount(text, decimals) {
  const units = parseUnits(text, decimals);
  return units !== null && units > 0n ? units : null;
}

export function formatAmount(units, decimals) {
  const digits = units.toString().padStart(decimals + 1, '0');
  if (decimals === 0) return digits;

  return `${digits.slice(0, -decimals)}.${digits.slice(-decimals)}`;
}

export function groupThousands(decimalString) {
  const [whole, fraction] = decimalString.split('.');
  const grouped = whole.replace(/\B(?=(\d{3})+(?!\d))/g, ',');

  return fraction === undefined ? grouped : `${grouped}.${fraction}`;
}
