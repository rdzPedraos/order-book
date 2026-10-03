// Wiring of the page: reads the API every 2 seconds, paints what it answers
// and sends orders and deposits. The rules live in the other modules.

import * as api from './api.js';
import { buildDepositBody, validateDeposit } from './deposit.js';
import { loadName, saveName, chooseName } from './name.js';
import { describeOrder, filterOrders, isActive } from './orders.js';
import { buildOrderBody, describeUnits, getReserve, getVisibleFields, validateOrder } from './order.js';
import { describeMovement, getAvailableFunds, getBalanceRows, getLevelRows, getSpread } from './present.js';
import { applyReadings } from './readings.js';

const POLLING_MS = 2000;
const BOOK_SIZE = 6;

const HINTS = {
  BUY_LIMIT: 'Compra VIB pagando como máximo el precio que elijas.',
  SELL_LIMIT: 'Vende VIB cobrando como mínimo el precio que elijas.',
  BUY_MARKET: 'Gasta un monto en BRL y compra al mejor precio disponible.',
  SELL_MARKET: 'Vende VIB ya, al mejor precio disponible.',
};

const DEPOSIT_DEFAULTS = { BRL: '150.00', VIB: '10' };

const state = { name: '', book: null, wallet: null, movements: null, orders: null, busy: false };
let paintedOrders = '';
const cancelRequested = new Set();

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

function createLevels(containerId, kind) {
  const rows = Array.from({ length: BOOK_SIZE }, () => {
    const level = makeElement('div', `level level-${kind}`);
    level.append(makeElement('span', 'level-bar'), makeElement('strong'), makeElement('span'), makeElement('span'));

    return level;
  });
  byId(containerId).replaceChildren(...rows);
}

function setText(element, text) {
  if (element.textContent !== text) element.textContent = text;
}

// The rows are made once and only their text and bar change, so the list
// does not blink or move when the book does. Sells are painted with the best
// price at the bottom, next to the spread.
function paintLevels(containerId, levels, kind) {
  const ordered = getLevelRows(levels ?? [], BOOK_SIZE);
  if (kind === 'ask') ordered.reverse();

  ordered.forEach((row, index) => {
    const [bar, price, volume, total] = byId(containerId).children[index].children;
    bar.style.width = `${row.barPercent}%`;
    setText(price, row.price);
    setText(volume, row.volume);
    setText(total, row.total);
  });
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

// The list is rebuilt only when what it shows changed, so a click on Cancelar
// is not lost to a repaint that swaps the button under the cursor.
function paintOrders() {
  forgetFinishedCancellations();
  const shown = filterOrders(state.orders, getChecked('orders-filter'))
    .map((order) => describeOrder(order, undefined, cancelRequested.has(order.orderId)));
  const signature = JSON.stringify(shown);
  byId('orders-empty').hidden = shown.length > 0;
  if (signature === paintedOrders) return;
  paintedOrders = signature;

  const rows = shown.map((order) => {
    const row = makeElement('div', 'order');
    row.dataset.buy = String(order.isBuy);
    row.dataset.status = order.status;
    const text = makeElement('span', 'order-text');
    text.append(
      makeElement('strong', 'order-title', `${order.title} · ${order.shortId}`),
      makeElement('span', 'order-sub', `${order.progress} · ${order.time}`),
    );
    if (order.reason) text.append(makeElement('span', 'order-reason', order.reason));
    const action = makeElement('span', 'order-action');
    if (order.canCancel) action.append(makeCancelButton(order.orderId));
    row.append(makeElement('span', 'order-side', order.sideLabel), text, makeElement('span', 'order-status', order.statusLabel), action);

    return row;
  });
  byId('orders').replaceChildren(...rows);
}

function forgetFinishedCancellations() {
  for (const order of state.orders ?? []) {
    if (!isActive(order)) cancelRequested.delete(order.orderId);
  }
}

function makeCancelButton(orderId) {
  const button = makeElement('button', 'order-cancel', 'Cancelar');
  button.type = 'button';
  button.addEventListener('click', () => cancelOrder(orderId));

  return button;
}

function paintUser() {
  byId('user-name').textContent = state.name || 'sin nombre';
  byId('user-initial').textContent = state.name ? state.name.charAt(0).toUpperCase() : '?';
  byId('name-input').value = state.name;
}

function paintData() {
  paintLevels('asks', state.book?.asks, 'ask');
  paintLevels('bids', state.book?.bids, 'bid');
  byId('spread').textContent = getSpread(state.book) || '—';
  paintBalances();
  paintMovements();
  paintOrders();
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

  const asked = { book: api.getMarket(BOOK_SIZE) };
  if (state.name) {
    asked.wallet = api.getWallet(state.name);
    asked.movements = api.getMovements(state.name);
    asked.orders = api.getOrders(state.name);
  }
  const results = await Promise.allSettled(Object.values(asked));
  const readings = Object.fromEntries(Object.keys(asked).map((name, index) => [name, results[index]]));

  Object.assign(state, applyReadings(state, readings), { busy: false });
  paintData();
}

function changeName(draft) {
  const name = chooseName(draft, state.name);
  if (name !== state.name) {
    Object.assign(state, { name, wallet: null, movements: null, orders: null });
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

async function cancelOrder(orderId) {
  try {
    await api.closeOrder(state.name, orderId);
  } catch (error) {
    return showMessage('orders-message', error.message, 'error');
  }
  cancelRequested.add(orderId);
  showMessage('orders-message', 'Pediste cancelar la orden. Se aplica en un momento.', 'ok');
  paintOrders();
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
  for (const radio of document.querySelectorAll('input[name="orders-filter"]')) {
    radio.addEventListener('change', paintOrders);
  }
  for (const radio of document.querySelectorAll('input[name="deposit-currency"]')) {
    radio.addEventListener('change', () => {
      byId('deposit-amount').value = DEPOSIT_DEFAULTS[radio.value];
      showMessage('deposit-message', '', 'ok');
    });
  }
}

state.name = loadName(localStorage);
createLevels('asks', 'ask');
createLevels('bids', 'bid');
listen();
paintUser();
paintCase();
refresh();
setInterval(refresh, POLLING_MS);
