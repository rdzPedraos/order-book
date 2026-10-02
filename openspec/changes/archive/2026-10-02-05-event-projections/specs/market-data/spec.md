# Spec Delta

## Purpose

Ofrece una vista pública y agregada del Order Book de cada book, por precio y volumen, para que las personas decidan sus órdenes sin acceder al estado interno del engine.

## ADDED Requirements

### Requirement: Profundidad agregada
`GET /market/orderbook/{book}` MUST devolver los bids (precio descendente) y los asks (precio ascendente) agregados por precio, con el volumen total en VIB y la cantidad de órdenes por nivel, construidos solo con los `OrderBookLevelChanged` del engine. MUST NOT exponer órdenes individuales ni identidades, y MUST NOT requerir `X-User-ID`.

#### Scenario: Book con varios niveles
- **WHEN** el book tiene compras de 10 y 20 VIB @ 100 y de 15 VIB @ 99
- **THEN** la respuesta contiene `bids = [{price:"100.00", volume:"30", orders:2}, {price:"99.00", volume:"15", orders:1}]`

#### Scenario: Nivel vaciado
- **WHEN** llega `OrderBookLevelChanged` con `volume = 0` para un nivel
- **THEN** ese nivel ya no aparece en la respuesta

#### Scenario: Book en minúsculas
- **WHEN** se consulta `GET /market/orderbook/brl-vib`
- **THEN** el sistema responde la profundidad del book `BRL-VIB`, con `book = "BRL-VIB"` en la respuesta

#### Scenario: Book desconocido
- **WHEN** se consulta `GET /market/orderbook/BTC-USD`
- **THEN** el sistema responde `404` con código `book_not_found`

### Requirement: Límite de profundidad
La consulta MUST aceptar un parámetro `depth` (de 1 a 100, por defecto 20) que limita la cantidad de niveles por lado a los mejores.

#### Scenario: Depth explícito
- **WHEN** se consulta `GET /market/orderbook/BRL-VIB?depth=5`
- **THEN** cada lado tiene como máximo 5 niveles, los mejores

#### Scenario: Depth inválido
- **WHEN** se consulta con `depth=500`
- **THEN** el sistema responde `400` con código `invalid_depth`
