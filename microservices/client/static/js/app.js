// Wiring of the page: reads the API every 2 seconds, paints what it answers
// and sends orders and deposits. The rules live in the other modules.

import * as api from './api.js';
import { buildDepositBody, validateDeposit } from './deposit.js';
import { loadName, saveName, chooseName } from './name.js';
import { buildOrderBody, describeUnits, getReserve, getVisibleFields, validateOrder } from './order.js';
import { describeMovement, getAvailableFunds, getBalanceRows, getLevelRows } from './present.js';
import { applyReadings } from './readings.js';

const POLLING_MS = 2000;

const HINTS = {
  BUY_LIMIT: 'Compra VIB pagando como máximo el precio que elijas.',
  SELL_LIMIT: 'Vende VIB cobrando como mínimo el precio que elijas.',
  BUY_MARKET: 'Gasta un monto en BRL y compra al mejor precio disponible.',
  SELL_MARKET: 'Vende VIB ya, al mejor precio disponible.',
};

const DEPOSIT_DEFAULTS = { BRL: '150.00', VIB: '10' };

const state = { name: '', book: null, wallet: null, movements: null, busy: false };

const byId = (id) => document.getElementById(id);
const getChecked = (name) => document.querySelector(`input[name="${name}"]:checked`).value;

function makeElement(tag, className, text) {
  const element = document.createElement(tag);
  element.className = className;
  if (text !== undefined) element.textContent = text;

  return element;
}

function showMessage(id, text, tone) {
  const box = byId(id);
  box.textContent = text;
  box.dataset.tone = tone;
  box.hidden = text === '';
}

function paintLevels(containerId, levels, kind) {
  const rows = getLevelRows(levels ?? []).map((row) => {
    const level = makeElement('div', `level level-${kind}`);
    const bar = makeElement('span', 'level-bar');
    bar.style.width = `${row.barPercent}%`;
    level.append(bar, makeElement('strong', '', row.price), makeElement('span', '', row.volume));

    return level;
  });
  byId(containerId).replaceChildren(...rows);
}

function paintBalances() {
  for (const row of getBalanceRows(state.wallet)) {
    const card = document.querySelector(`.balance[data-currency="${row.currency}"]`);
    card.querySelector('.balance-total').textContent = row.total;
    card.querySelector('.balance-reserved').textContent = `Reservado ${row.reserved}`;
  }
}

function paintMovements() {
  const rows = (state.movements ?? []).map((movement) => {
    const described = describeMovement(movement);
    const row = makeElement('div', 'movement');
    row.dataset.positive = String(described.isPositive);
    const text = makeElement('span', 'movement-text');
    text.append(makeElement('strong', 'movement-label', described.label), makeElement('span', 'movement-time', described.time));
    row.append(makeElement('span', 'movement-currency', described.currency), text, makeElement('strong', 'movement-amount', described.amount));

    return row;
  });
  byId('movements').replaceChildren(...rows);
}

function paintUser() {
  byId('user-name').textContent = state.name || 'sin nombre';
  byId('user-initial').textContent = state.name ? state.name.charAt(0).toUpperCase() : '?';
  byId('name-input').value = state.name;
}

function paintData() {
  paintLevels('asks', state.book?.asks, 'ask');
  paintLevels('bids', state.book?.bids, 'bid');
  paintBalances();
  paintMovements();
}

function paintCase() {
  const side = getChecked('side');
  const type = getChecked('type');
  const visible = getVisibleFields(side, type);

  byId('trade').dataset.side = side;
  for (const field of Object.keys(visible)) {
    document.querySelector(`[data-field="${field}"]`).hidden = !visible[field];
  }
  byId('limit-label').textContent = side === 'BUY' ? 'Pagar como máximo (por VIB)' : 'Cobrar como mínimo (por VIB)';
  byId('case-hint').textContent = HINTS[`${side}_${type}`];
  byId('confirm').textContent = side === 'BUY' ? 'Confirmar compra' : 'Confirmar venta';
  showMessage('order-message', '', 'ok');
}

async function refresh() {
  if (state.busy) return;
  state.busy = true;

  const asked = { book: api.getMarket() };
  if (state.name) {
    asked.wallet = api.getWallet(state.name);
    asked.movements = api.getMovements(state.name);
  }
  const results = await Promise.allSettled(Object.values(asked));
  const readings = Object.fromEntries(Object.keys(asked).map((name, index) => [name, results[index]]));

  Object.assign(state, applyReadings(state, readings), { busy: false });
  paintData();
}

function changeName(draft) {
  const name = chooseName(draft, state.name);
  if (name !== state.name) {
    Object.assign(state, { name, wallet: null, movements: null });
    saveName(localStorage, name);
    paintData();
    refresh();
  }
  paintUser();
}

async function sendOrder() {
  const side = getChecked('side');
  const type = getChecked('type');
  const values = { limit: byId('limit').value, quantity: byId('quantity').value, amount: byId('amount').value };
  if (!state.name) return showMessage('order-message', 'Elige un nombre para operar.', 'error');

  const problem = validateOrder(side, type, values, getAvailableFunds(state.wallet));
  if (problem) return showMessage('order-message', problem, 'error');

  try {
    await api.postOrder(state.name, buildOrderBody(side, type, values));
  } catch (error) {
    return showMessage('order-message', error.message, 'error');
  }
  const reserve = getReserve(side, type, values);
  showMessage('order-message', `Orden enviada. Reservamos ${describeUnits(reserve.currency, reserve.units)}.`, 'ok');
  refresh();
}

async function sendDeposit() {
  const currency = getChecked('deposit-currency');
  const amount = byId('deposit-amount').value;
  if (!state.name) return showMessage('deposit-message', 'Elige un nombre para cargar dinero.', 'error');

  const problem = validateDeposit(currency, amount);
  if (problem) return showMessage('deposit-message', problem, 'error');

  try {
    await api.postDeposit(state.name, buildDepositBody(currency, amount));
  } catch (error) {
    return showMessage('deposit-message', error.message, 'error');
  }
  showMessage('deposit-message', `Listo, cargaste ${amount.trim()} ${currency}.`, 'ok');
  refresh();
}

function listen() {
  byId('name-input').addEventListener('change', (event) => changeName(event.target.value));
  byId('confirm').addEventListener('click', sendOrder);
  byId('deposit-button').addEventListener('click', sendDeposit);
  for (const radio of document.querySelectorAll('input[name="side"], input[name="type"]')) {
    radio.addEventListener('change', paintCase);
  }
  for (const radio of document.querySelectorAll('input[name="deposit-currency"]')) {
    radio.addEventListener('change', () => {
      byId('deposit-amount').value = DEPOSIT_DEFAULTS[radio.value];
      showMessage('deposit-message', '', 'ok');
    });
  }
}

state.name = loadName(localStorage);
listen();
paintUser();
paintCase();
refresh();
setInterval(refresh, POLLING_MS);
