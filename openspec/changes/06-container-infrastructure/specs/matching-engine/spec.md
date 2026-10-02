# Spec Delta

## ADDED Requirements

### Requirement: Throughput sostenido
El sistema desplegado MUST sostener 5.000 órdenes por segundo en un único book durante al menos 10 minutos, contando como operación cada creación, modificación o cancelación aceptada por la API. Durante la carga, ningún saldo MUST quedar negativo y la suma total de BRL y VIB MUST conservarse.

#### Scenario: Carga sostenida
- **WHEN** se envían 5.000 órdenes/s durante 10 minutos con el escenario B (90 % reposa)
- **THEN** el engine procesa todas las órdenes aceptadas sin crecer su atraso respecto del log
- **AND** al terminar, la suma total de BRL y de VIB es igual a la suma de depósitos menos retiros

#### Scenario: Caída durante la carga
- **WHEN** a mitad de la carga se reinicia el engine, luego WalletService y luego OrderService
- **THEN** ninguna operación aceptada por la API se pierde y la conservación de saldos se mantiene
