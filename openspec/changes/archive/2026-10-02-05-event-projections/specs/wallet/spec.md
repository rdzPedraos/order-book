# Spec Delta

## ADDED Requirements

### Requirement: Pago de trades
Por cada `TradeExecuted` de Q VIB a precio P, el sistema MUST, en una sola transacción: descontar Q × P BRL del `reserved` del comprador y sumarle Q VIB en `available`; descontar Q VIB del `reserved` del vendedor y sumarle Q × P BRL en `available`. Cada movimiento MUST quedar en el ledger, con la orden y el `id` del evento. MUST pagar cada trade una sola vez y usar solo saldo reservado.

#### Scenario: Pago de un trade
- **WHEN** se paga un trade de 2 VIB a R$ 95,00 entre el comprador C y el vendedor V
- **THEN** C pierde R$ 190,00 de `reserved` y gana 2 VIB en `available`
- **AND** V pierde 2 VIB de `reserved` y gana R$ 190,00 en `available`
- **AND** el ledger tiene cuatro movimientos, dos por persona

#### Scenario: Trade duplicado
- **WHEN** el mismo `TradeExecuted` llega dos veces
- **THEN** los saldos y el ledger cambian una sola vez

#### Scenario: Conservación
- **WHEN** se pagan varios trades entre las mismas personas
- **THEN** la suma total de BRL y la suma total de VIB entre sus wallets no cambia
