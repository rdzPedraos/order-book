// The calls to the API. Paths are relative: the page and the API share a host.

async function call(method, path, userName, body) {
  const headers = {};
  if (userName) headers['X-User-ID'] = userName;
  if (body) headers['Content-Type'] = 'application/json';

  const response = await fetch(path, { method, headers, body: body && JSON.stringify(body) });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(payload.error?.message ?? `Error ${response.status}`);

  return payload.data;
}

export const getMarket = () => call('GET', '/market/orderbook/BRL-VIB?depth=5');
export const getWallet = (userName) => call('GET', '/wallet', userName);
export const getMovements = (userName) => call('GET', '/wallet/movements?limit=5', userName);
export const postOrder = (userName, body) => call('POST', '/orders', userName, body);
export const postDeposit = (userName, body) => call('POST', '/wallet/deposits', userName, body);
